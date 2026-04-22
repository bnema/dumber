package cef

import (
	"context"
	"fmt"
	neturl "net/url"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/webutil"
	"github.com/bnema/dumber/internal/logging"
)

type popupOpenerPostMessagePayload struct {
	Data         string `json:"data"`
	DataKind     string `json:"data_kind"`
	TargetOrigin string `json:"target_origin"`
	SourceOrigin string `json:"source_origin"`
	SourceHref   string `json:"source_href"`
}

type popupOpenerNavigatePayload struct {
	URL string `json:"url"`
}

func (wv *WebView) setPopupNoJavaScriptAccess(noJavaScriptAccess bool) {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	wv.popupNoJavaScriptAccess = noJavaScriptAccess
	wv.mu.Unlock()
}

// AddOpenerMessageCallback implements port.PopupOpenerMessageCapable.
func (wv *WebView) AddOpenerMessageCallback(fn func()) {
	if wv == nil || fn == nil {
		return
	}
	wv.mu.Lock()
	wv.openerMessageCallbacks = append(wv.openerMessageCallbacks, fn)
	wv.mu.Unlock()
}

// AddOpenerNavigationCallback implements port.PopupOpenerNavigationCapable.
func (wv *WebView) AddOpenerNavigationCallback(fn func(uri string)) {
	if wv == nil || fn == nil {
		return
	}
	wv.mu.Lock()
	wv.openerNavigationCallbacks = append(wv.openerNavigationCallbacks, fn)
	wv.mu.Unlock()
}

func (wv *WebView) runOpenerMessageCallbacks() {
	if wv == nil {
		return
	}
	wv.mu.RLock()
	callbacks := append([]func(){}, wv.openerMessageCallbacks...)
	wv.mu.RUnlock()
	if len(callbacks) == 0 {
		return
	}
	wv.runOnGTK(func() {
		for _, fn := range callbacks {
			if fn != nil {
				fn()
			}
		}
	})
}

func (wv *WebView) runOpenerNavigationCallbacks(uri string) {
	if wv == nil {
		return
	}
	wv.mu.RLock()
	callbacks := append([]func(string){}, wv.openerNavigationCallbacks...)
	wv.mu.RUnlock()
	if len(callbacks) == 0 {
		return
	}
	wv.runOnGTK(func() {
		for _, fn := range callbacks {
			if fn != nil {
				fn(uri)
			}
		}
	})
}

// HasActivePopupOpenerBridge implements port.PopupOpenerBridgeStateCapable.
func (wv *WebView) HasActivePopupOpenerBridge() bool {
	if wv == nil {
		return false
	}
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return !wv.popupNoJavaScriptAccess && wv.popupOpenerBridgeParent != nil
}

// EnablePopupOpenerBridge implements port.PopupOpenerBridgeCapable.
func (wv *WebView) EnablePopupOpenerBridge(parent port.WebView, noJavaScriptAccess bool) {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	defer wv.mu.Unlock()
	wv.popupNoJavaScriptAccess = noJavaScriptAccess
	if noJavaScriptAccess {
		wv.popupOpenerBridgeParent = nil
		wv.popupOpenerBridgeParentURI = ""
		return
	}
	parentWV, ok := parent.(*WebView)
	if !ok || parentWV == nil {
		wv.popupOpenerBridgeParent = nil
		wv.popupOpenerBridgeParentURI = ""
		return
	}
	wv.popupOpenerBridgeParent = parentWV
	wv.popupOpenerBridgeParentURI = parentWV.URI()
}

