package browser

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/bnema/dumber/internal/app/api"
	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/internal/app/environment"
	"github.com/bnema/dumber/internal/app/messaging"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/internal/filtering"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/services"
	"github.com/bnema/dumber/pkg/webkit"
	"golang.org/x/sync/errgroup"
)

// BrowserApp represents the main browser application
type BrowserApp struct {
	version   string
	commit    string
	buildDate string
	assets    embed.FS

	// Core components
	config   *config.Config
	database *sql.DB
	queries  *db.Queries

	// Services
	parserService  *services.ParserService
	browserService *services.BrowserService
	faviconService *services.FaviconService

	// WebView and controllers
	webView              *webkit.WebView
	zoomController       *control.ZoomController
	navigationController *control.NavigationController
	clipboardController  *control.ClipboardController

	// Tab and workspace management
	tabManager *TabManager

	// Convenience accessors (delegate to active tab's workspace)
	// These maintain compatibility with existing code
	panes      []*BrowserPane
	activePane *BrowserPane
	workspace  *WorkspaceManager

	// Content filtering
	filterManager          *filtering.FilterManager
	contentBlockingService *filtering.ContentBlockingService
	bypassRegistry         *filtering.BypassRegistry
	pendingFilteringMu     sync.Mutex
	pendingFiltering       []*webkit.WebView

	// Handlers
	schemeHandler         *api.SchemeHandler
	messageHandler        *messaging.Handler
	shortcutHandler       *ShortcutHandler
	windowShortcutHandler WindowShortcutHandlerInterface

	// Performance optimization components
	webViewPool     *WebViewPool     // Pre-created WebViews for instant tab/pane creation
	prefetchManager *PrefetchManager // DNS prefetching for faster page loads
}

// Run starts the browser application
func Run(assets embed.FS, version, commit, buildDate string) {
	startupStart := time.Now()
	logging.Info(fmt.Sprintf("Starting GUI mode (webkit_cgo=%v)", webkit.IsNativeAvailable()))

	app := &BrowserApp{
		version:   version,
		commit:    commit,
		buildDate: buildDate,
		assets:    assets,
	}

	if err := app.Initialize(); err != nil {
		logging.Error(fmt.Sprintf("Failed to initialize browser: %v", err))
		if webkit.IsNativeAvailable() {
			runtime.UnlockOSThread()
		}
		os.Exit(1)
	}

	startupElapsed := time.Since(startupStart)
	logging.Info(fmt.Sprintf("[startup] Application initialized in %v", startupElapsed))
	if startupElapsed > 500*time.Millisecond {
		logging.Warn(fmt.Sprintf("[startup] WARNING: Startup took %v (target: <500ms)", startupElapsed))
	}

	app.Run()
}

