package config

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestKeybindingsManager creates a Manager with a temp config dir.
func newTestKeybindingsManager(t *testing.T) *Manager {
	t.Helper()
	configHome := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", filepath.Join(configHome, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(configHome, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(configHome, "cache"))

	mgr, err := NewManager()
	require.NoError(t, err)
	require.NoError(t, mgr.Load())
	return mgr
}

// inMemoryManager creates a lightweight Manager without file I/O for tests
// that only query config data without saving.
func inMemoryManager() *Manager {
	return &Manager{
		config: DefaultConfig(),
		mu:     sync.RWMutex{},
	}
}

func TestKeybindingsGateway_VimModeIncludedInGetKeybindings(t *testing.T) {
	gw := NewKeybindingsGateway(inMemoryManager())
	cfg, err := gw.GetKeybindings(context.Background())
	require.NoError(t, err)

	var pageGroup *port.KeybindingGroup
	for i, g := range cfg.Groups {
		if g.Mode == "vim" {
			pageGroup = &cfg.Groups[i]
			break
		}
	}
	require.NotNil(t, pageGroup, "vim mode group must be present in GetKeybindings output")
	assert.Equal(t, "Vim Mode", pageGroup.DisplayName)
	assert.Equal(t, "ctrl+y", pageGroup.Activation)

	// Verify all expected actions are present.
	expectedActions := map[string]struct{}{
		"vim-scroll-left":      {},
		"vim-scroll-down":      {},
		"vim-scroll-up":        {},
		"vim-scroll-right":     {},
		"vim-scroll-down-fast": {},
		"vim-scroll-up-fast":   {},
		"confirm":              {},
		"cancel":               {},
	}

	for _, entry := range pageGroup.Bindings {
		delete(expectedActions, entry.Action)
		assert.NotEmpty(t, entry.Keys, "action %q has empty keys", entry.Action)
		assert.False(t, entry.IsCustom, "default vim mode config should not have IsCustom=true")
	}
	assert.Empty(t, expectedActions, "vim mode group missing actions: %v", expectedActions)

	// Must be placed after global, pane, tab groups (consistent ordering).
	groups := cfg.Groups
	foundIdx := -1
	for i, g := range groups {
		if g.Mode == "vim" {
			foundIdx = i
			break
		}
	}
	assert.Greater(t, foundIdx, 2, "vim mode group should be after pane/tab groups")
}

func TestKeybindingsGateway_VimModeIncludedInGetDefaultKeybindings(t *testing.T) {
	gw := NewKeybindingsGateway(inMemoryManager())
	cfg, err := gw.GetDefaultKeybindings(context.Background())
	require.NoError(t, err)

	var pageGroup *port.KeybindingGroup
	for i, g := range cfg.Groups {
		if g.Mode == "vim" {
			pageGroup = &cfg.Groups[i]
			break
		}
	}
	require.NotNil(t, pageGroup, "vim mode group must be present in GetDefaultKeybindings output")
	assert.Equal(t, "Vim Mode", pageGroup.DisplayName)
	assert.Equal(t, "ctrl+y", pageGroup.Activation)

	// Verify default keys match actual keys.
	for _, entry := range pageGroup.Bindings {
		assert.Equal(t, entry.DefaultKeys, entry.Keys,
			"default vim mode action %q should have keys == default_keys", entry.Action)
	}
}

func TestKeybindingsGateway_VimModeSetKeybindingUpdatesFile(t *testing.T) {
	mgr := newTestKeybindingsManager(t)
	gw := NewKeybindingsGateway(mgr)

	err := gw.SetKeybinding(context.Background(), port.SetKeybindingRequest{
		Mode:   "vim",
		Action: "vim-scroll-left",
		Keys:   []string{"a"},
	})
	require.NoError(t, err)

	// Reload and assert on the parsed Vim-mode configuration rather than raw TOML text.
	reloaded, reloadErr := NewManager()
	require.NoError(t, reloadErr)
	require.NoError(t, reloaded.Load())
	assert.Equal(t, []string{"a"}, reloaded.Get().Workspace.VimMode.Actions["vim-scroll-left"].Keys)
}

