package config

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newNotifyingTestManager returns a loaded manager that notifies on saves as
// when watching, without starting fsnotify: tests call handleFileChange
// directly. The counter counts change notifications.
func newNotifyingTestManager(t *testing.T) (*Manager, *atomic.Int32) {
	t.Helper()
	mgr := newTestOmniboxPreferencesGatewayManager(t, t.TempDir())
	var calls atomic.Int32
	mgr.OnConfigChangeGeneration(func(*Config, uint64) { calls.Add(1) })
	mgr.mu.Lock()
	mgr.watching = true
	mgr.mu.Unlock()
	return mgr, &calls
}

func TestHandleFileChange_SkipsOwnSave(t *testing.T) {
	mgr, calls := newNotifyingTestManager(t)

	require.NoError(t, mgr.Update(func(cfg *Config) { cfg.Workspace.Styling.ModeLegendDelayMs = 700 }))
	require.Equal(t, int32(1), calls.Load(), "Save must notify synchronously once")

	mgr.handleFileChange(mgr.GetConfigFile())
	require.Equal(t, int32(1), calls.Load(), "the event for our own write is skipped")
	require.Equal(t, 700, mgr.Get().Workspace.Styling.ModeLegendDelayMs)
}

func TestHandleFileChange_ExternalWriteAfterSaveReloads(t *testing.T) {
	mgr, calls := newNotifyingTestManager(t)

	require.NoError(t, mgr.Update(func(cfg *Config) { cfg.Workspace.Styling.ModeLegendDelayMs = 700 }))
	setLegendDelayInFile(t, mgr, "900")
	mgr.handleFileChange(mgr.GetConfigFile())

	require.Equal(t, 900, mgr.Get().Workspace.Styling.ModeLegendDelayMs)
	require.Equal(t, int32(2), calls.Load())
}

func TestHandleFileChange_InvalidContentKeepsConfigAndFixReloads(t *testing.T) {
	mgr, calls := newNotifyingTestManager(t)
	path := mgr.GetConfigFile()
	original, err := os.ReadFile(path)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path, []byte("this is = = not toml [["), 0o600))
	mgr.handleFileChange(path)
	require.Equal(t, int32(0), calls.Load())
	require.Equal(t, 500, mgr.Get().Workspace.Styling.ModeLegendDelayMs)

	// Restoring the last good content matches the stored hash: skip.
	require.NoError(t, os.WriteFile(path, original, 0o600))
	mgr.handleFileChange(path)
	require.Equal(t, int32(0), calls.Load())

	// A real fix after the bad write reloads.
	setLegendDelayInFile(t, mgr, "900")
	mgr.handleFileChange(path)
	require.Equal(t, 900, mgr.Get().Workspace.Styling.ModeLegendDelayMs)
	require.Equal(t, int32(1), calls.Load())
}

func TestHandleFileChange_InvalidValueKeepsConfigAndHash(t *testing.T) {
	mgr, calls := newNotifyingTestManager(t)
	before := mgr.lastContentHash

	setLegendDelayInFile(t, mgr, "-5")
	mgr.handleFileChange(mgr.GetConfigFile())
	require.Equal(t, int32(0), calls.Load())
	require.Equal(t, 500, mgr.Get().Workspace.Styling.ModeLegendDelayMs)
	mgr.mu.RLock()
	require.Equal(t, before, mgr.lastContentHash)
	mgr.mu.RUnlock()
}

// TestWatch_SmokeExternalChange runs the real fsnotify path once, and checks
// that StopWatching is idempotent and watching can restart.
func TestWatch_SmokeExternalChange(t *testing.T) {
	mgr := newTestOmniboxPreferencesGatewayManager(t, t.TempDir())
	mgr.watchDelay = 20 * time.Millisecond
	require.NoError(t, mgr.Watch())
	t.Cleanup(func() { _ = mgr.StopWatching() })

	setLegendDelayInFile(t, mgr, "900")
	require.Eventually(t, func() bool {
		return mgr.Get().Workspace.Styling.ModeLegendDelayMs == 900
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, mgr.StopWatching())
	require.NoError(t, mgr.StopWatching())
	require.NoError(t, mgr.Watch())
}
