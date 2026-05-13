package ui

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/stretchr/testify/assert"
)

type popupDestroySpy struct {
	destroyed bool
}

func (p *popupDestroySpy) Destroy() {
	p.destroyed = true
}

func TestDestroyFailedNativePopupSetup_DestroysWebViewAndShell(t *testing.T) {
	wv := portmocks.NewMockWebView(t)
	wv.EXPECT().IsDestroyed().Return(false).Once()
	wv.EXPECT().Destroy().Once()

	shell := &popupDestroySpy{}
	destroyFailedNativePopupSetup(shell, wv)

	assert.True(t, shell.destroyed)
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
