package cef

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time interface check.
var _ port.ContentInjector = (*contentInjector)(nil)

// internalSchemePrefix is used to detect internal pages that receive dark mode
// and message bridge scripts. Must match the engine's InternalSchemePath prefix.
const internalSchemePrefix = "dumb://"

// scrollbarCSS styles the scrollbar with auto-hide behavior: invisible by
// default, fades in on scroll, widens on hover, fades out after 1s idle.
// Uses --primary theme color for the thumb.
const scrollbarCSS = `
::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: transparent;
  border-radius: 3px;
  transition: background 0.3s ease;
}
.dumber-scrolling ::-webkit-scrollbar-thumb {
  background: var(--primary, rgba(128, 128, 128, 0.4));
}
.dumber-scrolling ::-webkit-scrollbar:hover {
  width: 10px;
  height: 10px;
}
.dumber-scrolling ::-webkit-scrollbar-track:hover {
  background: rgba(128, 128, 128, 0.1);
}
.dumber-scrolling ::-webkit-scrollbar-thumb:hover {
  background: var(--primary, rgba(128, 128, 128, 0.6));
}
::-webkit-scrollbar-corner {
  background: transparent;
}
`

// scrollbarAutoHideJS adds/removes the .dumber-scrolling class on <html>
// on scroll activity, with a 1s fade-out timeout after scrolling stops.
const scrollbarAutoHideJS = `(function(){
  var t, el = document.documentElement;
  function show() {
    el.classList.add('dumber-scrolling');
    clearTimeout(t);
    t = setTimeout(function(){ el.classList.remove('dumber-scrolling'); }, 1000);
  }
  window.addEventListener('scroll', show, {passive:true,capture:true});
  window.addEventListener('wheel', show, {passive:true});
  window.addEventListener('mouseenter', function(e){
    if(el.scrollHeight > el.clientHeight) show();
  });
})();`

// contentInjector implements port.ContentInjector for the CEF engine.
// It stores CSS strings and injects them into webviews via ExecuteJavaScript.
// Thread-safe: InjectThemeCSS may be called from the UI thread while OnLoadEnd
// fires on the CEF IO thread.
type contentInjector struct {
	mu               sync.RWMutex
	themeCSS         string
	findHighlightCSS string
	engine           *Engine
	colorResolver    port.ColorSchemeResolver
}

// setColorResolver updates the color scheme resolver used for dark mode detection.
func (ci *contentInjector) setColorResolver(resolver port.ColorSchemeResolver) {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	ci.colorResolver = resolver
}

// newContentInjector creates a content injector wired to the given engine.
func newContentInjector(engine *Engine, resolver port.ColorSchemeResolver) *contentInjector {
	return &contentInjector{
		engine:        engine,
		colorResolver: resolver,
	}
}

// InjectThemeCSS stores the theme CSS and broadcasts it to all active webviews.
func (ci *contentInjector) InjectThemeCSS(ctx context.Context, css string) error {
	log := logging.FromContext(ctx).With().Str("component", "cef-content-injector").Logger()

	ci.mu.Lock()
	ci.themeCSS = css
	ci.mu.Unlock()

	log.Debug().Int("css_len", len(css)).Msg("theme CSS set, broadcasting to active webviews")

	// Broadcast to all active webviews.
	ci.engine.activeWebViews.Range(func(_, value any) bool {
		if wv, ok := value.(*WebView); ok {
			ci.injectCSS(wv, "dumber-theme-vars", css)
		}
		return true
	})
	return nil
}

// InjectFindHighlightCSS stores the find highlight CSS and broadcasts it.
func (ci *contentInjector) InjectFindHighlightCSS(ctx context.Context, css string) error {
	log := logging.FromContext(ctx).With().Str("component", "cef-content-injector").Logger()

	ci.mu.Lock()
	ci.findHighlightCSS = css
	ci.mu.Unlock()

	log.Debug().Int("css_len", len(css)).Msg("find highlight CSS set, broadcasting to active webviews")

	ci.engine.activeWebViews.Range(func(_, value any) bool {
		if wv, ok := value.(*WebView); ok {
			ci.injectCSS(wv, "dumber-find-highlight", css)
		}
		return true
	})
	return nil
}

// RefreshScripts re-injects all scripts into a specific webview.
func (ci *contentInjector) RefreshScripts(ctx context.Context, wv port.WebView) error {
	log := logging.FromContext(ctx).With().Str("component", "cef-content-injector").Logger()
	if wv == nil {
		log.Debug().Msg("RefreshScripts: nil webview")
		return nil
	}
	cefWV, ok := wv.(*WebView)
	if !ok {
		log.Debug().Msg("RefreshScripts: webview is not *cef.WebView")
		return nil
	}

	ci.onLoadEnd(cefWV)
	return nil
}