func (wv *WebView) popupOpenerBridgeScript(bridgeNonce string) string {
	if wv == nil || bridgeNonce == "" {
		return ""
	}

	wv.mu.RLock()
	parentURI := wv.popupOpenerBridgeParentURI
	parent := wv.popupOpenerBridgeParent
	blocked := wv.popupNoJavaScriptAccess || parent == nil
	wv.mu.RUnlock()
	if blocked {
		return ""
	}

	return fmt.Sprintf(`(function() {
  if (typeof window === 'undefined') return;
  if (window.__dumberPopupOpenerBridgeInstalled) return;
  window.__dumberPopupOpenerBridgeInstalled = true;
  if (window.opener != null) return;

  var bridgeNonce = '%s';
  var openerHref = '%s';

  function postBridge(path, payload) {
    try {
      fetch('dumb:///api/' + path, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Dumber-Body': btoa(unescape(encodeURIComponent(JSON.stringify(payload)))),
          'X-Dumber-Bridge-Nonce': bridgeNonce
        }
      }).catch(function() {});
    } catch (_) {}
  }

  function normalizeURLValue(rawURL) {
    if (rawURL == null) return '';
    try {
      return String(rawURL);
    } catch (_) {
      return '';
    }
  }

  function navigateOpener(nextURL) {
    var rawURL = normalizeURLValue(nextURL);
    if (rawURL !== '') openerHref = rawURL;
    postBridge('popup-opener-navigate', { url: rawURL });
    return openerHref;
  }

  function serializeMessage(value) {
    try {
      return { kind: 'json', value: JSON.stringify(value) };
    } catch (_) {
      return { kind: 'string', value: String(value) };
    }
  }

  var locationProxy = {
    assign: function(nextURL) { return navigateOpener(nextURL); },
    replace: function(nextURL) { return navigateOpener(nextURL); },
    toString: function() { return openerHref; }
  };
  try {
    Object.defineProperty(locationProxy, 'href', {
      configurable: true,
      enumerable: true,
      get: function() { return openerHref; },
      set: function(nextURL) { navigateOpener(nextURL); }
    });
  } catch (_) {
    locationProxy.href = openerHref;
  }

  var openerProxy = {
    blur: function() { return undefined; },
    close: function() { return undefined; },
    focus: function() { return undefined; },
    postMessage: function(message, targetOrigin) {
      var serialized = serializeMessage(message);
      postBridge('popup-opener-post-message', {
        data: serialized.value,
        data_kind: serialized.kind,
        target_origin: targetOrigin == null ? '*' : String(targetOrigin),
        source_origin: (typeof location !== 'undefined' && location && location.origin) ? location.origin : '',
        source_href: (typeof location !== 'undefined' && location && location.href) ? location.href : ''
      });
      return undefined;
    }
  };
  openerProxy.self = openerProxy;
  openerProxy.window = openerProxy;
  try {
    Object.defineProperty(openerProxy, 'closed', {
      configurable: true,
      enumerable: true,
      get: function() { return false; }
    });
  } catch (_) {
    openerProxy.closed = false;
  }
  try {
    Object.defineProperty(openerProxy, 'location', {
      configurable: true,
      enumerable: true,
      get: function() { return locationProxy; },
      set: function(nextURL) { navigateOpener(nextURL); }
    });
  } catch (_) {
    openerProxy.location = locationProxy;
  }

  try {
    Object.defineProperty(window, 'opener', {
      configurable: true,
      enumerable: true,
      get: function() { return openerProxy; }
    });
  } catch (_) {
    try { window.opener = openerProxy; } catch (_) {}
  }
})();`, webutil.EscapeForJSString(bridgeNonce), webutil.EscapeForJSString(parentURI))
}

func (wv *WebView) handlePopupOpenerNavigate(targetURL string) {
	if wv == nil {
		return
	}

	trimmedURL := strings.TrimSpace(targetURL)
	if trimmedURL == "" {
		return
	}

	wv.mu.RLock()
	opener := wv.popupOpenerBridgeParent
	wv.mu.RUnlock()
	if opener == nil || opener.destroyed.Load() {
		return
	}

	resolvedURL := resolvePopupOpenerNavigationTarget(trimmedURL, opener.URI())
	if resolvedURL == "" {
		return
	}

	if err := opener.LoadURI(context.Background(), resolvedURL); err != nil && opener.ctx != nil {
		logging.FromContext(opener.ctx).Warn().
			Err(err).
			Str("uri", logging.TruncateURL(resolvedURL, logging.PermissionLogURLMaxLen)).
			Msg("cef: failed to navigate synthetic popup opener")
	}

	wv.mu.Lock()
	wv.popupOpenerBridgeParentURI = resolvedURL
	wv.mu.Unlock()
	wv.runOpenerNavigationCallbacks(resolvedURL)
}