// Initialize sets up all browser components
func (app *BrowserApp) Initialize() error {
	// Setup media pipeline hardening
	environment.SetupMediaPipeline()

	// Initialize configuration
	if err := config.Init(); err != nil {
		return err
	}
	app.config = config.Get()

	// Apply environment configurations
	environment.ApplyCodecConfiguration(app.config.CodecPreferences)

	// Setup crash handler
	logging.SetupCrashHandler()

	// Setup output capture if configured
	if err := app.setupOutputCapture(); err != nil {
		logging.Warn(fmt.Sprintf("Warning: failed to setup output capture: %v", err))
	}

	// Initialize WebKit log capture if configured
	if app.config.Logging.CaptureCOutput {
		if err := webkit.InitWebKitLogCapture(); err != nil {
			logging.Warn(fmt.Sprintf("Warning: failed to initialize WebKit log capture: %v", err))
		} else {
			defer webkit.StopWebKitLogCapture()
			webkit.StartWebKitOutputCapture()
		}
	}

	logging.Info(fmt.Sprintf("Config initialized"))

	// Detect keyboard layout in background (required for shortcuts)
	go environment.DetectAndSetKeyboardLocale()

	// Initialize database (migrations run automatically in InitDB)
	database, err := db.InitDB(app.config.Database.Path)
	if err != nil {
		return err
	}
	app.database = database
	app.queries = db.New(database)
	logging.Info(fmt.Sprintf("Database opened at %s", app.config.Database.Path))

	// DEFERRED: Cleanup expired certificate validations in background
	go func() {
		time.Sleep(500 * time.Millisecond) // Low priority maintenance task
		if err := webkit.CleanupExpiredCertificateValidations(); err != nil {
			logging.Warn(fmt.Sprintf("Warning: failed to cleanup expired certificate validations: %v", err))
		}
	}()

	// Initialize services
	app.parserService = services.NewParserService(app.config, app.queries)
	app.browserService = services.NewBrowserService(app.config, app.queries)

	// Load all caches in parallel for fast startup (target: <100ms)
	if err := app.loadCachesParallel(context.Background()); err != nil {
		logging.Warn(fmt.Sprintf("Warning: failed to load caches: %v", err))
		// Non-fatal - caches will fall back to defaults or DB queries
	}

	// Initialize handlers
	app.schemeHandler = api.NewSchemeHandler(app.assets, app.parserService, app.browserService)
	app.messageHandler = messaging.NewHandler(app.parserService, app.browserService)

	// Initialize prefetch manager for DNS preloading
	app.prefetchManager = NewPrefetchManager(DefaultPrefetchConfig(), app.queries)

	// Initialize WebView pool for instant tab/pane creation
	// Pool starts warming up after main window shows (see Run method)
	app.webViewPool = NewWebViewPool(2, 4, app.getWebViewConfig)

	return nil
}

// getWebViewConfig returns the configuration for creating pooled WebViews
func (app *BrowserApp) getWebViewConfig() *webkit.Config {
	dataDir, _ := config.GetDataDir()
	cacheDir, _ := config.GetFilterCacheDir()

	return &webkit.Config{
		EnableJavaScript:      true,
		EnableWebGL:           true,
		DefaultFontSize:       app.config.Appearance.DefaultFontSize,
		MinimumFontSize:       8, // Reasonable default
		UserAgent:             app.config.CodecPreferences.CustomUserAgent,
		DataDir:               dataDir,
		CacheDir:              cacheDir,
		EnablePageCache:       true,
		EnableSmoothScrolling: true,
		CreateWindow:          false, // Pool WebViews don't create their own window
	}
}

// GetWebViewPool returns the WebView pool for creating new tabs/panes.
// Returns nil if pool is not initialized.
func (app *BrowserApp) GetWebViewPool() *WebViewPool {
	return app.webViewPool
}

// GetPrefetchManager returns the prefetch manager for DNS preloading.
// Returns nil if prefetch manager is not initialized.
func (app *BrowserApp) GetPrefetchManager() *PrefetchManager {
	return app.prefetchManager
}

// loadCachesParallel loads all caches in parallel for fast startup.
// Uses errgroup to coordinate parallel loads and capture any errors.
// Target: <100ms for all caches combined.
func (app *BrowserApp) loadCachesParallel(ctx context.Context) error {
	startTime := time.Now()

	g, ctx := errgroup.WithContext(ctx)

	// Load zoom cache in parallel
	g.Go(func() error {
		if err := app.browserService.LoadZoomCacheFromDB(ctx); err != nil {
			logging.Error(fmt.Sprintf("[cache] Failed to load zoom cache: %v", err))
			return err
		}
		return nil
	})

	// Load certificate validation cache in parallel
	g.Go(func() error {
		if err := app.browserService.LoadCertCacheFromDB(ctx); err != nil {
			logging.Error(fmt.Sprintf("[cache] Failed to load cert cache: %v", err))
			return err
		}
		return nil
	})

	// Load favorites cache in parallel
	g.Go(func() error {
		if err := app.browserService.LoadFavoritesCacheFromDB(ctx); err != nil {
			logging.Error(fmt.Sprintf("[cache] Failed to load favorites cache: %v", err))
			return err
		}
		return nil
	})

	// Load fuzzy search cache in parallel for instant dmenu access
	g.Go(func() error {
		if err := app.browserService.LoadFuzzyCacheFromDB(ctx); err != nil {
			logging.Error(fmt.Sprintf("[cache] Failed to load fuzzy cache: %v", err))
			return err
		}
		return nil
	})

	// Wait for all cache loads to complete
	if err := g.Wait(); err != nil {
		return fmt.Errorf("cache loading failed: %w", err)
	}

	elapsed := time.Since(startTime)
	logging.Info(fmt.Sprintf("[cache] All caches loaded in %v", elapsed))

	// Warn if startup is slower than target
	if elapsed > 100*time.Millisecond {
		logging.Warn(fmt.Sprintf("[cache] Warning: Cache loading took %v (target: <100ms)", elapsed))
	}

	return nil
}

