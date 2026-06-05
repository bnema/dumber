package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerLoad_MergesMissingPaneModeActionsInMemory(t *testing.T) {
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

	eject, ok := cfg.Workspace.PaneMode.Actions["eject-pane-to-window"]
	require.True(t, ok)
	assert.Equal(t, []string{"w"}, eject.Keys)

	stack := cfg.Workspace.PaneMode.Actions["stack-pane"]
	assert.Equal(t, "Custom stack pane", stack.Desc)
}