func resolvePopupOpenerNavigationTarget(rawTarget, openerURI string) string {
	trimmedTarget := strings.TrimSpace(rawTarget)
	if trimmedTarget == "" {
		return ""
	}
	parsedTarget, err := neturl.Parse(trimmedTarget)
	if err == nil && parsedTarget.IsAbs() {
		return parsedTarget.String()
	}
	if strings.HasPrefix(trimmedTarget, "//") {
		base, baseErr := neturl.Parse(strings.TrimSpace(openerURI))
		if baseErr == nil && base != nil && base.Scheme != "" {
			parsedTarget.Scheme = base.Scheme
			return parsedTarget.String()
		}
		return ""
	}
	base, err := neturl.Parse(strings.TrimSpace(openerURI))
	if err != nil || base == nil || base.Scheme == "" || base.Host == "" {
		return ""
	}
	ref, err := neturl.Parse(trimmedTarget)
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

func (wv *WebView) handlePopupOpenerPostMessage(payload popupOpenerPostMessagePayload) {
	if wv == nil {
		return
	}

	wv.mu.RLock()
	opener := wv.popupOpenerBridgeParent
	wv.mu.RUnlock()
	if opener == nil || opener.destroyed.Load() {
		return
	}
	if !targetOriginMatchesPopupOpener(payload.TargetOrigin, opener.URI()) {
		return
	}

	sourceOrigin := popupSourceOrigin(payload)
	dataExpr := "undefined"
	switch strings.ToLower(strings.TrimSpace(payload.DataKind)) {
	case "json":
		dataExpr = fmt.Sprintf("JSON.parse('%s')", webutil.EscapeForJSString(payload.Data))
	case "string":
		dataExpr = fmt.Sprintf("'%s'", webutil.EscapeForJSString(payload.Data))
	default:
		dataExpr = fmt.Sprintf("'%s'", webutil.EscapeForJSString(payload.Data))
	}

	sourceHref := strings.TrimSpace(payload.SourceHref)
	script := fmt.Sprintf(`(function() {
  try {
    var data;
    try {
      data = %s;
    } catch (_) {
      data = undefined;
    }
    var event = new MessageEvent('message', {
      data: data,
      origin: '%s',
      source: null
    });
    var sourceProxy = {
      closed: false,
      close: function() { return undefined; },
      focus: function() { return undefined; },
      postMessage: function() { return undefined; },
      location: {
        href: '%s',
        toString: function() { return this.href; }
      }
    };
    try {
      Object.defineProperty(event, 'source', {
        configurable: true,
        enumerable: true,
        value: sourceProxy
      });
    } catch (_) {}
    window.dispatchEvent(event);
  } catch (_) {}
})();`, dataExpr, webutil.EscapeForJSString(sourceOrigin), webutil.EscapeForJSString(sourceHref))
	opener.RunJavaScript(context.Background(), script)
	wv.runOpenerMessageCallbacks()
}

func targetOriginMatchesPopupOpener(targetOrigin, openerURI string) bool {
	trimmedTarget := strings.TrimSpace(targetOrigin)
	if trimmedTarget == "" || trimmedTarget == "*" {
		return true
	}
	return canonicalOrigin(trimmedTarget) != "" && canonicalOrigin(trimmedTarget) == canonicalOrigin(openerURI)
}

func popupSourceOrigin(payload popupOpenerPostMessagePayload) string {
	if origin := strings.TrimSpace(payload.SourceOrigin); origin != "" {
		return origin
	}
	return originFromURL(payload.SourceHref)
}

func originFromURL(rawURL string) string {
	canonicalURL := toActualInternalURL(strings.TrimSpace(rawURL))
	parsed, err := neturl.Parse(canonicalURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func canonicalOrigin(rawURL string) string {
	canonicalURL := toActualInternalURL(strings.TrimSpace(rawURL))
	parsed, err := neturl.Parse(canonicalURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return ""
	}
	port := parsed.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	if port == "" {
		return scheme + "://" + host
	}
	return scheme + "://" + host + ":" + port
}
