package cef

import (
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"
)

type resizeOrderHost struct {
	calls []string
}

func (h *resizeOrderHost) WasResized() {
	h.calls = append(h.calls, "WasResized")
}

func (h *resizeOrderHost) Invalidate(_ purecef.PaintElementType) {
	h.calls = append(h.calls, "Invalidate")
}

func TestNotifyBrowserResize_CallsWasResizedThenInvalidate(t *testing.T) {
	host := &resizeOrderHost{}

	notifyBrowserResize(host)

	require.Equal(t, []string{"WasResized", "Invalidate"}, host.calls)
}
