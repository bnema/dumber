package config

import "runtime"

const (
	// maxSkiaCPUThreads is the maximum number of Skia CPU painting threads
	// supported by WebKitGTK (0-8 range).
	maxSkiaCPUThreads = 8
)

// ResolvedPerformanceSettings contains the computed performance settings
// after profile resolution. These are the actual values to apply.
type ResolvedPerformanceSettings struct {
	// Skia threading
	SkiaCPUPaintingThreads int
	SkiaGPUPaintingThreads int
	SkiaEnableCPURendering bool

	// Web process memory pressure
	WebProcessMemoryLimitMB               int
	WebProcessMemoryPollIntervalSec       float64
	WebProcessMemoryConservativeThreshold float64
	WebProcessMemoryStrictThreshold       float64
	WebProcessMemoryKillThreshold         float64

	// Network process memory pressure
	NetworkProcessMemoryLimitMB               int
	NetworkProcessMemoryPollIntervalSec       float64
	NetworkProcessMemoryConservativeThreshold float64
	NetworkProcessMemoryStrictThreshold       float64
	NetworkProcessMemoryKillThreshold         float64

	// WebView pool
	WebViewPoolPrewarmCount int
}

// ResolvePerformanceProfile computes the actual performance settings based on
// the selected profile. For ProfileCustom, it returns the user's configured values.
// For other profiles, it computes appropriate values.
func ResolvePerformanceProfile(cfg *PerformanceConfig) ResolvedPerformanceSettings {
	switch cfg.Profile {
	case ProfileLite:
		return resolveLiteProfile()
	case ProfileMax:
		return resolveMaxProfile()
	case ProfileCustom:
		return resolveCustomProfile(cfg)
	default: // ProfileDefault or empty
		return resolveDefaultProfile()
	}
}

// resolveDefaultProfile returns WebKit defaults (no tuning).
func resolveDefaultProfile() ResolvedPerformanceSettings {
	return ResolvedPerformanceSettings{
		// All zeros/negatives mean "unset, use WebKit defaults"
		SkiaCPUPaintingThreads: 0,
		SkiaGPUPaintingThreads: -1,
		SkiaEnableCPURendering: false,

		WebProcessMemoryLimitMB:               0,
		WebProcessMemoryPollIntervalSec:       0,
		WebProcessMemoryConservativeThreshold: 0,
		WebProcessMemoryStrictThreshold:       0,
		WebProcessMemoryKillThreshold:         -1,

		NetworkProcessMemoryLimitMB:               0,
		NetworkProcessMemoryPollIntervalSec:       0,
		NetworkProcessMemoryConservativeThreshold: 0,
		NetworkProcessMemoryStrictThreshold:       0,
		NetworkProcessMemoryKillThreshold:         -1,

		WebViewPoolPrewarmCount: 4,
	}
}

// resolveLiteProfile returns settings optimized for low-RAM systems.
func resolveLiteProfile() ResolvedPerformanceSettings {
	return ResolvedPerformanceSettings{
		SkiaCPUPaintingThreads: 2,
		SkiaGPUPaintingThreads: -1, // unset
		SkiaEnableCPURendering: false,

		WebProcessMemoryLimitMB:               512,
		WebProcessMemoryPollIntervalSec:       0, // use WebKit default (30s)
		WebProcessMemoryConservativeThreshold: 0.25,
		WebProcessMemoryStrictThreshold:       0.4,
		WebProcessMemoryKillThreshold:         0.8,

		NetworkProcessMemoryLimitMB:               256,
		NetworkProcessMemoryPollIntervalSec:       0,
		NetworkProcessMemoryConservativeThreshold: 0.25,
		NetworkProcessMemoryStrictThreshold:       0.4,
		NetworkProcessMemoryKillThreshold:         0.8,

		WebViewPoolPrewarmCount: 2,
	}
}

