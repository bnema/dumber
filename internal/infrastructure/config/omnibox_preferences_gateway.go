package config

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/domain/entity"
)

type OmniboxPreferencesGateway struct {
	mgr *Manager
}

func NewOmniboxPreferencesGateway(mgr *Manager) *OmniboxPreferencesGateway {
	return &OmniboxPreferencesGateway{mgr: mgr}
}

func (g *OmniboxPreferencesGateway) SaveOmniboxInitialBehavior(_ context.Context, behavior entity.OmniboxInitialBehavior) error {
	if g == nil || g.mgr == nil {
		return fmt.Errorf("config manager not initialized")
	}

	return g.mgr.Update(func(cfg *Config) {
		cfg.Omnibox.InitialBehavior = behavior
	})
}