// onLoadEnd is called from the load handler after a page finishes loading.
// It injects the appropriate scripts based on whether the page is internal.
func (ci *contentInjector) onLoadEnd(wv *WebView) {
	uri := wv.URI()
	isInternal := strings.HasPrefix(uri, internalSchemePrefix)

	ci.mu.RLock()
	themeCSS := ci.themeCSS
	findCSS := ci.findHighlightCSS
	ci.mu.RUnlock()

	// Internal pages get dark mode + message bridge + theme CSS.
	if isInternal {
		prefersDark := false
		if ci.colorResolver != nil {
			prefersDark = ci.colorResolver.Resolve().PrefersDark
		}
		ci.injectDarkModeScript(wv, prefersDark)
		ci.injectMessageBridgeShim(wv)
		if themeCSS != "" {
			ci.injectCSS(wv, "dumber-theme-vars", themeCSS)
		}
	}

	// All pages get find highlight CSS if set.
	if findCSS != "" {
		ci.injectCSS(wv, "dumber-find-highlight", findCSS)
	}

	// All pages get custom scrollbar styling with auto-hide.
	ci.injectCSS(wv, "dumber-scrollbar", scrollbarCSS)
	wv.RunJavaScript(context.Background(), scrollbarAutoHideJS)
}

// injectCSS injects a CSS string as a <style> element via JavaScript.
func (ci *contentInjector) injectCSS(wv *WebView, id, css string) {
	escapedID := escapeForJSString(id)
	escaped := escapeForJSString(css)
	script := fmt.Sprintf(`(function(){
  var el = document.getElementById('%s');
  if (!el) { el = document.createElement('style'); el.id = '%s'; document.head.appendChild(el); }
  el.textContent = '%s';
})();`, escapedID, escapedID, escaped)

	wv.RunJavaScript(context.Background(), script)
}

// injectDarkModeScript sets dark/light class on <html> and patches matchMedia
// for prefers-color-scheme queries on internal pages.
func (ci *contentInjector) injectDarkModeScript(wv *WebView, prefersDark bool) {
	script := fmt.Sprintf(`(function() {
  var prefersDark = %t;
  window.__dumber_cef_prefers_dark = prefersDark;
  var originalMatchMedia = window.matchMedia.bind(window);

  if (prefersDark) {
    document.documentElement.classList.add('dark');
    document.documentElement.classList.remove('light');
  } else {
    document.documentElement.classList.add('light');
    document.documentElement.classList.remove('dark');
  }

  function isColorSchemeQuery(query, scheme) {
    if (typeof query !== 'string') return false;
    var normalized = query.replace(/\s+/g, '').toLowerCase();
    return normalized.indexOf('prefers-color-scheme:' + scheme) !== -1;
  }

  function createFakeMediaQueryList(query, matches) {
    var listeners = [];
    var onchangeHandler = null;
    return {
      matches: matches,
      media: query,
      get onchange() { return onchangeHandler; },
      set onchange(fn) { onchangeHandler = fn; },
      addListener: function(cb) { if (typeof cb === 'function') listeners.push(cb); },
      removeListener: function(cb) { var idx = listeners.indexOf(cb); if (idx !== -1) listeners.splice(idx, 1); },
      addEventListener: function(type, cb) { if (type === 'change' && typeof cb === 'function') listeners.push(cb); },
      removeEventListener: function(type, cb) {
        if (type === 'change') { var idx = listeners.indexOf(cb); if (idx !== -1) listeners.splice(idx, 1); }
      },
      dispatchEvent: function(event) {
        for (var i = 0; i < listeners.length; i++) { try { listeners[i](event); } catch (e) {} }
        if (onchangeHandler) { try { onchangeHandler(event); } catch (e) {} }
        return true;
      }
    };
  }

  window.matchMedia = function(query) {
    if (isColorSchemeQuery(query, 'dark')) return createFakeMediaQueryList(query, prefersDark);
    if (isColorSchemeQuery(query, 'light')) return createFakeMediaQueryList(query, !prefersDark);
    return originalMatchMedia(query);
  };
})();`, prefersDark)

	wv.RunJavaScript(context.Background(), script)
}

// injectMessageBridgeShim injects the window.dumber.postMessage JS client shim
// so internal pages can communicate with Go handlers via fetch.
// The message body is base64-encoded into the X-Dumber-Body header to work
// around purego-cef's unexported PostData element wrapper.
func (ci *contentInjector) injectMessageBridgeShim(wv *WebView) {
	wv.RunJavaScript(context.Background(), MessageBridgeJS)
}

// escapeForJSString escapes a string for use inside a JS single-quoted string literal.
func escapeForJSString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\u2028", "\\u2028")
	s = strings.ReplaceAll(s, "\u2029", "\\u2029")
	return s
}
