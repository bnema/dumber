package browser

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/diamondburned/gotk4-webkitgtk/pkg/soup/v3"
	webkitv6 "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/core/gextras"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"golang.org/x/sync/errgroup"

	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/internal/app/environment"
	"github.com/bnema/dumber/internal/app/messaging"
	"github.com/bnema/dumber/internal/app/schemes"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/services"
	"github.com/bnema/dumber/internal/webext"
	"github.com/bnema/dumber/internal/webext/api"
	"github.com/bnema/dumber/pkg/webkit"
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

	// WebExtensions support
	extensionManager  *webext.Manager
	webExtPorts       map[string]*webExtPortBridge
	webRequestReplies map[string]chan *api.BlockingResponse
	webRequestPending map[string][]api.RequestDetails
	webRequestActive  map[uintptr]*webRequestTrack
	webRequestMu      sync.Mutex
	webExtLogger      *logging.LogRotator // Logger for WebExtension process logs
	webRequestServer  *WebRequestServer   // UNIX socket server for webRequest IPC

	// Handlers
	schemeHandler         *schemes.APIHandler
	messageHandler        *messaging.Handler
	shortcutHandler       *ShortcutHandler
	windowShortcutHandler WindowShortcutHandlerInterface

	// Extension popup manager
	popupManager *ExtensionPopupManager
}

// Run starts the browser application
func Run(assets embed.FS, version, commit, buildDate string) {
	startupStart := time.Now()
	log.Printf("Starting GUI mode (webkit_cgo=%v)", webkit.IsNativeAvailable())

	app := &BrowserApp{
		version:           version,
		commit:            commit,
		buildDate:         buildDate,
		assets:            assets,
		webExtPorts:       make(map[string]*webExtPortBridge),
		webRequestReplies: make(map[string]chan *api.BlockingResponse),
		webRequestPending: make(map[string][]api.RequestDetails),
		webRequestActive:  make(map[uintptr]*webRequestTrack),
	}

	// Set browser info for webext runtime.getBrowserInfo()
	api.SetBrowserInfo(version, commit, buildDate)

	if err := app.Initialize(); err != nil {
		log.Printf("Failed to initialize browser: %v", err)
		if webkit.IsNativeAvailable() {
			runtime.UnlockOSThread()
		}
		os.Exit(1)
	}

	startupElapsed := time.Since(startupStart)
	log.Printf("[startup] Application initialized in %v", startupElapsed)
	if startupElapsed > 500*time.Millisecond {
		log.Printf("[startup] WARNING: Startup took %v (target: <500ms)", startupElapsed)
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
		log.Printf("Warning: failed to setup output capture: %v", err)
	}

	// Initialize WebKit log capture if configured
	if app.config.Logging.CaptureCOutput {
		if err := webkit.InitWebKitLogCapture(); err != nil {
			log.Printf("Warning: failed to initialize WebKit log capture: %v", err)
		} else {
			defer webkit.StopWebKitLogCapture()
			webkit.StartWebKitOutputCapture()
		}
	}

	log.Printf("Config initialized")

	// Detect keyboard layout
	environment.DetectAndSetKeyboardLocale()

	// Initialize database (migrations run automatically in InitDB)
	database, err := db.InitDB(app.config.Database.Path)
	if err != nil {
		return err
	}
	app.database = database
	app.queries = db.New(database)
	log.Printf("Database opened at %s", app.config.Database.Path)

	// Cleanup expired certificate validations
	if err := webkit.CleanupExpiredCertificateValidations(); err != nil {
		log.Printf("Warning: failed to cleanup expired certificate validations: %v", err)
	}

	// Initialize services
	app.parserService = services.NewParserService(app.config, app.queries)
	app.browserService = services.NewBrowserService(app.config, app.queries)

	// Load all caches in parallel for fast startup (target: <100ms)
	if err := app.loadCachesParallel(context.Background()); err != nil {
		log.Printf("Warning: failed to load caches: %v", err)
		// Non-fatal - caches will fall back to defaults or DB queries
	}

	// Initialize handlers
	app.schemeHandler = schemes.NewAPIHandler(app.assets, app.parserService, app.browserService, app.config)
	app.messageHandler = messaging.NewHandler(app.parserService, app.browserService)

	// Initialize extension manager
	if err := app.setupExtensionManager(); err != nil {
		log.Printf("Warning: failed to setup extension manager: %v", err)
		// Non-fatal - browser can run without extensions
	}

	return nil
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
			log.Printf("[cache] Failed to load zoom cache: %v", err)
			return err
		}
		return nil
	})

	// Load certificate validation cache in parallel
	g.Go(func() error {
		if err := app.browserService.LoadCertCacheFromDB(ctx); err != nil {
			log.Printf("[cache] Failed to load cert cache: %v", err)
			return err
		}
		return nil
	})

	// Load favorites cache in parallel
	g.Go(func() error {
		if err := app.browserService.LoadFavoritesCacheFromDB(ctx); err != nil {
			log.Printf("[cache] Failed to load favorites cache: %v", err)
			return err
		}
		return nil
	})

	// Load fuzzy search cache in parallel for instant dmenu access
	g.Go(func() error {
		if err := app.browserService.LoadFuzzyCacheFromDB(ctx); err != nil {
			log.Printf("[cache] Failed to load fuzzy cache: %v", err)
			return err
		}
		return nil
	})

	// Wait for all cache loads to complete
	if err := g.Wait(); err != nil {
		return fmt.Errorf("cache loading failed: %w", err)
	}

	elapsed := time.Since(startTime)
	log.Printf("[cache] All caches loaded in %v", elapsed)

	// Warn if startup is slower than target
	if elapsed > 100*time.Millisecond {
		log.Printf("[cache] Warning: Cache loading took %v (target: <100ms)", elapsed)
	}

	return nil
}

// Run executes the main browser loop
func (app *BrowserApp) Run() {
	defer app.cleanup()
	defer logging.SetupPanicRecovery()

	// Register custom scheme resolver for "dumb://" URIs (will be applied after WebView creation)
	webkit.RegisterURIScheme(schemes.SchemeDumb, app.schemeHandler.Handle)
	if app.extensionManager != nil {
		webkit.RegisterURIScheme(schemes.SchemeDumbExtension, app.extensionManager.HandleSchemeRequest)
		webkit.RegisterSecureURIScheme(schemes.SchemeDumbExtension)
		// CRITICAL: Register as CORS-enabled to allow ES6 module imports from extension scheme
		webkit.RegisterCorsEnabledURIScheme(schemes.SchemeDumbExtension)
	}

	// Create and setup WebView
	if err := app.createWebView(); err != nil {
		log.Printf("Warning: failed to create WebView: %v", err)
		return
	}

	// Initialize cookie manager now that network session is created
	if app.extensionManager != nil {
		app.extensionManager.InitializeCookieManager()
	}

	// Apply URI scheme handlers after WebView creation
	if err := webkit.ApplyURISchemeHandlers(app.webView.GetWebView()); err != nil {
		log.Printf("Warning: failed to register URI scheme handlers: %v", err)
	}

	// Create background page WebViews now that WebContext is initialized
	if app.extensionManager != nil {
		exts := app.extensionManager.ListExtensions()
		for _, ext := range exts {
			if !app.extensionManager.IsEnabled(ext.ID) {
				continue
			}
			if ext.Manifest != nil && ext.Manifest.Background != nil {
				if err := app.extensionManager.StartBackgroundContext(ext); err != nil {
					log.Printf("[webext] Warning: failed to prepare background page for %s: %v", ext.ID, err)
				}
				if err := app.ensureBackgroundPage(ext); err != nil {
					log.Printf("[webext] Warning: failed to start background page for %s: %v", ext.ID, err)
				}
			}
		}
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
	log.Printf("Starting browser cleanup...")

	// Cleanup webRequest server first (before any WebViews are destroyed)
	if app.webRequestServer != nil {
		log.Printf("Stopping webRequest server")
		app.webRequestServer.Close()
		app.webRequestServer = nil
	}

	// Cleanup window shortcuts first
	if app.windowShortcutHandler != nil {
		log.Printf("Cleaning up window shortcuts")
		app.windowShortcutHandler.Cleanup()
		app.windowShortcutHandler = nil
	}

	// Cleanup tab manager (which cleans up all tabs and their workspaces)
	if app.tabManager != nil {
		log.Printf("Cleaning up tab manager")
		app.tabManager.Cleanup()
		app.tabManager = nil
	}

	// Flush all caches to ensure pending writes complete before database closes
	if app.browserService != nil {
		log.Printf("Flushing all caches...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := app.browserService.FlushAllCaches(ctx); err != nil {
			log.Printf("Warning: failed to flush caches: %v", err)
		}
	}

	// Close database with WAL checkpoint
	if app.database != nil {
		log.Printf("Performing WAL checkpoint and closing database...")

		// Run WAL checkpoint to commit all pending writes and truncate WAL file
		if _, err := app.database.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			log.Printf("Warning: WAL checkpoint failed: %v", err)
		} else {
			log.Printf("WAL checkpoint completed successfully")
		}

		// Close database connection
		if closeErr := app.database.Close(); closeErr != nil {
			log.Printf("Warning: failed to close database: %v", closeErr)
		} else {
			log.Printf("Database closed successfully")
		}
	}

	log.Printf("Browser cleanup completed")
}

