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

	_, ok = clipboardShortcutAction(gdkKeyLowercaseC, uint(gdk.ControlMaskValue|gdk.ShiftMaskValue))
	require.False(t, ok)
}

func TestInputBridgeMaybeMirrorClipboardShortcut_RoutesSelectionThroughExplicitHandler(t *testing.T) {
	ib := newInputBridge(context.Background(), 1)
	ib.selectionText = func() string { return "selected text" }
	routed := struct {
		action string
		text   string
	}{}
	ib.explicitCopyText = func(action, text string) {
		routed.action = action
		routed.text = text
	}

	ib.maybeMirrorClipboardShortcut(gdkKeyLowercaseC, uint(gdk.ControlMaskValue))

	require.Equal(t, "copy", routed.action)
	require.Equal(t, "selected text", routed.text)
}

func TestInputBridgeMaybeMirrorClipboardShortcut_IgnoresEmptySelection(t *testing.T) {
	ib := newInputBridge(context.Background(), 1)
	ib.selectionText = func() string { return "" }
	called := false
	ib.explicitCopyText = func(string, string) { called = true }

	ib.maybeMirrorClipboardShortcut(gdkKeyLowercaseX, uint(gdk.ControlMaskValue))

	require.False(t, called)
}
