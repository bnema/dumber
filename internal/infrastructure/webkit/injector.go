package webkit

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webutil"
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

// internalPageAllowList restricts script injection to internal dumb:// pages only.
var internalPageAllowList = []string{"dumb://*"}

// autoCopySelectionScript is injected to enable auto-copy on text selection (zellij-style).
// It listens for selection changes, debounces them, and posts to the Go message handler.
// Skips selections in input fields/textareas and requires minimum 2 characters.
const autoCopySelectionScript = `(function() {
  var debounceTimer = null;
  var MIN_LENGTH = 2;
  
  document.addEventListener('selectionchange', function() {
    // Skip if selection is inside input/textarea/contenteditable
    var activeEl = document.activeElement;
    if (activeEl && (activeEl.tagName === 'INPUT' || activeEl.tagName === 'TEXTAREA' || activeEl.isContentEditable)) {
      return;
    }
    
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(function() {
      var text = window.getSelection().toString().trim();
      if (text.length >= MIN_LENGTH && window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.dumber) {
        window.webkit.messageHandlers.dumber.postMessage({
          type: 'auto_copy_selection',
          payload: { text: text }
        });
      }
    }, 300);
  });
})();`

// accentDetectionScript is built at init from entity.AccentMap so the JS
// filter stays in sync with the Go-side accent table.
var accentDetectionScript string

func init() {
	// Build JS Set literal from AccentMap keys: "new Set(['a','c','e',...])"
	keys := make([]string, 0, len(entity.AccentMap))
	for k := range entity.AccentMap {
		keys = append(keys, fmt.Sprintf("'%c'", k))
	}
	sort.Strings(keys)

	accentDetectionScript = fmt.Sprintf(`(function() {
    'use strict';
    const accentKeys = new Set([%s]);
    let pressedKey = null;

    function isEditableTarget(target) {
        const el = target instanceof Element ? target : null;
        if (!el) return false;
        if (el.closest('[contenteditable=""], [contenteditable="true"]')) return true;
        if (el.closest('textarea')) return true;
        const input = el.closest('input');
        if (!input) return false;
        return !/^(button|checkbox|color|file|hidden|image|radio|range|reset|submit)$/i.test(input.type);
    }

    document.addEventListener('keydown', function(e) {
        if (!isEditableTarget(e.target)) return;
        if (e.ctrlKey || e.altKey || e.metaKey) return;
        const key = e.key.toLowerCase();
        if (key.length !== 1 || !accentKeys.has(key)) return;
        pressedKey = key;
        window.webkit.messageHandlers.dumber.postMessage({
            type: 'accent_key_press',
            payload: { char: key, shift: e.shiftKey }
        });
    }, true);

    document.addEventListener('keyup', function(e) {
        if (!isEditableTarget(e.target)) return;
        const key = e.key.toLowerCase();
        if (key === pressedKey) {
            pressedKey = null;
            window.webkit.messageHandlers.dumber.postMessage({
                type: 'accent_key_release',
                payload: { char: key }
            });
        }
    }, true);

	    document.addEventListener('focusin', function(e) {
	        if (isEditableTarget(e.target)) {
	            window.__dumber_lastEditableEl = e.target;
	        }
	    }, true);
})();`, strings.Join(keys, ","))
}

// accentDetectionInjectionMode controls which frames receive the accent detection script.
// Set to AllFrames so the script runs in iframes as well as the top-level document.
const accentDetectionInjectionMode = webkit.UserContentInjectAllFramesValue

// webRTCCompatScript maps legacy Safari-prefixed WebRTC globals to standard names.
// Some pages gate support on window.RTCPeerConnection and report false negatives
// when only webkit-prefixed constructors are present.
const webRTCCompatScript = `(function() {
  if (!window.RTCPeerConnection && window.webkitRTCPeerConnection) {
    window.RTCPeerConnection = window.webkitRTCPeerConnection;
  }
  if (!window.RTCSessionDescription && window.webkitRTCSessionDescription) {
    window.RTCSessionDescription = window.webkitRTCSessionDescription;
  }
  if (!window.RTCIceCandidate && window.webkitRTCIceCandidate) {
    window.RTCIceCandidate = window.webkitRTCIceCandidate;
  }
})();`

func buildWebRTCCompatScript() string {
	return webRTCCompatScript
}

