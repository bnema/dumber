package browser

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/app/constants"
	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/internal/app/environment"
	"github.com/bnema/dumber/internal/app/messaging"
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

func (app *BrowserApp) buildWebkitConfig() (*webkit.Config, error) {
	dataDir, err := config.GetDataDir()
	if err != nil {
		return nil, err
	}
	stateDir, err := config.GetStateDir()
	if err != nil {
		return nil, err
	}

	webkitData := filepath.Join(dataDir, "webkit")
	webkitCache := filepath.Join(stateDir, "webkit-cache")
	if err := os.MkdirAll(webkitData, constants.DirPerm); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(webkitCache, constants.DirPerm); err != nil {
		return nil, err
	}

	cfg := &webkit.Config{
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
	}

	return cfg, nil
}

func (app *BrowserApp) buildPane(view *webkit.WebView) (*BrowserPane, error) {
	pane := &BrowserPane{
		webView: view,
	}

	pane.clipboardController = control.NewClipboardController(view)
	pane.zoomController = control.NewZoomController(app.browserService, view)
	pane.navigationController = control.NewNavigationController(
		app.parserService,
		app.browserService,
		view,
		pane.zoomController,
	)
	pane.messageHandler = messaging.NewHandler(app.parserService, app.browserService)
	pane.messageHandler.SetWebView(view)
	pane.messageHandler.SetNavigationController(pane.navigationController)
	pane.shortcutHandler = NewShortcutHandler(view, pane.clipboardController, app.config, app)

	pane.zoomController.RegisterHandlers()
	pane.shortcutHandler.RegisterShortcuts()

	return pane, nil
}

func (app *BrowserApp) createPaneForView(view *webkit.WebView) (*BrowserPane, error) {
	pane, err := app.buildPane(view)
	if err != nil {
		return nil, err
	}

	// Generate unique pane ID
	pane.SetID(fmt.Sprintf("pane-%d-%p", time.Now().Unix(), view))
	pane.initializeGUITracking()

	// Inject minimal bootstrap script for pane initialization
	bootstrapScript := fmt.Sprintf(`
		window.__dumber_pane = {
			id: '%s',
			created: %d,
			active: false
		};

		// GUI manager will be loaded on-demand
		window.__dumber_gui_ready = true;

		console.log('[pane] Initialized pane %s');
	`, pane.ID(), time.Now().UnixMilli(), pane.ID())

	if err := view.InjectScript(bootstrapScript); err != nil {
		log.Printf("[pane-%s] Failed to inject bootstrap: %v", pane.ID(), err)
	}

	app.attachPaneHandlers(pane)
	return pane, nil
}

func (app *BrowserApp) attachPaneHandlers(pane *BrowserPane) {
	if pane == nil || pane.webView == nil {
		return
	}

	pane.webView.RegisterTitleChangedHandler(func(title string) {
		if title == "" {
			return
		}
		url := pane.webView.GetCurrentURL()
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

	pane.webView.RegisterScriptMessageHandler(func(payload string) {
		if app.workspace != nil {
			app.workspace.focusByView(pane.webView)
		}
		pane.messageHandler.Handle(payload)
	})

	if pane.messageHandler != nil && pane.navigationController != nil {
		pane.messageHandler.SetNavigationController(pane.navigationController)
	}

	pane.webView.RegisterPopupHandler(func(uri string) bool {
		handled := false
		if app.workspace != nil {
			handled = app.workspace.HandlePopup(pane.webView, uri)
		}
		if handled {
			return true
		}

		if pane.navigationController != nil && uri != "" {
			if err := pane.navigationController.NavigateToURL(uri); err != nil {
				log.Printf("[workspace] popup fallback navigation failed: %v", err)
				if pane.webView != nil {
					if loadErr := pane.webView.LoadURL(uri); loadErr != nil {
						log.Printf("[workspace] popup fallback load failed: %v", loadErr)
					}
				}
			}
			return true
		}

		if pane.webView != nil && uri != "" {
			if err := pane.webView.LoadURL(uri); err != nil {
				log.Printf("[workspace] popup direct load failed: %v", err)
			}
			return true
		}

		return false
	})
}

// createWebView creates and configures the WebView
func (app *BrowserApp) createWebView() error {
	log.Printf("Creating WebView (native backend expected: %v)", webkit.IsNativeAvailable())

	cfg, err := app.buildWebkitConfig()
	if err != nil {
		return err
	}

	// Main window needs a top-level window
	cfg.CreateWindow = true

	view, err := webkit.NewWebView(cfg)

	if err != nil {
		return err
	}

	pane, err := app.createPaneForView(view)
	if err != nil {
		return err
	}

	app.webView = view
	app.zoomController = pane.zoomController
	app.navigationController = pane.navigationController
	app.clipboardController = pane.clipboardController
	app.messageHandler = pane.messageHandler
	app.shortcutHandler = pane.shortcutHandler
	app.panes = []*BrowserPane{pane}
	app.activePane = pane

	if app.workspace == nil {
		app.workspace = NewWorkspaceManager(app, pane)
	}

	// Initialize window-level global shortcuts AFTER workspace is set up
	if window := view.Window(); window != nil {
		app.windowShortcutHandler = NewWindowShortcutHandler(window, app)
		if app.windowShortcutHandler != nil {
			log.Printf("Window-level global shortcuts initialized")
		} else {
			log.Printf("Warning: Failed to initialize window-level shortcuts")
		}
	}

	app.setupWebViewIntegration()

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
