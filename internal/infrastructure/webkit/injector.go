package webkit

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

const (
	// ScriptWorldName is the isolated world used for the injected UI.
	ScriptWorldName = "dumber"
	// MessageHandlerName is the name of the script message handler registered with WebKit.
	MessageHandlerName = "dumber"
)

// ContentInjector encapsulates script injection into WebViews.
// It injects minimal scripts for dark mode detection in web pages.
type ContentInjector struct {
	prefersDark bool
}

// NewContentInjector creates a new injector instance.
// The prefersDark parameter should come from ThemeManager.PrefersDark().
func NewContentInjector(prefersDark bool) *ContentInjector {
	return &ContentInjector{
		prefersDark: prefersDark,
	}
}

// PrefersDark returns the current dark mode preference.
func (ci *ContentInjector) PrefersDark() bool {
	return ci.prefersDark
}

// SetPrefersDark updates the dark mode preference.
// Call this when theme changes at runtime.
func (ci *ContentInjector) SetPrefersDark(prefersDark bool) {
	ci.prefersDark = prefersDark
}

// InjectScripts adds the minimal dark mode detection scripts to the given content manager.
// Only injects:
// - window.__dumber_gtk_prefers_dark flag
// - window.__dumber_webview_id (for debugging)
// - color-scheme.js (patches matchMedia for prefers-color-scheme)
func (ci *ContentInjector) InjectScripts(ctx context.Context, ucm *webkit.UserContentManager, webviewID WebViewID) {
	log := logging.FromContext(ctx).With().Str("component", "content-injector").Logger()

	if ucm == nil {
		log.Warn().Msg("cannot inject scripts: user content manager is nil")
		return
	}

	addScript := func(script *webkit.UserScript, label string) {
		if script == nil {
			log.Warn().Str("script", label).Msg("failed to create user script")
			return
		}
		ucm.AddScript(script)
		log.Debug().Str("script", label).Msg("injected user script")
	}

	// 1. Inject GTK dark mode preference (must be before color-scheme.js)
	darkModeScript := fmt.Sprintf("window.__dumber_gtk_prefers_dark=%t;", ci.prefersDark)
	addScript(
		webkit.NewUserScript(
			darkModeScript,
			webkit.UserContentInjectTopFrameValue,
			webkit.UserScriptInjectAtDocumentStartValue,
			nil,
			nil,
		),
		"gtk-dark-mode",
	)

	// 2. Inject WebView ID for debugging
	if webviewID != 0 {
		idScript := fmt.Sprintf("window.__dumber_webview_id=%d;", uint64(webviewID))
		addScript(
			webkit.NewUserScript(
				idScript,
				webkit.UserContentInjectTopFrameValue,
				webkit.UserScriptInjectAtDocumentStartValue,
				nil,
				nil,
			),
			"webview-id",
		)
	}

	// 3. Inject color-scheme handler (uses __dumber_gtk_prefers_dark to patch matchMedia)
	if assets.ColorSchemeScript != "" {
		addScript(
			webkit.NewUserScript(
				assets.ColorSchemeScript,
				webkit.UserContentInjectTopFrameValue,
				webkit.UserScriptInjectAtDocumentStartValue,
				nil,
				nil,
			),
			"color-scheme",
		)
	} else {
		log.Warn().Msg("ColorSchemeScript asset is empty; dark mode may not work correctly")
	}

	log.Debug().Bool("prefers_dark", ci.prefersDark).Msg("minimal scripts injected")
}
