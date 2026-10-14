package config

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"

	"github.com/stretchr/testify/require"
)

func TestModeLegendPreferencesGateway_SaveModeLegendCollapsed_Persists(t *testing.T) {
	configHome := t.TempDir()
	mgr := newTestOmniboxPreferencesGatewayManager(t, configHome)
	gateway := NewModeLegendPreferencesGateway(mgr)
	require.False(t, mgr.Get().Workspace.Styling.ModeLegendCollapsed)

	require.NoError(t, gateway.SaveModeLegendCollapsed(context.Background(), true))
	require.True(t, newTestOmniboxPreferencesGatewayManager(t, configHome).Get().Workspace.Styling.ModeLegendCollapsed)

	require.NoError(t, gateway.SaveModeLegendCollapsed(context.Background(), false))
	require.False(t, newTestOmniboxPreferencesGatewayManager(t, configHome).Get().Workspace.Styling.ModeLegendCollapsed)
}

func TestModeLegendPreferencesGateway_NilManagerReturnsError(t *testing.T) {
	require.Error(t, NewModeLegendPreferencesGateway(nil).SaveModeLegendCollapsed(context.Background(), true))
}

// newWatchedTestManager mirrors production, where the manager is in watching mode
// so Save() publishes the in-memory config directly instead of re-reading the
// file. No fsnotify watcher is started, which keeps the tests deterministic.
func newWatchedTestManager(t *testing.T, configHome string) *Manager {
	t.Helper()

	mgr := newTestOmniboxPreferencesGatewayManager(t, configHome)
	mgr.watching = true

	return mgr
}

// TestManagerUpdate_UpdateAfterStaleSnapshotDoesNotLoseWrites forces the
// interleaving a Get()-then-Save() implementation loses: another update
// publishes right after the snapshot the first update works from was taken.
// Update must apply its change on top of that update instead of overwriting it.
func TestManagerUpdate_UpdateAfterStaleSnapshotDoesNotLoseWrites(t *testing.T) {
	configHome := t.TempDir()
	mgr := newWatchedTestManager(t, configHome)
	legend := NewModeLegendPreferencesGateway(mgr)
	omnibox := NewOmniboxPreferencesGateway(mgr)

	saveOther := func() error {
		return omnibox.SaveOmniboxInitialBehavior(context.Background(), entity.OmniboxInitialBehaviorMostVisited)
	}
	queued := make(chan error, 1)
	var fired atomic.Bool
	mgr.afterSnapshot = func() {
		// The nested update calls this hook too; only the first snapshot reacts.
		if fired.Swap(true) {
			return
		}
		func() {
			if mgr.mu.TryLock() {
				// The snapshot was taken without keeping the lock: another
				// update publishes before this one continues.
				mgr.mu.Unlock()
				require.NoError(t, saveOther())
				queued <- nil
				return
			}
			// The lock is held across the whole update: the other update has
			// to queue behind it.
			go func() { queued <- saveOther() }()
		}()
	}

	require.NoError(t, legend.SaveModeLegendCollapsed(context.Background(), true))
	require.NoError(t, <-queued)

	for name, got := range map[string]*Config{
		"in-memory": mgr.Get(),
		"on-disk":   newTestOmniboxPreferencesGatewayManager(t, configHome).Get(),
	} {
		require.True(t, got.Workspace.Styling.ModeLegendCollapsed, name)
		require.Equal(t, entity.OmniboxInitialBehaviorMostVisited, got.Omnibox.InitialBehavior, name)
	}
}

func TestManagerUpdate_AppliesToLatestConfigAfterSave(t *testing.T) {
	mgr := newWatchedTestManager(t, t.TempDir())

	cfg := mgr.Get()
	cfg.Omnibox.InitialBehavior = entity.OmniboxInitialBehaviorMostVisited
	require.NoError(t, mgr.Save(cfg))

	require.NoError(t, NewModeLegendPreferencesGateway(mgr).SaveModeLegendCollapsed(context.Background(), true))

	got := mgr.Get()
	require.True(t, got.Workspace.Styling.ModeLegendCollapsed)
	require.Equal(t, entity.OmniboxInitialBehaviorMostVisited, got.Omnibox.InitialBehavior)
}

func TestManagerUpdate_RejectsNilAndInvalidMutations(t *testing.T) {
	mgr := newWatchedTestManager(t, t.TempDir())
	before := mgr.Get().Workspace.Styling.ModeLegendDelayMs

	require.Error(t, mgr.Update(nil))
	require.Error(t, mgr.Update(func(cfg *Config) { cfg.Workspace.Styling.ModeLegendDelayMs = -1 }))
	require.Equal(t, before, mgr.Get().Workspace.Styling.ModeLegendDelayMs)
}

func TestManagerUpdate_ConcurrentUpdatesAreAllPersisted(t *testing.T) {
	configHome := t.TempDir()
	mgr := newWatchedTestManager(t, configHome)

	mutations := []func(*Config){
		func(cfg *Config) { cfg.Workspace.Styling.ModeLegendCollapsed = true },
		func(cfg *Config) { cfg.Omnibox.InitialBehavior = entity.OmniboxInitialBehaviorNone },
		func(cfg *Config) { cfg.Workspace.Styling.ModeLegendDelayMs = 1234 },
		func(cfg *Config) { cfg.Workspace.Styling.TransitionDuration = 321 },
		func(cfg *Config) { cfg.Workspace.Styling.ModeBorderWidth = 7 },
		func(cfg *Config) { cfg.Workspace.Styling.ModeLegendLinger = false },
	}

	start := make(chan struct{})
	errs := make(chan error, len(mutations))
	for _, mutate := range mutations {
		go func() {
			<-start
			errs <- mgr.Update(mutate)
		}()
	}
	close(start)
	for range mutations {
		require.NoError(t, <-errs)
	}

	for name, got := range map[string]*Config{
		"in-memory": mgr.Get(),
		"on-disk":   newTestOmniboxPreferencesGatewayManager(t, configHome).Get(),
	} {
		require.True(t, got.Workspace.Styling.ModeLegendCollapsed, name)
		require.Equal(t, entity.OmniboxInitialBehaviorNone, got.Omnibox.InitialBehavior, name)
		require.Equal(t, 1234, got.Workspace.Styling.ModeLegendDelayMs, name)
		require.Equal(t, 321, got.Workspace.Styling.TransitionDuration, name)
		require.Equal(t, 7, got.Workspace.Styling.ModeBorderWidth, name)
		require.False(t, got.Workspace.Styling.ModeLegendLinger, name)
	}
}
