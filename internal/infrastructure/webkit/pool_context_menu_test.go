package webkit

import "testing"

func TestNeedsExplicitContextMenuSignalOnAcquire(t *testing.T) {
	t.Run("true for prewarmed view that already has other signals", func(t *testing.T) {
		wv := &WebView{signalIDs: []uintptr{1, 2, 3}}
		if !needsExplicitContextMenuSignalOnAcquire(wv, &contextMenuPipeline{}) {
			t.Fatal("expected prewarmed webview to require explicit context-menu signal connection")
		}
	})

	t.Run("false when pipeline missing", func(t *testing.T) {
		wv := &WebView{signalIDs: []uintptr{1}}
		if needsExplicitContextMenuSignalOnAcquire(wv, nil) {
			t.Fatal("expected missing pipeline to skip explicit context-menu signal connection")
		}
	})

	t.Run("false for reused view with disconnected signals", func(t *testing.T) {
		wv := &WebView{}
		if needsExplicitContextMenuSignalOnAcquire(wv, &contextMenuPipeline{}) {
			t.Fatal("expected reused webview to rely on full signal reconnection")
		}
	})
}
