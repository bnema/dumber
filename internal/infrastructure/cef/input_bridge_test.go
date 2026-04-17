package cef

import (
	"context"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/bnema/puregotk/v4/gdk"
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

func TestClipboardShortcutAction_RecognizesCopyAndCut(t *testing.T) {
	action, ok := clipboardShortcutAction(gdkKeyLowercaseC, uint(gdk.ControlMaskValue))
	require.True(t, ok)
	require.Equal(t, "copy", action)

	action, ok = clipboardShortcutAction(gdkKeyUppercaseX, uint(gdk.ControlMaskValue))
	require.True(t, ok)
	require.Equal(t, "cut", action)
}

func TestClipboardShortcutAction_IgnoresCtrlShiftC(t *testing.T) {
	action, ok := clipboardShortcutAction(gdkKeyUppercaseC, uint(gdk.ControlMaskValue|gdk.ShiftMaskValue))
	require.False(t, ok)
	require.Empty(t, action)
}

func TestMaybeMirrorClipboardShortcut_DelegatesExplicitCopyAction(t *testing.T) {
	var gotAction string
	var gotText string
	ib := &inputBridge{
		ctx:           context.Background(),
		selectionText: func() string { return "selected text" },
		explicitCopyText: func(action, text string) {
			gotAction = action
			gotText = text
		},
	}

	ib.maybeMirrorClipboardShortcut(gdkKeyLowercaseC, uint(gdk.ControlMaskValue))

	require.Equal(t, "copy", gotAction)
	require.Equal(t, "selected text", gotText)
}

func TestMaybeMirrorClipboardShortcut_SkipsWhenSelectionEmpty(t *testing.T) {
	called := false
	ib := &inputBridge{
		ctx:           context.Background(),
		selectionText: func() string { return "" },
		explicitCopyText: func(string, string) {
			called = true
		},
	}

	ib.maybeMirrorClipboardShortcut(gdkKeyLowercaseC, uint(gdk.ControlMaskValue))

	require.False(t, called)
}
