package browser

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/app/constants"
	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/internal/app/environment"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/filtering"
	"github.com/bnema/dumber/pkg/webkit"
)

// buildMemoryConfig converts config.WebkitMemoryConfig to webkit.MemoryConfig
func buildMemoryConfig(cfg config.WebkitMemoryConfig) webkit.MemoryConfig {
	mc := webkit.MemoryConfig{
		MemoryLimitMB:           cfg.MemoryLimitMB,
		ConservativeThreshold:   cfg.ConservativeThreshold,
		StrictThreshold:         cfg.StrictThreshold,
		KillThreshold:           cfg.KillThreshold,
		PollIntervalSeconds:     cfg.PollIntervalSeconds,
		EnableGCInterval:        cfg.EnableGCInterval,
		ProcessRecycleThreshold: cfg.ProcessRecycleThreshold,
		EnablePageCache:         cfg.EnablePageCache,
		EnableMemoryMonitoring:  cfg.EnableMemoryMonitoring,
	}

	// Convert string cache model to webkit constant
	switch strings.ToLower(cfg.CacheModel) {
	case "document_viewer", "documentviewer", "doc":
		mc.CacheModel = webkit.CacheModelDocumentViewer
	case "primary_web_browser", "primary", "primarywebbrowser":
		mc.CacheModel = webkit.CacheModelPrimaryWebBrowser
	default:
		mc.CacheModel = webkit.CacheModelWebBrowser
	}

	return mc
}

// createWebView creates and configures the WebView
func (app *BrowserApp) createWebView() error {
	log.Printf("Creating WebView (native backend expected: %v)", webkit.IsNativeAvailable())
	dataDir, _ := config.GetDataDir()
	stateDir, _ := config.GetStateDir()
	webkitData := filepath.Join(dataDir, "webkit")
	webkitCache := filepath.Join(stateDir, "webkit-cache")
	_ = os.MkdirAll(webkitData, constants.DirPerm)
	_ = os.MkdirAll(webkitCache, constants.DirPerm)

	view, err := webkit.NewWebView(&webkit.Config{
		Assets:                app.assets,
		InitialURL:            "dumb://homepage",
		ZoomDefault:           1.0,
		EnableDeveloperExtras: true,
		DataDir:               webkitData,
		CacheDir:              webkitCache,
		APIToken:              app.config.APISecurity.Token,
		DefaultSansFont:       app.config.Appearance.SansFont,
		DefaultSerifFont:      app.config.Appearance.SerifFont,
		DefaultMonospaceFont:  app.config.Appearance.MonospaceFont,
		DefaultFontSize:       app.config.Appearance.DefaultFontSize,
		Rendering:             webkit.RenderingConfig{Mode: string(app.config.RenderingMode)},
		UseDomZoom:            app.config.UseDomZoom,
		VideoAcceleration: webkit.VideoAccelerationConfig{
			EnableVAAPI:      app.config.VideoAcceleration.EnableVAAPI,
			AutoDetectGPU:    app.config.VideoAcceleration.AutoDetectGPU,
			VAAPIDriverName:  app.config.VideoAcceleration.VAAPIDriverName,
			EnableAllDrivers: app.config.VideoAcceleration.EnableAllDrivers,
			LegacyVAAPI:      app.config.VideoAcceleration.LegacyVAAPI,
		},
		Memory: buildMemoryConfig(app.config.WebkitMemory),
		CodecPreferences: webkit.CodecPreferencesConfig{
			PreferredCodecs:           strings.Split(app.config.CodecPreferences.PreferredCodecs, ","),
			BlockedCodecs:             environment.BuildBlockedCodecsList(app.config.CodecPreferences),
			ForceAV1:                  app.config.CodecPreferences.ForceAV1,
			CustomUserAgent:           app.config.CodecPreferences.CustomUserAgent,
			DisableTwitchCodecControl: app.config.CodecPreferences.DisableTwitchCodecControl,
		},
	})

	if err != nil {
		return err
	}

	app.webView = view
	app.setupWebViewIntegration()
	app.setupWebViewHandlers()
	app.setupControllers()
	app.setupKeyboardShortcuts()

	// Apply initial zoom and show window
	app.zoomController.ApplyInitialZoom()

	log.Printf("Showing WebView window…")
	if err := view.Show(); err != nil {
		log.Printf("Warning: failed to show WebView: %v", err)
	} else if !webkit.IsNativeAvailable() {
		log.Printf("Notice: running without webkit_cgo tag — no native window will be displayed.")
	}

	return nil
}

