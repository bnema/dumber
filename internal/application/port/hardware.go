// Package port defines interfaces for external dependencies.
package port

import "context"

// HardwareInfo contains detected system hardware specifications.
// Used for performance profile auto-tuning.
type HardwareInfo struct {
	// CPU information
	CPUCores   int // Number of physical CPU cores
	CPUThreads int // Number of logical CPU threads (with hyperthreading)

	// Memory information (in bytes)
	TotalRAM     uint64 // Total system RAM
	AvailableRAM uint64 // Currently available RAM

	// GPU information
	GPUVendor GPUVendor // AMD, Intel, NVIDIA, or Unknown
	GPUName   string    // Human-readable GPU name (e.g., "AMD Radeon RX 9070 XT")
	VRAM      uint64    // Video memory in bytes (0 if unknown)
}

// CPUCoresOrDefault returns CPUCores if > 0, otherwise returns the fallback.
func (h HardwareInfo) CPUCoresOrDefault(fallback int) int {
	if h.CPUCores > 0 {
		return h.CPUCores
	}
	return fallback
}

// CPUThreadsOrDefault returns CPUThreads if > 0, otherwise returns the fallback.
func (h HardwareInfo) CPUThreadsOrDefault(fallback int) int {
	if h.CPUThreads > 0 {
		return h.CPUThreads
	}
	return fallback
}

// TotalRAMMB returns total RAM in megabytes.
// Returns 0 if RAM exceeds int range (unlikely on any real system).
func (h HardwareInfo) TotalRAMMB() int {
	mb := h.TotalRAM / (1024 * 1024)
	if mb > uint64(^uint(0)>>1) { // Check against max int
		return 0
	}
	return int(mb)
}

// VRAMMB returns VRAM in megabytes.
// Returns 0 if VRAM exceeds int range (unlikely on any real system).
func (h HardwareInfo) VRAMMB() int {
	mb := h.VRAM / (1024 * 1024)
	if mb > uint64(^uint(0)>>1) { // Check against max int
		return 0
	}
	return int(mb)
}

// HasDedicatedGPU returns true if a discrete GPU with VRAM was detected.
func (h HardwareInfo) HasDedicatedGPU() bool {
	return h.VRAM > 0 && h.GPUVendor != GPUVendorUnknown
}

// HardwareSurveyor detects system hardware specifications.
type HardwareSurveyor interface {
	// Survey detects and returns hardware information.
	// This should be called once at startup and cached.
	Survey(ctx context.Context) HardwareInfo
}
