package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/shared/syncdispatch"
	"github.com/bnema/dumber/internal/ui/coordinator/content"
	layoutmocks "github.com/bnema/dumber/internal/ui/layout/mocks"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type popupDestroySpy struct {
	destroyed int
	detached  int
	closed    int
	shown     int
}

func (p *popupDestroySpy) Destroy()                   { p.destroyed++ }
func (p *popupDestroySpy) DetachContent() *gtk.Widget { p.detached++; return &gtk.Widget{} }
func (p *popupDestroySpy) Close()                     { p.closed++ }
func (p *popupDestroySpy) Show()                      { p.shown++ }

func TestPrepareNativePopupContentWidget_ExpandsWrappedWidget(t *testing.T) {
	widget := layoutmocks.NewMockWidget(t)
	gtkWidget := &gtk.Widget{}
	widget.EXPECT().GtkWidget().Return(gtkWidget).Once()
	widget.EXPECT().SetHexpand(true).Once()
	widget.EXPECT().SetVexpand(true).Once()

	got, err := prepareNativePopupContentWidget(widget)
	require.NoError(t, err)
	assert.Same(t, gtkWidget, got)
}

func TestPrepareNativePopupContentWidget_ErrorsWhenWrappedGTKWidgetMissing(t *testing.T) {
	widget := layoutmocks.NewMockWidget(t)
	widget.EXPECT().GtkWidget().Return(nil).Once()

	got, err := prepareNativePopupContentWidget(widget)
	require.Error(t, err)
	assert.Nil(t, got)
}

func TestOpenNativePopupWindowLeavesWebViewOwnedByCallerOnSetupFailure(t *testing.T) {
	oldNewPopupWindow := newPopupWindow
	t.Cleanup(func() {
		newPopupWindow = oldNewPopupWindow
	})

	expectedErr := errors.New("boom")
	newPopupWindow = func(context.Context, *gtk.Application) (*window.PopupWindow, error) {
		return nil, expectedErr
	}

	wv := portmocks.NewMockWebView(t)
	app := &App{gtkApp: &gtk.Application{}}
	err := app.openNativePopupWindow(context.Background(), content.NativePopupInput{PopupWebView: wv})
	require.ErrorIs(t, err, expectedErr)
}

func TestDestroyFailedNativePopupSetupDestroysOnlyShell(t *testing.T) {
	shell := &popupDestroySpy{}
	destroyFailedNativePopupSetup(shell)
	assert.Equal(t, 1, shell.destroyed)
}

func TestDispatchNativePopupLifecycleSkipsWorkWhenDispatchTimesOutBeforeStart(t *testing.T) {
	var ran bool
	app := &App{
		dispatchOnMainThread: func(label string, fn func()) syncdispatch.SyncDispatchResult {
			return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchTimedOut, Elapsed: 5 * time.Millisecond}
		},
	}

	app.dispatchNativePopupLifecycle("ui.native_popup.close", port.WebViewID(99), func() { ran = true })

	assert.False(t, ran)
}

func TestDispatchNativePopupLifecycleTreatsCompletedAfterTimeoutAsCompleted(t *testing.T) {
	var ran bool
	app := &App{
		dispatchOnMainThread: func(label string, fn func()) syncdispatch.SyncDispatchResult {
			fn()
			return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchCompletedAfterTimeout, Elapsed: 10 * time.Millisecond}
		},
	}

	app.dispatchNativePopupLifecycle("ui.native_popup.close", port.WebViewID(99), func() { ran = true })

	assert.True(t, ran)
}

func TestDetachNativePopupWindowRemovesWidgetWithoutDestroyingWebView(t *testing.T) {
	wv := portmocks.NewMockWebView(t)
	shell := &popupDestroySpy{}
	app := &App{nativePopupWindows: map[port.WebViewID]*nativePopupWindow{
		1: {popupID: 1, webView: wv, popupWindow: shell},
	}}

	detached := app.releaseNativePopupWindow(1, nativePopupReleaseDetach)
	assert.Same(t, wv, detached)
	assert.Equal(t, 1, shell.detached)
	assert.Equal(t, 1, shell.destroyed)
}

func TestNativePopupAbortTransfersEligibleWebViewToBrowserFallback(t *testing.T) {
	wv := portmocks.NewMockWebView(t)
	shell := &popupDestroySpy{}
	app := &App{nativePopupWindows: map[port.WebViewID]*nativePopupWindow{
		1: {popupID: 1, webView: wv, popupWindow: shell},
	}}
	callbackCalls := 0
	app.abortNativePopupWindow(context.Background(), 1, content.NativePopupInput{
		AllowBrowserWindowFallback: true,
		OnNativeHostAbort: func(_ context.Context, got port.WebView) bool {
			callbackCalls++
			assert.Same(t, wv, got)
			assert.Equal(t, 1, shell.detached, "widget must detach before fallback adoption")
			return true
		},
	})

	assert.Equal(t, 1, callbackCalls)
	assert.Equal(t, 1, shell.destroyed)
}

func TestNativePopupAbortDestroysOpenerRequiredWebView(t *testing.T) {
	wv := portmocks.NewMockWebView(t)
	wv.EXPECT().IsDestroyed().Return(false).Once()
	wv.EXPECT().Destroy().Once()
	shell := &popupDestroySpy{}
	app := &App{nativePopupWindows: map[port.WebViewID]*nativePopupWindow{
		1: {popupID: 1, webView: wv, popupWindow: shell},
	}}
	app.abortNativePopupWindow(context.Background(), 1, content.NativePopupInput{
		AllowBrowserWindowFallback: false,
		OnNativeHostAbort:          func(context.Context, port.WebView) bool { return false },
	})
	assert.Equal(t, 1, shell.detached)
}

func TestReleaseNativePopupWindowDestroysOnlyOnce(t *testing.T) {
	wv := portmocks.NewMockWebView(t)
	wv.EXPECT().IsDestroyed().Return(false).Once()
	wv.EXPECT().Destroy().Once()
	shell := &popupDestroySpy{}
	app := &App{nativePopupWindows: map[port.WebViewID]*nativePopupWindow{
		1: {popupID: 1, webView: wv, popupWindow: shell},
	}}

	app.releaseNativePopupWindow(1, nativePopupReleaseDestroy)
	app.releaseNativePopupWindow(1, nativePopupReleaseDestroy)
	_, ok := app.nativePopupWindows[1]
	assert.False(t, ok)
	assert.Equal(t, 1, shell.destroyed)
}
