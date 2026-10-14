package config

import (
	"context"
	"fmt"
)

// ModeLegendPreferencesGateway persists mode legend preferences.
type ModeLegendPreferencesGateway struct {
	mgr *Manager
}

func NewModeLegendPreferencesGateway(mgr *Manager) *ModeLegendPreferencesGateway {
	return &ModeLegendPreferencesGateway{mgr: mgr}
}

func (g *ModeLegendPreferencesGateway) SaveModeLegendCollapsed(_ context.Context, collapsed bool) error {
	if g == nil || g.mgr == nil {
		return fmt.Errorf("config manager not initialized")
	}

	return g.mgr.Update(func(cfg *Config) {
		cfg.Workspace.Styling.ModeLegendCollapsed = collapsed
	})
}
