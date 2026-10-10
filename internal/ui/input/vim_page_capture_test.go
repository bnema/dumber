package input

import (
	"context"
	"testing"

	"github.com/bnema/puregotk/v4/gdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/domain/entity"
)

func TestPageKeyCaptureForwardsVimModeKeys(t *testing.T) {
	ws := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"vim-scroll-down": {Keys: []string{"j"}},
		"yank-section":    {Keys: []string{"yah"}},
	})
	h := NewKeyboardHandler(context.Background(), ws, newTestSession())
	actions := captureSequenceActions(h)
	enterVimMode(t, h)

	var forwarded []string
	h.SetPageKeyCapture(func(key string) { forwarded = append(forwarded, key) })

	require.True(t, h.handleKeyPress(uint('y'), 0, 0))
	require.True(t, h.handleKeyPress(uint('J'), 0, gdk.ShiftMaskValue))
	require.True(t, h.handleKeyPress(uint(gdk.KEY_Shift_L), 0, 0))
	require.True(t, h.handleKeyPress(uint(gdk.KEY_Return), 0, 0))

	assert.Equal(t, []string{"y", "J", "<Return>"}, forwarded)
	assert.Empty(t, *actions, "captured keys must not reach Vim bindings")
	assert.Equal(t, ModeVim, h.Mode(), "Enter is forwarded instead of confirming Vim Mode")
	assert.Empty(t, h.PendingSequence())
}

func TestPageKeyCaptureEscapeIsForwardedAndKeepsCaptureForPageToEnd(t *testing.T) {
	h := NewKeyboardHandler(context.Background(), vimModeSequenceWorkspace(nil), newTestSession())
	enterVimMode(t, h)
	var forwarded []string
	h.SetPageKeyCapture(func(key string) { forwarded = append(forwarded, key) })

	require.True(t, h.handleKeyPress(uint(gdk.KEY_Escape), 0, 0))

	assert.Equal(t, []string{"<Escape>"}, forwarded)
	assert.True(t, h.PageKeyCaptureActive(), "the page steps back or reports the end itself")
	assert.Equal(t, ModeVim, h.Mode())

	h.ClearPageKeyCapture()
	assert.False(t, h.PageKeyCaptureActive())
}

func TestPageKeyCaptureClearedWhenLeavingVimMode(t *testing.T) {
	h := NewKeyboardHandler(context.Background(), vimModeSequenceWorkspace(nil), newTestSession())
	enterVimMode(t, h)
	h.SetPageKeyCapture(func(string) {})

	h.ExitMode()

	assert.False(t, h.PageKeyCaptureActive())
}

func TestPageKeyCaptureModifiedEscapeForwardsPlainEscape(t *testing.T) {
	h := NewKeyboardHandler(context.Background(), vimModeSequenceWorkspace(nil), newTestSession())
	enterVimMode(t, h)
	var forwarded []string
	h.SetPageKeyCapture(func(key string) { forwarded = append(forwarded, key) })

	require.True(t, h.handleKeyPress(uint(gdk.KEY_Escape), 0, gdk.ShiftMaskValue))

	assert.Equal(t, []string{"<Escape>"}, forwarded)
}

// Capturing keys for the page must not report a cleared pending sequence:
// the pending callback repaints the mode toast and would erase the sub-mode
// label shown an instant earlier.
func TestSetPageKeyCaptureDoesNotNotifyPendingListeners(t *testing.T) {
	h := NewKeyboardHandler(context.Background(), vimModeSequenceWorkspace(nil), newTestSession())
	enterVimMode(t, h)
	pending := capturePending(h)

	h.SetPageKeyCapture(func(string) {})

	assert.Empty(t, *pending)
	assert.Empty(t, h.PendingSequence())
}

// The Vim Mode toggle shortcut is the escape hatch from a page interaction:
// it must reach Go, leave Vim Mode and release the capture, never the page.
func TestPageKeyCaptureLeavesVimModeToggleShortcutToGo(t *testing.T) {
	h := NewKeyboardHandler(context.Background(), vimModeSequenceWorkspace(nil), newTestSession())
	enterVimMode(t, h)
	var forwarded []string
	h.SetPageKeyCapture(func(key string) { forwarded = append(forwarded, key) })
	h.handleKeyRelease(uint('y')) // the activation press is over; this is a new press

	require.True(t, h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue))

	assert.Empty(t, forwarded, "the toggle shortcut must not be forwarded to the page")
	assert.Equal(t, ModeNormal, h.Mode())
	assert.False(t, h.PageKeyCaptureActive(), "leaving Vim Mode releases the capture")
}

func TestPageKeyCaptureHonorsConfiguredToggleShortcut(t *testing.T) {
	ws := vimModeSequenceWorkspace(nil)
	ws.VimMode.ActivationShortcut = "ctrl+g"
	h := NewKeyboardHandler(context.Background(), ws, newTestSession())
	h.EnterVimMode()
	var forwarded []string
	h.SetPageKeyCapture(func(key string) { forwarded = append(forwarded, key) })

	require.True(t, h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue), "ctrl+y is an ordinary key now")
	assert.Equal(t, []string{"<C-y>"}, forwarded)
	require.True(t, h.handleKeyPress(uint('g'), 0, gdk.ControlMaskValue))

	assert.Equal(t, []string{"<C-y>"}, forwarded)
	assert.Equal(t, ModeNormal, h.Mode())
}
