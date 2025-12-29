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

// internalDarkModeScript is injected ONLY on internal pages (dumb://*).
// It sets dark/light class on <html> for Tailwind CSS theming and patches matchMedia
// for JS-based dark mode detection in WebUI components.
//
// NOTE: This script is NOT injected on external pages. External pages receive
// dark mode preference via libadwaita's StyleManager, which WebKit respects
// for the native prefers-color-scheme media query.
//
// The matchMedia patch handles various query formats:
// - (prefers-color-scheme: dark)
// - (prefers-color-scheme:dark)  -- no space
// - screen and (prefers-color-scheme: dark)
const internalDarkModeScript = `(function() {
  var prefersDark = window.__dumber_gtk_prefers_dark || false;
  var originalMatchMedia = window.matchMedia.bind(window);

  // Apply dark/light class to document element for Tailwind CSS theming
  if (prefersDark) {
    document.documentElement.classList.add('dark');
    document.documentElement.classList.remove('light');
  } else {
    document.documentElement.classList.add('light');
    document.documentElement.classList.remove('dark');
  }

  // Helper: Check if query is a prefers-color-scheme query
  function isColorSchemeQuery(query, scheme) {
    if (typeof query !== 'string') return false;
    var normalized = query.replace(/\s+/g, '').toLowerCase();
    return normalized.indexOf('prefers-color-scheme:' + scheme) !== -1;
  }

  // Create a fake MediaQueryList that implements the full interface
  function createFakeMediaQueryList(query, matches) {
    var listeners = [];
    var onchangeHandler = null;

    return {
      matches: matches,
      media: query,
      get onchange() { return onchangeHandler; },
      set onchange(fn) { onchangeHandler = fn; },
      addListener: function(cb) {
        if (typeof cb === 'function') listeners.push(cb);
      },
      removeListener: function(cb) {
        var idx = listeners.indexOf(cb);
        if (idx !== -1) listeners.splice(idx, 1);
      },
      addEventListener: function(type, cb) {
        if (type === 'change' && typeof cb === 'function') {
          listeners.push(cb);
        }
      },
      removeEventListener: function(type, cb) {
        if (type === 'change') {
          var idx = listeners.indexOf(cb);
          if (idx !== -1) listeners.splice(idx, 1);
        }
      },
      dispatchEvent: function(event) {
        for (var i = 0; i < listeners.length; i++) {
          try { listeners[i](event); } catch (e) {}
        }
        if (onchangeHandler) {
          try { onchangeHandler(event); } catch (e) {}
        }
        return true;
      }
    };
  }

  // Patch matchMedia for prefers-color-scheme queries
  window.matchMedia = function(query) {
    if (isColorSchemeQuery(query, 'dark')) {
      return createFakeMediaQueryList(query, prefersDark);
    }
    if (isColorSchemeQuery(query, 'light')) {
      return createFakeMediaQueryList(query, !prefersDark);
    }
    return originalMatchMedia(query);
  };
})();`

// internalPageAllowList restricts script injection to internal dumb:// pages only.
var internalPageAllowList = []string{"dumb://*"}

// ContentInjector encapsulates script injection into WebViews.
// It injects dark mode detection scripts for internal pages (dumb://)
// and theme CSS variables for WebUI styling.
// External pages receive dark mode preference via libadwaita's StyleManager.
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

// InjectScripts adds scripts to the given content manager.
// For internal pages (dumb://*):
//   - Injects dark mode detection (class on <html>, matchMedia patch)
//   - Injects theme CSS variables for WebUI styling
//
// For external pages:
//   - Only injects find highlight CSS
//   - Dark mode is handled natively via libadwaita's StyleManager
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

	// 1. Inject GTK dark mode preference for internal pages only
	darkModePrefScript := fmt.Sprintf("window.__dumber_gtk_prefers_dark=%t;", ci.prefersDark)
	addScript(
		webkit.NewUserScript(
			darkModePrefScript,
			webkit.UserContentInjectTopFrameValue,
			webkit.UserScriptInjectAtDocumentStartValue,
			internalPageAllowList,
			nil,
		),
		"gtk-dark-mode-pref",
	)

	// 2. Inject WebView ID for debugging (internal pages only)
	if webviewID != 0 {
		idScript := fmt.Sprintf("window.__dumber_webview_id=%d;", uint64(webviewID))
		addScript(
			webkit.NewUserScript(
				idScript,
				webkit.UserContentInjectTopFrameValue,
				webkit.UserScriptInjectAtDocumentStartValue,
				internalPageAllowList,
				nil,
			),
			"webview-id",
		)
	}

	// 3. Inject dark mode handler for internal pages only
	// This sets .dark/.light class on <html> and patches matchMedia for WebUI
	addScript(
		webkit.NewUserScript(
			internalDarkModeScript,
			webkit.UserContentInjectTopFrameValue,
			webkit.UserScriptInjectAtDocumentStartValue,
			internalPageAllowList,
			nil,
		),
		"internal-dark-mode-handler",
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
				internalPageAllowList,
				nil,
			),
			"theme-css-vars",
		)
		log.Debug().Msg("theme CSS vars injection configured for internal pages")
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

	log.Debug().Bool("prefers_dark", ci.prefersDark).Msg("scripts injected")
}
