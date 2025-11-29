package webkit

import (
	"fmt"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/logging"
	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
)

const (
	// DumberIsolatedWorld is the name of the isolated JavaScript world
	// where Dumber's GUI scripts run. This prevents page scripts from
	// interfering with the browser's UI components.
	DumberIsolatedWorld = "dumber-isolated"
)

// SetupUserContentManager configures UserContentManager for the WebView
// This injects GUI scripts at document-start and registers message handlers
func SetupUserContentManager(view *webkit.WebView, appearanceConfigJSON string, webviewID uint64) error {
	if view == nil {
		return nil
	}

	// Get the UserContentManager from the WebView
	ucm := view.UserContentManager()
	if ucm == nil {
		logging.Warn(fmt.Sprintf("[webkit] UserContentManager is nil, skipping script injection"))
		return nil
	}

	// Inject webview ID in BOTH main world and isolated world
	// The isolated world (GUI scripts) needs this, and pages might use it for communication
	// Note: webviewID is uint64, formatted as number (not string) to avoid any injection concerns
	webviewIDScript := fmt.Sprintf(`
		window.__dumber_webview_id = %d;
		console.log('[webkit] WebView ID set in JavaScript:', window.__dumber_webview_id);
	`, webviewID)

	// Main world (for page scripts)
	ucm.AddScript(webkit.NewUserScript(
		webviewIDScript,
		webkit.UserContentInjectTopFrame,
		webkit.UserScriptInjectAtDocumentStart,
		nil,
		nil,
	))

	// Isolated world (for GUI scripts)
	ucm.AddScript(webkit.NewUserScriptForWorld(
		webviewIDScript,
		webkit.UserContentInjectTopFrame,
		webkit.UserScriptInjectAtDocumentStart,
		DumberIsolatedWorld,
		nil,
		nil,
	))
	logging.Debug(fmt.Sprintf("[webkit] Injected webview ID script for ID: %d (main + isolated world)", webviewID))

	// Inject GTK theme detection in BOTH worlds
	// The color-scheme.ts expects window.__dumber_gtk_prefers_dark to be set
	// Respect the ColorScheme config setting
	cfg := config.Get()
	var prefersDark bool
	switch cfg.Appearance.ColorScheme {
	case "prefer-dark":
		prefersDark = true
		logging.Debug(fmt.Sprintf("[webkit] Using config-forced dark theme (color_scheme: prefer-dark)"))
	case "prefer-light":
		prefersDark = false
		logging.Debug(fmt.Sprintf("[webkit] Using config-forced light theme (color_scheme: prefer-light)"))
	default:
		// "default" or empty - follow system GTK preference
		prefersDark = PrefersDarkTheme()
		logging.Debug(fmt.Sprintf("[webkit] Using system GTK theme preference (color_scheme: %s)", cfg.Appearance.ColorScheme))
	}

	gtkThemeScript := fmt.Sprintf(`window.__dumber_gtk_prefers_dark = %t;`, prefersDark)

	// Main world
	ucm.AddScript(webkit.NewUserScript(
		gtkThemeScript,
		webkit.UserContentInjectTopFrame,
		webkit.UserScriptInjectAtDocumentStart,
		nil,
		nil,
	))

	// Isolated world
	ucm.AddScript(webkit.NewUserScriptForWorld(
		gtkThemeScript,
		webkit.UserContentInjectTopFrame,
		webkit.UserScriptInjectAtDocumentStart,
		DumberIsolatedWorld,
		nil,
		nil,
	))
	logging.Debug(fmt.Sprintf("[webkit] Injected theme preference: prefersDark=%t (main + isolated world)", prefersDark))

	// Inject palette config in BOTH worlds
	// The GUI expects window.__dumber_palette = { "light": {...}, "dark": {...} }
	if appearanceConfigJSON != "" {
		paletteScript := fmt.Sprintf(`window.__dumber_palette = %s;`, appearanceConfigJSON)

		// Main world
		ucm.AddScript(webkit.NewUserScript(
			paletteScript,
			webkit.UserContentInjectTopFrame,
			webkit.UserScriptInjectAtDocumentStart,
			nil,
			nil,
		))

		// Isolated world
		ucm.AddScript(webkit.NewUserScriptForWorld(
			paletteScript,
			webkit.UserContentInjectTopFrame,
			webkit.UserScriptInjectAtDocumentStart,
			DumberIsolatedWorld,
			nil,
			nil,
		))
		logging.Debug(fmt.Sprintf("[webkit] Injected palette config (%d bytes, main + isolated world)", len(paletteScript)))
	}

	// Inject color-scheme script in BOTH worlds
	// Main world: manipulates document.documentElement.classList for page scripts
	// Isolated world: sets CSS variables that GUI scripts can read via getComputedStyle
	if assets.ColorSchemeScript != "" {
		// Main world
		ucm.AddScript(webkit.NewUserScript(
			assets.ColorSchemeScript,
			webkit.UserContentInjectTopFrame,
			webkit.UserScriptInjectAtDocumentStart,
			nil,
			nil,
		))

		// Isolated world - needed so GUI scripts can access CSS variables
		ucm.AddScript(webkit.NewUserScriptForWorld(
			assets.ColorSchemeScript,
			webkit.UserContentInjectTopFrame,
			webkit.UserScriptInjectAtDocumentStart,
			DumberIsolatedWorld,
			nil,
			nil,
		))
		logging.Debug(fmt.Sprintf("[webkit] Injected color-scheme script (%d bytes, main + isolated world)", len(assets.ColorSchemeScript)))
	}

	// Inject main-world script in MAIN world
	// This script needs webkit.messageHandlers access to forward isolated world messages
	// It also handles theme, zoom, and other main-world only APIs
	if assets.MainWorldScript != "" {
		ucm.AddScript(webkit.NewUserScript(
			assets.MainWorldScript,
			webkit.UserContentInjectTopFrame,
			webkit.UserScriptInjectAtDocumentStart,
			nil,
			nil,
		))
		logging.Debug(fmt.Sprintf("[webkit] Injected main-world script (%d bytes, main world)", len(assets.MainWorldScript)))
	}

	// Inject GUI controls script in ISOLATED world at document-start
	// This contains Svelte components and must be protected from page interference
	// Loading at document-start ensures the omnibox is available as early as possible
	if assets.GUIScript != "" {
		ucm.AddScript(webkit.NewUserScriptForWorld(
			assets.GUIScript,
			webkit.UserContentInjectTopFrame,
			webkit.UserScriptInjectAtDocumentStart,
			DumberIsolatedWorld,
			nil,
			nil,
		))
		logging.Debug(fmt.Sprintf("[webkit] Injected GUI controls script (%d bytes, isolated world, document-start)", len(assets.GUIScript)))
	}

	// Inject component CSS styles as a JavaScript variable in ISOLATED world at document-start
	// These styles need to be injected into the shadow root by shadowHost.ts
	// because Shadow DOM has style encapsulation - external stylesheets don't penetrate it
	// Keep at document-start to prevent flash of unstyled content
	if assets.ComponentStyles != "" {
		// Use fmt.Sprintf with %q to properly escape the CSS string for JavaScript
		componentStylesScript := fmt.Sprintf(`window.__dumber_component_styles = %q;`, assets.ComponentStyles)

		ucm.AddScript(webkit.NewUserScriptForWorld(
			componentStylesScript,
			webkit.UserContentInjectTopFrame,
			webkit.UserScriptInjectAtDocumentStart,
			DumberIsolatedWorld,
			nil,
			nil,
		))
		logging.Debug(fmt.Sprintf("[webkit] Injected component styles string (%d bytes, isolated world, document-start)", len(assets.ComponentStyles)))
	}

	// Register script message handler "dumber" in the MAIN world
	// Isolated world GUI scripts dispatch CustomEvents to main world, which forwards to this handler
	// This architecture is required because webkit.messageHandlers is only available in main world
	if !ucm.RegisterScriptMessageHandler("dumber", "") {
		logging.Warn(fmt.Sprintf("[webkit] Warning: failed to register 'dumber' script message handler in main world"))
	} else {
		logging.Debug(fmt.Sprintf("[webkit] Registered 'dumber' script message handler in main world (default)"))
	}

	return nil
}
