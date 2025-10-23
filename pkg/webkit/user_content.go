package webkit

import (
	"fmt"
	"log"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/dumber/internal/config"
	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
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
		log.Printf("[webkit] UserContentManager is nil, skipping script injection")
		return nil
	}

	// Inject webview ID FIRST, so GUI scripts can access it immediately
	// Note: webviewID is uint64, formatted as number (not string) to avoid any injection concerns
	webviewIDScript := fmt.Sprintf(`
		window.__dumber_webview_id = %d;
		console.log('[webkit] WebView ID set in JavaScript:', window.__dumber_webview_id);
	`, webviewID)
	ucm.AddScript(webkit.NewUserScript(
		webviewIDScript,
		webkit.UserContentInjectTopFrame, // Only top frame needs the webview ID
		webkit.UserScriptInjectAtDocumentStart,
		nil,
		nil,
	))
	log.Printf("[webkit] Injected webview ID script for ID: %d", webviewID)

	// Inject GTK theme detection SECOND, before color-scheme script
	// The color-scheme.ts expects window.__dumber_gtk_prefers_dark to be set
	// Respect the ColorScheme config setting
	cfg := config.Get()
	var prefersDark bool
	switch cfg.Appearance.ColorScheme {
	case "prefer-dark":
		prefersDark = true
		log.Printf("[webkit] Using config-forced dark theme (color_scheme: prefer-dark)")
	case "prefer-light":
		prefersDark = false
		log.Printf("[webkit] Using config-forced light theme (color_scheme: prefer-light)")
	default:
		// "default" or empty - follow system GTK preference
		prefersDark = PrefersDarkTheme()
		log.Printf("[webkit] Using system GTK theme preference (color_scheme: %s)", cfg.Appearance.ColorScheme)
	}

	gtkThemeScript := fmt.Sprintf(`window.__dumber_gtk_prefers_dark = %t;`, prefersDark)
	ucm.AddScript(webkit.NewUserScript(
		gtkThemeScript,
		webkit.UserContentInjectTopFrame, // Only top frame needs theme preference
		webkit.UserScriptInjectAtDocumentStart,
		nil,
		nil,
	))
	log.Printf("[webkit] Injected theme preference: prefersDark=%t", prefersDark)

	// Inject palette config SECOND, before GUI scripts
	// The GUI expects window.__dumber_palette = { "light": {...}, "dark": {...} }
	if appearanceConfigJSON != "" {
		paletteScript := fmt.Sprintf(`window.__dumber_palette = %s;`, appearanceConfigJSON)
		ucm.AddScript(webkit.NewUserScript(
			paletteScript,
			webkit.UserContentInjectTopFrame, // Only top frame needs palette config
			webkit.UserScriptInjectAtDocumentStart,
			nil,
			nil,
		))
		log.Printf("[webkit] Injected palette config at document-start (%d bytes)", len(paletteScript))
	}

	// Inject color-scheme script at document-start
	if assets.ColorSchemeScript != "" {
		ucm.AddScript(webkit.NewUserScript(
			assets.ColorSchemeScript,
			webkit.UserContentInjectTopFrame, // Only top frame needs color scheme detection
			webkit.UserScriptInjectAtDocumentStart,
			nil, // whitelist (nil = all)
			nil, // blacklist (nil = none)
		))
		log.Printf("[webkit] Injected color-scheme script (%d bytes)", len(assets.ColorSchemeScript))
	}

	// Inject main-world script at document-start
	if assets.MainWorldScript != "" {
		ucm.AddScript(webkit.NewUserScript(
			assets.MainWorldScript,
			webkit.UserContentInjectTopFrame, // Only top frame needs bridge/UI functionality
			webkit.UserScriptInjectAtDocumentStart,
			nil,
			nil,
		))
		log.Printf("[webkit] Injected main-world script (%d bytes)", len(assets.MainWorldScript))
	}

	// Inject GUI controls script at document-start
	if assets.GUIScript != "" {
		ucm.AddScript(webkit.NewUserScript(
			assets.GUIScript,
			webkit.UserContentInjectTopFrame, // Only top frame needs GUI controls
			webkit.UserScriptInjectAtDocumentStart,
			nil,
			nil,
		))
		log.Printf("[webkit] Injected GUI controls script (%d bytes)", len(assets.GUIScript))
	}

	// Register script message handler "dumber" for communication from JS
	// Pass empty string for worldName to use the default world
	if !ucm.RegisterScriptMessageHandler("dumber", "") {
		log.Printf("[webkit] Warning: failed to register 'dumber' script message handler")
	} else {
		log.Printf("[webkit] Registered 'dumber' script message handler")
	}

	return nil
}