func TestKeybindingsGateway_VimModeResetKeybinding(t *testing.T) {
	mgr := newTestKeybindingsManager(t)
	gw := NewKeybindingsGateway(mgr)

	// Set a keybinding.
	require.NoError(t, gw.SetKeybinding(context.Background(), port.SetKeybindingRequest{
		Mode:   "vim",
		Action: "vim-scroll-left",
		Keys:   []string{"a"},
	}))

	// Reset it.
	err := gw.ResetKeybinding(context.Background(), port.ResetKeybindingRequest{
		Mode:   "vim",
		Action: "vim-scroll-left",
	})
	require.NoError(t, err)

	// Reload and assert on the parsed default binding.
	reloaded, reloadErr := NewManager()
	require.NoError(t, reloadErr)
	require.NoError(t, reloaded.Load())
	assert.Equal(t, []string{"h"}, reloaded.Get().Workspace.VimMode.Actions["vim-scroll-left"].Keys)
}

func TestKeybindingsGateway_VimModeIsPartOfResetAll(t *testing.T) {
	mgr := newTestKeybindingsManager(t)
	gw := NewKeybindingsGateway(mgr)

	// Change a vim mode binding.
	require.NoError(t, gw.SetKeybinding(context.Background(), port.SetKeybindingRequest{
		Mode:   "vim",
		Action: "vim-scroll-left",
		Keys:   []string{"a"},
	}))

	// Change another action.
	require.NoError(t, gw.SetKeybinding(context.Background(), port.SetKeybindingRequest{
		Mode:   "vim",
		Action: "vim-scroll-down",
		Keys:   []string{"b"},
	}))

	// Reset all.
	err := gw.ResetAllKeybindings(context.Background())
	require.NoError(t, err)

	// Reload and assert on the parsed default bindings.
	reloaded, reloadErr := NewManager()
	require.NoError(t, reloadErr)
	require.NoError(t, reloaded.Load())
	assert.Equal(t, []string{"h"}, reloaded.Get().Workspace.VimMode.Actions["vim-scroll-left"].Keys, "vim-scroll-left should be restored to default")
	assert.Equal(t, []string{"j"}, reloaded.Get().Workspace.VimMode.Actions["vim-scroll-down"].Keys, "vim-scroll-down should be restored to default")
}

func TestKeybindingsGateway_VimModeConflictsWithinSelf(t *testing.T) {
	gw := NewKeybindingsGateway(inMemoryManager())
	conflicts, err := gw.CheckConflicts(context.Background(), "vim", "custom", []string{"h"})
	require.NoError(t, err)
	require.NotEmpty(t, conflicts, "should detect conflict with vim-scroll-left which uses 'h'")

	// The conflict should reference the same mode.
	assert.Equal(t, "vim-scroll-left", conflicts[0].ConflictingAction)
	assert.Equal(t, "vim", conflicts[0].ConflictingMode)
}

func TestKeybindingsGateway_VimModeConflictsWithOtherModes(t *testing.T) {
	gw := NewKeybindingsGateway(inMemoryManager())
	// "enter" is used by vim mode's confirm - but also by other modes like pane_mode.
	conflicts, err := gw.CheckConflicts(context.Background(), "vim", "custom", []string{"enter"})
	require.NoError(t, err)

	// There should be conflicts since "enter" is used in other modes too.
	assert.NotEmpty(t, conflicts, "should detect cross-mode conflict for 'enter'")
}

func TestKeybindingsGateway_VimModeNoConflictForUnusedKey(t *testing.T) {
	gw := NewKeybindingsGateway(inMemoryManager())
	conflicts, err := gw.CheckConflicts(context.Background(), "vim", "custom", []string{"ctrl+9"})
	require.NoError(t, err)
	assert.Empty(t, conflicts, "no conflict expected for unused key")
}
