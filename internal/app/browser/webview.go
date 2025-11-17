package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/bnema/dumber/internal/app/constants"
	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/internal/app/messaging"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/filtering"
	"github.com/bnema/dumber/internal/services"
	"github.com/bnema/dumber/pkg/webkit"
)

// Note: Memory management and color palettes are now handled at the application level
// The gotk4 webkit.Config only contains basic WebKit settings

func (app *BrowserApp) buildWebkitConfig() (*webkit.Config, error) {
	// Ensure data directories exist for WebKit
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

	// Build WebKit configuration with data directories for persistence
	cfg := &webkit.Config{
		UserAgent:                 buildUserAgent(app.config),
		EnableJavaScript:          true,
		EnableWebGL:               true,
		EnableMediaStream:         true,
		HardwareAcceleration:      true,
		DefaultFontSize:           app.config.Appearance.DefaultFontSize,
		MinimumFontSize:           8,
		EnablePageCache:           true, // Instant back/forward navigation (bfcache)
		EnableSmoothScrolling:     true, // Smooth scrolling animations
		DataDir:                   webkitData,
		CacheDir:                  webkitCache,
		AppearanceConfigJSON:      app.buildAppearanceConfigJSON(),
		CreateWindow:              true, // Default to creating a window for standalone WebViews
		EnableTurnstileWorkaround: true,
	}

	return cfg, nil
}

// buildUserAgent constructs the user agent string from config
func buildUserAgent(cfg *config.Config) string {
	if cfg.CodecPreferences.CustomUserAgent != "" {
		return cfg.CodecPreferences.CustomUserAgent
	}
	// Use default WebKit user agent
	return "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36"
}

// buildAppearanceConfigJSON builds the palette configuration as JSON
// The GUI expects window.__dumber_palette with just the palette data
func (app *BrowserApp) buildAppearanceConfigJSON() string {
	if app.config == nil {
		return ""
	}

	// Build palette config in the format main-world.ts expects
	// It looks for window.__dumber_palette = { "light": {...}, "dark": {...} }
	paletteConfig := map[string]interface{}{
		"light": map[string]string{
			"background":      app.config.Appearance.LightPalette.Background,
			"surface":         app.config.Appearance.LightPalette.Surface,
			"surface_variant": app.config.Appearance.LightPalette.SurfaceVariant,
			"text":            app.config.Appearance.LightPalette.Text,
			"muted":           app.config.Appearance.LightPalette.Muted,
			"accent":          app.config.Appearance.LightPalette.Accent,
			"border":          app.config.Appearance.LightPalette.Border,
		},
		"dark": map[string]string{
			"background":      app.config.Appearance.DarkPalette.Background,
			"surface":         app.config.Appearance.DarkPalette.Surface,
			"surface_variant": app.config.Appearance.DarkPalette.SurfaceVariant,
			"text":            app.config.Appearance.DarkPalette.Text,
			"muted":           app.config.Appearance.DarkPalette.Muted,
			"accent":          app.config.Appearance.DarkPalette.Accent,
			"border":          app.config.Appearance.DarkPalette.Border,
		},
	}

	payload, err := json.Marshal(paletteConfig)
	if err != nil {
		log.Printf("[webview] Failed to marshal palette config: %v", err)
		return ""
	}

	return string(payload)
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

	// Register navigation handler for workspace focus events if workspace exists
	if app.workspace != nil {
		app.workspace.RegisterNavigationHandler(view)
	}

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

	// GUI manager will be loaded on-demand when needed
	log.Printf("[webview] Created pane for webview: %d", view.ID())

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

		// Update database with new title
		go func(url, title string) {
			ctx := context.Background()
			if err := app.browserService.UpdatePageTitle(ctx, url, title); err != nil {
				log.Printf("Warning: failed to update page title: %v", err)
			}
		}(url, title)

		// Update stacked pane title bar if this pane is in a stack
		if app.workspace != nil {
			app.workspace.UpdateTitleBar(pane.webView, title)
		}
	})

	pane.webView.RegisterScriptMessageHandler(func(payload string) {
		if app.workspace != nil && shouldFocusForScriptMessage(payload) {
			app.workspace.focusByView(pane.webView)
		}
		pane.messageHandler.Handle(payload)
	})

	if pane.messageHandler != nil && pane.navigationController != nil {
		pane.messageHandler.SetNavigationController(pane.navigationController)
	}

	// Setup WebKit-native popup lifecycle using create/ready-to-show/close signals
	if app.workspace != nil {
		node := app.workspace.GetNodeForWebView(pane.webView)
		if node != nil {
			app.workspace.setupPopupHandling(pane.webView, node)
			log.Printf("[webview] Setup native popup handling for WebView ID: %d", pane.webView.ID())
		}
	}
}

