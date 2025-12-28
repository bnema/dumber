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
	// --- GStreamer settings (from MediaConfig) ---
	// ForceVSync enables vertical sync for video playback.
	ForceVSync bool
	// GLRenderingMode controls OpenGL API selection: "auto", "gles2", "gl3", "none".
	GLRenderingMode string
	// GStreamerDebugLevel sets GStreamer debug verbosity (0-5).
	GStreamerDebugLevel int
	// VideoBufferSizeMB sets buffer size in megabytes for video streaming.
	VideoBufferSizeMB int
	// QueueBufferTimeSec sets queue buffer time in seconds for prebuffering.
	QueueBufferTimeSec int

	// --- WebKit compositor settings (from RenderingConfig) ---
	DisableDMABufRenderer  bool
	ForceCompositingMode   bool
	DisableCompositingMode bool

	// --- GTK/GSK settings (from RenderingConfig) ---
	GSKRenderer    string
	DisableMipmaps bool
	PreferGL       bool

	// --- Debug settings ---
	ShowFPS      bool
	SampleMemory bool
	DebugFrames  bool

	// --- Skia rendering thread settings (from PerformanceConfig) ---
	// SkiaCPUPaintingThreads sets WEBKIT_SKIA_CPU_PAINTING_THREADS.
	// 0 means unset (use WebKit default).
	SkiaCPUPaintingThreads int
	// SkiaGPUPaintingThreads sets WEBKIT_SKIA_GPU_PAINTING_THREADS.
	// -1 means unset; 0 disables GPU tile painting.
	SkiaGPUPaintingThreads int
	// SkiaEnableCPURendering forces CPU rendering via WEBKIT_SKIA_ENABLE_CPU_RENDERING=1.
	SkiaEnableCPURendering bool
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
