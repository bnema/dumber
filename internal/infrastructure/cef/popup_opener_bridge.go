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

const popupOpenerHTTPScheme = "http"

func (wv *WebView) setPopupNoJavaScriptAccess(noJavaScriptAccess bool) {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	wv.popupNoJavaScriptAccess = noJavaScriptAccess
	wv.mu.Unlock()
}

// AddOpenerMessageCallback implements port.PopupOpenerCapable.
func (wv *WebView) AddOpenerMessageCallback(fn func()) {
	if wv == nil || fn == nil {
		return
	}
	wv.mu.Lock()
	wv.openerMessageCallbacks = append(wv.openerMessageCallbacks, fn)
	wv.mu.Unlock()
}

// AddOpenerNavigationCallback implements port.PopupOpenerCapable.
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

// HasActivePopupOpenerBridge implements port.PopupOpenerCapable.
func (wv *WebView) HasActivePopupOpenerBridge() bool {
	_, active, _ := wv.popupOpenerBridgeState()
	return active
}

func (wv *WebView) popupOpenerBridgeState() (parentURI string, active, blocked bool) {
	if wv == nil {
		return "", false, false
	}
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	blocked = wv.popupNoJavaScriptAccess
	active = !blocked && wv.popupOpenerBridgeParent != nil
	parentURI = wv.popupOpenerBridgeParentURI
	return parentURI, active, blocked
}

func (wv *WebView) ensureBridgeNonceLocked() string {
	if wv == nil {
		return ""
	}
	if wv.bridgeNonce != "" {
		return wv.bridgeNonce
	}
	nonce := newBridgeNonce()
	if nonce == "" {
		return ""
	}
	wv.bridgeNonce = nonce
	return nonce
}

func (wv *WebView) syncPopupOpenerBridgeExtraInfoLocked() {
	if wv == nil || wv.pendingCreate == nil || wv.pendingCreate.windowInfo == nil {
		return
	}
	if wv.popupNoJavaScriptAccess || wv.popupOpenerBridgeParent == nil || wv.popupOpenerBridgeParentURI == "" {
		wv.pendingCreate.extraInfo = nil
		return
	}
	bridgeNonce := wv.ensureBridgeNonceLocked()
	if bridgeNonce == "" {
		wv.pendingCreate.extraInfo = nil
		return
	}
	wv.pendingCreate.extraInfo = popupOpenerRenderExtraInfoBuilder(wv.popupOpenerBridgeParentURI, bridgeNonce)
}

// EnablePopupOpenerBridge implements port.PopupOpenerCapable.
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
		wv.syncPopupOpenerBridgeExtraInfoLocked()
		if wv.ctx != nil {
			logging.FromContext(wv.ctx).Debug().
				Uint64("webview_id", uint64(wv.id)).
				Bool("no_javascript_access", true).
				Msg("cef: popup opener bridge disabled")
		}
		return
	}
	parentWV, ok := parent.(*WebView)
	if !ok || parentWV == nil {
		wv.popupOpenerBridgeParent = nil
		wv.popupOpenerBridgeParentURI = ""
		wv.syncPopupOpenerBridgeExtraInfoLocked()
		if wv.ctx != nil {
			logging.FromContext(wv.ctx).Debug().
				Uint64("webview_id", uint64(wv.id)).
				Msg("cef: popup opener bridge unavailable: parent missing")
		}
		return
	}
	wv.popupOpenerBridgeParent = parentWV
	wv.popupOpenerBridgeParentURI = parentWV.URI()
	wv.syncPopupOpenerBridgeExtraInfoLocked()
	if wv.ctx != nil {
		logging.FromContext(wv.ctx).Debug().
			Uint64("webview_id", uint64(wv.id)).
			Uint64("parent_webview_id", uint64(parentWV.id)).
			Str("parent_uri", logging.TruncateURL(wv.popupOpenerBridgeParentURI, logging.PermissionLogURLMaxLen)).
			Msg("cef: popup opener bridge enabled")
	}
}

