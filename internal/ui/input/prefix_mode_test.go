package input

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTmuxWorkspace() *entity.WorkspaceConfig {
	ws := newTestWorkspace()
	ws.Keymap = entity.KeymapTmux
	ws.VimMode.ActivationShortcut = "ctrl+y"
	ws.PrefixMode = entity.PrefixModeConfig{
		ActivationShortcut: "ctrl+space",
		Actions: map[string]entity.ActionBinding{
			"split-right":       {Keys: []string{"%"}},
			"split-down":        {Keys: []string{"\""}},
			"new-tab":           {Keys: []string{"c"}},
			"next-tab":          {Keys: []string{"n"}},
			"enter-resize-mode": {Keys: []string{"r"}},
			"cancel":            {Keys: []string{"escape"}},
		},
	}
	return ws
}

func recordActions(h *KeyboardHandler) *[]Action {
	var got []Action
	h.SetOnAction(func(_ context.Context, action Action) error {
		got = append(got, action)
		return nil
	})
	return &got
}

func TestShortcutSet_TmuxKeymapReplacesModeActivations(t *testing.T) {
	set := NewShortcutSet(context.Background(), newTmuxWorkspace(), newTestSession())

	prefix, _ := ParseKeyString("ctrl+space")
	assert.Equal(t, ActionEnterPrefixMode, set.Global[prefix])
	for _, key := range []string{"ctrl+t", "ctrl+p", "ctrl+alt+r", "ctrl+s"} {
		binding, ok := ParseKeyString(key)
		require.True(t, ok)
		_, bound := set.Global[binding]
		assert.Falsef(t, bound, "%s must not activate a mode in the tmux keymap", key)
	}
	vim, _ := ParseKeyString("ctrl+y")
	assert.Equal(t, ActionEnterVimMode, set.Global[vim], "vim mode keeps its own activation")
}

func TestShortcutSet_ZellijKeymapIgnoresPrefix(t *testing.T) {
	ws := newTmuxWorkspace()
	ws.Keymap = entity.KeymapZellij
	set := NewShortcutSet(context.Background(), ws, newTestSession())

	prefix, _ := ParseKeyString("ctrl+space")
	_, bound := set.Global[prefix]
	assert.False(t, bound)
	tab, _ := ParseKeyString("ctrl+t")
	assert.Equal(t, ActionEnterTabMode, set.Global[tab])
}

func TestHandleKeyPress_PrefixDispatchesOneActionThenExits(t *testing.T) {
	h := NewKeyboardHandler(context.Background(), newTmuxWorkspace(), newTestSession())
	got := recordActions(h)

	require.True(t, h.handleKeyPress(uint(gdk.KEY_space), 0, gdk.ControlMaskValue))
	require.Equal(t, ModePrefix, h.Mode())

	// '%' is typed with Shift on most layouts; the binding omits Shift.
	require.True(t, h.handleKeyPress(uint(gdk.KEY_percent), 0, gdk.ShiftMaskValue))
	assert.Equal(t, []Action{ActionSplitRight}, *got)
	assert.Equal(t, ModeNormal, h.Mode(), "prefix is one-shot")

	// A plain key after the prefix exited must not dispatch again.
	assert.False(t, h.handleKeyPress(uint('n'), 0, 0))
	assert.Len(t, *got, 1)
}

func TestHandleKeyPress_PrefixUnboundKeyCancels(t *testing.T) {
	h := NewKeyboardHandler(context.Background(), newTmuxWorkspace(), newTestSession())
	got := recordActions(h)

	h.handleKeyPress(uint(gdk.KEY_space), 0, gdk.ControlMaskValue)
	assert.True(t, h.handleKeyPress(uint('z'), 0, 0), "unbound key is consumed")
	assert.Equal(t, ModeNormal, h.Mode())
	assert.Empty(t, *got)
}

func TestHandleKeyPress_PrefixCanEnterPersistentMode(t *testing.T) {
	h := NewKeyboardHandler(context.Background(), newTmuxWorkspace(), newTestSession())

	h.handleKeyPress(uint(gdk.KEY_space), 0, gdk.ControlMaskValue)
	h.handleKeyPress(uint('r'), 0, 0)
	assert.Equal(t, ModeResize, h.Mode(), "prefix action may switch into another mode")
}

func TestHandleKeyPress_PrefixPressedTwiceExits(t *testing.T) {
	h := NewKeyboardHandler(context.Background(), newTmuxWorkspace(), newTestSession())

	h.handleKeyPress(uint(gdk.KEY_space), 0, gdk.ControlMaskValue)
	h.handleKeyRelease(uint(gdk.KEY_space))
	h.handleKeyPress(uint(gdk.KEY_space), 0, gdk.ControlMaskValue)
	assert.Equal(t, ModeNormal, h.Mode())
}

