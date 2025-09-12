package browser

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnema/dumber/internal/app/constants"
	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/internal/app/environment"
	"github.com/bnema/dumber/internal/config"
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
		InitialURL:            "dumb://homepage",
		ZoomDefault:           1.0,
		EnableDeveloperExtras: true,
		DataDir:               webkitData,
		CacheDir:              webkitCache,
		DefaultSansFont:       app.config.Appearance.SansFont,
		DefaultSerifFont:      app.config.Appearance.SerifFont,
		DefaultMonospaceFont:  app.config.Appearance.MonospaceFont,
		DefaultFontSize:       app.config.Appearance.DefaultFontSize,
		Rendering:             webkit.RenderingConfig{Mode: string(app.config.RenderingMode)},
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
	// Use native window as title updater
	if win := app.webView.Window(); win != nil {
		app.browserService.SetWindowTitleUpdater(win)
	}
}

// setupWebViewHandlers configures WebView event handlers
func (app *BrowserApp) setupWebViewHandlers() {
	// Persist page titles to DB when they change
	app.webView.RegisterTitleChangedHandler(func(title string) {
		ctx := context.Background()
		url := app.webView.GetCurrentURL()
		if url != "" && title != "" {
			if err := app.browserService.UpdatePageTitle(ctx, url, title); err != nil {
				log.Printf("Warning: failed to update page title: %v", err)
			}
		}
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

	// Register controller handlers
	app.zoomController.RegisterHandlers()
}

// setupKeyboardShortcuts registers keyboard shortcuts
func (app *BrowserApp) setupKeyboardShortcuts() {
	app.shortcutHandler = NewShortcutHandler(app.webView, app.clipboardController)
	app.shortcutHandler.RegisterShortcuts()
}
