package cef

import (
	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/logging"
)

func (e *Engine) webViewForBrowser(browser purecef.Browser) *WebView {
	if e == nil || browser == nil {
		return nil
	}

	browserID := browser.GetIdentifier()
	var matched *WebView
	e.activeWebViews.Range(func(_, value any) bool {
		wv, ok := value.(*WebView)
		if !ok || wv == nil {
			return true
		}
		wv.mu.RLock()
		wvBrowser := wv.browser
		wv.mu.RUnlock()
		if wvBrowser == nil || wvBrowser.GetIdentifier() != browserID {
			return true
		}
		matched = wv
		return false
	})
	return matched
}

func (e *Engine) handlePopupBridgeOpen(browser purecef.Browser, payload rendererBridgePopupOpenPayload) {
	if e == nil {
		return
	}
	wv := e.webViewForBrowser(browser)
	if wv == nil {
		logging.FromContext(e.currentContext()).Warn().
			Int32("browser_id", browser.GetIdentifier()).
			Str("proxy_id", payload.ProxyID).
			Msg("cef: popup-open bridge could not resolve source webview")
		return
	}
	wv.handleSyntheticPopupOpen(payload.URL, payload.FrameName, payload.ProxyID, payload.UserGesture, payload.NoJavaScriptAccess)
}

func (e *Engine) handlePopupBridgeNavigate(browser purecef.Browser, payload rendererBridgePopupNavigatePayload) {
	if e == nil {
		return
	}
	wv := e.webViewForBrowser(browser)
	if wv == nil {
		logging.FromContext(e.currentContext()).Warn().
			Int32("browser_id", browser.GetIdentifier()).
			Str("proxy_id", payload.ProxyID).
			Msg("cef: popup-navigate bridge could not resolve source webview")
		return
	}
	wv.handleSyntheticPopupNavigate(payload.ProxyID, payload.URL)
}

func (e *Engine) handlePopupBridgeClose(browser purecef.Browser, payload rendererBridgePopupClosePayload) {
	if e == nil {
		return
	}
	wv := e.webViewForBrowser(browser)
	if wv == nil {
		logging.FromContext(e.currentContext()).Warn().
			Int32("browser_id", browser.GetIdentifier()).
			Str("proxy_id", payload.ProxyID).
			Msg("cef: popup-close bridge could not resolve source webview")
		return
	}
	wv.handleSyntheticPopupClose(payload.ProxyID)
}
