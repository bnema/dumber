package cef

import (
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