// resolveMaxProfile returns settings optimized for maximum responsiveness.
func resolveMaxProfile() ResolvedPerformanceSettings {
	// Use half of available CPUs, minimum 4, maximum 8 (WebKitGTK Skia limit)
	cpuThreads := runtime.NumCPU() / 2
	if cpuThreads < 4 {
		cpuThreads = 4
	}
	if cpuThreads > maxSkiaCPUThreads {
		cpuThreads = maxSkiaCPUThreads
	}

	return ResolvedPerformanceSettings{
		SkiaCPUPaintingThreads: cpuThreads,
		SkiaGPUPaintingThreads: 2,
		SkiaEnableCPURendering: false,

		WebProcessMemoryLimitMB:               2048,
		WebProcessMemoryPollIntervalSec:       0, // use WebKit default
		WebProcessMemoryConservativeThreshold: 0.5,
		WebProcessMemoryStrictThreshold:       0.7,
		WebProcessMemoryKillThreshold:         -1, // never kill

		NetworkProcessMemoryLimitMB:               512,
		NetworkProcessMemoryPollIntervalSec:       0,
		NetworkProcessMemoryConservativeThreshold: 0.5,
		NetworkProcessMemoryStrictThreshold:       0.7,
		NetworkProcessMemoryKillThreshold:         -1,

		WebViewPoolPrewarmCount: 8,
	}
}

// resolveCustomProfile returns the user's configured values directly.
func resolveCustomProfile(cfg *PerformanceConfig) ResolvedPerformanceSettings {
	return ResolvedPerformanceSettings{
		SkiaCPUPaintingThreads: cfg.SkiaCPUPaintingThreads,
		SkiaGPUPaintingThreads: cfg.SkiaGPUPaintingThreads,
		SkiaEnableCPURendering: cfg.SkiaEnableCPURendering,

		WebProcessMemoryLimitMB:               cfg.WebProcessMemoryLimitMB,
		WebProcessMemoryPollIntervalSec:       cfg.WebProcessMemoryPollIntervalSec,
		WebProcessMemoryConservativeThreshold: cfg.WebProcessMemoryConservativeThreshold,
		WebProcessMemoryStrictThreshold:       cfg.WebProcessMemoryStrictThreshold,
		WebProcessMemoryKillThreshold:         cfg.WebProcessMemoryKillThreshold,

		NetworkProcessMemoryLimitMB:               cfg.NetworkProcessMemoryLimitMB,
		NetworkProcessMemoryPollIntervalSec:       cfg.NetworkProcessMemoryPollIntervalSec,
		NetworkProcessMemoryConservativeThreshold: cfg.NetworkProcessMemoryConservativeThreshold,
		NetworkProcessMemoryStrictThreshold:       cfg.NetworkProcessMemoryStrictThreshold,
		NetworkProcessMemoryKillThreshold:         cfg.NetworkProcessMemoryKillThreshold,

		WebViewPoolPrewarmCount: cfg.WebViewPoolPrewarmCount,
	}
}

// HasCustomPerformanceFields returns true if any individual performance tuning
// fields are set to non-default values. Used for validation.
func HasCustomPerformanceFields(cfg *PerformanceConfig) bool {
	return cfg.SkiaCPUPaintingThreads != 0 ||
		cfg.SkiaGPUPaintingThreads != -1 ||
		cfg.SkiaEnableCPURendering ||
		cfg.WebProcessMemoryLimitMB != 0 ||
		cfg.WebProcessMemoryPollIntervalSec != 0 ||
		cfg.WebProcessMemoryConservativeThreshold != 0 ||
		cfg.WebProcessMemoryStrictThreshold != 0 ||
		cfg.WebProcessMemoryKillThreshold != -1 ||
		cfg.NetworkProcessMemoryLimitMB != 0 ||
		cfg.NetworkProcessMemoryPollIntervalSec != 0 ||
		cfg.NetworkProcessMemoryConservativeThreshold != 0 ||
		cfg.NetworkProcessMemoryStrictThreshold != 0 ||
		cfg.NetworkProcessMemoryKillThreshold != -1
}

// IsValidPerformanceProfile returns true if the profile name is recognized.
func IsValidPerformanceProfile(profile PerformanceProfile) bool {
	switch profile {
	case ProfileDefault, ProfileLite, ProfileMax, ProfileCustom, "":
		return true
	default:
		return false
	}
}
