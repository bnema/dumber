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
	destroyed bool
}

func (p *popupDestroySpy) Destroy() {
	p.destroyed = true
}

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

func TestOpenNativePopupWindow_DestroysWebViewWhenPopupShellCreationFails(t *testing.T) {
	oldNewPopupWindow := newPopupWindow
	t.Cleanup(func() {
		newPopupWindow = oldNewPopupWindow
	})

	expectedErr := errors.New("boom")
	newPopupWindow = func(context.Context, *gtk.Application) (*window.PopupWindow, error) {
		return nil, expectedErr
	}

	wv := portmocks.NewMockWebView(t)
	wv.EXPECT().IsDestroyed().Return(false).Once()
	wv.EXPECT().Destroy().Once()

	app := &App{gtkApp: &gtk.Application{}}
	err := app.openNativePopupWindow(context.Background(), content.NativePopupInput{PopupWebView: wv})
	require.ErrorIs(t, err, expectedErr)
}

func TestDestroyFailedNativePopupSetup_DestroysWebViewAndShell(t *testing.T) {
	wv := portmocks.NewMockWebView(t)
	wv.EXPECT().IsDestroyed().Return(false).Once()
	wv.EXPECT().Destroy().Once()

	shell := &popupDestroySpy{}
	destroyFailedNativePopupSetup(shell, wv)

	assert.True(t, shell.destroyed)
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

func TestReleaseNativePopupWindow_DestroysWebViewAndRemovesState(t *testing.T) {
	wv := portmocks.NewMockWebView(t)
	wv.EXPECT().IsDestroyed().Return(false).Once()
	wv.EXPECT().Destroy().Once()

	app := &App{nativePopupWindows: map[port.WebViewID]*nativePopupWindow{
		port.WebViewID(1): {
			popupID: port.WebViewID(1),
			webView: wv,
		},
	}}

	app.releaseNativePopupWindow(port.WebViewID(1), false, false)
	_, ok := app.nativePopupWindows[port.WebViewID(1)]
	assert.False(t, ok)
}