// setupOutputCapture initializes stdout/stderr capture if configured
func (app *BrowserApp) setupOutputCapture() error {
	if app.config.Logging.CaptureStdout || app.config.Logging.CaptureStderr {
		// Capturing stdout/stderr works for non-GTK builds, but in native GTK mode
		// it interferes with WebKit's own pipe management and crashes immediately.
		if webkit.IsNativeAvailable() {
			log.Printf("Warning: stdout/stderr capture is not supported in native GTK mode; skipping")
			return nil
		}

		log.Printf("Warning: stdout/stderr capture is experimental and may interfere with normal operations")
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
		log.Printf("Received signal %v - shutting down gracefully", sig)
		webkit.QuitMainLoop()
	}()
}

// runMainLoop starts the appropriate main loop based on WebKit availability
func (app *BrowserApp) runMainLoop() {
	if webkit.IsNativeAvailable() {
		log.Printf("Entering GTK main loop…")
		webkit.RunMainLoop()
		log.Printf("GTK main loop exited")

		// Flush pending history writes immediately after main loop exit
		// This ensures database operations complete while GTK is still in a valid state
		// MUST happen before cleanup() deferred call, which happens after this function returns
		if app.browserService != nil {
			log.Printf("Flushing pending history writes...")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := app.browserService.FlushHistoryQueue(ctx); err != nil {
				log.Printf("Warning: history queue flush incomplete: %v", err)
			} else {
				log.Printf("History queue flushed successfully")
			}
		}
	} else {
		log.Printf("Not entering GUI loop (non-CGO build)")
		// In non-CGO mode, just wait for signals
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		log.Printf("Received signal %v - exiting", sig)
	}
}

// setupExtensionManager initializes the WebExtensions system
func (app *BrowserApp) setupExtensionManager() error {
	log.Printf("[webext] Setting up extension manager...")

	// Get data directory for extension storage
	dataDir, err := config.GetDataDir()
	if err != nil {
		return fmt.Errorf("failed to get data directory: %w", err)
	}
	extensionsDir := filepath.Join(dataDir, "extensions")
	extDataDir := filepath.Join(dataDir, "extension-data")

	if err := os.MkdirAll(extensionsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create extensions directory: %w", err)
	}
	if err := os.MkdirAll(extDataDir, 0o755); err != nil {
		return fmt.Errorf("failed to create extension data directory: %w", err)
	}

	// Initialize WebExtension logger
	// This logger will receive log messages from the WebExtension process via message passing
	webExtLogger, err := logging.NewLogRotatorWithName(
		app.config.Logging.LogDir,
		"dumber-webext.log", // Explicit filename for WebExtension logs
		10,                  // maxSize (MB)
		5,                   // maxBackups
		7,                   // maxAge (days)
		false,               // compress
	)
	if err != nil {
		log.Printf("[webext] Warning: failed to initialize WebExtension logger: %v", err)
	} else {
		app.webExtLogger = webExtLogger
		log.Printf("[webext] Initialized WebExtension logger at %s", filepath.Join(app.config.Logging.LogDir, "dumber-webext.log"))
	}

	// Create extension manager (reuses app.database for extension storage)
	app.extensionManager = webext.NewManager(extensionsDir, extDataDir, app.database, app.queries)

	// Create extension popup manager
	app.popupManager = NewExtensionPopupManager(app, app.extensionManager)

	// Set BrowserApp as the ViewLookup for the extension manager
	// This allows the dispatcher to find WebViews by ID for port messaging
	app.extensionManager.SetViewLookup(app)
	// Provide workspace pane callbacks so tabs.* APIs can return real tab info
	app.extensionManager.SetPaneCallbacks(app.webExtGetAllPanes, app.webExtGetActivePane)
	// Register popup manager as storage change notifier
	app.extensionManager.SetStorageChangeNotifier(app.popupManager.NotifyStorageChange)
	// Wire popup manager to browserAction API for openPopup support
	if dispatcher := app.extensionManager.GetDispatcher(); dispatcher != nil {
		dispatcher.SetPopupManager(app.popupManager)
		// Set popup info provider for runtime.connect sender context
		dispatcher.SetPopupInfoProvider(app.popupManager)
	}

	// Load installed extensions from database before touching disk, keeping DB as source of truth.
	if err := app.extensionManager.LoadExtensionsFromDB(); err != nil {
		log.Printf("[webext] Warning: failed to load extensions from database: %v", err)
	}

	// Ensure uBlock Origin is installed (downloads latest version if not present)
	if err := app.extensionManager.EnsureUBlockOrigin(); err != nil {
		log.Printf("[webext] Warning: failed to ensure uBlock Origin: %v", err)
	}

	// Load extensions from unified directory (~/.local/share/dumber/extensions)
	if err := app.extensionManager.LoadExtensions(extensionsDir); err != nil {
		log.Printf("[webext] Warning: failed to load extensions: %v", err)
	}

	// List all loaded extensions
	exts := app.extensionManager.ListExtensions()
	log.Printf("[webext] Loaded %d extension(s)", len(exts))
	for _, ext := range exts {
		status := "disabled"
		if app.extensionManager.IsEnabled(ext.ID) {
			status = "enabled"
		}
		bundled := ""
		if ext.Bundled {
			bundled = " [bundled]"
		}
		log.Printf("[webext]   - %s v%s (%s)%s", ext.Manifest.Name, ext.Manifest.Version, status, bundled)
	}

	// Start background contexts so extensions can register listeners (e.g., webRequest)
	// NOTE: Background WebViews are created AFTER the main WebView in Run() to ensure WebContext is initialized
	for _, ext := range exts {
		if !app.extensionManager.IsEnabled(ext.ID) {
			continue
		}
		if err := app.extensionManager.StartBackgroundContext(ext); err != nil {
			log.Printf("[webext] Warning: failed to start background context for %s: %v", ext.ID, err)
		}
	}
	log.Printf("[webext] extensions with webRequest capability: %v", app.extensionManager.GetEnabledExtensionsWithWebRequest())

	// Start webRequest socket server for blocking IPC
	// This MUST happen before SerializeInitData so the socket path is available
	var socketPath string
	if err := app.startWebRequestServer(); err != nil {
		log.Printf("[webext] Warning: failed to start webRequest server: %v", err)
		log.Printf("[webext] webRequest blocking will be DISABLED")
	} else if app.webRequestServer != nil {
		socketPath = app.webRequestServer.SocketPath()
	}

	// Serialize extension data for WebProcess
	initDataOpts := &webext.SerializeInitDataOpts{
		EnableWebRequestMetrics: app.config.Debug.EnableWebRequestMetrics,
		WebRequestSocketPath:    socketPath,
	}
	log.Printf("[webext] Serializing init data with socket path: %s", socketPath)
	initData, err := app.extensionManager.SerializeInitData(initDataOpts)
	if err != nil {
		log.Printf("[webext] Warning: failed to serialize extension data: %v", err)
		initData = "" // Continue without extension data
	}

	// Extract embedded WebProcess extension .so to user's libexec directory
	webExtDir, err := webext.EnsureWebExtSO(app.assets)
	if err != nil {
		return fmt.Errorf("failed to extract WebProcess extension: %w", err)
	}

	// Setup WebContext to load WebProcess extension
	// This must be done BEFORE creating any WebViews
	webExtConfig := &webkit.WebExtensionConfig{
		ExtensionsDirectory: webExtDir,
		InitUserData:        initData, // Pass enabled extensions to WebProcess
	}

	// Add socket directory to sandbox so WebProcess can connect
	if socketPath != "" {
		socketDir := filepath.Dir(socketPath)
		webExtConfig.SandboxPaths = []string{socketDir}
	}

	if err := webkit.InitializeWebProcessExtensions(webExtConfig); err != nil {
		return fmt.Errorf("failed to initialize WebProcess extensions: %w", err)
	}

	// NOTE: We do NOT register a WebContext-level message handler here.
	// Per-WebView handlers are registered in registerExtensionMessageHandler() which
	// provides the viewID for proper context. Using both would cause double message handling.

	log.Printf("[webext] Extension manager initialized successfully")
	return nil
}