// shouldFocusForScriptMessage filters script messages and only allows focus handoff
// for explicit user-driven actions (navigation, history commands, popup lifecycle).
// Background bootstrap messages (palette sync, workspace bridge chatter, etc.) are
// ignored so they don't steal focus from the user's active pane.
func shouldFocusForScriptMessage(payload string) bool {
	var msg messaging.Message
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		return true // keep legacy behaviour if parsing fails
	}

	switch msg.Type {
	case "navigate", "query", "wails", "history_recent", "history_stats", "history_search", "history_delete", "close-popup":
		return true
	default:
		return false
	}
}

// createWebView creates and configures the WebView
func (app *BrowserApp) createWebView() error {
	log.Printf("Creating WebView (native backend expected: %v)", webkit.IsNativeAvailable())

	cfg, err := app.buildWebkitConfig()
	if err != nil {
		return err
	}

	// Note: Window creation is now handled automatically by webkit.NewWebView
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

	// Initialize tab manager (which manages workspaces)
	window := view.Window()
	if window != nil && app.tabManager == nil {
		app.tabManager = NewTabManager(app, window)

		// Determine initial URL
		initialURL := "about:blank"

		// Initialize tab system (creates root container with tab bar and first tab)
		if err := app.tabManager.Initialize(initialURL); err != nil {
			log.Printf("ERROR: failed to initialize tab manager: %v", err)
			return err
		}

		// Set tab manager's root container as window content
		rootContainer := app.tabManager.GetRootContainer()
		if rootContainer != nil {
			window.SetChild(rootContainer)
			log.Printf("[tabs] Tab system initialized and set as window content")
		} else {
			log.Printf("ERROR: tab manager root container is nil")
			return fmt.Errorf("failed to get tab manager root container")
		}
	}

	// Initialize window-level global shortcuts AFTER tab manager is set up
	if window != nil {
		app.windowShortcutHandler = NewWindowShortcutHandler(window, app)
		if app.windowShortcutHandler != nil {
			log.Printf("Window-level global shortcuts initialized")
		} else {
			log.Printf("Warning: failed to initialize window-level shortcuts")
		}
	}

	app.setupWebViewIntegration()

	// Initialize FaviconService after WebView is created
	if err := app.initializeFaviconService(); err != nil {
		log.Printf("Warning: failed to initialize favicon service: %v", err)
		// Continue without favicon service - not critical
	}

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

	// GUI bundle is loaded via WebKit user scripts in SetupUserContentManager
	// Appearance config is also injected at document-start via UserContentManager

	// Use native window as title updater
	if win := app.webView.Window(); win != nil {
		app.browserService.SetWindowTitleUpdater(win)
	}
}

// initializeFaviconService creates and initializes the FaviconService
func (app *BrowserApp) initializeFaviconService() error {
	// Get the FaviconDatabase from the WebView
	faviconDB := app.webView.GetFaviconDatabase()
	if faviconDB == nil {
		return fmt.Errorf("favicon database not available")
	}

	// Get the WebKit data directory
	dataDir, err := config.GetDataDir()
	if err != nil {
		return fmt.Errorf("failed to get data directory: %w", err)
	}
	webkitData := filepath.Join(dataDir, "webkit")

	// Create FaviconService
	faviconService, err := services.NewFaviconService(faviconDB, app.queries, webkitData)
	if err != nil {
		return fmt.Errorf("failed to create favicon service: %w", err)
	}

	app.faviconService = faviconService
	log.Printf("[favicon] FaviconService initialized successfully")
	return nil
}

// setupContentBlocking initializes the content blocking system with proper timing
func (app *BrowserApp) setupContentBlocking() error {
	log.Printf("Initializing content blocking system...")

	// Enable WebKit debug logging if requested
	webkit.SetupWebKitDebugLogging()

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

	// Set up callback to apply network filters when they become ready
	// This callback will be called from the async filter loading process
	filterManager.SetFiltersReadyCallback(func() {
		log.Printf("[filtering] Filters ready, applying to WebView...")

		// Get the WebKit JSON rules from filter manager
		filterJSON, err := filterManager.GetNetworkFilters()
		if err != nil {
			log.Printf("[filtering] Failed to get network filters: %v", err)
			return
		}

		if len(filterJSON) == 0 {
			log.Printf("[filtering] No network filters to apply")
			return
		}

		log.Printf("[filtering] Got %d bytes of WebKit JSON rules", len(filterJSON))

		// Apply filters to the WebView
		// This must be done on the main thread since it touches GTK/WebKit
		if app.webView != nil {
			app.webView.RunOnMainThread(func() {
				if err := app.webView.InitializeContentBlocking(filterJSON); err != nil {
					log.Printf("[filtering] Failed to apply content filters: %v", err)
				} else {
					log.Printf("[filtering] ✅ Content blocking enabled successfully")
				}
			})
		}
	})

	// Start async filter loading
	// This allows filters to compile in the background while the browser starts
	go func() {
		if err := filtering.InitializeFiltersAsync(filterManager); err != nil {
			log.Printf("Warning: failed to initialize filters asynchronously: %v", err)
		}
	}()

	log.Printf("Content blocking system initialization started")
	return nil
}
