package config

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testWatchDelay = 20 * time.Millisecond

// newWatchingTestManager returns a loaded manager watching its config file and
// a counter of callback invocations.
func newWatchingTestManager(t *testing.T) (*Manager, *atomic.Int32) {
	t.Helper()
	mgr := newTestOmniboxPreferencesGatewayManager(t, t.TempDir())
	mgr.watchDelay = testWatchDelay
	var calls atomic.Int32
	mgr.OnConfigChange(func(*Config) { calls.Add(1) })
	require.NoError(t, mgr.Watch())
	t.Cleanup(func() { _ = mgr.StopWatching() })
	return mgr, &calls
}

// settle waits long enough for any debounced watcher event to be handled.
func settle() { time.Sleep(10 * testWatchDelay) }

func TestWatch_SaveNotifiesOnceWithoutReload(t *testing.T) {
	mgr, calls := newWatchingTestManager(t)

	require.NoError(t, mgr.Update(func(cfg *Config) { cfg.Workspace.Styling.ModeLegendDelayMs = 700 }))
	require.Equal(t, int32(1), calls.Load(), "Save must notify synchronously once")

	settle()
	require.Equal(t, int32(1), calls.Load(), "watcher must skip our own write")
	require.Equal(t, 700, mgr.Get().Workspace.Styling.ModeLegendDelayMs)
}

func TestWatch_ExternalWriteRightAfterSaveReloads(t *testing.T) {
	mgr, calls := newWatchingTestManager(t)

	require.NoError(t, mgr.Update(func(cfg *Config) { cfg.Workspace.Styling.ModeLegendDelayMs = 700 }))
	setLegendDelayInFile(t, mgr, "900")

	require.Eventually(t, func() bool {
		return mgr.Get().Workspace.Styling.ModeLegendDelayMs == 900
	}, 5*time.Second, testWatchDelay)
	settle()
	require.Equal(t, int32(2), calls.Load())
}

func TestWatch_ExternalChangeReloadsAndNotifiesOnce(t *testing.T) {
	mgr, calls := newWatchingTestManager(t)

	setLegendDelayInFile(t, mgr, "900")

	require.Eventually(t, func() bool {
		return mgr.Get().Workspace.Styling.ModeLegendDelayMs == 900
	}, 5*time.Second, testWatchDelay)
	settle()
	require.Equal(t, int32(1), calls.Load())
}

func TestWatch_InvalidContentKeepsConfigAndFixReloads(t *testing.T) {
	mgr, calls := newWatchingTestManager(t)
	path := mgr.GetConfigFile()
	original, err := os.ReadFile(path)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path, []byte("this is = = not toml [["), 0o600))
	settle()
	require.Equal(t, int32(0), calls.Load())
	require.Equal(t, 500, mgr.Get().Workspace.Styling.ModeLegendDelayMs)

	// Restoring the last good content is unchanged from the stored hash: skip.
	require.NoError(t, os.WriteFile(path, original, 0o600))
	settle()
	require.Equal(t, int32(0), calls.Load())

	// A real fix after the bad write reloads.
	setLegendDelayInFile(t, mgr, "900")
	require.Eventually(t, func() bool {
		return mgr.Get().Workspace.Styling.ModeLegendDelayMs == 900
	}, 5*time.Second, testWatchDelay)
}

func TestWatch_InvalidValueKeepsConfigAndHash(t *testing.T) {
	mgr, calls := newWatchingTestManager(t)
	before := mgr.lastContentHash

	setLegendDelayInFile(t, mgr, "-5")
	settle()
	require.Equal(t, int32(0), calls.Load())
	require.Equal(t, 500, mgr.Get().Workspace.Styling.ModeLegendDelayMs)
	mgr.mu.RLock()
	require.Equal(t, before, mgr.lastContentHash)
	mgr.mu.RUnlock()
}

func TestStopWatching_Idempotent(t *testing.T) {
	mgr, _ := newWatchingTestManager(t)
	require.NoError(t, mgr.StopWatching())
	require.NoError(t, mgr.StopWatching())
	require.NoError(t, mgr.Watch())
}
