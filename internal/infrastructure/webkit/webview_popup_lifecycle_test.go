package webkit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPopupCloseCallbacksRunExactlyOnce(t *testing.T) {
	wv := &WebView{}
	baseCalls := 0
	lifecycleCalls := 0
	wv.AddCloseCallback(func() { baseCalls++ })
	wv.SetOnClose(func() { lifecycleCalls++ })

	wv.runCloseCallbacks()
	wv.runCloseCallbacks()

	assert.Equal(t, 1, baseCalls)
	assert.Equal(t, 1, lifecycleCalls)
}

func TestPopupLifecycleCallbacksCanBeDisarmed(t *testing.T) {
	wv := &WebView{}
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
