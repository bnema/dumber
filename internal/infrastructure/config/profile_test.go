package config

import (
	"runtime"
	"testing"
)

func TestResolvePerformanceProfile_Default(t *testing.T) {
	cfg := &PerformanceConfig{
		Profile: ProfileDefault,
	}

	result := ResolvePerformanceProfile(cfg)

	// Default should have no tuning (unset values)
	if result.SkiaCPUPaintingThreads != 0 {
		t.Errorf("expected SkiaCPUPaintingThreads=0, got %d", result.SkiaCPUPaintingThreads)
	}
	if result.SkiaGPUPaintingThreads != -1 {
		t.Errorf("expected SkiaGPUPaintingThreads=-1, got %d", result.SkiaGPUPaintingThreads)
	}
	if result.WebProcessMemoryLimitMB != 0 {
		t.Errorf("expected WebProcessMemoryLimitMB=0, got %d", result.WebProcessMemoryLimitMB)
	}
	if result.WebViewPoolPrewarmCount != 4 {
		t.Errorf("expected WebViewPoolPrewarmCount=4, got %d", result.WebViewPoolPrewarmCount)
	}
}

func TestResolvePerformanceProfile_Empty(t *testing.T) {
	cfg := &PerformanceConfig{
		Profile: "", // Empty should behave like default
	}

	result := ResolvePerformanceProfile(cfg)

	if result.SkiaCPUPaintingThreads != 0 {
		t.Errorf("expected SkiaCPUPaintingThreads=0, got %d", result.SkiaCPUPaintingThreads)
	}
	if result.WebViewPoolPrewarmCount != 4 {
		t.Errorf("expected WebViewPoolPrewarmCount=4, got %d", result.WebViewPoolPrewarmCount)
	}
}

func TestResolvePerformanceProfile_Lite(t *testing.T) {
	cfg := &PerformanceConfig{
		Profile: ProfileLite,
	}

	result := ResolvePerformanceProfile(cfg)

	// Lite should have reduced resource usage
	if result.SkiaCPUPaintingThreads != 2 {
		t.Errorf("expected SkiaCPUPaintingThreads=2, got %d", result.SkiaCPUPaintingThreads)
	}
	if result.WebProcessMemoryLimitMB != 512 {
		t.Errorf("expected WebProcessMemoryLimitMB=512, got %d", result.WebProcessMemoryLimitMB)
	}
	if result.NetworkProcessMemoryLimitMB != 256 {
		t.Errorf("expected NetworkProcessMemoryLimitMB=256, got %d", result.NetworkProcessMemoryLimitMB)
	}
	if result.WebViewPoolPrewarmCount != 2 {
		t.Errorf("expected WebViewPoolPrewarmCount=2, got %d", result.WebViewPoolPrewarmCount)
	}
	if result.WebProcessMemoryKillThreshold != 0.8 {
		t.Errorf("expected WebProcessMemoryKillThreshold=0.8, got %f", result.WebProcessMemoryKillThreshold)
	}
}

func TestResolvePerformanceProfile_Max(t *testing.T) {
	cfg := &PerformanceConfig{
		Profile: ProfileMax,
	}

	result := ResolvePerformanceProfile(cfg)

	// Max should have high resource allocation
	// CPU threads: NumCPU/2, clamped to [4, 8] range (WebKitGTK Skia limit)
	expectedCPUThreads := runtime.NumCPU() / 2
	if expectedCPUThreads < 4 {
		expectedCPUThreads = 4
	}
	if expectedCPUThreads > 8 {
		expectedCPUThreads = 8
	}
	if result.SkiaCPUPaintingThreads != expectedCPUThreads {
		t.Errorf("expected SkiaCPUPaintingThreads=%d, got %d", expectedCPUThreads, result.SkiaCPUPaintingThreads)
	}
	if result.SkiaGPUPaintingThreads != 2 {
		t.Errorf("expected SkiaGPUPaintingThreads=2, got %d", result.SkiaGPUPaintingThreads)
	}
	if result.WebProcessMemoryLimitMB != 2048 {
		t.Errorf("expected WebProcessMemoryLimitMB=2048, got %d", result.WebProcessMemoryLimitMB)
	}
	if result.WebViewPoolPrewarmCount != 8 {
		t.Errorf("expected WebViewPoolPrewarmCount=8, got %d", result.WebViewPoolPrewarmCount)
	}
	if result.WebProcessMemoryKillThreshold != -1 {
		t.Errorf("expected WebProcessMemoryKillThreshold=-1 (never kill), got %f", result.WebProcessMemoryKillThreshold)
	}
}

