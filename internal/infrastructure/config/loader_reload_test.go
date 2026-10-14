package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// setLegendDelayInFile rewrites the mode_legend_delay_ms line of the manager's
// config file, simulating an external edit.
func setLegendDelayInFile(t *testing.T, mgr *Manager, delayMs string) {
	t.Helper()

	path := mgr.GetConfigFile()
	require.NotEmpty(t, path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	lines := strings.Split(string(data), "\n")
	replaced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "mode_legend_delay_ms") {
			lines[i] = "mode_legend_delay_ms = " + delayMs
			replaced = true
		}
	}
	require.True(t, replaced, "mode_legend_delay_ms not found in written config")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600))
}

func TestManagerReload_KeepsSavedValue(t *testing.T) {
	mgr := newTestOmniboxPreferencesGatewayManager(t, t.TempDir())
	require.Equal(t, 500, mgr.Get().Workspace.Styling.ModeLegendDelayMs)

	require.NoError(t, mgr.Update(func(cfg *Config) { cfg.Workspace.Styling.ModeLegendDelayMs = 700 }))
	require.Equal(t, 700, mgr.Get().Workspace.Styling.ModeLegendDelayMs)

	mgr.mu.Lock()
	err := mgr.reload()
	mgr.mu.Unlock()
	require.NoError(t, err)
	require.Equal(t, 700, mgr.Get().Workspace.Styling.ModeLegendDelayMs)
}

func TestManagerReload_PicksUpExternalEditAfterSave(t *testing.T) {
	mgr := newTestOmniboxPreferencesGatewayManager(t, t.TempDir())
	require.NoError(t, mgr.Update(func(cfg *Config) { cfg.Workspace.Styling.ModeLegendDelayMs = 700 }))

	setLegendDelayInFile(t, mgr, "900")

	mgr.mu.Lock()
	err := mgr.reload()
	mgr.mu.Unlock()
	require.NoError(t, err)
	require.Equal(t, 900, mgr.Get().Workspace.Styling.ModeLegendDelayMs)
}

func TestManagerReload_PicksUpExternalEditWithoutPriorSave(t *testing.T) {
	mgr := newTestOmniboxPreferencesGatewayManager(t, t.TempDir())
	require.Equal(t, 500, mgr.Get().Workspace.Styling.ModeLegendDelayMs)

	setLegendDelayInFile(t, mgr, "900")

	mgr.mu.Lock()
	err := mgr.reload()
	mgr.mu.Unlock()
	require.NoError(t, err)
	require.Equal(t, 900, mgr.Get().Workspace.Styling.ModeLegendDelayMs)

	setLegendDelayInFile(t, mgr, "300")
	mgr.mu.Lock()
	err = mgr.reload()
	mgr.mu.Unlock()
	require.NoError(t, err)
	require.Equal(t, 300, mgr.Get().Workspace.Styling.ModeLegendDelayMs)
}

func TestManagerUpdate_ConsecutiveUpdatesEachPersist(t *testing.T) {
	configHome := t.TempDir()
	mgr := newTestOmniboxPreferencesGatewayManager(t, configHome)

	require.NoError(t, mgr.Update(func(cfg *Config) { cfg.Workspace.Styling.ModeLegendCollapsed = true }))
	require.NoError(t, mgr.Update(func(cfg *Config) { cfg.Workspace.Styling.ModeLegendDelayMs = 700 }))
	require.NoError(t, mgr.Update(func(cfg *Config) { cfg.Workspace.Styling.ModeLegendDelayMs = 800 }))
	require.NoError(t, mgr.Update(func(cfg *Config) { cfg.Workspace.Styling.ModeLegendCollapsed = false }))

	for name, got := range map[string]*Config{
		"in-memory": mgr.Get(),
		"on-disk":   newTestOmniboxPreferencesGatewayManager(t, configHome).Get(),
	} {
		require.False(t, got.Workspace.Styling.ModeLegendCollapsed, name)
		require.Equal(t, 800, got.Workspace.Styling.ModeLegendDelayMs, name)
	}
}
