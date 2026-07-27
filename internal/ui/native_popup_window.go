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

type nativePopupShell interface {
	DetachContent() *gtk.Widget
	Destroy()
	Close()
	Show()
}

type nativePopupWindow struct {
	popupID        port.WebViewID
	parentPaneID   entity.PaneID
	parentWindowID string
	popupWindow    nativePopupShell
	webView        port.WebView
	closeOnce      sync.Once
}

type nativePopupReleaseMode int

const (
	nativePopupReleaseDestroy nativePopupReleaseMode = iota
	nativePopupReleaseDetach
)

var newPopupWindow = window.NewPopup

func destroyFailedNativePopupSetup(popupShell nativePopupShell) {
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
		return err
	}
	if a.contentCoord == nil {
		destroyFailedNativePopupSetup(popupShell)
		return fmt.Errorf("content coordinator not available for native popup")
	}
	widget := a.contentCoord.WrapWidget(ctx, input.PopupWebView)
	gtkWidget, err := prepareNativePopupContentWidget(widget)
	if err != nil {
		destroyFailedNativePopupSetup(popupShell)
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
			a.releaseNativePopupWindow(popupID, nativePopupReleaseDestroy)
			return false
		}
		popupShell.Window().ConnectCloseRequest(&closeRequestCb)
	}

	if aborter, ok := input.PopupWebView.(port.NativePopupHostAbortCapable); ok {
		aborter.SetNativePopupHostAbort(func() {
			a.dispatchNativePopupLifecycle("ui.native_popup.abort", popupID, func() {
				a.abortNativePopupWindow(context.Background(), popupID, input)
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
				a.releaseNativePopupWindow(popupID, nativePopupReleaseDestroy)
			})
		})
	} else {
		popupShell.Show()
	}
	if oauthWV, ok := input.PopupWebView.(port.OAuthCallbackCapable); ok {
		oauthWV.AddCloseCallback(func() {
			a.dispatchNativePopupLifecycle("ui.native_popup.oauth_close", popupID, func() {
				a.releaseNativePopupWindow(popupID, nativePopupReleaseDestroy)
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

func (a *App) abortNativePopupWindow(ctx context.Context, popupID port.WebViewID, input content.NativePopupInput) {
	wv := a.releaseNativePopupWindow(popupID, nativePopupReleaseDetach)
	if wv == nil {
		return
	}
	transferred := false
	if input.AllowBrowserWindowFallback && input.OnNativeHostAbort != nil {
		transferred = input.OnNativeHostAbort(ctx, wv)
	} else if input.OnNativeHostAbort != nil {
		input.OnNativeHostAbort(ctx, wv)
	}
	if !transferred && !wv.IsDestroyed() {
		wv.Destroy()
	}
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

func (a *App) releaseNativePopupWindow(popupID port.WebViewID, mode nativePopupReleaseMode) port.WebView {
	if a == nil || a.nativePopupWindows == nil {
		return nil
	}
	state := a.nativePopupWindows[popupID]
	if state == nil {
		return nil
	}
	var detached port.WebView
	state.closeOnce.Do(func() {
		delete(a.nativePopupWindows, popupID)
		if mode == nativePopupReleaseDetach {
			if state.popupWindow != nil && state.popupWindow.DetachContent() != nil {
				detached = state.webView
			} else if state.webView != nil && !state.webView.IsDestroyed() {
				state.webView.Destroy()
			}
		} else if state.webView != nil && !state.webView.IsDestroyed() {
			state.webView.Destroy()
		}
		if state.popupWindow != nil {
			state.popupWindow.Destroy()
		}
	})
	return detached
}