func TestResolvePerformanceProfile_Custom(t *testing.T) {
	cfg := &PerformanceConfig{
		Profile:                               ProfileCustom,
		SkiaCPUPaintingThreads:                6,
		SkiaGPUPaintingThreads:                3,
		SkiaEnableCPURendering:                true,
		WebProcessMemoryLimitMB:               1024,
		WebProcessMemoryConservativeThreshold: 0.4,
		WebViewPoolPrewarmCount:               5,
	}

	result := ResolvePerformanceProfile(cfg)

	// Custom should use user-specified values
	if result.SkiaCPUPaintingThreads != 6 {
		t.Errorf("expected SkiaCPUPaintingThreads=6, got %d", result.SkiaCPUPaintingThreads)
	}
	if result.SkiaGPUPaintingThreads != 3 {
		t.Errorf("expected SkiaGPUPaintingThreads=3, got %d", result.SkiaGPUPaintingThreads)
	}
	if !result.SkiaEnableCPURendering {
		t.Error("expected SkiaEnableCPURendering=true")
	}
	if result.WebProcessMemoryLimitMB != 1024 {
		t.Errorf("expected WebProcessMemoryLimitMB=1024, got %d", result.WebProcessMemoryLimitMB)
	}
	if result.WebViewPoolPrewarmCount != 5 {
		t.Errorf("expected WebViewPoolPrewarmCount=5, got %d", result.WebViewPoolPrewarmCount)
	}
}

func TestHasCustomPerformanceFields(t *testing.T) {
	tests := []struct {
		name     string
		cfg      PerformanceConfig
		expected bool
	}{
		{
			name: "all defaults - no custom fields",
			cfg: PerformanceConfig{
				SkiaCPUPaintingThreads:            0,
				SkiaGPUPaintingThreads:            -1,
				SkiaEnableCPURendering:            false,
				WebProcessMemoryKillThreshold:     -1,
				NetworkProcessMemoryKillThreshold: -1,
			},
			expected: false,
		},
		{
			name: "skia cpu threads set",
			cfg: PerformanceConfig{
				SkiaCPUPaintingThreads:            4,
				SkiaGPUPaintingThreads:            -1,
				WebProcessMemoryKillThreshold:     -1,
				NetworkProcessMemoryKillThreshold: -1,
			},
			expected: true,
		},
		{
			name: "memory limit set",
			cfg: PerformanceConfig{
				SkiaGPUPaintingThreads:            -1,
				WebProcessMemoryLimitMB:           1024,
				WebProcessMemoryKillThreshold:     -1,
				NetworkProcessMemoryKillThreshold: -1,
			},
			expected: true,
		},
		{
			name: "kill threshold changed from -1",
			cfg: PerformanceConfig{
				SkiaGPUPaintingThreads:            -1,
				WebProcessMemoryKillThreshold:     0.9,
				NetworkProcessMemoryKillThreshold: -1,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasCustomPerformanceFields(&tt.cfg)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsValidPerformanceProfile(t *testing.T) {
	tests := []struct {
		profile  PerformanceProfile
		expected bool
	}{
		{ProfileDefault, true},
		{ProfileLite, true},
		{ProfileMax, true},
		{ProfileCustom, true},
		{"", true}, // Empty is valid (treated as default)
		{"invalid", false},
		{"LITE", false}, // Case sensitive
		{"Maximum", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.profile), func(t *testing.T) {
			result := IsValidPerformanceProfile(tt.profile)
			if result != tt.expected {
				t.Errorf("IsValidPerformanceProfile(%q): expected %v, got %v", tt.profile, tt.expected, result)
			}
		})
	}
}
