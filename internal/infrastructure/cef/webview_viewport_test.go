package cef

import (
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"
)

type viewportSyncOrderHost struct {
	calls []string
}

func (h *viewportSyncOrderHost) WasHidden(state int32) {
	h.calls = append(h.calls, "WasHidden")
	if state != 0 {
		h.calls = append(h.calls, "WasHidden(nonzero)")
	}
}

func (h *viewportSyncOrderHost) NotifyScreenInfoChanged() {
	h.calls = append(h.calls, "NotifyScreenInfoChanged")
}

func (h *viewportSyncOrderHost) WasResized() {
	h.calls = append(h.calls, "WasResized")
}

func (h *viewportSyncOrderHost) Invalidate(_ purecef.PaintElementType) {
	h.calls = append(h.calls, "Invalidate")
}

func TestNotifyBrowserViewportSync_VisibleCallsFullSequence(t *testing.T) {
	host := &viewportSyncOrderHost{}

	notifyBrowserViewportSync(host, true)

	require.Equal(t, []string{"WasHidden", "NotifyScreenInfoChanged", "WasResized", "Invalidate"}, host.calls)
}

func TestNotifyBrowserViewportSync_HiddenSkipsWasHidden(t *testing.T) {
	host := &viewportSyncOrderHost{}

	notifyBrowserViewportSync(host, false)

	require.Equal(t, []string{"NotifyScreenInfoChanged", "WasResized", "Invalidate"}, host.calls)
}
