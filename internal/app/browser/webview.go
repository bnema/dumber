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
		UserAgent:            buildUserAgent(app.config),
		EnableJavaScript:     true,
		EnableWebGL:          true,
		EnableMediaStream:    true,
		HardwareAcceleration: true,
		DefaultFontSize:      app.config.Appearance.DefaultFontSize,
		MinimumFontSize:      8,
		DataDir:              webkitData,
		CacheDir:             webkitCache,
		AppearanceConfigJSON: app.buildAppearanceConfigJSON(),
		CreateWindow:         true, // Default to creating a window for standalone WebViews
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
			"background":       app.config.Appearance.LightPalette.Background,
			"surface":          app.config.Appearance.LightPalette.Surface,
			"surface_variant":  app.config.Appearance.LightPalette.SurfaceVariant,
			"text":             app.config.Appearance.LightPalette.Text,
			"muted":            app.config.Appearance.LightPalette.Muted,
			"accent":           app.config.Appearance.LightPalette.Accent,
			"border":           app.config.Appearance.LightPalette.Border,
		},
		"dark": map[string]string{
			"background":       app.config.Appearance.DarkPalette.Background,
			"surface":          app.config.Appearance.DarkPalette.Surface,
			"surface_variant":  app.config.Appearance.DarkPalette.SurfaceVariant,
			"text":             app.config.Appearance.DarkPalette.Text,
			"muted":            app.config.Appearance.DarkPalette.Muted,
			"accent":           app.config.Appearance.DarkPalette.Accent,
			"border":           app.config.Appearance.DarkPalette.Border,
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

	// Register favicon handlers for this webview (needed for each pane)
	// This ensures favicons are detected and cached for all panes, not just the main webview
	view.RegisterFaviconURIChangedHandler(func(pageURI, faviconURI string) {
		// Call the private handler through a public wrapper or directly via browserService
		// We can't call handleFaviconURIChanged directly as it's private, so we need to expose it
		// For now, duplicate the minimal logic here
		app.browserService.ProcessFaviconURI(pageURI, faviconURI)
	})

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
		if app.workspace != nil {
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
	app.panes = []*BrowserPane{pane}
	app.activePane = pane

	if app.workspace == nil {
		app.workspace = NewWorkspaceManager(app, pane)
		// CRITICAL: Register navigation handler for the root webview
		// This is needed because buildPane() couldn't register it when workspace was nil
		app.workspace.RegisterNavigationHandler(view)

		// CRITICAL: Setup popup handling for the root webview
		// This must be done after workspace is created so WebKit's create signal is connected
		node := app.workspace.GetNodeForWebView(view)
		if node != nil {
			app.workspace.setupPopupHandling(view, node)
			log.Printf("[webview] Setup native popup handling for root WebView ID: %d", view.ID())
		} else {
			log.Printf("[webview] WARNING: Could not find node for root WebView ID: %d", view.ID())
		}
	}

	// Initialize window-level global shortcuts AFTER workspace is set up
	if window := view.Window(); window != nil {
		app.windowShortcutHandler = NewWindowShortcutHandler(window, app)
		if app.windowShortcutHandler != nil {
			log.Printf("Window-level global shortcuts initialized")
		} else {
			log.Printf("Warning: failed to initialize window-level shortcuts")
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

	// GUI bundle is loaded via WebKit user scripts in SetupUserContentManager
	// Appearance config is also injected at document-start via UserContentManager

	// Use native window as title updater
	if win := app.webView.Window(); win != nil {
		app.browserService.SetWindowTitleUpdater(win)
	}
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

	// Set up callback to re-apply network filters when they become ready
	// This callback will be called from the async filter loading process
	filterManager.SetFiltersReadyCallback(func() {
		// Add small delay to avoid race conditions with WebView initialization
		go func() {
			time.Sleep(300 * time.Millisecond)
			if app.webView != nil {
				// Note: Content filtering is now handled via WebKit's UserContentManager
				// UpdateContentFilters would need to be implemented if needed
				log.Printf("Filters loaded - content blocking via UserContentManager")
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

		// Note: Content blocking is now handled via WebKit's UserContentManager
		// The InitializeContentBlocking API would need to be implemented if needed
		log.Printf("Content blocking enabled via UserContentManager")

		// Register navigation handler for GUI injection
		// Content filtering is handled by UserContentManager automatically
		app.webView.RegisterURIChangedHandler(func(uri string) {
			// Add small delay to avoid conflicts with page load
			go func() {
				time.Sleep(200 * time.Millisecond)

				// Note: Content filtering is automatic via UserContentManager
				// GUI bundle (controls and toast) is injected as User Script
				log.Printf("[webview] Navigated to: %s", uri)
			}()
		})
	}()

	log.Printf("Content blocking system initialization started")
	return nil
}