// Run executes the main browser loop
func (app *BrowserApp) Run() {
	defer app.cleanup()
	defer logging.SetupPanicRecovery()

	// Set config on scheme handler
	app.schemeHandler.SetConfig(app.config)

	// Register custom scheme resolver for "dumb://" URIs (will be applied after WebView creation)
	webkit.RegisterURIScheme("dumb", app.schemeHandler.Handle)

	// Create and setup WebView (this also creates TabManager which creates more WebViews)
	if err := app.createWebView(); err != nil {
		logging.Warn(fmt.Sprintf("Warning: failed to create WebView: %v", err))
		return
	}

	// Apply URI scheme handlers after WebView creation
	if err := webkit.ApplyURISchemeHandlers(app.webView.GetWebView()); err != nil {
		logging.Warn(fmt.Sprintf("Warning: failed to register URI scheme handlers: %v", err))
	}

	// POST-VISIBLE: Start WebView pool warmup for instant tab/pane creation
	if app.webViewPool != nil {
		app.webViewPool.Start()
	}

	// POST-VISIBLE: Start DNS prefetching from history and common domains
	if app.prefetchManager != nil {
		app.prefetchManager.PrefetchCommonDomains()
		app.prefetchManager.PrefetchFromHistory(context.Background())
	}

	// Handle browse command if present (must use active tab's navigation controller)
	if app.tabManager != nil {
		activeTab := app.tabManager.GetActiveTab()
		if activeTab != nil && activeTab.workspace != nil {
			activePane := activeTab.workspace.GetActivePane()
			if activePane != nil && activePane.navigationController != nil {
				activePane.navigationController.HandleBrowseCommand()
			}
		}
	} else {
		// Fallback to legacy behavior if no tab manager
		app.navigationController.HandleBrowseCommand()
	}

	// Setup signal handling
	app.setupSignalHandling()

	// Run main loop
	app.runMainLoop()
}

// cleanup handles cleanup on shutdown
func (app *BrowserApp) cleanup() {
	logging.Info(fmt.Sprintf("Starting browser cleanup..."))

	// Stop WebView pool first (destroys pre-created WebViews)
	if app.webViewPool != nil {
		logging.Info(fmt.Sprintf("Stopping WebView pool"))
		app.webViewPool.Stop()
		app.webViewPool = nil
	}

	// Cleanup window shortcuts first
	if app.windowShortcutHandler != nil {
		logging.Info(fmt.Sprintf("Cleaning up window shortcuts"))
		app.windowShortcutHandler.Cleanup()
		app.windowShortcutHandler = nil
	}

	// Cleanup tab manager (which cleans up all tabs and their workspaces)
	if app.tabManager != nil {
		logging.Info(fmt.Sprintf("Cleaning up tab manager"))
		app.tabManager.Cleanup()
		app.tabManager = nil
	}

	// Flush all caches to ensure pending writes complete before database closes
	if app.browserService != nil {
		logging.Info(fmt.Sprintf("Flushing all caches..."))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := app.browserService.FlushAllCaches(ctx); err != nil {
			logging.Warn(fmt.Sprintf("Warning: failed to flush caches: %v", err))
		}
	}

	// Close database with WAL checkpoint
	if app.database != nil {
		logging.Info(fmt.Sprintf("Performing WAL checkpoint and closing database..."))

		// Run WAL checkpoint to commit all pending writes and truncate WAL file
		if _, err := app.database.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			logging.Warn(fmt.Sprintf("Warning: WAL checkpoint failed: %v", err))
		} else {
			logging.Info(fmt.Sprintf("WAL checkpoint completed successfully"))
		}

		// Close database connection
		if closeErr := app.database.Close(); closeErr != nil {
			logging.Warn(fmt.Sprintf("Warning: failed to close database: %v", closeErr))
		} else {
			logging.Info(fmt.Sprintf("Database closed successfully"))
		}
	}

	logging.Info(fmt.Sprintf("Browser cleanup completed"))
}

