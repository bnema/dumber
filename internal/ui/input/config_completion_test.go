package input

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadShortcutsFromTOML loads a config file through the real config loader
// (legacy transforms, default completion, validation) and builds the shortcut
// set the application would build from it.
func loadShortcutsFromTOML(t *testing.T, content string) *ShortcutSet {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	configFile, err := config.GetConfigFile()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(configFile), 0o755))
	require.NoError(t, os.WriteFile(configFile, []byte(content), 0o644))

	mgr, err := config.NewManager()
	require.NoError(t, err)
	require.NoError(t, mgr.Load())

	cfg := mgr.Get()
	return NewShortcutSet(context.Background(), &cfg.Workspace, &cfg.Session)
}

func lookupGlobal(set *ShortcutSet, keyval uint, mods Modifier) (Action, bool) {
	return set.Lookup(KeyBinding{Keyval: keyval, Modifiers: mods}, ModeNormal)
}

func TestConfigCompletion_UserKeyConflictIsNotResurrectedByInput(t *testing.T) {
	// ctrl+r is a default key of reload, so completion leaves reload out. The
	// input layer must not bind reload's other default key (f5) on its own.
	set := loadShortcutsFromTOML(t, `
[workspace.shortcuts.actions.new-tab]
keys = ["ctrl+r"]
`)

	action, ok := lookupGlobal(set, uint(gdk.KEY_r), ModCtrl)
	require.True(t, ok)
	assert.Equal(t, ActionNewTab, action)

	_, ok = lookupGlobal(set, uint(gdk.KEY_F5), ModNone)
	assert.False(t, ok, "f5 belongs to the skipped reload default and must stay unbound")

	action, ok = lookupGlobal(set, uint(gdk.KEY_l), ModCtrl)
	require.True(t, ok, "unrelated defaults are still completed")
	assert.Equal(t, ActionOpenOmnibox, action)
}

func TestConfigCompletion_DisabledBuiltinStaysDisabled(t *testing.T) {
	set := loadShortcutsFromTOML(t, `
[workspace.shortcuts.actions.open-find]
keys = []
`)

	_, ok := lookupGlobal(set, uint(gdk.KEY_f), ModCtrl)
	assert.False(t, ok)

	action, ok := lookupGlobal(set, uint(gdk.KEY_l), ModCtrl)
	require.True(t, ok)
	assert.Equal(t, ActionOpenOmnibox, action)
}

func TestConfigCompletion_AliasSpellingBlocksDefaultAndInputAgrees(t *testing.T) {
	set := loadShortcutsFromTOML(t, `
[workspace.pane_mode.actions.close-pane]
keys = ["right"]
`)

	action, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_Right)}, ModePane)
	require.True(t, ok)
	assert.Equal(t, ActionClosePane, action, "the user binding wins deterministically over split-right's arrowright")

	action, ok = set.Lookup(KeyBinding{Keyval: uint('r')}, ModePane)
	assert.False(t, ok, "split-right is skipped entirely, including its other key %v", action)
}

func TestConfigCompletion_UnderscoreNamesWorkEndToEnd(t *testing.T) {
	set := loadShortcutsFromTOML(t, `
[workspace.pane_mode.actions.stack_pane]
keys = ["ctrl+alt+f12"]

[workspace.pane_mode.actions.focus_left]
keys = ["ctrl+alt+f11"]

[workspace.resize_mode.actions.resize_increase_left]
keys = ["z"]

[workspace.prefix_mode.actions.enter_resize_mode]
keys = ["ctrl+alt+f10"]

[workspace.tab_mode.actions.new_tab]
keys = ["ctrl+alt+f9"]

[workspace.shortcuts.actions.toggle_floating_pane]
keys = ["alt+g"]

[workspace.shortcuts.actions.open_find]
keys = ["ctrl+shift+f"]
`)

	paneBinding := func(keyval uint) KeyBinding {
		return KeyBinding{Keyval: keyval, Modifiers: ModCtrl | ModAlt}
	}

	tests := []struct {
		name    string
		binding KeyBinding
		mode    Mode
		want    Action
	}{
		{"pane stack_pane", paneBinding(uint(gdk.KEY_F12)), ModePane, ActionStackPane},
		{"pane focus_left", paneBinding(uint(gdk.KEY_F11)), ModePane, ActionFocusLeft},
		{"resize resize_increase_left", KeyBinding{Keyval: uint('z')}, ModeResize, ActionResizeIncreaseLeft},
		{"prefix enter_resize_mode", paneBinding(uint(gdk.KEY_F10)), ModePrefix, ActionEnterResizeMode},
		{"tab new_tab", paneBinding(uint(gdk.KEY_F9)), ModeTab, ActionNewTab},
		{"global toggle_floating_pane", KeyBinding{Keyval: uint('g'), Modifiers: ModAlt}, ModeNormal, ActionToggleFloatingPane},
		{"global open_find", KeyBinding{Keyval: uint('f'), Modifiers: ModCtrl | ModShift}, ModeNormal, ActionOpenFind},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := set.Lookup(tt.binding, tt.mode)
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}

	// The underscore-named user bindings replaced the defaults they renamed.
	_, ok := lookupGlobal(set, uint(gdk.KEY_f), ModCtrl)
	assert.False(t, ok, "default ctrl+f is replaced by the user's open_find binding")
	_, ok = set.Lookup(KeyBinding{Keyval: uint('s')}, ModePane)
	assert.False(t, ok, "default stack-pane key is replaced by the user's stack_pane binding")
	_, ok = set.Lookup(KeyBinding{Keyval: uint('h')}, ModeResize)
	assert.False(t, ok, "default resize-increase-left key is replaced")
}
