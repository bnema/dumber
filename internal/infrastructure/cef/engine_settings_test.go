package cef

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

func TestEngineUpdateSettingsUpdatesApplicationScaleForNewViews(t *testing.T) {
	e := &Engine{applicationScale: 1}
	cfg := &config.Config{DefaultUIScale: 1.25}

	if err := e.UpdateSettings(context.Background(), port.EngineSettingsUpdate{Raw: cfg}); err != nil {
		t.Fatalf("UpdateSettings returned error: %v", err)
	}
	if e.applicationScale != 1.25 {
		t.Fatalf("applicationScale=%v, want 1.25", e.applicationScale)
	}
}

func TestEngineUpdateSettingsRejectsUnexpectedConfig(t *testing.T) {
	e := &Engine{}
	if err := e.UpdateSettings(context.Background(), port.EngineSettingsUpdate{Raw: "wrong"}); err == nil {
		t.Fatal("expected error for unexpected config type")
	}
}
