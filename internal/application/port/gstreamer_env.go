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

// GStreamerEnvSettings contains settings for GStreamer environment configuration.
// This is a port-local type to avoid import cycles with config package.
type GStreamerEnvSettings struct {
	// ForceVSync enables vertical sync for video playback
	ForceVSync bool
	// GLRenderingMode controls OpenGL API selection: "auto", "gles2", "gl3", "none"
	GLRenderingMode string
	// GStreamerDebugLevel sets GStreamer debug verbosity (0-5)
	GStreamerDebugLevel int
	// VideoBufferSizeMB sets buffer size in megabytes for video streaming
	VideoBufferSizeMB int
	// QueueBufferTimeSec sets queue buffer time in seconds for prebuffering
	QueueBufferTimeSec int
}

// GStreamerEnvManager configures GStreamer environment variables
// based on system configuration and detected GPU vendor.
// Environment variables must be set BEFORE GTK/WebKit initialization.
type GStreamerEnvManager interface {
	// DetectGPUVendor identifies the primary GPU vendor from system info.
	// Uses /sys/class/drm/card*/device/vendor as primary detection method.
	DetectGPUVendor(ctx context.Context) GPUVendor

	// ApplyEnvironment sets GStreamer environment variables based on
	// the provided settings and detected GPU vendor.
	// Must be called before any GTK/GStreamer initialization.
	ApplyEnvironment(ctx context.Context, settings GStreamerEnvSettings) error

	// GetAppliedVars returns a map of environment variables that were set.
	// Useful for logging and debugging.
	GetAppliedVars() map[string]string
}
