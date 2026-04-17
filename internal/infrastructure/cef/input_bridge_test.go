package cef

import (
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"
)

type focusSyncHostRecorder struct {
	calls         []string
	hiddenValues  []int32
	focusValues   []int32
	invalidations []purecef.PaintElementType
}

func (h *focusSyncHostRecorder) WasHidden(hidden int32) {
	h.calls = append(h.calls, "WasHidden")
	h.hiddenValues = append(h.hiddenValues, hidden)
}

func (h *focusSyncHostRecorder) SetFocus(focus int32) {
	h.calls = append(h.calls, "SetFocus")
	h.focusValues = append(h.focusValues, focus)
}

func (h *focusSyncHostRecorder) Invalidate(elementType purecef.PaintElementType) {
	h.calls = append(h.calls, "Invalidate")
	h.invalidations = append(h.invalidations, elementType)
}

func TestSyncWindowlessBrowserFocus_ReassertsVisibleFocusedAndInvalidates(t *testing.T) {
	host := &focusSyncHostRecorder{}

	syncWindowlessBrowserFocus(host)

	require.Equal(t, []string{"WasHidden", "SetFocus", "Invalidate"}, host.calls)
	require.Equal(t, []int32{0}, host.hiddenValues)
	require.Equal(t, []int32{1}, host.focusValues)
	require.Equal(t, []purecef.PaintElementType{purecef.PaintElementTypePetView}, host.invalidations)
}
