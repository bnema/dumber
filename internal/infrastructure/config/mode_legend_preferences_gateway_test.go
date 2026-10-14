package config

import (
	"context"
	"testing"

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