func TestShortcutSet_BuiltinFallbacksAndOverrides(t *testing.T) {
	ws := newTestWorkspace()
	ws.Shortcuts.Actions = map[string]entity.ActionBinding{
		"reload": {Keys: []string{}},               // disabled by user
		"quit":   {Keys: []string{"ctrl+shift+q"}}, // rebound by user
	}
	set := NewShortcutSet(context.Background(), ws, newTestSession())

	lookup := func(key string) (Action, bool) {
		binding, ok := ParseKeyString(key)
		require.True(t, ok, key)
		action, found := set.Global[binding]
		return action, found
	}

	action, ok := lookup("ctrl+l")
	assert.True(t, ok, "missing builtin action falls back to its default key")
	assert.Equal(t, ActionOpenOmnibox, action)

	_, ok = lookup("ctrl+r")
	assert.False(t, ok, "empty keys disable a builtin shortcut")

	_, ok = lookup("ctrl+q")
	assert.False(t, ok, "rebound builtin drops its default key")
	action, _ = lookup("ctrl+shift+q")
	assert.Equal(t, ActionQuit, action)
}

func TestParseKeyString_PrintableSymbols(t *testing.T) {
	for _, key := range []string{"%", "\"", "&", ",", "!"} {
		binding, ok := ParseKeyString(key)
		require.Truef(t, ok, "%q should parse", key)
		assert.Equal(t, uint(key[0]), binding.Keyval)
		assert.Equal(t, ModNone, binding.Modifiers)
	}
}

type keyEvent struct {
	keyval uint
	state  gdk.ModifierType
}

// TestHandleKeyPress_PrefixRealKeySequences replays key events as GTK delivers
// them: modifier and dead-key presses arrive before the character.
func TestHandleKeyPress_PrefixRealKeySequences(t *testing.T) {
	shift := gdk.ShiftMaskValue
	tests := []struct {
		name       string
		events     []keyEvent
		wantAction []Action
		wantMode   Mode
		lastResult bool
	}{
		{
			name:       "shift then percent splits right",
			events:     []keyEvent{{uint(gdk.KEY_Shift_L), 0}, {uint(gdk.KEY_percent), shift}},
			wantAction: []Action{ActionSplitRight},
			wantMode:   ModeNormal,
			lastResult: true,
		},
		{
			name:       "shift then quote splits down",
			events:     []keyEvent{{uint(gdk.KEY_Shift_R), 0}, {uint(gdk.KEY_quotedbl), shift}},
			wantAction: []Action{ActionSplitDown},
			wantMode:   ModeNormal,
			lastResult: true,
		},
		{
			name:       "dead diaeresis splits down on international layouts",
			events:     []keyEvent{{uint(gdk.KEY_dead_diaeresis), 0}},
			wantAction: []Action{ActionSplitDown},
			wantMode:   ModeNormal,
			lastResult: true,
		},
		{
			name:       "altgr alone keeps the prefix waiting",
			events:     []keyEvent{{uint(gdk.KEY_ISO_Level3_Shift), 0}},
			wantMode:   ModePrefix,
			lastResult: true,
		},
		{
			name:       "unbound modified key cancels and passes through",
			events:     []keyEvent{{uint(gdk.KEY_Alt_L), 0}, {uint('1'), gdk.AltMaskValue}},
			wantMode:   ModeNormal,
			lastResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewKeyboardHandler(context.Background(), newTmuxWorkspace(), newTestSession())
			got := recordActions(h)
			require.True(t, h.handleKeyPress(uint(gdk.KEY_space), 0, gdk.ControlMaskValue))
			h.handleKeyRelease(uint(gdk.KEY_space))

			var result bool
			for _, ev := range tt.events {
				result = h.handleKeyPress(ev.keyval, 0, ev.state)
			}
			assert.Equal(t, tt.lastResult, result)
			assert.Equal(t, tt.wantMode, h.Mode())
			assert.Equal(t, tt.wantAction, *got)
		})
	}
}

func TestShortcutSet_BuiltinWinsSharedGlobalKey(t *testing.T) {
	ws := newTestWorkspace()
	ws.Shortcuts.Actions = map[string]entity.ActionBinding{
		"toggle-floating-pane": {Keys: []string{"ctrl+l"}},
		"open-omnibox":         {Keys: []string{"ctrl+l"}},
	}
	for range 20 {
		set := NewShortcutSet(context.Background(), ws, newTestSession())
		binding, _ := ParseKeyString("ctrl+l")
		require.Equal(t, ActionOpenOmnibox, set.Global[binding])
	}
}
