package config

import (
	"runtime"

	"github.com/bnema/dumber/internal/application/port"
)

const (
	// maxSkiaCPUThreads is the maximum number of Skia CPU painting threads
	// supported by WebKitGTK (0-8 range).
	maxSkiaCPUThreads = 8

	// maxSkiaGPUThreads is the maximum number of Skia GPU painting threads
	// supported by WebKitGTK (-1 to 8 range, where -1 means "let WebKit decide").
	maxSkiaGPUThreads = 8

	// Memory tier thresholds (in bytes)
	// Explicitly typed as uint64 to prevent overflow on 32-bit systems.
	ramTierLow    uint64 = 8 * 1024 * 1024 * 1024  // 8 GB
	ramTierMedium uint64 = 16 * 1024 * 1024 * 1024 // 16 GB
	ramTierHigh   uint64 = 32 * 1024 * 1024 * 1024 // 32 GB

	// VRAM tier thresholds (in bytes)
	vramTierLow  uint64 = 4 * 1024 * 1024 * 1024  // 4 GB (low-end discrete / integrated)
	vramTierMid  uint64 = 8 * 1024 * 1024 * 1024  // 8 GB (mid-range discrete)
	vramTierHigh uint64 = 16 * 1024 * 1024 * 1024 // 16 GB (high-end discrete)

	// GPU thread scaling values
	gpuThreadsHigh = 8 // For 16GB+ VRAM
	gpuThreadsMid  = 6 // For 8-16GB VRAM
	gpuThreadsLow  = 4 // For 4-8GB VRAM
	gpuThreadsMin  = 2 // For <4GB VRAM or unknown

	// Memory limit values (in MB)
	webMemHigh     = 4096 // For 32GB+ RAM
	webMemMedium   = 3072 // For 16-32GB RAM
	webMemLow      = 2048 // For 8-16GB RAM
	webMemMin      = 1024 // For <8GB RAM
	netMemHigh     = 1024 // For 32GB+ RAM
	netMemMedium   = 768  // For 16-32GB RAM
	netMemLow      = 512  // For 8-16GB RAM
	netMemMin      = 256  // For <8GB RAM
	webMemFallback = 2048 // When RAM unknown
	netMemFallback = 512  // When RAM unknown

	// WebView pool prewarm counts
	poolPrewarmHigh     = 12 // For 32GB+ RAM
	poolPrewarmMedium   = 8  // For 16-32GB RAM
	poolPrewarmLow      = 6  // For 8-16GB RAM
	poolPrewarmMin      = 4  // For <8GB RAM
	poolPrewarmFallback = 8  // When RAM unknown
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

	// Network process memory pressure
	NetworkProcessMemoryLimitMB               int
	NetworkProcessMemoryPollIntervalSec       float64
	NetworkProcessMemoryConservativeThreshold float64
	NetworkProcessMemoryStrictThreshold       float64

	// WebView pool
	WebViewPoolPrewarmCount int
}

// ResolvePerformanceProfile computes the actual performance settings based on
// the selected profile. For ProfileCustom, it returns the user's configured values.
// For other profiles, it computes appropriate values based on detected hardware.
//
// The hw parameter is optional - if nil, fallback values are used.
func ResolvePerformanceProfile(cfg *PerformanceConfig, hw *port.HardwareInfo) ResolvedPerformanceSettings {
	switch cfg.Profile {
	case ProfileLite:
		return resolveLiteProfile(hw)
	case ProfileMax:
		return resolveMaxProfile(hw)
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

		NetworkProcessMemoryLimitMB:               0,
		NetworkProcessMemoryPollIntervalSec:       0,
		NetworkProcessMemoryConservativeThreshold: 0,
		NetworkProcessMemoryStrictThreshold:       0,

		WebViewPoolPrewarmCount: 4,
	}
}

// resolveLiteProfile returns settings optimized for low-RAM systems.
// Uses hardware info to scale CPU threads if available.
func resolveLiteProfile(hw *port.HardwareInfo) ResolvedPerformanceSettings {
	// Use 2 CPU threads, or scale down on very low-core systems
	cpuThreads := 2
	if hw != nil && hw.CPUCores > 0 && hw.CPUCores < 4 {
		cpuThreads = 1
	}

	return ResolvedPerformanceSettings{
		SkiaCPUPaintingThreads: cpuThreads,
		SkiaGPUPaintingThreads: -1, // unset
		SkiaEnableCPURendering: false,

		WebProcessMemoryLimitMB:               768,
		WebProcessMemoryPollIntervalSec:       0, // use WebKit default (30s)
		WebProcessMemoryConservativeThreshold: 0.25,
		WebProcessMemoryStrictThreshold:       0.4,

		NetworkProcessMemoryLimitMB:               384,
		NetworkProcessMemoryPollIntervalSec:       0,
		NetworkProcessMemoryConservativeThreshold: 0.25,
		NetworkProcessMemoryStrictThreshold:       0.4,

		WebViewPoolPrewarmCount: 2,
	}
}

