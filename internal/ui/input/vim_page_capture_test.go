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

func TestPageKeyCaptureEscapeReleasesCaptureAndStaysInVimMode(t *testing.T) {
	h := NewKeyboardHandler(context.Background(), vimModeSequenceWorkspace(nil), newTestSession())
	enterVimMode(t, h)
	var forwarded []string
	h.SetPageKeyCapture(func(key string) { forwarded = append(forwarded, key) })

	require.True(t, h.handleKeyPress(uint(gdk.KEY_Escape), 0, 0))

	assert.Equal(t, []string{"<Escape>"}, forwarded)
	assert.False(t, h.PageKeyCaptureActive())
	assert.Equal(t, ModeVim, h.Mode())
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
	assert.False(t, h.PageKeyCaptureActive())
}