// setupOutputCapture initializes stdout/stderr capture if configured
func (app *BrowserApp) setupOutputCapture() error {
	if app.config.Logging.CaptureStdout || app.config.Logging.CaptureStderr {
		// Capturing stdout/stderr works for non-GTK builds, but in native GTK mode
		// it interferes with WebKit's own pipe management and crashes immediately.
		if webkit.IsNativeAvailable() {
			logging.Warn(fmt.Sprintf("Warning: stdout/stderr capture is not supported in native GTK mode; skipping"))
			return nil
		}

		logging.Warn(fmt.Sprintf("Warning: stdout/stderr capture is experimental and may interfere with normal operations"))
		if logger := logging.GetLogger(); logger != nil {
			outputCapture := logging.NewOutputCapture(logger)
			if err := outputCapture.Start(); err != nil {
				return err
			}
			// Note: defer outputCapture.Stop() should be handled by the caller
		}
	}
	return nil
}

// setupSignalHandling configures graceful shutdown on signals
func (app *BrowserApp) setupSignalHandling() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Handle signals in a goroutine to quit the main loop
	go func() {
		sig := <-sigChan
		logging.Info(fmt.Sprintf("Received signal %v - shutting down gracefully", sig))
		webkit.QuitMainLoop()
	}()
}

// runMainLoop starts the appropriate main loop based on WebKit availability
func (app *BrowserApp) runMainLoop() {
	if webkit.IsNativeAvailable() {
		if os.Getenv("DUMBER_DISABLE_CONTENT_BLOCKING") != "1" {
			// Schedule content blocking initialization to run on the first main loop iteration
			// This ensures GTK is fully ready before we start loading filters
			webkit.RunOnMainThread(func() {
				if err := app.setupContentBlocking(); err != nil {
					logging.Warn(fmt.Sprintf("Warning: failed to setup content blocking: %v", err))
				}
			})
		} else {
			logging.Warn("Content blocking skipped (DUMBER_DISABLE_CONTENT_BLOCKING=1)")
		}

		logging.Info(fmt.Sprintf("Entering GTK main loop…"))
		logging.Debug("[browser] Calling webkit.RunMainLoop()")
		webkit.RunMainLoop()
		logging.Info(fmt.Sprintf("GTK main loop exited"))

		// Flush pending history writes immediately after main loop exit
		// This ensures database operations complete while GTK is still in a valid state
		// MUST happen before cleanup() deferred call, which happens after this function returns
		if app.browserService != nil {
			logging.Info(fmt.Sprintf("Flushing pending history writes..."))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := app.browserService.FlushHistoryQueue(ctx); err != nil {
				logging.Warn(fmt.Sprintf("Warning: history queue flush incomplete: %v", err))
			} else {
				logging.Info(fmt.Sprintf("History queue flushed successfully"))
			}
		}
	} else {
		logging.Info(fmt.Sprintf("Not entering GUI loop (non-CGO build)"))
		// In non-CGO mode, just wait for signals
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		logging.Info(fmt.Sprintf("Received signal %v - exiting", sig))
	}
}
