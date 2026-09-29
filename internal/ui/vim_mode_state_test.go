package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestVimModeStateEditableFocus(t *testing.T) {
	var s vimModeState

	assert.False(t, s.editableFocused("pane-a"), "zero value is safe")
	s.setEditableFocused("pane-a", true)
	s.setEditableFocused("", true)
	assert.True(t, s.editableFocused("pane-a"))
	assert.False(t, s.editableFocused(""))

	s.setEditableFocused("pane-a", false)
	assert.False(t, s.editableFocused("pane-a"))
	assert.Empty(t, s.editableFocus, "cleared panes are removed, not stored as false")
}

func TestVimModeStatePulseDebounce(t *testing.T) {
	var s vimModeState
	start := time.Unix(1000, 0)

	assert.True(t, s.pulseIdle())
	assert.True(t, s.allowPulse(start))
	assert.False(t, s.allowPulse(start.Add(vimModePulseInterval/2)), "too soon")
	assert.True(t, s.allowPulse(start.Add(vimModePulseInterval)))

	s.resetPulse()
	assert.True(t, s.pulseIdle())
	assert.True(t, s.allowPulse(start.Add(vimModePulseInterval+time.Millisecond)), "reset never skips re-entry")
}