// ContentInjector encapsulates script injection into WebViews.
// It injects dark mode detection scripts for internal pages (dumb://)
// and theme CSS variables for WebUI styling.
// External pages receive dark mode preference via libadwaita's StyleManager.
// Implements port.ContentInjector interface.
type ContentInjector struct {
	colorResolver        port.ColorSchemeResolver
	themeCSSVars         string      // CSS custom property declarations for WebUI
	findCSS              string      // CSS for find-in-page highlight styling
	autoCopyConfigGetter func() bool // Dynamic getter for auto-copy config
}

// NewContentInjector creates a new injector instance.
// The resolver is used to dynamically determine dark mode preference.
func NewContentInjector(resolver port.ColorSchemeResolver) *ContentInjector {
	return &ContentInjector{
		colorResolver: resolver,
	}
}

// SetAutoCopyConfigGetter sets the function to dynamically check if auto-copy is enabled.
// This is called during script injection to determine whether to inject the selection listener.
func (ci *ContentInjector) SetAutoCopyConfigGetter(getter func() bool) {
	ci.autoCopyConfigGetter = getter
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

// PrefersDark returns the current dark mode preference from the resolver.
func (ci *ContentInjector) PrefersDark() bool {
	return ci.colorResolver.Resolve().PrefersDark
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

	prefersDark := ci.PrefersDark()

	// 1. Inject WebRTC compatibility aliases for all pages.
	addScript(
		webkit.NewUserScript(
			buildWebRTCCompatScript(),
			webkit.UserContentInjectTopFrameValue,
			webkit.UserScriptInjectAtDocumentStartValue,
			nil,
			nil,
		),
		"webrtc-compat-shim",
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
		log.Debug().Uint64("webview_id", uint64(webviewID)).Msg("webview ID script injected")
	} else {
		log.Warn().Msg("webview ID is 0, skipping ID injection")
	}

	// 3. Inject dark mode handler for internal pages only
	// This sets .dark/.light class on <html> and patches matchMedia for WebUI
	internalDarkModeScript := webutil.DarkModeScript(prefersDark, "__dumber_gtk_prefers_dark")
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
		escapedCSS := webutil.EscapeForJSString(ci.themeCSSVars)
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

	// 6. Inject auto-copy selection script for all pages (if enabled)
	autoCopyEnabled := ci.autoCopyConfigGetter != nil && ci.autoCopyConfigGetter()
	if autoCopyEnabled {
		addScript(
			webkit.NewUserScript(
				autoCopySelectionScript,
				webkit.UserContentInjectTopFrameValue,
				webkit.UserScriptInjectAtDocumentEndValue,
				nil, // All pages
				nil,
			),
			"auto-copy-selection",
		)
		log.Debug().Msg("auto-copy selection script injected")
	}

	// 7. Inject accent key detection for all pages and all frames (unconditional).
	// JS only reports keydown/keyup events; Go handles timing and picker display.
	addScript(
		webkit.NewUserScript(
			accentDetectionScript,
			accentDetectionInjectionMode,
			webkit.UserScriptInjectAtDocumentEndValue,
			nil, // all pages
			nil,
		),
		"accent-key-detection",
	)

	log.Debug().Bool("prefers_dark", prefersDark).Bool("auto_copy", autoCopyEnabled).Msg("scripts injected")
}

// RefreshScripts clears and re-injects user scripts for a single WebView.
// Implements port.ContentInjector.
// Returns nil for "not applicable" cases (nil wv, wrong engine type, nil UCM).
func (ci *ContentInjector) RefreshScripts(ctx context.Context, wv port.WebView) error {
	log := logging.FromContext(ctx).With().Str("component", "content-injector").Logger()
	if wv == nil {
		log.Debug().Msg("RefreshScripts: nil webview")
		return nil
	}
	wkWV, ok := wv.(*WebView)
	if !ok {
		log.Debug().Msg("RefreshScripts: webview is not *webkit.WebView")
		return nil
	}
	ucm := wkWV.UserContentManager()
	if ucm == nil {
		log.Debug().Uint64("webview_id", uint64(wv.ID())).Msg("RefreshScripts: nil UserContentManager")
		return nil
	}
	ucm.RemoveAllScripts()
	ucm.RemoveAllStyleSheets()
	ci.InjectScripts(ctx, ucm, wv.ID())
	return nil
}
