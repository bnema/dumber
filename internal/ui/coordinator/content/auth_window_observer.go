package content

import (
	"context"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/glib"
)

const nativePopupOAuthCloseDelay = 500 * time.Millisecond

func (c *Coordinator) ObserveNativePopupAuth(ctx context.Context, input NativePopupInput) {
	if c == nil || input.PopupWebView == nil || !IsOAuthURL(input.TargetURI) {
		return
	}

	popupID := input.PopupWebView.ID()

	oauthWV, ok := input.PopupWebView.(port.OAuthCallbackCapable)
	if !ok {
		logging.FromContext(ctx).Debug().
			Uint64("popup_id", uint64(popupID)).
			Msg("native popup auth observation skipped: webview lacks OAuth callbacks")
		return
	}
	c.trackOAuthPopup(popupID, input.ParentPaneID, input.ParentURIAtOpen)

	requestClose := func(reason string) {
		logging.FromContext(ctx).Info().
			Uint64("popup_id", uint64(popupID)).
			Str("reason", reason).
			Msg("native popup oauth callback detected, closing popup")
		time.AfterFunc(nativePopupOAuthCloseDelay, func() {
			cb := glib.SourceFunc(func(_ uintptr) bool {
				if input.PopupWebView != nil && !input.PopupWebView.IsDestroyed() {
					oauthWV.Close()
				}
				return false
			})
			glib.IdleAdd(&cb, 0)
		})
	}

	oauthWV.AddNavigationCallback(func(uri string) {
		if !ShouldAutoClose(uri) {
			return
		}
		c.capturePopupOAuthState(popupID, uri)
		requestClose("navigation")
	})
	oauthWV.AddCloseCallback(func() {
		c.handlePopupOAuthClose(context.Background(), popupID)
	})

	logging.FromContext(ctx).Debug().
		Uint64("popup_id", uint64(popupID)).
		Str("target_uri", logging.TruncateURL(input.TargetURI, logURLMaxLen)).
		Msg("native popup auth observer configured")
}
