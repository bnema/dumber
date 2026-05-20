package config

import "testing"

func TestCEFAdaptiveWindowlessFrameRateDefaults(t *testing.T) {
	cfg := DefaultConfig().Engine.CEF

	if !cfg.AdaptiveWindowlessFrameRate {
		t.Fatal("adaptive windowless frame-rate should be enabled by default")
	}
	if cfg.WindowlessFrameRate != 0 {
		t.Fatalf("default explicit windowless frame-rate = %d, want 0 for adaptive", cfg.WindowlessFrameRate)
	}
	if got := cfg.CEFWindowlessFrameRate(); got != int32(defaultCEFWindowlessFrameRate) {
		t.Fatalf("initial/fallback frame-rate = %d, want %d", got, defaultCEFWindowlessFrameRate)
	}
	if got := cfg.CEFWindowlessFrameRateMax(); got != int32(defaultCEFWindowlessFrameRateMax) {
		t.Fatalf("max frame-rate = %d, want %d", got, defaultCEFWindowlessFrameRateMax)
	}
	if !cfg.CEFAdaptiveWindowlessFrameRate() {
		t.Fatal("adaptive should be effective when enabled and no explicit frame-rate is set")
	}
}

func TestCEFAdaptiveWindowlessFrameRateDisabledByExplicitFrameRate(t *testing.T) {
	cfg := CEFEngineConfig{AdaptiveWindowlessFrameRate: true, WindowlessFrameRate: 144}

	if cfg.CEFAdaptiveWindowlessFrameRate() {
		t.Fatal("explicit windowless_frame_rate should disable adaptive frame-rate")
	}
	if got := cfg.CEFWindowlessFrameRate(); got != 144 {
		t.Fatalf("explicit frame-rate = %d, want 144", got)
	}
}
