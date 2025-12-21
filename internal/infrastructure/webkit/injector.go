package webkit

import (
	"context"
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

const (
	// ScriptWorldName is the isolated world used for the injected UI.
	ScriptWorldName = "dumber"
	// MessageHandlerName is the name of the script message handler registered with WebKit.
	MessageHandlerName = "dumber"
)

// themeCSSScript injects theme CSS into the page.
// The %s placeholder is replaced with CSS text (newlines escaped as \n).
const themeCSSScript = `(function() {
  var style = document.createElement('style');
  style.setAttribute('data-dumber-theme-vars', '');
  style.textContent = '%s';
  (document.head || document.documentElement).appendChild(style);
})();`

// darkModeScript patches window.matchMedia and sets theme class on <html>.
// It must be injected at document start, after __dumber_gtk_prefers_dark is set.
const darkModeScript = `(function() {
  const prefersDark = window.__dumber_gtk_prefers_dark || false;
  const originalMatchMedia = window.matchMedia.bind(window);

  // Apply dark/light class to document element for CSS theming
  if (prefersDark) {
    document.documentElement.classList.add('dark');
    document.documentElement.classList.remove('light');
  } else {
    document.documentElement.classList.add('light');
    document.documentElement.classList.remove('dark');
  }

  // Inject color-scheme meta tag
  const meta = document.createElement('meta');
  meta.name = 'color-scheme';
  meta.content = prefersDark ? 'dark light' : 'light dark';
  document.documentElement.appendChild(meta);

  // Inject root color-scheme style
  const style = document.createElement('style');
  style.setAttribute('data-dumber-color-scheme', '');
  style.textContent = ':root{color-scheme:' + (prefersDark ? 'dark' : 'light') + ';}';
  document.documentElement.appendChild(style);

  // Patch matchMedia for prefers-color-scheme queries
  window.matchMedia = function(query) {
    if (typeof query === 'string') {
      if (query.includes('prefers-color-scheme: dark')) {
        return {
          matches: prefersDark,
          media: query,
          onchange: null,
          addListener: function() {},
          removeListener: function() {},
          addEventListener: function() {},
          removeEventListener: function() {},
          dispatchEvent: function() { return false; }
        };
      }
      if (query.includes('prefers-color-scheme: light')) {
        return {
          matches: !prefersDark,
          media: query,
          onchange: null,
          addListener: function() {},
          removeListener: function() {},
          addEventListener: function() {},
          removeEventListener: function() {},
          dispatchEvent: function() { return false; }
        };
      }
    }
    return originalMatchMedia(query);
  };
})();`

// ContentInjector encapsulates script injection into WebViews.
// It injects minimal scripts for dark mode detection in web pages
// and theme CSS variables for internal pages (dumb://).
// Implements port.ContentInjector interface.
type ContentInjector struct {
	prefersDark  bool
	themeCSSVars string // CSS custom property declarations for WebUI
	findCSS      string // CSS for find-in-page highlight styling
}

// NewContentInjector creates a new injector instance.
// The prefersDark parameter should come from ThemeManager.PrefersDark().
func NewContentInjector(prefersDark bool) *ContentInjector {
	return &ContentInjector{
		prefersDark: prefersDark,
	}
}

// InjectThemeCSS stores CSS variables for injection into internal pages.
// Implements port.ContentInjector interface.
// The CSS will be injected when InjectScripts is called on WebView creation.
func (ci *ContentInjector) InjectThemeCSS(ctx context.Context, css string) error {
	log := logging.FromContext(ctx).With().Str("component", "content-injector").Logger()
	ci.themeCSSVars = css
	log.Debug().Int("css_len", len(css)).Msg("theme CSS vars set for injection")
	return nil
}

// InjectFindHighlightCSS stores CSS for find-in-page highlight styling.
func (ci *ContentInjector) InjectFindHighlightCSS(ctx context.Context, css string) error {
	log := logging.FromContext(ctx).With().Str("component", "content-injector").Logger()
	ci.findCSS = css
	log.Debug().Int("css_len", len(css)).Msg("find highlight CSS set for injection")
	return nil
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
// - darkModeScript (patches matchMedia for prefers-color-scheme)
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

	// 1. Inject GTK dark mode preference (must be before dark mode handler)
	darkModePrefScript := fmt.Sprintf("window.__dumber_gtk_prefers_dark=%t;", ci.prefersDark)
	addScript(
		webkit.NewUserScript(
			darkModePrefScript,
			webkit.UserContentInjectTopFrameValue,
			webkit.UserScriptInjectAtDocumentStartValue,
			nil,
			nil,
		),
		"gtk-dark-mode-pref",
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

	// 3. Inject dark mode handler (patches matchMedia using __dumber_gtk_prefers_dark)
	addScript(
		webkit.NewUserScript(
			darkModeScript,
			webkit.UserContentInjectTopFrameValue,
			webkit.UserScriptInjectAtDocumentStartValue,
			nil,
			nil,
		),
		"dark-mode-handler",
	)

	// 4. Inject theme CSS for internal pages (dumb://* only)
	if ci.themeCSSVars != "" {
		// Escape for JS string literal
		escapedCSS := strings.ReplaceAll(ci.themeCSSVars, "\\", "\\\\")
		escapedCSS = strings.ReplaceAll(escapedCSS, "'", "\\'")
		escapedCSS = strings.ReplaceAll(escapedCSS, "\n", "\\n")
		themeCSSInjectionScript := fmt.Sprintf(themeCSSScript, escapedCSS)
		addScript(
			webkit.NewUserScript(
				themeCSSInjectionScript,
				webkit.UserContentInjectTopFrameValue,
				webkit.UserScriptInjectAtDocumentEndValue,
				nil, // Inject on all pages - allowlist patterns don't work for custom schemes
				nil,
			),
			"theme-css-vars",
		)
		log.Debug().Msg("theme CSS vars injection configured")
	}

	// 5. Inject find highlight CSS for all pages
	if ci.findCSS != "" {
		stylesheet := webkit.NewUserStyleSheet(
			ci.findCSS,
			webkit.UserContentInjectAllFramesValue,
			webkit.UserStyleLevelUserValue,
			nil,
			nil,
		)
		if stylesheet == nil {
			log.Warn().Msg("failed to create find highlight stylesheet")
		} else {
			ucm.AddStyleSheet(stylesheet)
			log.Debug().Msg("find highlight stylesheet injected")
		}
	}

	log.Debug().Bool("prefers_dark", ci.prefersDark).Msg("minimal scripts injected")
}
