package config

import (
	"runtime"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
)

func TestResolvePerformanceProfile_Default(t *testing.T) {
	cfg := &PerformanceConfig{
		Profile: ProfileDefault,
	}

	result := ResolvePerformanceProfile(cfg, nil)

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

	result := ResolvePerformanceProfile(cfg, nil)

	if result.SkiaCPUPaintingThreads != 0 {
		t.Errorf("expected SkiaCPUPaintingThreads=0, got %d", result.SkiaCPUPaintingThreads)
	}
	if result.WebViewPoolPrewarmCount != 4 {
		t.Errorf("expected WebViewPoolPrewarmCount=4, got %d", result.WebViewPoolPrewarmCount)
	}
}

func TestResolvePerformanceProfile_Lite_NoHardware(t *testing.T) {
	cfg := &PerformanceConfig{
		Profile: ProfileLite,
	}

	result := ResolvePerformanceProfile(cfg, nil)

	// Lite without hardware info should use default lite values
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

func TestResolvePerformanceProfile_Lite_LowCoreSystem(t *testing.T) {
	cfg := &PerformanceConfig{
		Profile: ProfileLite,
	}
	hw := &port.HardwareInfo{
		CPUCores:   2, // Very low core count
		CPUThreads: 4,
	}

	result := ResolvePerformanceProfile(cfg, hw)

	// Lite on low-core system should scale down
	if result.SkiaCPUPaintingThreads != 1 {
		t.Errorf("expected SkiaCPUPaintingThreads=1 for 2-core system, got %d", result.SkiaCPUPaintingThreads)
	}
}

func TestResolvePerformanceProfile_Max_NoHardware(t *testing.T) {
	cfg := &PerformanceConfig{
		Profile: ProfileMax,
	}

	result := ResolvePerformanceProfile(cfg, nil)

	// Max without hardware info should use fallback values
	// CPU threads: NumCPU/2 (assuming hyperthreading), clamped to [4, 8]
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
	// GPU threads: conservative default when no VRAM info
	if result.SkiaGPUPaintingThreads != 2 {
		t.Errorf("expected SkiaGPUPaintingThreads=2 (fallback), got %d", result.SkiaGPUPaintingThreads)
	}
	// Memory: fallback assumes 16GB system
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

func TestResolvePerformanceProfile_Max_HighEndSystem(t *testing.T) {
	cfg := &PerformanceConfig{
		Profile: ProfileMax,
	}
	hw := &port.HardwareInfo{
		CPUCores:   16,
		CPUThreads: 32,
		TotalRAM:   64 * 1024 * 1024 * 1024, // 64 GB
		VRAM:       16 * 1024 * 1024 * 1024, // 16 GB (high-end GPU)
		GPUVendor:  port.GPUVendorAMD,
	}

	result := ResolvePerformanceProfile(cfg, hw)

	// CPU threads: use physical cores capped at 8 (WebKitGTK limit)
	if result.SkiaCPUPaintingThreads != 8 {
		t.Errorf("expected SkiaCPUPaintingThreads=8 (capped), got %d", result.SkiaCPUPaintingThreads)
	}
	// GPU threads: high-end tier (16GB+ VRAM)
	if result.SkiaGPUPaintingThreads != 8 {
		t.Errorf("expected SkiaGPUPaintingThreads=8 for 16GB VRAM, got %d", result.SkiaGPUPaintingThreads)
	}
	// Memory: high-end tier (32GB+ RAM)
	if result.WebProcessMemoryLimitMB != 4096 {
		t.Errorf("expected WebProcessMemoryLimitMB=4096 for 64GB RAM, got %d", result.WebProcessMemoryLimitMB)
	}
	if result.NetworkProcessMemoryLimitMB != 1024 {
		t.Errorf("expected NetworkProcessMemoryLimitMB=1024 for 64GB RAM, got %d", result.NetworkProcessMemoryLimitMB)
	}
	// Pool prewarm: high-end tier
	if result.WebViewPoolPrewarmCount != 12 {
		t.Errorf("expected WebViewPoolPrewarmCount=12 for 64GB RAM, got %d", result.WebViewPoolPrewarmCount)
	}
}

func TestResolvePerformanceProfile_Max_MidRangeSystem(t *testing.T) {
	cfg := &PerformanceConfig{
		Profile: ProfileMax,
	}
	hw := &port.HardwareInfo{
		CPUCores:   8,
		CPUThreads: 16,
		TotalRAM:   16 * 1024 * 1024 * 1024, // 16 GB
		VRAM:       8 * 1024 * 1024 * 1024,  // 8 GB (mid-range GPU)
		GPUVendor:  port.GPUVendorNVIDIA,
	}

	result := ResolvePerformanceProfile(cfg, hw)

	// CPU threads: use physical cores (8)
	if result.SkiaCPUPaintingThreads != 8 {
		t.Errorf("expected SkiaCPUPaintingThreads=8, got %d", result.SkiaCPUPaintingThreads)
	}
	// GPU threads: mid-range tier (8-16GB VRAM)
	if result.SkiaGPUPaintingThreads != 6 {
		t.Errorf("expected SkiaGPUPaintingThreads=6 for 8GB VRAM, got %d", result.SkiaGPUPaintingThreads)
	}
	// Memory: mid-range tier (16-32GB RAM)
	if result.WebProcessMemoryLimitMB != 3072 {
		t.Errorf("expected WebProcessMemoryLimitMB=3072 for 16GB RAM, got %d", result.WebProcessMemoryLimitMB)
	}
	// Pool prewarm: mid-range tier
	if result.WebViewPoolPrewarmCount != 8 {
		t.Errorf("expected WebViewPoolPrewarmCount=8 for 16GB RAM, got %d", result.WebViewPoolPrewarmCount)
	}
}

func TestResolvePerformanceProfile_Max_LowEndSystem(t *testing.T) {
	cfg := &PerformanceConfig{
		Profile: ProfileMax,
	}
	hw := &port.HardwareInfo{
		CPUCores:   4,
		CPUThreads: 4,
		TotalRAM:   4 * 1024 * 1024 * 1024, // 4 GB
		VRAM:       2 * 1024 * 1024 * 1024, // 2 GB (integrated/old discrete)
		GPUVendor:  port.GPUVendorIntel,
	}

	result := ResolvePerformanceProfile(cfg, hw)

	// CPU threads: use physical cores (4)
	if result.SkiaCPUPaintingThreads != 4 {
		t.Errorf("expected SkiaCPUPaintingThreads=4, got %d", result.SkiaCPUPaintingThreads)
	}
	// GPU threads: low tier (<4GB VRAM)
	if result.SkiaGPUPaintingThreads != 2 {
		t.Errorf("expected SkiaGPUPaintingThreads=2 for 2GB VRAM, got %d", result.SkiaGPUPaintingThreads)
	}
	// Memory: low tier (<8GB RAM)
	if result.WebProcessMemoryLimitMB != 1024 {
		t.Errorf("expected WebProcessMemoryLimitMB=1024 for 4GB RAM, got %d", result.WebProcessMemoryLimitMB)
	}
	// Pool prewarm: low tier
	if result.WebViewPoolPrewarmCount != 4 {
		t.Errorf("expected WebViewPoolPrewarmCount=4 for 4GB RAM, got %d", result.WebViewPoolPrewarmCount)
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
	// Hardware info should be ignored for custom profile
	hw := &port.HardwareInfo{
		CPUCores:   16,
		CPUThreads: 32,
		TotalRAM:   64 * 1024 * 1024 * 1024,
		VRAM:       16 * 1024 * 1024 * 1024,
	}

	result := ResolvePerformanceProfile(cfg, hw)

	// Custom should use user-specified values, ignoring hardware
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

func TestComputeMaxGPUThreads_VRAMTiers(t *testing.T) {
	tests := []struct {
		name     string
		vramGB   int
		expected int
	}{
		{"no VRAM info", 0, 2},
		{"2GB VRAM (integrated)", 2, 2},
		{"4GB VRAM (entry discrete)", 4, 4},
		{"6GB VRAM (low-mid discrete)", 6, 4},
		{"8GB VRAM (mid discrete)", 8, 6},
		{"12GB VRAM (mid-high discrete)", 12, 6},
		{"16GB VRAM (high-end)", 16, 8},
		{"24GB VRAM (workstation)", 24, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hw := &port.HardwareInfo{
				VRAM: uint64(tt.vramGB) * 1024 * 1024 * 1024,
			}
			if tt.vramGB == 0 {
				hw = nil
			}
			result := computeMaxGPUThreads(hw)
			if result != tt.expected {
				t.Errorf("computeMaxGPUThreads(%dGB VRAM): expected %d, got %d", tt.vramGB, tt.expected, result)
			}
		})
	}
}

func TestComputeMaxMemoryLimits_RAMTiers(t *testing.T) {
	tests := []struct {
		name         string
		ramGB        int
		expectedWeb  int
		expectedNet  int
		expectedPool int
	}{
		{"no RAM info", 0, 2048, 512, 8},
		{"4GB RAM", 4, 1024, 256, 4},
		{"8GB RAM", 8, 2048, 512, 6},
		{"16GB RAM", 16, 3072, 768, 8},
		{"32GB RAM", 32, 4096, 1024, 12},
		{"64GB RAM", 64, 4096, 1024, 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hw := &port.HardwareInfo{
				TotalRAM: uint64(tt.ramGB) * 1024 * 1024 * 1024,
			}
			if tt.ramGB == 0 {
				hw = nil
			}
			webMB, netMB := computeMaxMemoryLimits(hw)
			poolPrewarm := computeMaxPoolPrewarm(hw)

			if webMB != tt.expectedWeb {
				t.Errorf("computeMaxMemoryLimits(%dGB RAM) web: expected %d, got %d", tt.ramGB, tt.expectedWeb, webMB)
			}
			if netMB != tt.expectedNet {
				t.Errorf("computeMaxMemoryLimits(%dGB RAM) net: expected %d, got %d", tt.ramGB, tt.expectedNet, netMB)
			}
			if poolPrewarm != tt.expectedPool {
				t.Errorf("computeMaxPoolPrewarm(%dGB RAM): expected %d, got %d", tt.ramGB, tt.expectedPool, poolPrewarm)
			}
		})
	}
}
