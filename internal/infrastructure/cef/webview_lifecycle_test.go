package cef

import (
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"
)

func TestDestroy_WithHostDefersBridgeDestroyUntilBeforeClose(t *testing.T) {
	bridge := &Cef2gtkAdapter{}
	host := &stubBrowserHost{}
	wv := &WebView{
		viewBridge: bridge,
		host:       host,
	}

	wv.Destroy()

	require.True(t, wv.IsDestroyed())
	require.True(t, host.closeBrowserCalled)
	require.False(t, bridge.IsDestroyed(), "Destroy should request CEF close before destroying the GTK bridge")

	wv.destroyViewBridgeOnGTKSync()

	require.True(t, bridge.IsDestroyed())
}

func TestDestroy_WithoutHostDestroysBridgeImmediately(t *testing.T) {
	bridge := &Cef2gtkAdapter{}
	wv := &WebView{viewBridge: bridge}

	wv.Destroy()

	require.True(t, bridge.IsDestroyed())
}

// stubBrowserHost intentionally only stubs CloseBrowser. The embedded
// purecef.BrowserHost may be nil; calling any other BrowserHost method will
// panic and should be avoided in these lifecycle tests.
type stubBrowserHost struct {
	purecef.BrowserHost
	closeBrowserCalled bool
}

func (s *stubBrowserHost) CloseBrowser(int32) {
	s.closeBrowserCalled = true
}
