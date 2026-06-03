package ui

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/coordinator/content"
	"github.com/bnema/dumber/internal/ui/layout"
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

type nativePopupDestroyer interface {
	Destroy()
}

var newPopupWindow = window.NewPopup

func destroyFailedNativePopupSetup(popupShell nativePopupDestroyer, wv port.WebView) {
	if wv != nil && !wv.IsDestroyed() {
		wv.Destroy()
	}
	if popupShell != nil {
		popupShell.Destroy()
	}
}

func prepareNativePopupContentWidget(widget layout.Widget) (*gtk.Widget, error) {
	if widget == nil {
		return nil, fmt.Errorf("failed to wrap native popup webview widget")
	}
	gtkWidget := widget.GtkWidget()
	if gtkWidget == nil {
		return nil, fmt.Errorf("failed to wrap native popup webview widget")
	}
	widget.SetHexpand(true)
	widget.SetVexpand(true)
	return gtkWidget, nil
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

	popupShell, err := newPopupWindow(ctx, a.gtkApp)
	if err != nil {
		destroyFailedNativePopupSetup(nil, input.PopupWebView)
		return err
	}
	if a.contentCoord == nil {
		destroyFailedNativePopupSetup(popupShell, input.PopupWebView)
		return fmt.Errorf("content coordinator not available for native popup")
	}
	widget := a.contentCoord.WrapWidget(ctx, input.PopupWebView)
	gtkWidget, err := prepareNativePopupContentWidget(widget)
	if err != nil {
		destroyFailedNativePopupSetup(popupShell, input.PopupWebView)
		return err
	}
	popupShell.SetContent(gtkWidget)

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
			a.dispatchNativePopupLifecycle("ui.native_popup.abort", popupID, func() {
				a.releaseNativePopupWindow(popupID, true, false)
			})
		})
	}
	if lifecycle, ok := input.PopupWebView.(port.PopupLifecycleCapable); ok {
		lifecycle.SetOnReadyToShow(func() {
			a.dispatchNativePopupLifecycle("ui.native_popup.ready_to_show", popupID, func() {
				a.showNativePopupWindow(popupID)
			})
		})
		lifecycle.SetOnClose(func() {
			a.dispatchNativePopupLifecycle("ui.native_popup.close", popupID, func() {
				a.releaseNativePopupWindow(popupID, true, false)
			})
		})
	} else {
		popupShell.Show()
	}
	if oauthWV, ok := input.PopupWebView.(port.OAuthCallbackCapable); ok {
		oauthWV.AddCloseCallback(func() {
			a.dispatchNativePopupLifecycle("ui.native_popup.oauth_close", popupID, func() {
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

func (a *App) dispatchNativePopupLifecycle(label string, popupID port.WebViewID, fn func()) {
	if a == nil || fn == nil {
		return
	}
	if a.dispatchOnMainThread == nil {
		fn()
		return
	}
	result := a.dispatchOnMainThread(label, fn)
	if result.Completed() {
		return
	}
	ctx := context.Background()
	if a.deps != nil && a.deps.Ctx != nil {
		ctx = a.deps.Ctx
	}
	logging.FromContext(ctx).Warn().
		Uint64("popup_id", uint64(popupID)).
		Str("dispatch_label", result.Label).
		Dur("elapsed", result.Elapsed).
		Str("dispatch_status", string(result.Status)).
		Msg("native popup lifecycle dispatch did not complete")
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
