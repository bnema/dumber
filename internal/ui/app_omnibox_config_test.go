package ui

import (
	"testing"

	configpkg "github.com/bnema/dumber/internal/infrastructure/config"
)

func TestBuildOmniboxConfig_UsesConfiguredMostVisitedDays(t *testing.T) {
	deps := &Dependencies{
		Config: &configpkg.Config{
			Omnibox: configpkg.OmniboxConfig{
				MostVisitedDays: 7,
			},
		},
	}

	got := buildOmniboxConfig(deps, nil, omniboxCallbacks{})
	if got.MostVisitedDays != 7 {
		t.Fatalf("MostVisitedDays = %d, want 7", got.MostVisitedDays)
	}
}
