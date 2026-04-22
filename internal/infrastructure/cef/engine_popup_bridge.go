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
	if current, ok := e.browserWebViews.Load(browserID); ok {
		if wv, ok := current.(*WebView); ok && wv != nil {
			wv.mu.RLock()
			wvBrowser := wv.browser
			wv.mu.RUnlock()
			if wvBrowser != nil && wvBrowser.GetIdentifier() == browserID {
				return wv
			}
			e.unbindBrowserWebView(browserID, wv)
		}
	}

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
		e.bindBrowserWebView(browser, wv)
		return false
	})
	return matched
}

func (e *Engine) withBridgeSourceWebView(browser purecef.Browser, proxyID, action string, fn func(*WebView)) {
	if e == nil || browser == nil {
		return
	}
	wv := e.webViewForBrowser(browser)
	if wv == nil {
		logging.FromContext(e.currentContext()).Warn().
			Int32("browser_id", browser.GetIdentifier()).
			Str("proxy_id", proxyID).
			Msg("cef: " + action + " bridge could not resolve source webview")
		return
	}
	fn(wv)
}

func (e *Engine) handlePopupBridgeOpen(browser purecef.Browser, payload rendererBridgePopupOpenPayload) {
	e.withBridgeSourceWebView(browser, payload.ProxyID, "popup-open", func(wv *WebView) {
		wv.handleSyntheticPopupOpen(payload.URL, payload.FrameName, payload.ProxyID, payload.UserGesture, payload.NoJavaScriptAccess)
	})
}

func (e *Engine) handlePopupBridgeNavigate(browser purecef.Browser, payload rendererBridgePopupNavigatePayload) {
	e.withBridgeSourceWebView(browser, payload.ProxyID, "popup-navigate", func(wv *WebView) {
		wv.handleSyntheticPopupNavigate(payload.ProxyID, payload.URL)
	})
}

func (e *Engine) handlePopupBridgeClose(browser purecef.Browser, payload rendererBridgePopupClosePayload) {
	e.withBridgeSourceWebView(browser, payload.ProxyID, "popup-close", func(wv *WebView) {
		wv.handleSyntheticPopupClose(payload.ProxyID)
	})
}

func (e *Engine) handlePopupOpenerNavigate(browser purecef.Browser, payload popupOpenerNavigatePayload) {
	e.withBridgeSourceWebView(browser, "", "popup-opener-navigate", func(wv *WebView) {
		wv.handlePopupOpenerNavigate(payload.URL)
	})
}

func (e *Engine) handlePopupOpenerPostMessage(browser purecef.Browser, payload popupOpenerPostMessagePayload) {
	e.withBridgeSourceWebView(browser, "", "popup-opener-post-message", func(wv *WebView) {
		wv.handlePopupOpenerPostMessage(payload)
	})
}