// setupWebViewIntegration connects the WebView to browser services
func (app *BrowserApp) setupWebViewIntegration() {
	app.browserService.AttachWebView(app.webView)

	// GUI bundle is loaded via WebKit user scripts in enableUserContentManager
	// No need to load separately in browser service

	// Use native window as title updater
	if win := app.webView.Window(); win != nil {
		app.browserService.SetWindowTitleUpdater(win)
	}
}

// setupWebViewHandlers configures WebView event handlers
func (app *BrowserApp) setupWebViewHandlers() {
	// Persist page titles to DB when they change
	app.webView.RegisterTitleChangedHandler(func(title string) {
		if title == "" {
			return
		}
		url := app.webView.GetCurrentURL()
		if url == "" {
			return
		}
		go func(url, title string) {
			ctx := context.Background()
			if err := app.browserService.UpdatePageTitle(ctx, url, title); err != nil {
				log.Printf("Warning: failed to update page title: %v", err)
			}
		}(url, title)
	})

	// Set WebView reference for message handler and register script messages
	app.messageHandler.SetWebView(app.webView)
	app.webView.RegisterScriptMessageHandler(app.messageHandler.Handle)

}

// setupControllers initializes and configures controller objects
func (app *BrowserApp) setupControllers() {
	// Initialize controllers
	app.zoomController = control.NewZoomController(app.browserService, app.webView)
	app.navigationController = control.NewNavigationController(
		app.parserService,
		app.browserService,
		app.webView,
		app.zoomController,
	)
	app.clipboardController = control.NewClipboardController(app.webView)

	if app.messageHandler != nil && app.navigationController != nil {
		app.messageHandler.SetNavigationController(app.navigationController)
	}

	// Register controller handlers
	app.zoomController.RegisterHandlers()
}

// setupKeyboardShortcuts registers keyboard shortcuts
func (app *BrowserApp) setupKeyboardShortcuts() {
	app.shortcutHandler = NewShortcutHandler(app.webView, app.clipboardController)
	app.shortcutHandler.RegisterShortcuts()
}

// setupContentBlocking initializes the content blocking system with proper timing
func (app *BrowserApp) setupContentBlocking() error {
	log.Printf("Initializing content blocking system...")

	// Enable WebKit debug logging if requested
	webkit.SetupWebKitDebugLogging(app.config)

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Content blocking setup panic recovered: %v", r)
		}
	}()

	// Setup filter system
	filterManager, err := filtering.SetupFilterSystem()
	if err != nil {
		log.Printf("Warning: Failed to setup filter system: %v", err)
		return nil // Don't fail browser startup, continue without filters
	}

	// Store reference to filter manager
	app.filterManager = filterManager

	// Set up callback to re-apply network filters when they become ready
	// This callback will be called from the async filter loading process
	filterManager.SetFiltersReadyCallback(func() {
		// Add small delay to avoid race conditions with WebView initialization
		go func() {
			time.Sleep(300 * time.Millisecond)
			if app.webView != nil {
				if err := app.webView.UpdateContentFilters(filterManager); err != nil {
					log.Printf("Failed to apply filters after loading: %v", err)
				} else {
					log.Printf("Successfully applied filters after async loading")
				}
			}
		}()
	})

	// Start async filter loading early (before WebView content blocking initialization)
	// This allows filters to be ready when the WebView needs them
	go func() {
		if err := filtering.InitializeFiltersAsync(filterManager); err != nil {
			log.Printf("Warning: failed to initialize filters asynchronously: %v", err)
		}
	}()

	// Initialize content blocking in WebView with delay
	// Wait for WebView to be fully loaded before setting up content blocking
	go func() {
		// Wait extra time for the first navigation to avoid preconnect interference
		log.Printf("Waiting for WebView to complete initial load before enabling content blocking...")
		time.Sleep(3000 * time.Millisecond) // 3 seconds for first load stability

		if err := app.webView.InitializeContentBlocking(filterManager); err != nil {
			log.Printf("Warning: Failed to initialize content blocking: %v", err)
			// Continue without content blocking rather than failing
		}

		// Register navigation handler for domain-specific filtering and GUI injection
		// Only register after content blocking is initialized
		app.webView.RegisterURIChangedHandler(func(uri string) {
			// Add small delay to avoid conflicts with page load
			go func() {
				time.Sleep(200 * time.Millisecond) // Slightly longer delay for stability

				// Apply content filtering if available
				if app.filterManager != nil {
					app.webView.OnNavigate(uri, app.filterManager)
				}

				// Note: GUI bundle (controls and toast) is now injected as User Script
				// in WebKit's enableUserContentManager, so it persists across all navigations
			}()
		})
	}()

	log.Printf("Content blocking system initialization started")
	return nil
}
