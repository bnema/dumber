package config

import (
	"context"
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

func TestManagerUpdate_OverlappingUpdatesDoNotLoseWrites(t *testing.T) {
	configHome := t.TempDir()
	mgr := newWatchedTestManager(t, configHome)
	legend := NewModeLegendPreferencesGateway(mgr)
	omnibox := NewOmniboxPreferencesGateway(mgr)

	otherStarted := make(chan struct{})
	otherDone := make(chan error, 1)

	// The first update starts a second one while it still holds the lock. The
	// second update must wait and then be applied on top of the first result.
	err := mgr.Update(func(cfg *Config) {
		go func() {
			close(otherStarted)
			otherDone <- omnibox.SaveOmniboxInitialBehavior(context.Background(), entity.OmniboxInitialBehaviorMostVisited)
		}()
		<-otherStarted
		cfg.Workspace.Styling.ModeLegendCollapsed = true
	})
	require.NoError(t, err)
	require.NoError(t, <-otherDone)

	got := mgr.Get()
	require.True(t, got.Workspace.Styling.ModeLegendCollapsed)
	require.Equal(t, entity.OmniboxInitialBehaviorMostVisited, got.Omnibox.InitialBehavior)

	reloaded := newTestOmniboxPreferencesGatewayManager(t, configHome).Get()
	require.True(t, reloaded.Workspace.Styling.ModeLegendCollapsed)
	require.Equal(t, entity.OmniboxInitialBehaviorMostVisited, reloaded.Omnibox.InitialBehavior)

	require.NoError(t, legend.SaveModeLegendCollapsed(context.Background(), false))
	got = mgr.Get()
	require.False(t, got.Workspace.Styling.ModeLegendCollapsed)
	require.Equal(t, entity.OmniboxInitialBehaviorMostVisited, got.Omnibox.InitialBehavior)
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
