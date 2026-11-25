package ui

import (
	"testing"

	portmocks "bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/domain/entity"
)

func TestInitialWindowURL_UsesExplicitThenConfiguredThenHome(t *testing.T) {
	snapshot := func(startup string) entity.RuntimeConfigSnapshot {
		var s entity.RuntimeConfigSnapshot
		s.UI.Workspace.StartupURL = startup
		return s
	}
	tests := []struct {
		name, initial, configured, want string
	}{
		{"explicit URL wins", "https://example.com", "dumb://history", "https://example.com"},
		{"configured startup URL", "", "dumb://history", "dumb://history"},
		{"empty falls back to home", "", "", "dumb://home"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := portmocks.NewMockRuntimeConfigProvider(t)
			provider.EXPECT().Current().Return(snapshot(tt.configured))
			app := &App{deps: &Dependencies{RuntimeConfig: provider, InitialURL: tt.initial}}
			if got := app.initialWindowURL(); got != tt.want {
				t.Fatalf("initialWindowURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