// startWebRequestServer initializes the UNIX socket server for webRequest IPC.
func (app *BrowserApp) startWebRequestServer() error {
	// Determine socket path from config or auto-detect
	socketPath := app.config.WebRequest.SocketPath
	if socketPath == "" {
		runtimeDir, err := config.GetRuntimeDir()
		if err != nil {
			return fmt.Errorf("failed to get runtime dir: %w", err)
		}
		socketPath = filepath.Join(runtimeDir, "webrequest.sock")
	}

	// Create and start the server
	app.webRequestServer = NewWebRequestServer(app.extensionManager)
	if err := app.webRequestServer.Start(socketPath); err != nil {
		app.webRequestServer = nil
		return err
	}

	return nil
}

// registerExtensionMessageHandler registers the extension message handler on a WebView
// This must be called for every WebView to enable WebProcess extension communication
func (app *BrowserApp) registerExtensionMessageHandler(view *webkit.WebView) {
	if view == nil {
		return
	}
	// Capture the WebView ID in a closure so we can pass it to the handler
	viewID := view.ID()
	view.OnUserMessage(func(message *webkit.UserMessage) bool {
		return app.handleExtensionMessageWithView(viewID, message)
	})
	app.registerWebRequestSignals(view)
	log.Printf("[webext] Registered extension message handler on WebView %d", viewID)
}

func (app *BrowserApp) registerWebRequestSignals(view *webkit.WebView) {
	if view == nil || app.extensionManager == nil {
		return
	}

	// DISABLED: UI-process resource tracking causes WebKit assertion failures
	// because resource.Response() returns invalid wrappers during the request lifecycle.
	// The WebProcess extension handles onBeforeRequest/onBeforeSendHeaders blocking.
	// Response events (onHeadersReceived, onCompleted) are not yet properly implemented.
	//
	// TODO: Implement response tracking properly, possibly by:
	// 1. Only accessing response in Finished signal
	// 2. Adding nil checks for underlying C pointers
	// 3. Using WebProcess-side response events instead
	log.Printf("[webRequest] UI-process resource tracking disabled (WebProcess handles blocking)")
}

// handleExtensionMessageWithView handles messages from WebProcess extensions with WebView context
func (app *BrowserApp) handleExtensionMessageWithView(viewID uint64, message *webkit.UserMessage) bool {
	name := message.Name()

	switch {
	case name == "webRequest:onBeforeRequest":
		return app.handleWebRequestOnBeforeRequest(viewID, message)

	// Note: onBeforeSendHeaders is now handled together with onBeforeRequest
	// in a single IPC call to reduce overhead

	case strings.HasPrefix(name, "webext:api"):
		// Route to webext API dispatcher with WebView context
		if app.extensionManager != nil {
			dispatcher := app.extensionManager.GetDispatcher()
			if dispatcher != nil {
				return dispatcher.HandleUserMessageWithView(viewID, message)
			}
		}
		log.Printf("[webext] No dispatcher available for webext:api message")
		// Return true to acknowledge the message even if dispatcher is unavailable
		// (false would make WebKit log "message not handled" error)
		return true

	case name == "debug:log":
		// Debug messages from web process extension
		params := message.Parameters()
		if params != nil {
			logMsg := params.String()
			// Remove GVariant quotes if present
			if len(logMsg) >= 2 && logMsg[0] == '"' && logMsg[len(logMsg)-1] == '"' {
				logMsg = logMsg[1 : len(logMsg)-1]
			}
			log.Printf("[WebProcess-DEBUG] %s", logMsg)
		}
		return true

	case name == "extension:log":
		// Structured log messages from WebExtension process
		return app.handleExtensionLog(message)

	default:
		log.Printf("[webext] Unhandled message type: %s", name)
	}

	// Return false for truly unhandled messages (not recognized)
	return false
}

