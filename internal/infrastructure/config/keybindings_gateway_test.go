package config

import (
	"context"
	"os"
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

func TestKeybindingsGateway_PageModeIncludedInGetKeybindings(t *testing.T) {
	gw := NewKeybindingsGateway(inMemoryManager())
	cfg, err := gw.GetKeybindings(context.Background())
	require.NoError(t, err)

	var pageGroup *port.KeybindingGroup
	for i, g := range cfg.Groups {
		if g.Mode == "page" {
			pageGroup = &cfg.Groups[i]
			break
		}
	}
	require.NotNil(t, pageGroup, "page mode group must be present in GetKeybindings output")
	assert.Equal(t, "Page Mode", pageGroup.DisplayName)
	assert.Equal(t, "ctrl+y", pageGroup.Activation)

	// Verify all expected actions are present.
	expectedActions := map[string]struct{}{
		"page-scroll-left":      {},
		"page-scroll-down":      {},
		"page-scroll-up":        {},
		"page-scroll-right":     {},
		"page-scroll-down-fast": {},
		"page-scroll-up-fast":   {},
		"confirm":               {},
		"cancel":                {},
	}

	for _, entry := range pageGroup.Bindings {
		delete(expectedActions, entry.Action)
		assert.NotEmpty(t, entry.Keys, "action %q has empty keys", entry.Action)
		assert.False(t, entry.IsCustom, "default page mode config should not have IsCustom=true")
	}
	assert.Empty(t, expectedActions, "page mode group missing actions: %v", expectedActions)

	// Must be placed after global, pane, tab groups (consistent ordering).
	groups := cfg.Groups
	foundIdx := -1
	for i, g := range groups {
		if g.Mode == "page" {
			foundIdx = i
			break
		}
	}
	assert.Greater(t, foundIdx, 2, "page mode group should be after pane/tab groups")
}

func TestKeybindingsGateway_PageModeIncludedInGetDefaultKeybindings(t *testing.T) {
	gw := NewKeybindingsGateway(inMemoryManager())
	cfg, err := gw.GetDefaultKeybindings(context.Background())
	require.NoError(t, err)

	var pageGroup *port.KeybindingGroup
	for i, g := range cfg.Groups {
		if g.Mode == "page" {
			pageGroup = &cfg.Groups[i]
			break
		}
	}
	require.NotNil(t, pageGroup, "page mode group must be present in GetDefaultKeybindings output")
	assert.Equal(t, "Page Mode", pageGroup.DisplayName)
	assert.Equal(t, "ctrl+y", pageGroup.Activation)

	// Verify default keys match actual keys.
	for _, entry := range pageGroup.Bindings {
		assert.Equal(t, entry.DefaultKeys, entry.Keys,
			"default page mode action %q should have keys == default_keys", entry.Action)
	}
}

func TestKeybindingsGateway_PageModeSetKeybindingUpdatesFile(t *testing.T) {
	mgr := newTestKeybindingsManager(t)
	gw := NewKeybindingsGateway(mgr)

	err := gw.SetKeybinding(context.Background(), port.SetKeybindingRequest{
		Mode:   "page",
		Action: "page-scroll-left",
		Keys:   []string{"a"},
	})
	require.NoError(t, err)

	// Verify the file was written with the updated keybinding.
	configFile := mgr.GetConfigFile()
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, `keys = ['a']`)
	assert.NotContains(t, content, `keys = ['h']`)
}

func TestKeybindingsGateway_PageModeResetKeybinding(t *testing.T) {
	mgr := newTestKeybindingsManager(t)
	gw := NewKeybindingsGateway(mgr)

	// Set a keybinding.
	require.NoError(t, gw.SetKeybinding(context.Background(), port.SetKeybindingRequest{
		Mode:   "page",
		Action: "page-scroll-left",
		Keys:   []string{"a"},
	}))

	// Reset it.
	err := gw.ResetKeybinding(context.Background(), port.ResetKeybindingRequest{
		Mode:   "page",
		Action: "page-scroll-left",
	})
	require.NoError(t, err)

	// Verify the file was updated back to default.
	configFile := mgr.GetConfigFile()
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, `keys = ['h']`)
	assert.NotContains(t, content, `keys = ['a']`)
}

func TestKeybindingsGateway_PageModeIsPartOfResetAll(t *testing.T) {
	mgr := newTestKeybindingsManager(t)
	gw := NewKeybindingsGateway(mgr)

	// Change a page mode binding.
	require.NoError(t, gw.SetKeybinding(context.Background(), port.SetKeybindingRequest{
		Mode:   "page",
		Action: "page-scroll-left",
		Keys:   []string{"a"},
	}))

	// Change another action.
	require.NoError(t, gw.SetKeybinding(context.Background(), port.SetKeybindingRequest{
		Mode:   "page",
		Action: "page-scroll-down",
		Keys:   []string{"b"},
	}))

	// Reset all.
	err := gw.ResetAllKeybindings(context.Background())
	require.NoError(t, err)

	// Verify file has defaults.
	configFile := mgr.GetConfigFile()
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, `keys = ['h']`, "page-scroll-left should be restored to default")
	assert.Contains(t, content, `keys = ['j']`, "page-scroll-down should be restored to default")
	assert.NotContains(t, content, "keys = ['a']")
	assert.NotContains(t, content, "keys = ['b']")
}

func TestKeybindingsGateway_PageModeConflictsWithinSelf(t *testing.T) {
	gw := NewKeybindingsGateway(inMemoryManager())
	conflicts, err := gw.CheckConflicts(context.Background(), "page", "custom", []string{"h"})
	require.NoError(t, err)
	require.NotEmpty(t, conflicts, "should detect conflict with page-scroll-left which uses 'h'")

	// The conflict should reference the same mode.
	assert.Equal(t, "page-scroll-left", conflicts[0].ConflictingAction)
	assert.Equal(t, "page", conflicts[0].ConflictingMode)
}

func TestKeybindingsGateway_PageModeConflictsWithOtherModes(t *testing.T) {
	gw := NewKeybindingsGateway(inMemoryManager())
	// "enter" is used by page mode's confirm - but also by other modes like pane_mode.
	conflicts, err := gw.CheckConflicts(context.Background(), "page", "custom", []string{"enter"})
	require.NoError(t, err)

	// There should be conflicts since "enter" is used in other modes too.
	assert.NotEmpty(t, conflicts, "should detect cross-mode conflict for 'enter'")
}

func TestKeybindingsGateway_PageModeNoConflictForUnusedKey(t *testing.T) {
	gw := NewKeybindingsGateway(inMemoryManager())
	conflicts, err := gw.CheckConflicts(context.Background(), "page", "custom", []string{"ctrl+9"})
	require.NoError(t, err)
	assert.Empty(t, conflicts, "no conflict expected for unused key")
}