func (wv *WebView) popupOpenerBridgeScript(bridgeNonce string) string {
	if wv == nil || bridgeNonce == "" {
		return ""
	}

	wv.mu.RLock()
	parentURI := wv.popupOpenerBridgeParentURI
	parent := wv.popupOpenerBridgeParent
	noJavaScriptAccess := wv.popupNoJavaScriptAccess
	blocked := noJavaScriptAccess || parent == nil
	wv.mu.RUnlock()
	if blocked {
		if wv.ctx != nil {
			logging.FromContext(wv.ctx).Debug().
				Uint64("webview_id", uint64(wv.id)).
				Bool("blocked", true).
				Bool("has_parent", parent != nil).
				Bool("no_javascript_access", noJavaScriptAccess).
				Msg("cef: popup opener bridge script skipped")
		}
		return ""
	}
	if wv.ctx != nil {
		logging.FromContext(wv.ctx).Debug().
			Uint64("webview_id", uint64(wv.id)).
			Str("parent_uri", logging.TruncateURL(parentURI, logging.PermissionLogURLMaxLen)).
			Msg("cef: popup opener bridge script prepared")
	}

	return buildPopupOpenerBridgeJS(bridgeNonce, parentURI)
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
		if wv.ctx != nil {
			logging.FromContext(wv.ctx).Debug().
				Uint64("webview_id", uint64(wv.id)).
				Str("target_url", logging.TruncateURL(trimmedURL, logging.PermissionLogURLMaxLen)).
				Msg("cef: popup opener navigate ignored: opener unavailable")
		}
		return
	}

	resolvedURL := resolvePopupOpenerNavigationTarget(trimmedURL, opener.URI())
	if resolvedURL == "" {
		if wv.ctx != nil {
			logging.FromContext(wv.ctx).Warn().
				Uint64("webview_id", uint64(wv.id)).
				Str("target_url", logging.TruncateURL(trimmedURL, logging.PermissionLogURLMaxLen)).
				Msg("cef: popup opener navigate rejected")
		}
		return
	}
	if wv.ctx != nil {
		logging.FromContext(wv.ctx).Debug().
			Uint64("webview_id", uint64(wv.id)).
			Str("target_url", logging.TruncateURL(trimmedURL, logging.PermissionLogURLMaxLen)).
			Str("resolved_url", logging.TruncateURL(resolvedURL, logging.PermissionLogURLMaxLen)).
			Msg("cef: popup opener navigate received")
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
		return sanitizePopupOpenerNavigationTarget(parsedTarget)
	}
	if strings.HasPrefix(trimmedTarget, "//") {
		base, baseErr := neturl.Parse(strings.TrimSpace(openerURI))
		if baseErr == nil && base != nil && base.Scheme != "" {
			parsedTarget.Scheme = base.Scheme
			return sanitizePopupOpenerNavigationTarget(parsedTarget)
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
	return sanitizePopupOpenerNavigationTarget(base.ResolveReference(ref))
}

func sanitizePopupOpenerNavigationTarget(target *neturl.URL) string {
	if target == nil {
		return ""
	}
	switch scheme := strings.ToLower(strings.TrimSpace(target.Scheme)); scheme {
	case popupOpenerHTTPScheme, actualInternalScheme:
		return target.String()
	default:
		return ""
	}
}

func (wv *WebView) handlePopupOpenerPostMessage(payload popupOpenerPostMessagePayload) {
	if wv == nil {
		return
	}

	wv.mu.RLock()
	opener := wv.popupOpenerBridgeParent
	wv.mu.RUnlock()
	if opener == nil || opener.destroyed.Load() {
		if wv.ctx != nil {
			logging.FromContext(wv.ctx).Debug().
				Uint64("webview_id", uint64(wv.id)).
				Str("target_origin", payload.TargetOrigin).
				Msg("cef: popup opener postMessage ignored: opener unavailable")
		}
		return
	}
	if !targetOriginMatchesPopupOpener(payload.TargetOrigin, opener.URI()) {
		if wv.ctx != nil {
			logging.FromContext(wv.ctx).Warn().
				Uint64("webview_id", uint64(wv.id)).
				Str("target_origin", payload.TargetOrigin).
				Str("opener_uri", logging.TruncateURL(opener.URI(), logging.PermissionLogURLMaxLen)).
				Msg("cef: popup opener postMessage rejected")
		}
		return
	}
	if wv.ctx != nil {
		logging.FromContext(wv.ctx).Debug().
			Uint64("webview_id", uint64(wv.id)).
			Str("target_origin", payload.TargetOrigin).
			Str("source_origin", payload.SourceOrigin).
			Str("source_href", logging.TruncateURL(payload.SourceHref, logging.PermissionLogURLMaxLen)).
			Msg("cef: popup opener postMessage received")
	}

	sourceOrigin := popupSourceOrigin(payload)
	var dataExpr string
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
	targetCanonical := canonicalOrigin(trimmedTarget)
	return targetCanonical != "" && targetCanonical == canonicalOrigin(openerURI)
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
	portStr := parsed.Port()
	if portStr == "" {
		switch scheme {
		case popupOpenerHTTPScheme:
			portStr = "80"
		case actualInternalScheme:
			portStr = "443"
		}
	}
	if portStr == "" {
		return scheme + "://" + host
	}
	return scheme + "://" + host + ":" + portStr
}
