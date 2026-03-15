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

// RenderingEnvSettings contains all rendering-related environment settings.
// This is a port-local type to avoid import cycles with config package.
type RenderingEnvSettings struct {
}

// RenderingEnvManager configures rendering environment variables
// for GStreamer, WebKit, and GTK/GSK.
// Environment variables must be set BEFORE GTK/WebKit initialization.
type RenderingEnvManager interface {
	// DetectGPUVendor identifies the primary GPU vendor from system info.
	// Uses /sys/class/drm/card*/device/vendor as primary detection method.
	DetectGPUVendor(ctx context.Context) GPUVendor

	// ApplyEnvironment sets all rendering environment variables.
	// Must be called before any GTK/GStreamer initialization.
	ApplyEnvironment(ctx context.Context, settings RenderingEnvSettings) error

	// GetAppliedVars returns a map of environment variables that were set.
	// Useful for logging and debugging.
	GetAppliedVars() map[string]string
}
