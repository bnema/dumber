package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerLoad_UpgradesObsoleteCEFPreciseScrollDefaultInMemory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	configFile, err := GetConfigFile()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(configFile), 0o755))
	require.NoError(t, os.WriteFile(configFile, []byte(`
[engine.cef.input]
scroll_precise_multiplier = 0.35
touchpad_navigation_max_vertical_ratio = 2.0
`), 0o644))

	mgr, err := NewManager()
	require.NoError(t, err)
	require.NoError(t, mgr.Load())

	cfg := mgr.Get()
	require.NotNil(t, cfg)
	assert.InDelta(t, defaultCEFScrollPreciseMultiplier, cfg.Engine.CEF.Input.ScrollPreciseMultiplier, 0.001)
	assert.InDelta(t, 2.0, cfg.Engine.CEF.Input.TouchpadNavigationMaxVerticalRatio, 0.001)
}

func TestManagerLoad_DoesNotMergeMissingPaneModeActionsInMemory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	configFile, err := GetConfigFile()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(configFile), 0o755))
	require.NoError(t, os.WriteFile(configFile, []byte(`
[workspace.pane_mode]
activation_shortcut = "ctrl+p"
timeout_ms = 3000

[workspace.pane_mode.actions.stack-pane]
keys = ["s"]
desc = "Custom stack pane"
`), 0o644))

	mgr, err := NewManager()
	require.NoError(t, err)
	require.NoError(t, mgr.Load())

	cfg := mgr.Get()
	require.NotNil(t, cfg)

	_, ok := cfg.Workspace.PaneMode.Actions["eject-pane-to-window"]
	assert.False(t, ok)

	stack := cfg.Workspace.PaneMode.Actions["stack-pane"]
	assert.Equal(t, "Custom stack pane", stack.Desc)
}

func TestManagerLoad_DoesNotMigrateLegacyFavoritesShortcut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	configFile, err := GetConfigFile()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(configFile), 0o755))
	require.NoError(t, os.WriteFile(configFile, []byte(`
[workspace.shortcuts.actions.toggle-favorites-systemview]
keys = []
desc = "Custom favorites shortcut"
`), 0o644))

	mgr, err := NewManager()
	require.NoError(t, err)
	require.NoError(t, mgr.Load())

	favorites := mgr.Get().Workspace.Shortcuts.Actions["toggle-favorites-systemview"]
	assert.Empty(t, favorites.Keys)
	assert.Equal(t, "Custom favorites shortcut", favorites.Desc)
}

func TestManagerLoad_DoesNotMigrateLegacyFavoritesShortcutDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	configFile, err := GetConfigFile()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(configFile), 0o755))
	require.NoError(t, os.WriteFile(configFile, []byte(`
[workspace.shortcuts.actions.toggle-favorites-systemview]
keys = []
desc = "Toggle Favorites in right split"
`), 0o644))

	mgr, err := NewManager()
	require.NoError(t, err)
	require.NoError(t, mgr.Load())

	cfg := mgr.Get()
	require.NotNil(t, cfg)
	favorites := cfg.Workspace.Shortcuts.Actions["toggle-favorites-systemview"]
	assert.Empty(t, favorites.Keys)
	assert.Equal(t, "Toggle Favorites in right split", favorites.Desc)
	_, ok := cfg.Workspace.Shortcuts.Actions["toggle-favorites-sidebar"]
	assert.False(t, ok)
}
