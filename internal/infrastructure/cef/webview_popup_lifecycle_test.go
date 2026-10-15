package cef

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPopupLifecycleCallbacksCanBeDisarmed(t *testing.T) {
	wv := &WebView{inputAttached: true}
	readyCalls := 0
	lifecycleCloseCalls := 0
	oauthCloseCalls := 0
	wv.SetOnReadyToShow(func() { readyCalls++ })
	wv.SetOnClose(func() { lifecycleCloseCalls++ })
	wv.AddCloseCallback(func() { oauthCloseCalls++ })

	wv.SetOnReadyToShow(nil)
	wv.SetOnClose(nil)
	wv.fireReadyToShow()
	wv.runCloseCallbacks()

	assert.Zero(t, readyCalls)
	assert.Zero(t, lifecycleCloseCalls)
	assert.Equal(t, 1, oauthCloseCalls)
}

func TestOnBeforePopupAborted_ReleasesUnrecoverablePaneHostedPopup(t *testing.T) {
	parent := &WebView{ctx: context.Background()}
	popup := &WebView{ctx: context.Background(), nativePopupCandidate: true, nativePopupParent: parent, nativePopupID: 77}
	closeCalls := 0
	popup.SetOnClose(func() { closeCalls++ })
	parent.trackPendingNativePopup(77, popup)

	(&handlerSet{wv: parent}).OnBeforePopupAborted(nil, 77)

	assert.Equal(t, 1, closeCalls)
	assert.True(t, popup.IsDestroyed())
	assert.Nil(t, parent.takePendingNativePopup(77))
}

func TestOnBeforePopupAborted_KeepsRecoverablePopupForDirectFallback(t *testing.T) {
	parent := &WebView{ctx: context.Background()}
	popup := &WebView{
		ctx: context.Background(), nativePopupCandidate: true, nativePopupParent: parent, nativePopupID: 78,
		pendingCreate: &pendingBrowserCreate{},
	}
	closeCalls := 0
	popup.SetOnClose(func() { closeCalls++ })
	parent.trackPendingNativePopup(78, popup)

	(&handlerSet{wv: parent}).OnBeforePopupAborted(nil, 78)

	assert.Zero(t, closeCalls)
	assert.False(t, popup.IsDestroyed())
	assert.False(t, popup.isNativePopupCandidate())
}

func TestOnBeforePopupAborted_LeavesNativeHostedPopupToHost(t *testing.T) {
	parent := &WebView{ctx: context.Background()}
	popup := &WebView{ctx: context.Background(), nativePopupCandidate: true, nativePopupParent: parent, nativePopupID: 79}
	closeCalls := 0
	popup.SetOnClose(func() { closeCalls++ })
	popup.SetNativePopupHostAbort(func() {})
	parent.trackPendingNativePopup(79, popup)

	(&handlerSet{wv: parent}).OnBeforePopupAborted(nil, 79)

	assert.Zero(t, closeCalls)
	assert.False(t, popup.IsDestroyed())
}

func TestOnBeforePopupAborted_UnknownPopupIsNoop(_ *testing.T) {
	(&handlerSet{wv: &WebView{ctx: context.Background()}}).OnBeforePopupAborted(nil, 123)
}
