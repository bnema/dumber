package cef

import (
	"context"
	"sync"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
)

func TestEngineUpdateSettingsUpdatesApplicationScaleFromBoundaryPayload(t *testing.T) {
	e := &Engine{applicationScale: 1}

	if err := e.UpdateSettings(context.Background(), entity.EngineSettingsUpdate{
		Settings: entity.EngineSettingsPayload{DefaultUIScale: 1.25},
	}); err != nil {
		t.Fatalf("UpdateSettings returned error: %v", err)
	}
	if got := e.currentApplicationScale(); got != 1.25 {
		t.Fatalf("applicationScale=%v, want 1.25", got)
	}
}

func TestEngineUpdateSettingsUsesZeroScaleWhenPayloadIsEmpty(t *testing.T) {
	e := &Engine{applicationScale: 1.25}

	if err := e.UpdateSettings(context.Background(), entity.EngineSettingsUpdate{}); err != nil {
		t.Fatalf("UpdateSettings returned error: %v", err)
	}
	if got := e.currentApplicationScale(); got != 1 {
		t.Fatalf("applicationScale=%v, want default 1", got)
	}
}

func TestEngineApplicationScaleConcurrentAccess(_ *testing.T) {
	e := &Engine{applicationScale: 1}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = e.currentApplicationScale()
		}()
		go func(scale float64) {
			defer wg.Done()
			_ = e.UpdateSettings(context.Background(), entity.EngineSettingsUpdate{
				Settings: entity.EngineSettingsPayload{DefaultUIScale: scale},
			})
		}(1 + float64(i)/10)
	}
	wg.Wait()
}