// handleExtensionLog handles structured log messages from the WebExtension process
func (app *BrowserApp) handleExtensionLog(message *webkit.UserMessage) bool {
	params := message.Parameters()
	if params == nil {
		return true
	}

	jsonStr := params.String()
	// Remove GVariant quotes if present
	if len(jsonStr) >= 2 && jsonStr[0] == '"' && jsonStr[len(jsonStr)-1] == '"' {
		jsonStr = jsonStr[1 : len(jsonStr)-1]
	}

	// Parse the log entry
	var logEntry struct {
		Level     string `json:"level"`
		Message   string `json:"message"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &logEntry); err != nil {
		log.Printf("[webext] Failed to parse extension log: %v", err)
		return true
	}

	// Format log entry with timestamp and level
	timestamp := time.UnixMilli(logEntry.Timestamp)
	formattedLog := fmt.Sprintf("[%s] [%s] %s\n",
		timestamp.Format("2006-01-02 15:04:05.000"),
		strings.ToUpper(logEntry.Level),
		logEntry.Message,
	)

	// Write to WebExtension logger if available
	if app.webExtLogger != nil {
		if _, err := app.webExtLogger.Write([]byte(formattedLog)); err != nil {
			log.Printf("[webext] Failed to write to WebExtension log: %v", err)
		}
	} else {
		// Fallback to standard log if WebExtension logger not initialized
		log.Printf("[WebExt-%s] %s", strings.ToUpper(logEntry.Level), logEntry.Message)
	}

	return true
}

func (app *BrowserApp) handleWebRequestOnBeforeRequest(viewID uint64, message *webkit.UserMessage) bool {
	if app.extensionManager == nil {
		log.Printf("[webRequest] extensionManager is nil!")
		return false
	}

	params := message.Parameters()
	requestStr := variantToString(params)
	if requestStr == "" {
		app.replyWebRequestDecision(message, webRequestDecision{})
		return true
	}

	var details api.RequestDetails
	if err := json.Unmarshal([]byte(requestStr), &details); err != nil {
		log.Printf("[webRequest] Failed to decode request details: %v", err)
		app.replyWebRequestDecision(message, webRequestDecision{})
		return true
	}

	// Fix TabID: the web process uses page.ID() which differs from our WebView ID.
	// Override with the correct viewID from the message handler context.
	if viewID > 0 {
		details.TabID = int64(viewID)
		details.FrameID = int64(viewID) // Main frame ID should match tab ID
	}

	app.rememberPendingRequest(details)

	// Avoid routing internal extension resource requests back into the extension background.
	if strings.HasPrefix(details.URL, "dumb-extension://") || strings.HasPrefix(details.URL, "about:") {
		app.replyWebRequestDecision(message, webRequestDecision{})
		return true
	}

	enabledExts := app.extensionManager.GetEnabledExtensionsWithWebRequest()
	resp := webRequestDecision{}
	for _, extID := range enabledExts {
		bgResp, err := app.extensionManager.DispatchWebRequestEvent(extID, "onBeforeRequest", details)
		if err != nil {
			log.Printf("[webRequest] onBeforeRequest error for %s: %v", extID, err)
			continue
		}
		if bgResp != nil {
			resp.Cancel = resp.Cancel || bgResp.Cancel
			if resp.RedirectURL == "" {
				resp.RedirectURL = bgResp.RedirectURL
			}
			if resp.RequestHeaders == nil && bgResp.RequestHeaders != nil {
				resp.RequestHeaders = bgResp.RequestHeaders
			}
			break
		}
	}

	app.replyWebRequestDecision(message, resp)
	return true
}

// variantToString safely extracts a string from a GLib variant.
func variantToString(v *glib.Variant) string {
	if v == nil {
		return ""
	}

	if v.TypeString() == "s" {
		return strings.Trim(v.String(), "'")
	}

	printed := v.Print(false)
	if len(printed) >= 2 && printed[0] == '\'' && printed[len(printed)-1] == '\'' {
		return printed[1 : len(printed)-1]
	}

	return ""
}

type webRequestDecision struct {
	Cancel         bool              `json:"cancel"`
	RedirectURL    string            `json:"redirectUrl,omitempty"`
	RequestHeaders map[string]string `json:"requestHeaders,omitempty"`
}

func (app *BrowserApp) replyWebRequestDecision(message *webkit.UserMessage, decision webRequestDecision) {
	payload, err := json.Marshal(decision)
	if err != nil {
		log.Printf("[webRequest] Failed to marshal response: %v", err)
		return
	}

	// Reply messages must have an empty name - WebKit routes them back by message ID
	reply := webkitv6.NewUserMessage("", glib.NewVariantString(string(payload)))
	message.SendReply(reply)
}

func (app *BrowserApp) forwardWebRequestOnBeforeRequest(extID string, details api.RequestDetails) *api.BlockingResponse {
	if app.extensionManager == nil {
		return nil
	}

	resp, err := app.extensionManager.DispatchWebRequestEvent(extID, "onBeforeRequest", details)
	if err != nil {
		log.Printf("[webRequest] onBeforeRequest dispatch failed: %v", err)
		return nil
	}
	return resp
}

func (app *BrowserApp) registerWebRequestWaiter(requestID string) chan *api.BlockingResponse {
	ch := make(chan *api.BlockingResponse, 1)
	app.webRequestMu.Lock()
	if app.webRequestReplies == nil {
		app.webRequestReplies = make(map[string]chan *api.BlockingResponse)
	}
	app.webRequestReplies[requestID] = ch
	app.webRequestMu.Unlock()
	return ch
}

func (app *BrowserApp) clearWebRequestWaiter(requestID string) {
	app.webRequestMu.Lock()
	delete(app.webRequestReplies, requestID)
	app.webRequestMu.Unlock()
}

func (app *BrowserApp) rememberPendingRequest(details api.RequestDetails) {
	key := webRequestKey(details.TabID, details.URL, details.Method)
	app.webRequestMu.Lock()
	app.webRequestPending[key] = append(app.webRequestPending[key], details)
	app.webRequestMu.Unlock()
}

func (app *BrowserApp) takePendingRequest(tabID int64, url, method string) (api.RequestDetails, bool) {
	key := webRequestKey(tabID, url, method)
	app.webRequestMu.Lock()
	defer app.webRequestMu.Unlock()

	queue := app.webRequestPending[key]
	if len(queue) == 0 {
		return api.RequestDetails{}, false
	}

	details := queue[0]
	if len(queue) == 1 {
		delete(app.webRequestPending, key)
	} else {
		app.webRequestPending[key] = queue[1:]
	}

	return details, true
}

func webRequestKey(tabID int64, url, method string) string {
	return fmt.Sprintf("%d|%s|%s", tabID, method, url)
}

func (app *BrowserApp) handleResourceLoadStarted(view *webkit.WebView, resource *webkit.WebResource, request *webkit.URIRequest) {
	if view == nil || resource == nil || request == nil || app.extensionManager == nil {
		return
	}

	// Skip if no extensions have webRequest capability - avoids unnecessary processing
	// and prevents assertion failures from accessing invalid response objects
	if !app.extensionManager.HasWebRequestCapability() {
		return
	}

	tabID := int64(view.ID())
	method := request.HTTPMethod()
	url := request.URI()

	// Skip non-HTTP(S) resources - they don't have valid HTTP responses
	// This prevents assertion failures when accessing response properties
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return
	}

	details, ok := app.takePendingRequest(tabID, url, method)
	if !ok {
		details = app.buildRequestDetailsFromRequest(view, request)
	}

	details.RequestHeaders = extractRequestHeaders(request)
	details.TimeStamp = float64(time.Now().UnixMilli())

	track := &webRequestTrack{details: details}
	app.setActiveRequest(resource, track)

	resource.ConnectSentRequest(func(req *webkit.URIRequest, redirectedResponse *webkit.URIResponse) {
		app.handleResourceSentRequest(view, resource, req, redirectedResponse)
	})

	resource.ConnectFinished(func() {
		app.handleResourceFinished(view, resource)
	})

	resource.ConnectFailed(func(err error) {
		app.handleResourceFailed(view, resource, err)
	})

	resource.ConnectFailedWithTLSErrors(func(_ gio.TLSCertificater, _ gio.TLSCertificateFlags) {
		app.handleResourceFailed(view, resource, fmt.Errorf("tls error"))
	})
}

func (app *BrowserApp) handleResourceSentRequest(view *webkit.WebView, resource *webkit.WebResource, request *webkit.URIRequest, redirectedResponse *webkit.URIResponse) {
	track := app.getActiveRequest(resource)
	if track == nil || request == nil {
		return
	}

	track.details.URL = request.URI()
	track.details.Method = request.HTTPMethod()
	track.details.RequestHeaders = extractRequestHeaders(request)
	track.details.TimeStamp = float64(time.Now().UnixMilli())

	// Handle redirect response - this is valid since it's from the previous request
	if redirectedResponse != nil {
		app.handleResponseEvent(view, track, redirectedResponse)
	}
	// NOTE: Do NOT call resource.Response() here - it's invalid during SentRequest.
	// The response will be processed in handleResourceFinished when it's actually available.
}

func (app *BrowserApp) handleResourceFinished(view *webkit.WebView, resource *webkit.WebResource) {
	track := app.getActiveRequest(resource)
	if track == nil {
		return
	}

	if resp := resource.Response(); resp != nil {
		app.handleResponseEvent(view, track, resp)
		app.dispatchCompletion(view, track, resp)
	} else {
		app.dispatchCompletion(view, track, nil)
	}

	app.clearActiveRequest(resource)
}

func (app *BrowserApp) handleResourceFailed(_ *webkit.WebView, resource *webkit.WebResource, err error) {
	track := app.getActiveRequest(resource)
	if track == nil {
		return
	}

	details := track.details
	payload := map[string]interface{}{
		"details": details,
		"error":   err.Error(),
	}
	for _, extID := range app.extensionManager.GetEnabledExtensionsWithWebRequest() {
		if _, dispatchErr := app.extensionManager.DispatchWebRequestEvent(extID, "onErrorOccurred", payload); dispatchErr != nil {
			log.Printf("[webRequest] onErrorOccurred dispatch failed for %s: %v", extID, dispatchErr)
		}
	}

	app.clearActiveRequest(resource)
}

func (app *BrowserApp) handleResponseEvent(view *webkit.WebView, track *webRequestTrack, resp *webkit.URIResponse) {
	if track == nil || resp == nil || app.extensionManager == nil {
		return
	}

	// Skip response processing if no extensions have webRequest capability
	if !app.extensionManager.HasWebRequestCapability() {
		return
	}

	details := app.buildResponseDetails(track.details, resp)
	matching := app.extensionManager.GetEnabledExtensionsWithWebRequest()

	if !track.headersSent {
		for _, extID := range matching {
			bgResp, err := app.extensionManager.DispatchWebRequestEvent(extID, "onHeadersReceived", details)
			if err != nil {
				log.Printf("[webRequest] onHeadersReceived dispatch failed for %s: %v", extID, err)
				continue
			}
			if bgResp == nil {
				continue
			}
			if bgResp.ResponseHeaders != nil {
				if headers := resp.HTTPHeaders(); isMessageHeadersValid(headers) {
					for name, value := range bgResp.ResponseHeaders {
						headers.Replace(name, value)
					}
				}
			}
			if bgResp.RedirectURL != "" && view != nil {
				view.LoadURI(bgResp.RedirectURL)
			}
			if bgResp.Cancel && view != nil {
				view.StopLoading()
				break
			}
		}
		track.headersSent = true
	}

	if !track.responseStarted {
		for _, extID := range matching {
			if _, err := app.extensionManager.DispatchWebRequestEvent(extID, "onResponseStarted", details); err != nil {
				log.Printf("[webRequest] onResponseStarted dispatch failed for %s: %v", extID, err)
			}
		}
		track.responseStarted = true
	}
}

func (app *BrowserApp) dispatchCompletion(_ *webkit.WebView, track *webRequestTrack, resp *webkit.URIResponse) {
	if track == nil || app.extensionManager == nil {
		return
	}

	// Skip if no extensions have webRequest capability
	if !app.extensionManager.HasWebRequestCapability() {
		return
	}

	var details api.ResponseDetails
	if resp != nil {
		details = app.buildResponseDetails(track.details, resp)
	} else {
		details = api.ResponseDetails{
			RequestID: track.details.RequestID,
			URL:       track.details.URL,
			Method:    track.details.Method,
			FrameID:   track.details.FrameID,
			TabID:     track.details.TabID,
			Type:      track.details.Type,
			TimeStamp: float64(time.Now().UnixMilli()),
		}
	}

	for _, extID := range app.extensionManager.GetEnabledExtensionsWithWebRequest() {
		if _, err := app.extensionManager.DispatchWebRequestEvent(extID, "onCompleted", details); err != nil {
			log.Printf("[webRequest] onCompleted dispatch failed for %s: %v", extID, err)
		}
	}
}

func (app *BrowserApp) buildRequestDetailsFromRequest(view *webkit.WebView, request *webkit.URIRequest) api.RequestDetails {
	tabID := int64(0)
	if view != nil {
		tabID = int64(view.ID())
	}

	return api.RequestDetails{
		RequestID:     fmt.Sprintf("%d-%d", tabID, time.Now().UnixNano()),
		URL:           request.URI(),
		Method:        request.HTTPMethod(),
		FrameID:       tabID,
		ParentFrameID: -1,
		TabID:         tabID,
		Type:          api.ResourceTypeOther,
		TimeStamp:     float64(time.Now().UnixMilli()),
		Initiator:     "",
	}
}

func (app *BrowserApp) buildResponseDetails(req api.RequestDetails, resp *webkit.URIResponse) api.ResponseDetails {
	headers := map[string]string{}
	var statusCode uint

	// Only access response if valid - we filter non-HTTP URLs upstream
	// so responses here should be valid HTTP responses
	if resp != nil {
		statusCode = resp.StatusCode()
		if respHeaders := resp.HTTPHeaders(); isMessageHeadersValid(respHeaders) {
			respHeaders.ForEach(func(name, value string) {
				headers[name] = value
			})
		}
	}

	// Use request URL - response URL should be the same for non-redirected requests
	return api.ResponseDetails{
		RequestID:       req.RequestID,
		URL:             req.URL,
		Method:          req.Method,
		FrameID:         req.FrameID,
		ParentFrameID:   req.ParentFrameID,
		TabID:           req.TabID,
		Type:            req.Type,
		TimeStamp:       float64(time.Now().UnixMilli()),
		StatusCode:      int(statusCode),
		StatusLine:      fmt.Sprintf("HTTP %d", statusCode),
		ResponseHeaders: headers,
	}
}

func (app *BrowserApp) setActiveRequest(resource *webkit.WebResource, track *webRequestTrack) {
	key := uintptr(unsafe.Pointer(resource))
	app.webRequestMu.Lock()
	app.webRequestActive[key] = track
	app.webRequestMu.Unlock()
}

func (app *BrowserApp) getActiveRequest(resource *webkit.WebResource) *webRequestTrack {
	key := uintptr(unsafe.Pointer(resource))
	app.webRequestMu.Lock()
	defer app.webRequestMu.Unlock()
	return app.webRequestActive[key]
}

func (app *BrowserApp) clearActiveRequest(resource *webkit.WebResource) {
	key := uintptr(unsafe.Pointer(resource))
	app.webRequestMu.Lock()
	delete(app.webRequestActive, key)
	app.webRequestMu.Unlock()
}

// isMessageHeadersValid checks if the underlying C pointer of a soup.MessageHeaders
// is not NULL. The gotk4 binding for HTTPHeaders() has a bug where it creates a Go
// wrapper even when the C function returns NULL, causing crashes when ForEach is called.
func isMessageHeadersValid(hdrs *soup.MessageHeaders) bool {
	if hdrs == nil {
		return false
	}
	return gextras.StructNative(unsafe.Pointer(hdrs)) != nil
}

func extractRequestHeaders(request *webkit.URIRequest) map[string]string {
	headers := map[string]string{}
	if request == nil {
		return headers
	}

	if httpHeaders := request.HTTPHeaders(); isMessageHeadersValid(httpHeaders) {
		httpHeaders.ForEach(func(name, value string) {
			headers[name] = value
		})
	}
	return headers
}

// handleWebRequestResponse is invoked by the extension manager when a JS listener replies.
func (app *BrowserApp) handleWebRequestResponse(requestID string, resp *api.BlockingResponse) {
	app.webRequestMu.Lock()
	ch, ok := app.webRequestReplies[requestID]
	app.webRequestMu.Unlock()
	if !ok {
		return
	}

	select {
	case ch <- resp:
	default:
	}
}

func (app *BrowserApp) sendWebRequestEvent(view *webkit.WebView, evt map[string]interface{}) {
	if view == nil || evt == nil {
		return
	}

	payload, err := json.Marshal(evt)
	if err != nil {
		log.Printf("[webRequest] Failed to marshal event: %v", err)
		return
	}

	script := fmt.Sprintf(`try {
		if (window.__dumberWebExtReceive) {
			window.__dumberWebExtReceive(%s);
		} else {
			console.error('[webRequest] __dumberWebExtReceive missing');
		}
	} catch (e) { console.error('[webRequest] deliver failed', e); }`, string(payload))

	view.RunOnMainThread(func() {
		webkit.EvaluateJavascript(view.GetWebView(), script)
	})
}

type webExtRequest struct {
	Action      string          `json:"action"`
	ExtensionID string          `json:"extensionId"`
	RequestID   string          `json:"requestId"`
	Payload     json.RawMessage `json:"payload"`
}

type webExtResponse struct {
	RequestID string      `json:"requestId"`
	Success   bool        `json:"success"`
	Result    interface{} `json:"result,omitempty"`
	Error     string      `json:"error,omitempty"`
}

type webExtPortBridge struct {
	extensionID string
	view        *webkit.WebView
}

type webRequestTrack struct {
	details         api.RequestDetails
	responseStarted bool
	headersSent     bool
}

// handleWebExtMessage bridges extension page API calls to native implementations.
func (app *BrowserApp) handleWebExtMessage(view *webkit.WebView, payload string) {
	if app.extensionManager == nil || view == nil {
		return
	}

	var req webExtRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		log.Printf("[webext] Failed to parse message from extension page: %v", err)
		return
	}

	log.Printf("[webext] bridge request action=%s extension=%s", req.Action, req.ExtensionID)

	resp := webExtResponse{
		RequestID: req.RequestID,
	}

	ext, ok := app.extensionManager.GetExtension(req.ExtensionID)
	if !ok || ext == nil || !app.extensionManager.IsEnabled(req.ExtensionID) {
		resp.Error = "extension unavailable"
		app.sendWebExtResponse(view, resp)
		return
	}

	switch req.Action {
	case "init":
		resp.Success = true
		defaultLocale := ""
		if ext.Manifest != nil {
			defaultLocale = ext.Manifest.DefaultLocale
		}
		resp.Result = map[string]string{
			"defaultLocale": defaultLocale,
		}
	case "runtime.connect":
		resp = app.dispatchRuntimeConnect(ext, view, req)
	case "runtime.sendMessage":
		resp = app.dispatchRuntimeMessage(ext, req)
	case "runtime.port.postMessage":
		resp = app.dispatchPortPostMessage(ext, view, req)
	case "runtime.port.disconnect":
		resp = app.dispatchPortDisconnect(ext, view, req)
	case "storage.get":
		resp = app.dispatchStorageGet(ext, req)
	case "storage.set":
		resp = app.dispatchStorageSet(ext, req)
	case "storage.remove":
		resp = app.dispatchStorageRemove(ext, req)
	case "storage.clear":
		resp = app.dispatchStorageClear(ext, req)
	default:
		resp.Error = "unknown action"
	}

	app.sendWebExtResponse(view, resp)
}

func (app *BrowserApp) dispatchRuntimeMessage(ext *webext.Extension, req webExtRequest) webExtResponse {
	resp := webExtResponse{RequestID: req.RequestID}

	var payload struct {
		Message interface{} `json:"message"`
		URL     string      `json:"url"`
	}

	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			resp.Error = fmt.Sprintf("invalid runtime payload: %v", err)
			return resp
		}
	}

	sender := api.MessageSender{
		ID:  ext.ID,
		URL: payload.URL,
	}

	result, err := app.extensionManager.DispatchRuntimeMessage(ext.ID, sender, payload.Message)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	resp.Success = true
	resp.Result = result

	return resp
}

func (app *BrowserApp) dispatchRuntimeConnect(ext *webext.Extension, view *webkit.WebView, req webExtRequest) webExtResponse {
	resp := webExtResponse{RequestID: req.RequestID}

	var payload struct {
		PortID string `json:"portId"`
		Name   string `json:"name"`
		URL    string `json:"url"`
	}

	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			resp.Error = fmt.Sprintf("invalid runtime.connect payload: %v", err)
			return resp
		}
	}

	if payload.PortID == "" {
		payload.PortID = fmt.Sprintf("port-%d", time.Now().UnixNano())
	}

	sender := api.MessageSender{
		ID:  ext.ID,
		URL: payload.URL,
	}

	callbacks := api.PortCallbacks{
		OnMessage: func(msg interface{}) {
			app.sendWebExtPortEvent(view, map[string]interface{}{
				"type":    "port-message",
				"portId":  payload.PortID,
				"message": msg,
			})
		},
		OnDisconnect: func() {
			app.sendWebExtPortEvent(view, map[string]interface{}{
				"type":   "port-disconnect",
				"portId": payload.PortID,
			})
			app.removePortBridge(payload.PortID)
		},
	}

	desc := api.PortDescriptor{
		ID:        payload.PortID,
		Name:      payload.Name,
		Sender:    sender,
		Callbacks: callbacks,
	}

	if err := app.extensionManager.ConnectBackgroundPort(ext.ID, desc); err != nil {
		resp.Error = fmt.Sprintf("connect failed: %v", err)
		return resp
	}

	app.registerPortBridge(payload.PortID, ext.ID, view)
	log.Printf("[webext] runtime.connect bridged to background context ext=%s port=%s name=%s", ext.ID, payload.PortID, payload.Name)

	resp.Success = true
	resp.Result = map[string]string{
		"portId": payload.PortID,
	}
	return resp
}

func (app *BrowserApp) dispatchPortPostMessage(ext *webext.Extension, senderView *webkit.WebView, req webExtRequest) webExtResponse {
	resp := webExtResponse{RequestID: req.RequestID}

	var payload struct {
		PortID  string      `json:"portId"`
		Message interface{} `json:"message"`
	}
	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			resp.Error = fmt.Sprintf("invalid port.postMessage payload: %v", err)
			return resp
		}
	}

	if payload.PortID == "" {
		resp.Error = "portId is required"
		return resp
	}

	if err := app.extensionManager.DeliverPortMessage(ext.ID, payload.PortID, payload.Message); err != nil {
		resp.Error = err.Error()
		return resp
	}

	log.Printf("[webext] port.postMessage ext=%s port=%s", ext.ID, payload.PortID)
	resp.Success = true
	return resp
}

func (app *BrowserApp) dispatchPortDisconnect(ext *webext.Extension, senderView *webkit.WebView, req webExtRequest) webExtResponse {
	resp := webExtResponse{RequestID: req.RequestID}

	var payload struct {
		PortID string `json:"portId"`
	}
	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			resp.Error = fmt.Sprintf("invalid port.disconnect payload: %v", err)
			return resp
		}
	}

	if payload.PortID == "" {
		resp.Error = "portId is required"
		return resp
	}

	app.extensionManager.DisconnectPort(ext.ID, payload.PortID)
	app.removePortBridge(payload.PortID)
	log.Printf("[webext] port.disconnect ext=%s port=%s", ext.ID, payload.PortID)
	resp.Success = true
	return resp
}

func (app *BrowserApp) dispatchStorageGet(ext *webext.Extension, req webExtRequest) webExtResponse {
	resp := webExtResponse{RequestID: req.RequestID}

	if ext.Storage == nil {
		resp.Error = "storage unavailable"
		return resp
	}

	var payload struct {
		Keys interface{} `json:"keys"`
	}

	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			resp.Error = fmt.Sprintf("invalid storage payload: %v", err)
			return resp
		}
	}

	keys := normalizeStorageKeys(payload.Keys)
	items, err := ext.Storage.Local().Get(keys)
	if err != nil {
		resp.Error = fmt.Sprintf("storage.get failed: %v", err)
		return resp
	}

	resp.Success = true
	resp.Result = items
	return resp
}

func (app *BrowserApp) dispatchStorageSet(ext *webext.Extension, req webExtRequest) webExtResponse {
	resp := webExtResponse{RequestID: req.RequestID}

	if ext.Storage == nil {
		resp.Error = "storage unavailable"
		return resp
	}

	var payload struct {
		Items map[string]interface{} `json:"items"`
	}

	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			resp.Error = fmt.Sprintf("invalid storage payload: %v", err)
			return resp
		}
	}

	if payload.Items == nil {
		resp.Error = "storage.set requires items"
		return resp
	}

	if err := ext.Storage.Local().Set(payload.Items); err != nil {
		resp.Error = fmt.Sprintf("storage.set failed: %v", err)
		return resp
	}

	resp.Success = true
	return resp
}

func (app *BrowserApp) dispatchStorageRemove(ext *webext.Extension, req webExtRequest) webExtResponse {
	resp := webExtResponse{RequestID: req.RequestID}

	if ext.Storage == nil {
		resp.Error = "storage unavailable"
		return resp
	}

	var payload struct {
		Keys interface{} `json:"keys"`
	}

	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			resp.Error = fmt.Sprintf("invalid storage payload: %v", err)
			return resp
		}
	}

	keys, ok := extractStringKeys(payload.Keys)
	if !ok {
		resp.Error = "storage.remove keys must be string or array"
		return resp
	}

	if err := ext.Storage.Local().Remove(keys); err != nil {
		resp.Error = fmt.Sprintf("storage.remove failed: %v", err)
		return resp
	}

	resp.Success = true
	return resp
}

func (app *BrowserApp) dispatchStorageClear(ext *webext.Extension, req webExtRequest) webExtResponse {
	resp := webExtResponse{RequestID: req.RequestID}

	if ext.Storage == nil {
		resp.Error = "storage unavailable"
		return resp
	}

	if err := ext.Storage.Local().Clear(); err != nil {
		resp.Error = fmt.Sprintf("storage.clear failed: %v", err)
		return resp
	}

	resp.Success = true
	return resp
}

func (app *BrowserApp) sendWebExtResponse(view *webkit.WebView, resp webExtResponse) {
	if view == nil || resp.RequestID == "" {
		return
	}

	payload, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[webext] Failed to marshal response: %v", err)
		return
	}

	script := fmt.Sprintf(`try { window.__dumberWebExtReceive && window.__dumberWebExtReceive(%s); } catch (e) { console.error('[webext] response delivery failed', e); }`, string(payload))

	view.RunOnMainThread(func() {
		webkit.EvaluateJavascript(view.GetWebView(), script)
	})
}

func (app *BrowserApp) registerPortBridge(portID, extID string, view *webkit.WebView) {
	if portID == "" || view == nil {
		return
	}
	if app.webExtPorts == nil {
		app.webExtPorts = make(map[string]*webExtPortBridge)
	}
	app.webExtPorts[portID] = &webExtPortBridge{
		extensionID: extID,
		view:        view,
	}
}

func (app *BrowserApp) removePortBridge(portID string) {
	if portID == "" || app.webExtPorts == nil {
		return
	}
	delete(app.webExtPorts, portID)
}

func (app *BrowserApp) getPortBridge(portID string) (*webExtPortBridge, bool) {
	if app.webExtPorts == nil || portID == "" {
		return nil, false
	}
	bridge, ok := app.webExtPorts[portID]
	return bridge, ok
}

func (app *BrowserApp) sendWebExtPortEvent(view *webkit.WebView, evt map[string]interface{}) {
	if view == nil || evt == nil {
		log.Printf("[webext] sendWebExtPortEvent: view or evt is nil")
		return
	}

	payload, err := json.Marshal(evt)
	if err != nil {
		log.Printf("[webext] Failed to marshal port event: %v", err)
		return
	}

	log.Printf("[webext] Sending port event to WebView %d: %s", view.ID(), string(payload))
	script := fmt.Sprintf(`try {
		console.log('[webext] Background page receiving port event:', %s);
		if (!window.__dumberWebExtReceive) {
			console.error('[webext] window.__dumberWebExtReceive is not defined!');
		} else {
			window.__dumberWebExtReceive(%s);
		}
	} catch (e) { console.error('[webext] port event delivery failed', e, e.stack); }`, string(payload), string(payload))
	view.RunOnMainThread(func() {
		log.Printf("[webext] Executing port event script on WebView %d", view.ID())
		webkit.EvaluateJavascript(view.GetWebView(), script)
	})
}

// GetViewByID finds a WebView by its ID across all contexts (popups, tabs)
func (app *BrowserApp) GetViewByID(viewID uint64) *webkit.WebView {
	// Check all tabs' workspaces
	if app.tabManager != nil {
		for _, tab := range app.tabManager.tabs {
			if tab.workspace != nil {
				// Check all panes in the workspace
				for view := range tab.workspace.viewToNode {
					if view != nil && view.ID() == viewID {
						return view
					}
				}
			}
		}
	}

	// Fallback: check the main webView if it exists
	if app.webView != nil && app.webView.ID() == viewID {
		return app.webView
	}

	return nil
}

// GetPaneInfoByViewID finds pane information for a WebView by its ID
func (app *BrowserApp) GetPaneInfoByViewID(viewID uint64) *api.PaneInfo {
	// Search through all tabs' workspaces for this WebView
	if app.tabManager != nil {
		for tabIndex, tab := range app.tabManager.tabs {
			if tab.workspace != nil {
				// Check all panes in the workspace
				paneIndex := 0
				for view := range tab.workspace.viewToNode {
					if view != nil && view.ID() == viewID {
						// Found the pane! Build PaneInfo
						url := view.GetCurrentURL()
						title := view.GetTitle()
						if title == "" {
							title = url
						}

						return &api.PaneInfo{
							ID:       viewID,
							Index:    paneIndex,
							Active:   tab.isActive, // Tab-level activity (pane-level focus TBD)
							URL:      url,
							Title:    title,
							WindowID: 1, // Single window for now
						}
					}
					paneIndex++
				}
			}

			// For simpler tab access without workspace, check the tab's webViews map
			if tab.webViews != nil {
				for view := range tab.webViews {
					if view != nil && view.ID() == viewID {
						url := view.GetCurrentURL()
						title := view.GetTitle()
						if title == "" {
							title = url
						}

						return &api.PaneInfo{
							ID:       viewID,
							Index:    tabIndex,
							Active:   tab.isActive,
							URL:      url,
							Title:    title,
							WindowID: 1,
						}
					}
				}
			}
		}
	}

	return nil
}

// buildCORSAllowlist builds a CORS allowlist from extension permissions
// This follows Epiphany's pattern of using manifest permissions for CORS configuration
func buildCORSAllowlist(ext *webext.Extension) []string {
	if ext == nil {
		return nil
	}

	allowlist := []string{}
	seen := make(map[string]struct{})
	addPattern := func(pattern string) {
		if pattern == "" {
			return
		}
		if _, exists := seen[pattern]; exists {
			return
		}
		seen[pattern] = struct{}{}
		allowlist = append(allowlist, pattern)
	}

	hostPerms := ext.HostPermissions
	if len(hostPerms) == 0 && ext.Manifest != nil {
		hostPerms = ext.Manifest.Permissions
	}

	for _, perm := range hostPerms {
		if perm == "" {
			continue
		}

		if perm == "<all_urls>" {
			addPattern("*://*/*")
			addPattern("http://*/*")
			addPattern("https://*/*")
			continue
		}

		if strings.Contains(perm, "://") || strings.HasPrefix(perm, "*://") {
			addPattern(perm)
		}
	}

	// Always allow the extension's own scheme (for loading resources)
	addPattern(fmt.Sprintf("dumb-extension://%s/*", ext.ID))

	log.Printf("[webext] CORS allowlist for %s: %v", ext.ID, allowlist)
	return allowlist
}

// webExtGetAllPanes builds PaneInfo slices for the current workspace (used by tabs.* APIs).
func (app *BrowserApp) webExtGetAllPanes() []webext.PaneInfo {
	ws := app.workspace
	if ws == nil {
		return nil
	}

	panes := ws.GetAllPanes()
	active := ws.GetActivePane()
	result := make([]webext.PaneInfo, 0, len(panes))

	for idx, pane := range panes {
		if pane == nil || pane.webView == nil {
			continue
		}

		result = append(result, webext.PaneInfo{
			ID:       pane.webView.ID(),
			Index:    idx,
			Active:   pane == active,
			URL:      pane.webView.GetCurrentURL(),
			Title:    pane.webView.GetTitle(),
			WindowID: 1, // Single-window setup for now
		})
	}

	return result
}

// webExtGetActivePane returns PaneInfo for the active workspace pane.
func (app *BrowserApp) webExtGetActivePane() *webext.PaneInfo {
	ws := app.workspace
	if ws == nil {
		return nil
	}

	active := ws.GetActivePane()
	if active == nil || active.webView == nil {
		return nil
	}

	// Index is best-effort: find the active pane's position in the slice
	index := 0
	for i, pane := range ws.GetAllPanes() {
		if pane == active {
			index = i
			break
		}
	}

	return &webext.PaneInfo{
		ID:       active.webView.ID(),
		Index:    index,
		Active:   true,
		URL:      active.webView.GetCurrentURL(),
		Title:    active.webView.GetTitle(),
		WindowID: 1,
	}
}

// ensureBackgroundPage creates a hidden WebView for background pages so runtime.connect can target real background code.
func (app *BrowserApp) ensureBackgroundPage(ext *webext.Extension) error {
	if ext == nil || ext.Manifest == nil || ext.Manifest.Background == nil {
		return nil
	}

	if app.extensionManager != nil {
		if err := app.extensionManager.StartBackgroundContext(ext); err != nil {
			return err
		}
	}

	return nil
}

func normalizeStorageKeys(keys interface{}) interface{} {
	switch v := keys.(type) {
	case nil:
		return nil
	case string:
		return v
	case map[string]interface{}:
		return v
	case []interface{}:
		res := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				res = append(res, s)
			}
		}
		return res
	case []string:
		return v
	default:
		return nil
	}
}

func extractStringKeys(keys interface{}) (interface{}, bool) {
	switch v := keys.(type) {
	case string:
		return v, true
	case []string:
		return v, true
	case []interface{}:
		res := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				res = append(res, s)
			}
		}
		return res, true
	default:
		return nil, false
	}
}

// WebExtension Tab Operations - these bridge the WebExtension API to browser pane management
// Note: In WebExtension terminology, a "tab" is what dumber calls a "pane" (individual web page)

// CreateTab creates a new pane (WebExtension "tab") in the active workspace
func (app *BrowserApp) CreateTab(url string, active bool) (*api.Tab, error) {
	// Get active workspace
	var workspace *WorkspaceManager
	if app.tabManager != nil {
		activeTab := app.tabManager.GetActiveTab()
		if activeTab != nil {
			workspace = activeTab.workspace
		}
	} else {
		workspace = app.workspace
	}

	if workspace == nil {
		return nil, fmt.Errorf("no active workspace")
	}

	// Get the active pane from workspace to split from
	activePane := workspace.GetActivePane()
	if activePane == nil || activePane.webView == nil {
		return nil, fmt.Errorf("no active pane to split from")
	}

	// Find the pane node for the active pane's webview
	var targetNode *paneNode
	if workspace.viewToNode != nil {
		targetNode = workspace.viewToNode[activePane.webView]
	}

	if targetNode == nil {
		return nil, fmt.Errorf("active pane node not found in workspace tree")
	}

	// Split the pane to the right (default for new "tabs")
	newNode, err := workspace.SplitPane(targetNode, DirectionRight)
	if err != nil {
		return nil, fmt.Errorf("failed to create pane: %w", err)
	}

	// Get the new pane's webview
	if newNode == nil || newNode.pane == nil || newNode.pane.webView == nil {
		return nil, fmt.Errorf("new pane has no webview")
	}

	// Load URL if provided
	if url != "" {
		if err := newNode.pane.webView.LoadURL(url); err != nil {
			return nil, fmt.Errorf("failed to load URL: %w", err)
		}
	}

	// Get the pane info for the newly created pane
	newPaneInfo := app.GetPaneInfoByViewID(newNode.pane.webView.ID())
	if newPaneInfo == nil {
		return nil, fmt.Errorf("failed to get new pane info")
	}

	return &api.Tab{
		ID:       int(newPaneInfo.ID),
		Index:    newPaneInfo.Index,
		WindowID: int(newPaneInfo.WindowID),
		Active:   newPaneInfo.Active,
		URL:      newPaneInfo.URL,
		Title:    newPaneInfo.Title,
	}, nil
}

// RemoveTab closes a pane (WebExtension "tab") by its WebView ID
func (app *BrowserApp) RemoveTab(tabID int64) error {
	// Find the webview by ID
	view := app.GetViewByID(uint64(tabID))
	if view == nil {
		return fmt.Errorf("tab not found: %d", tabID)
	}

	// Find which tab/workspace contains this view and close the pane
	if app.tabManager != nil {
		for _, tab := range app.tabManager.tabs {
			if tab.workspace != nil {
				// Check if this workspace contains the view
				if node, ok := tab.workspace.viewToNode[view]; ok {
					// Found the pane node, close it
					return tab.workspace.ClosePane(node)
				}
			}
		}
	}

	// Fallback for legacy single-workspace mode (no tab manager)
	if app.workspace != nil {
		if node, ok := app.workspace.viewToNode[view]; ok {
			return app.workspace.ClosePane(node)
		}
	}

	return fmt.Errorf("tab not found in any workspace: %d", tabID)
}

// UpdateTab navigates a tab to a new URL for WebExtension API
func (app *BrowserApp) UpdateTab(tabID int64, url string) (*api.Tab, error) {
	// Find the webview by ID and navigate it
	view := app.GetViewByID(uint64(tabID))
	if view == nil {
		return nil, fmt.Errorf("tab not found: %d", tabID)
	}

	// Navigate to the URL
	if err := view.LoadURL(url); err != nil {
		return nil, fmt.Errorf("failed to navigate tab: %w", err)
	}

	// Return updated tab info
	paneInfo := app.GetPaneInfoByViewID(uint64(tabID))
	if paneInfo == nil {
		return nil, fmt.Errorf("failed to get tab info after update")
	}

	return &api.Tab{
		ID:       int(paneInfo.ID),
		Index:    paneInfo.Index,
		WindowID: int(paneInfo.WindowID),
		Active:   paneInfo.Active,
		URL:      url, // Use the new URL since GetPaneInfoByViewID might return old URL
		Title:    paneInfo.Title,
	}, nil
}

// ReloadTab reloads a tab's page for WebExtension API
func (app *BrowserApp) ReloadTab(tabID int64) error {
	// Find the webview by ID and reload it
	view := app.GetViewByID(uint64(tabID))
	if view == nil {
		return fmt.Errorf("tab not found: %d", tabID)
	}

	return view.Reload()
}

// NotifyActivePaneChanged notifies components that the active pane has changed.
// This is called by the focus state machine after a successful pane focus transition.
func (app *BrowserApp) NotifyActivePaneChanged() {
	if app == nil || app.tabManager == nil {
		return
	}

	// Notify extensions overlay to hide (prevents it from appearing on wrong pane)
	if app.tabManager.extensionsOverlay != nil {
		app.tabManager.extensionsOverlay.OnActivePaneChanged()
	}
}
