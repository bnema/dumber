package cef

import (
	"context"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

type syntheticPopupState struct {
	WebView            port.WebView
	PendingURI         string
	NoJavaScriptAccess bool
	Closed             bool
}

func (wv *WebView) syntheticPopupState(proxyID string) *syntheticPopupState {
	if wv == nil || proxyID == "" {
		return nil
	}

	wv.syntheticPopupMu.Lock()
	defer wv.syntheticPopupMu.Unlock()
	if wv.syntheticPopups == nil {
		return nil
	}
	return wv.syntheticPopups[proxyID]
}

// syntheticPopupStateLocked returns the synthetic popup state for proxyID.
// Caller must hold wv.syntheticPopupMu.
func (wv *WebView) syntheticPopupStateLocked(proxyID string) *syntheticPopupState {
	if wv == nil || proxyID == "" {
		return nil
	}
	if wv.syntheticPopups == nil {
		wv.syntheticPopups = make(map[string]*syntheticPopupState)
	}
	state := wv.syntheticPopups[proxyID]
	if state == nil {
		state = &syntheticPopupState{}
		wv.syntheticPopups[proxyID] = state
	}
	return state
}

func (wv *WebView) deleteSyntheticPopupState(proxyID string) {
	if wv == nil || proxyID == "" {
		return
	}

	wv.syntheticPopupMu.Lock()
	defer wv.syntheticPopupMu.Unlock()
	delete(wv.syntheticPopups, proxyID)
}

func (wv *WebView) handleSyntheticPopupOpen(targetURL, frameName, proxyID string, userGesture, noJavaScriptAccess bool) {
	if wv == nil || proxyID == "" || wv.destroyed.Load() {
		return
	}

	wv.syntheticPopupMu.Lock()
	state := wv.syntheticPopupStateLocked(proxyID)
	if state.Closed {
		delete(wv.syntheticPopups, proxyID)
		wv.syntheticPopupMu.Unlock()
		return
	}
	state.NoJavaScriptAccess = noJavaScriptAccess
	wv.syntheticPopupMu.Unlock()

	wv.runOnGTK(func() {
		if wv.destroyed.Load() {
			return
		}

		wv.syntheticPopupMu.Lock()
		if state := wv.syntheticPopups[proxyID]; state != nil && state.Closed {
			delete(wv.syntheticPopups, proxyID)
			wv.syntheticPopupMu.Unlock()
			return
		}
		wv.syntheticPopupMu.Unlock()

		wv.mu.RLock()
		cb := wv.callbacks
		wv.mu.RUnlock()
		if cb == nil || cb.OnCreate == nil {
			wv.deleteSyntheticPopupState(proxyID)
			return
		}

		popupWV := cb.OnCreate(port.PopupRequest{
			TargetURI:          targetURL,
			FrameName:          frameName,
			IsUserGesture:      userGesture,
			NoJavaScriptAccess: noJavaScriptAccess,
			ParentViewID:       wv.id,
		})
		if popupWV == nil {
			wv.deleteSyntheticPopupState(proxyID)
			return
		}
		if cefPopup, ok := popupWV.(*WebView); ok {
			cefPopup.setPopupNoJavaScriptAccess(noJavaScriptAccess)
		}

		pendingURI := ""
		wv.syntheticPopupMu.Lock()
		state := wv.syntheticPopupStateLocked(proxyID)
		if state.Closed {
			delete(wv.syntheticPopups, proxyID)
			wv.syntheticPopupMu.Unlock()
			closeSyntheticPopupWebView(popupWV)
			return
		}
		state.WebView = popupWV
		state.NoJavaScriptAccess = noJavaScriptAccess
		pendingURI = strings.TrimSpace(state.PendingURI)
		wv.syntheticPopupMu.Unlock()

		if pendingURI == "" || pendingURI == strings.TrimSpace(targetURL) || popupWV.IsDestroyed() {
			return
		}
		if err := popupWV.LoadURI(context.Background(), pendingURI); err != nil {
			logging.FromContext(wv.ctx).Warn().
				Err(err).
				Str("proxy_id", proxyID).
				Str("uri", logging.TruncateURL(pendingURI, logging.PermissionLogURLMaxLen)).
				Msg("cef: failed to replay synthetic popup navigation")
		}
	})
}

func (wv *WebView) handleSyntheticPopupNavigate(proxyID, targetURL string) {
	if wv == nil || proxyID == "" || wv.destroyed.Load() {
		return
	}

	trimmedURI := strings.TrimSpace(targetURL)
	wv.syntheticPopupMu.Lock()
	state := wv.syntheticPopupStateLocked(proxyID)
	if state.Closed {
		wv.syntheticPopupMu.Unlock()
		return
	}
	state.PendingURI = trimmedURI
	popupWV := state.WebView
	wv.syntheticPopupMu.Unlock()

	if trimmedURI == "" || popupWV == nil {
		return
	}

	wv.runOnGTK(func() {
		if popupWV.IsDestroyed() {
			wv.syntheticPopupMu.Lock()
			delete(wv.syntheticPopups, proxyID)
			wv.syntheticPopupMu.Unlock()
			return
		}
		if err := popupWV.LoadURI(context.Background(), trimmedURI); err != nil {
			logging.FromContext(wv.ctx).Warn().
				Err(err).
				Str("proxy_id", proxyID).
				Str("uri", logging.TruncateURL(trimmedURI, logging.PermissionLogURLMaxLen)).
				Msg("cef: failed to navigate synthetic popup")
		}
	})
}

func (wv *WebView) handleSyntheticPopupClose(proxyID string) {
	if wv == nil || proxyID == "" || wv.destroyed.Load() {
		return
	}

	wv.syntheticPopupMu.Lock()
	if wv.syntheticPopups == nil {
		wv.syntheticPopupMu.Unlock()
		return
	}
	state := wv.syntheticPopups[proxyID]
	if state == nil {
		wv.syntheticPopupMu.Unlock()
		return
	}
	state.Closed = true
	popupWV := state.WebView
	delete(wv.syntheticPopups, proxyID)
	wv.syntheticPopupMu.Unlock()
	if popupWV == nil {
		return
	}

	wv.runOnGTK(func() {
		closeSyntheticPopupWebView(popupWV)
	})
}

func closeSyntheticPopupWebView(popupWV port.WebView) {
	if popupWV == nil {
		return
	}
	if closeable, ok := popupWV.(port.OAuthCallbackCapable); ok {
		closeable.Close()
		return
	}
	popupWV.Destroy()
}
