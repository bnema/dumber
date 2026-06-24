package ui

import (
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
)

func TestBuildOmniboxConfigUsesRuntimeUIConfig(t *testing.T) {
	deps := &Dependencies{}
	runtimeCfg := entity.RuntimeUIConfig{
		DefaultUIScale:      1.35,
		DefaultSearchEngine: "https://search.example/?q=%s",
		SearchShortcuts: map[string]entity.RuntimeSearchShortcut{
			"gh": {
				URL:         "https://github.com/search?q=%s",
				Description: "GitHub",
			},
		},
		Omnibox: entity.RuntimeOmniboxConfig{
			InitialBehavior: entity.OmniboxInitialBehaviorMostVisited,
			MostVisitedDays: 7,
		},
	}

	got := buildOmniboxConfig(deps, runtimeCfg, nil, omniboxCallbacks{})
	if got.MostVisitedDays != 7 {
		t.Fatalf("MostVisitedDays = %d, want 7", got.MostVisitedDays)
	}
	if got.DefaultSearch != "https://search.example/?q=%s" {
		t.Fatalf("DefaultSearch = %q, want runtime default search", got.DefaultSearch)
	}
	if got.InitialBehavior != entity.OmniboxInitialBehaviorMostVisited {
		t.Fatalf("InitialBehavior = %q, want most_visited", got.InitialBehavior)
	}
	if got.UIScale != 1.35 {
		t.Fatalf("UIScale = %v, want 1.35", got.UIScale)
	}
	if got.ShortcutsUC == nil {
		t.Fatal("ShortcutsUC is nil")
	}
}
