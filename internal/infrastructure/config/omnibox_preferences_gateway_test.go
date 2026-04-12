package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

func TestOmniboxPreferencesGateway_SaveOmniboxInitialBehavior_UpdatesBehavior(t *testing.T) {
	configHome := t.TempDir()
	mgr := newTestOmniboxPreferencesGatewayManager(t, configHome)
	gateway := NewOmniboxPreferencesGateway(mgr)

	require.NoError(t, gateway.SaveOmniboxInitialBehavior(context.Background(), entity.OmniboxInitialBehaviorMostVisited))

	reloaded := newTestOmniboxPreferencesGatewayManager(t, configHome)
	got := reloaded.Get()
	require.Equal(t, entity.OmniboxInitialBehaviorMostVisited, got.Omnibox.InitialBehavior)
}

func TestOmniboxPreferencesGateway_SaveOmniboxInitialBehavior_PreservesDefaultSearchEngine(t *testing.T) {
	configHome := t.TempDir()
	mgr := newTestOmniboxPreferencesGatewayManager(t, configHome)
	mgr.config.DefaultSearchEngine = "https://example.com/search?q=%s"
	gateway := NewOmniboxPreferencesGateway(mgr)

	require.NoError(t, gateway.SaveOmniboxInitialBehavior(context.Background(), entity.OmniboxInitialBehaviorMostVisited))

	reloaded := newTestOmniboxPreferencesGatewayManager(t, configHome)
	got := reloaded.Get()
	require.Equal(t, "https://example.com/search?q=%s", got.DefaultSearchEngine)
}

func newTestOmniboxPreferencesGatewayManager(t *testing.T, configHome string) *Manager {
	t.Helper()

	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", filepath.Join(configHome, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(configHome, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(configHome, "cache"))

	mgr, err := NewManager()
	require.NoError(t, err)
	require.NoError(t, mgr.Load())

	return mgr
}
