package webkit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHandleLoadChanged_LoadStartedDispatchesOutsideLock(t *testing.T) {
	wv := &WebView{uri: "https://example.com/start", title: "start"}
	var sawURI string
	var sawStateURI string
	done := make(chan struct{})
	wv.OnEditableFocusChanged = func(editable bool) {
		require.False(t, editable)
		// Same-goroutine re-entry would deadlock if wv.mu were still held.
		sawURI = wv.URI()
		state := wv.State()
		sawStateURI = state.URI
		close(done)
	}
	var loadEvents []LoadEvent
	wv.OnLoadChanged = func(ev LoadEvent) {
		loadEvents = append(loadEvents, ev)
	}

	finished := make(chan struct{})
	go func() {
		wv.handleLoadChanged(LoadStarted, "https://example.com/next", "next", 0.1)
		close(finished)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("editable focus callback deadlocked re-entering URI/State")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("handleLoadChanged did not finish")
	}

	require.Equal(t, "https://example.com/next", sawURI)
	require.Equal(t, "https://example.com/next", sawStateURI)
	require.Equal(t, []LoadEvent{LoadStarted}, loadEvents)
	require.True(t, wv.isLoading)
	require.True(t, wv.navigationActive.Load())
}