// resolveMaxProfile returns settings optimized for maximum responsiveness.
// Scales values based on detected hardware capabilities.
func resolveMaxProfile(hw *port.HardwareInfo) ResolvedPerformanceSettings {
	// CPU threads: use physical cores (not threads), capped at WebKitGTK limit
	cpuThreads := computeMaxCPUThreads(hw)

	// GPU threads: scale based on VRAM
	gpuThreads := computeMaxGPUThreads(hw)

	// WebView pool: scale based on RAM
	poolPrewarm := computeMaxPoolPrewarm(hw)

	return ResolvedPerformanceSettings{
		SkiaCPUPaintingThreads: cpuThreads,
		SkiaGPUPaintingThreads: gpuThreads,
		SkiaEnableCPURendering: false,

		// Max profile: no memory limits, let WebKit use defaults
		WebProcessMemoryLimitMB:               0,
		WebProcessMemoryPollIntervalSec:       0,
		WebProcessMemoryConservativeThreshold: 0,
		WebProcessMemoryStrictThreshold:       0,

		NetworkProcessMemoryLimitMB:               0,
		NetworkProcessMemoryPollIntervalSec:       0,
		NetworkProcessMemoryConservativeThreshold: 0,
		NetworkProcessMemoryStrictThreshold:       0,

		WebViewPoolPrewarmCount: poolPrewarm,
	}
}

// computeMaxCPUThreads calculates Skia CPU threads for max profile.
// Uses physical cores (not hyperthreaded) capped at WebKitGTK's limit.
func computeMaxCPUThreads(hw *port.HardwareInfo) int {
	var cpuThreads int

	if hw != nil && hw.CPUCores > 0 {
		// Use physical cores directly - they're better for parallel painting
		cpuThreads = hw.CPUCores
	} else {
		// Fallback: assume hyperthreading, use half of logical CPUs
		cpuThreads = runtime.NumCPU() / 2
	}

	// Minimum 4 for meaningful parallelism
	if cpuThreads < 4 {
		cpuThreads = 4
	}
	// Cap at WebKitGTK Skia limit
	if cpuThreads > maxSkiaCPUThreads {
		cpuThreads = maxSkiaCPUThreads
	}

	return cpuThreads
}

// computeMaxGPUThreads calculates Skia GPU threads based on VRAM.
func computeMaxGPUThreads(hw *port.HardwareInfo) int {
	if hw == nil || hw.VRAM == 0 {
		// No VRAM info - use conservative default
		return gpuThreadsMin
	}

	// Scale GPU threads based on VRAM tiers
	switch {
	case hw.VRAM >= vramTierHigh: // 16GB+ VRAM (high-end: RX 7900, RTX 4080+)
		return gpuThreadsHigh
	case hw.VRAM >= vramTierMid: // 8-16GB VRAM (mid-range: RX 7800, RTX 4070)
		return gpuThreadsMid
	case hw.VRAM >= vramTierLow: // 4-8GB VRAM (entry discrete)
		return gpuThreadsLow
	default: // <4GB VRAM (integrated / old discrete)
		return gpuThreadsMin
	}
}

// computeMaxMemoryLimits returns web process and network process memory limits.
func computeMaxMemoryLimits(hw *port.HardwareInfo) (webMB, netMB int) {
	if hw == nil || hw.TotalRAM == 0 {
		// Fallback: assume 16GB system
		return webMemFallback, netMemFallback
	}

	// Scale memory limits based on system RAM
	switch {
	case hw.TotalRAM >= ramTierHigh: // 32GB+ RAM
		return webMemHigh, netMemHigh
	case hw.TotalRAM >= ramTierMedium: // 16-32GB RAM
		return webMemMedium, netMemMedium
	case hw.TotalRAM >= ramTierLow: // 8-16GB RAM
		return webMemLow, netMemLow
	default: // <8GB RAM
		return webMemMin, netMemMin
	}
}

// computeMaxPoolPrewarm returns WebView pool prewarm count based on RAM.
func computeMaxPoolPrewarm(hw *port.HardwareInfo) int {
	if hw == nil || hw.TotalRAM == 0 {
		return poolPrewarmFallback
	}

	switch {
	case hw.TotalRAM >= ramTierHigh: // 32GB+
		return poolPrewarmHigh
	case hw.TotalRAM >= ramTierMedium: // 16-32GB
		return poolPrewarmMedium
	case hw.TotalRAM >= ramTierLow: // 8-16GB
		return poolPrewarmLow
	default: // <8GB
		return poolPrewarmMin
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

		NetworkProcessMemoryLimitMB:               cfg.NetworkProcessMemoryLimitMB,
		NetworkProcessMemoryPollIntervalSec:       cfg.NetworkProcessMemoryPollIntervalSec,
		NetworkProcessMemoryConservativeThreshold: cfg.NetworkProcessMemoryConservativeThreshold,
		NetworkProcessMemoryStrictThreshold:       cfg.NetworkProcessMemoryStrictThreshold,

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
		cfg.NetworkProcessMemoryLimitMB != 0 ||
		cfg.NetworkProcessMemoryPollIntervalSec != 0 ||
		cfg.NetworkProcessMemoryConservativeThreshold != 0 ||
		cfg.NetworkProcessMemoryStrictThreshold != 0
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
