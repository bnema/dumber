// Package port defines interfaces for external dependencies.
package port

import "context"

// GPUVendor identifies the GPU manufacturer for vendor-specific optimizations.
type GPUVendor string

const (
	// GPUVendorAMD represents AMD/ATI GPUs (vendor ID 0x1002)
	GPUVendorAMD GPUVendor = "amd"
	// GPUVendorIntel represents Intel integrated/discrete GPUs (vendor ID 0x8086)
	GPUVendorIntel GPUVendor = "intel"
	// GPUVendorNVIDIA represents NVIDIA GPUs (vendor ID 0x10de)
	GPUVendorNVIDIA GPUVendor = "nvidia"
	// GPUVendorUnknown represents undetected or unsupported GPUs
	GPUVendorUnknown GPUVendor = "unknown"
)

// RenderingEnvManager configures rendering environment variables
// for GStreamer, WebKit, and GTK/GSK.
// Environment variables must be set BEFORE GTK/WebKit initialization.
//
// Note: ApplyEnvironment was removed from this interface because rendering
// settings are engine-specific (WebKit/GTK/GStreamer) and have moved to
// internal/infrastructure/env.RenderingSettings to avoid port bloat.
type RenderingEnvManager interface {
	// DetectGPUVendor identifies the primary GPU vendor from system info.
	// Uses /sys/class/drm/card*/device/vendor as primary detection method.
	DetectGPUVendor(ctx context.Context) GPUVendor

	// GetAppliedVars returns a map of environment variables that were set.
	// Useful for logging and debugging.
	GetAppliedVars() map[string]string
}
