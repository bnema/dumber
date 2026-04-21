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
}

func (wv *WebView) syntheticPopupState(proxyID string) *syntheticPopupState {
	if wv == nil || proxyID == "" {
		return nil
	}

	wv.syntheticPopupMu.Lock()
	defer wv.syntheticPopupMu.Unlock()
	return wv.syntheticPopupStateLocked(proxyID)
}

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
	if state := wv.syntheticPopups[proxyID]; state != nil {
		state.NoJavaScriptAccess = noJavaScriptAccess
	}
	wv.syntheticPopupMu.Unlock()

	wv.runOnGTK(func() {
		if wv.destroyed.Load() {
			return
		}

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

		pendingURI := ""
		wv.syntheticPopupMu.Lock()
		state := wv.syntheticPopupStateLocked(proxyID)
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
	state := wv.syntheticPopupState(proxyID)
	if state == nil {
		return
	}

	wv.syntheticPopupMu.Lock()
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
	state := wv.syntheticPopups[proxyID]
	if state != nil {
		delete(wv.syntheticPopups, proxyID)
	}
	wv.syntheticPopupMu.Unlock()
	if state == nil || state.WebView == nil {
		return
	}

	closeable, ok := state.WebView.(port.OAuthCallbackCapable)
	if !ok {
		return
	}

	wv.runOnGTK(func() {
		closeable.Close()
	})
}
