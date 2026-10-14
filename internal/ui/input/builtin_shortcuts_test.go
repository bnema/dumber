package input

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func globalShortcutsConfig(actions map[string]entity.ActionBinding) *entity.WorkspaceConfig {
	return &entity.WorkspaceConfig{Shortcuts: entity.GlobalShortcutsConfig{Actions: actions}}
}

func lookupGlobal(set *ShortcutSet, keyval uint, mods Modifier) (Action, bool) {
	return set.Lookup(KeyBinding{Keyval: keyval, Modifiers: mods}, ModeNormal)
}

func TestBuiltinShortcutFallback_AppliesWhenActionIsUnconfigured(t *testing.T) {
	set := NewShortcutSet(context.Background(), globalShortcutsConfig(nil), nil)

	action, ok := lookupGlobal(set, uint(gdk.KEY_f), ModCtrl)
	require.True(t, ok)
	assert.Equal(t, ActionOpenFind, action)
}

func TestBuiltinShortcutFallback_HyphenNameWithNoKeysDisablesDefault(t *testing.T) {
	set := NewShortcutSet(context.Background(), globalShortcutsConfig(map[string]entity.ActionBinding{
		"open-find": {Keys: []string{}},
	}), nil)

	_, ok := lookupGlobal(set, uint(gdk.KEY_f), ModCtrl)
	assert.False(t, ok)
}

func TestBuiltinShortcutFallback_LegacyNameWithNoKeysDisablesDefault(t *testing.T) {
	set := NewShortcutSet(context.Background(), globalShortcutsConfig(map[string]entity.ActionBinding{
		"open_find": {Keys: []string{}},
	}), nil)

	_, ok := lookupGlobal(set, uint(gdk.KEY_f), ModCtrl)
	assert.False(t, ok, "a legacy-named action with no keys disables the built-in default")
}

func TestBuiltinShortcutFallback_LegacyNameRebindReplacesDefault(t *testing.T) {
	set := NewShortcutSet(context.Background(), globalShortcutsConfig(map[string]entity.ActionBinding{
		"open_find": {Keys: []string{"ctrl+shift+f"}},
	}), nil)

	_, ok := lookupGlobal(set, uint(gdk.KEY_f), ModCtrl)
	assert.False(t, ok, "the default ctrl+f is not active next to the rebinding")

	action, ok := lookupGlobal(set, uint(gdk.KEY_f), ModCtrl|ModShift)
	require.True(t, ok, "the legacy-named binding is registered")
	assert.Equal(t, ActionOpenFind, action)
}

func TestBuiltinShortcutFallback_LegacyNameOnlyAffectsItsOwnAction(t *testing.T) {
	set := NewShortcutSet(context.Background(), globalShortcutsConfig(map[string]entity.ActionBinding{
		"open_find": {Keys: []string{}},
	}), nil)

	action, ok := lookupGlobal(set, uint(gdk.KEY_l), ModCtrl)
	require.True(t, ok)
	assert.Equal(t, ActionOpenOmnibox, action)
}

func TestBuiltinShortcutFallback_LegacyMultiWordNames(t *testing.T) {
	set := NewShortcutSet(context.Background(), globalShortcutsConfig(map[string]entity.ActionBinding{
		"toggle_fullscreen": {Keys: []string{}},
		"hard_reload":       {Keys: []string{"ctrl+alt+r"}},
	}), nil)

	_, ok := lookupGlobal(set, uint(gdk.KEY_F11), ModNone)
	assert.False(t, ok, "toggle_fullscreen with no keys disables f11")

	_, ok = lookupGlobal(set, uint(gdk.KEY_R), ModCtrl|ModShift)
	assert.False(t, ok, "the default hard-reload key is replaced")

	action, ok := lookupGlobal(set, uint(gdk.KEY_r), ModCtrl|ModAlt)
	require.True(t, ok)
	assert.Equal(t, ActionHardReload, action)
}
