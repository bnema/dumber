package config

import "testing"

func TestResolveEngineTypeDefaultsToCEF(t *testing.T) {
	t.Parallel()

	cfg := EngineConfig{}
	if got := cfg.ResolveEngineType(); got != EngineTypeCEF {
		t.Fatalf("default engine type = %q, want %q", got, EngineTypeCEF)
	}
}

func TestCEFWindowlessFrameRateFallsBackToDefault(t *testing.T) {
	t.Parallel()

	cfg := CEFEngineConfig{}
	if got := cfg.CEFWindowlessFrameRate(); got != int32(defaultCEFWindowlessFrameRate) {
		t.Fatalf("default frame rate = %d, want %d", got, defaultCEFWindowlessFrameRate)
	}

	cfg.WindowlessFrameRate = 0
	if got := cfg.CEFWindowlessFrameRate(); got != int32(defaultCEFWindowlessFrameRate) {
		t.Fatalf("zero frame rate fallback = %d, want %d", got, defaultCEFWindowlessFrameRate)
	}

	cfg.WindowlessFrameRate = 144
	if got := cfg.CEFWindowlessFrameRate(); got != 144 {
		t.Fatalf("explicit frame rate = %d, want 144", got)
	}
}
