package ui

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/coordinator/content"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/bnema/puregotk/v4/gtk"
)

type nativePopupWindow struct {
	popupID        port.WebViewID
	parentPaneID   entity.PaneID
	parentWindowID string
	popupWindow    *window.PopupWindow
	webView        port.WebView
	closeOnce      sync.Once
}

func (a *App) ensureNativePopupWindows() {
	if a.nativePopupWindows == nil {
		a.nativePopupWindows = make(map[port.WebViewID]*nativePopupWindow)
	}
}

func (a *App) openNativePopupWindow(ctx context.Context, input content.NativePopupInput) error {
	if a == nil || a.gtkApp == nil {
		return fmt.Errorf("gtk application not available for native popup")
	}
	if input.PopupWebView == nil {
		return fmt.Errorf("native popup webview is nil")
	}

	popupShell, err := window.NewPopup(ctx, a.gtkApp)
	if err != nil {
		return err
	}
	widget := a.contentCoord.WrapWidget(ctx, input.PopupWebView)
	if widget == nil || widget.GtkWidget() == nil {
		popupShell.Destroy()
		return fmt.Errorf("failed to wrap native popup webview widget")
	}
	popupShell.SetContent(widget.GtkWidget())

	parentWindowID := ""
	if bw := a.browserWindowForAnyPane(input.ParentPaneID); bw != nil {
		parentWindowID = bw.id
	}

	popupID := input.PopupWebView.ID()
	state := &nativePopupWindow{
		popupID:        popupID,
		parentPaneID:   input.ParentPaneID,
		parentWindowID: parentWindowID,
		popupWindow:    popupShell,
		webView:        input.PopupWebView,
	}
	popupShell.SetTitle("Dumber")

	a.ensureNativePopupWindows()
	a.nativePopupWindows[popupID] = state

	if popupShell.Window() != nil {
		closeRequestCb := func(_ gtk.Window) bool {
			a.releaseNativePopupWindow(popupID, false, false)
			return false
		}
		popupShell.Window().ConnectCloseRequest(&closeRequestCb)
	}

	if aborter, ok := input.PopupWebView.(port.NativePopupHostAbortCapable); ok {
		aborter.SetNativePopupHostAbort(func() {
			a.dispatchOnMainThread(func() {
				a.releaseNativePopupWindow(popupID, true, false)
			})
		})
	}
	if lifecycle, ok := input.PopupWebView.(port.PopupLifecycleCapable); ok {
		lifecycle.SetOnReadyToShow(func() {
			a.dispatchOnMainThread(func() {
				a.showNativePopupWindow(popupID)
			})
		})
		lifecycle.SetOnClose(func() {
			a.dispatchOnMainThread(func() {
				a.releaseNativePopupWindow(popupID, true, false)
			})
		})
	} else {
		popupShell.Show()
	}
	if oauthWV, ok := input.PopupWebView.(port.OAuthCallbackCapable); ok {
		oauthWV.AddCloseCallback(func() {
			a.dispatchOnMainThread(func() {
				a.releaseNativePopupWindow(popupID, true, false)
			})
		})
	}
	if a.contentCoord != nil && input.ObserveOAuthAutoClose {
		a.contentCoord.ObserveNativePopupAuth(ctx, input)
	}

	logging.FromContext(ctx).Info().
		Uint64("popup_id", uint64(popupID)).
		Str("target_uri", input.TargetURI).
		Str("parent_pane_id", string(input.ParentPaneID)).
		Msg("native popup host created")
	return nil
}

func (a *App) showNativePopupWindow(popupID port.WebViewID) {
	if a == nil || a.nativePopupWindows == nil {
		return
	}
	if state := a.nativePopupWindows[popupID]; state != nil && state.popupWindow != nil {
		state.popupWindow.Show()
	}
}

func (a *App) releaseNativePopupWindow(popupID port.WebViewID, closeWindow, destroyWindow bool) {
	if a == nil || a.nativePopupWindows == nil {
		return
	}
	state := a.nativePopupWindows[popupID]
	if state == nil {
		return
	}
	state.closeOnce.Do(func() {
		delete(a.nativePopupWindows, popupID)
		if state.webView != nil && !state.webView.IsDestroyed() {
			state.webView.Destroy()
		}
		if state.popupWindow != nil {
			switch {
			case destroyWindow:
				state.popupWindow.Destroy()
			case closeWindow:
				state.popupWindow.Close()
			}
		}
	})
}
