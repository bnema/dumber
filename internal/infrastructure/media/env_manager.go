// Package media provides video playback configuration and diagnostics.
package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// EnvManager implements port.GStreamerEnvManager for configuring
// GStreamer environment variables based on GPU vendor and user config.
type EnvManager struct {
	appliedVars map[string]string
	gpuVendor   port.GPUVendor
}

// NewEnvManager creates a new GStreamer environment manager.
func NewEnvManager() *EnvManager {
	return &EnvManager{
		appliedVars: make(map[string]string),
		gpuVendor:   port.GPUVendorUnknown,
	}
}

// DetectGPUVendor identifies the primary GPU vendor from system info.
func (e *EnvManager) DetectGPUVendor(ctx context.Context) port.GPUVendor {
	log := logging.FromContext(ctx)

	// Try reading from /sys/class/drm/card*/device/vendor
	vendor := e.detectFromDRM()
	if vendor != port.GPUVendorUnknown {
		e.gpuVendor = vendor
		log.Debug().Str("vendor", string(vendor)).Msg("detected GPU vendor from DRM")
		return vendor
	}

	log.Debug().Msg("could not detect GPU vendor, using unknown")
	e.gpuVendor = port.GPUVendorUnknown
	return port.GPUVendorUnknown
}

// detectFromDRM reads GPU vendor from /sys/class/drm/card*/device/vendor.
func (*EnvManager) detectFromDRM() port.GPUVendor {
	const drmBase = "/sys/class/drm"
	// Check card0 first (primary GPU), then card1 if needed
	for _, card := range []string{"card0", "card1"} {
		vendorPath := filepath.Join(drmBase, card, "device", "vendor")
		data, err := os.ReadFile(vendorPath)
		if err != nil {
			continue
		}

		vendorID := strings.TrimSpace(string(data))
		switch vendorID {
		case "0x1002":
			return port.GPUVendorAMD
		case "0x8086":
			return port.GPUVendorIntel
		case "0x10de":
			return port.GPUVendorNVIDIA
		}
	}
	return port.GPUVendorUnknown
}

// ApplyEnvironment sets GStreamer environment variables based on settings and GPU.
// Must be called before GTK/WebKit initialization.
func (e *EnvManager) ApplyEnvironment(ctx context.Context, settings port.GStreamerEnvSettings) error {
	log := logging.FromContext(ctx)

	// Clear previous applied vars
	e.appliedVars = make(map[string]string)

	// 1. Apply VSync settings
	if settings.ForceVSync {
		e.setEnv("__GL_SYNC_TO_VBLANK", "1")
		e.setEnv("vblank_mode", "3") // Mesa DRI config: always sync
	}

	// 2. Apply GL rendering mode
	switch strings.ToLower(settings.GLRenderingMode) {
	case "gles2":
		e.setEnv("GST_GL_API", "gles2")
	case "gl3":
		e.setEnv("GST_GL_API", "opengl3")
	case "none":
		e.setEnv("GST_GL_API", "none")
	case "auto", "":
		// Don't set - let GStreamer decide
	}

	// 3. GStreamer debug level
	if settings.GStreamerDebugLevel > 0 {
		level := settings.GStreamerDebugLevel
		if level > 5 {
			level = 5 // Cap at max verbosity
		}
		e.setEnv("GST_DEBUG", fmt.Sprintf("%d", level))
	}

	// 4. GPU-specific optimizations (only if not already set by user)
	e.applyGPUSpecificEnv()

	// 5. Video buffer size (for smoother streaming)
	if settings.VideoBufferSizeMB > 0 {
		bufferBytes := settings.VideoBufferSizeMB * 1024 * 1024
		e.setEnv("GST_BUFFER_SIZE", fmt.Sprintf("%d", bufferBytes))
		e.setEnv("GST_QUEUE2_MAX_SIZE_BYTES", fmt.Sprintf("%d", bufferBytes))
	}

	// 6. Queue buffer time (prebuffering duration)
	if settings.QueueBufferTimeSec > 0 {
		// Convert seconds to nanoseconds for GStreamer
		bufferTimeNs := settings.QueueBufferTimeSec * 1_000_000_000
		e.setEnv("GST_QUEUE2_MAX_SIZE_TIME", fmt.Sprintf("%d", bufferTimeNs))
	}

	log.Debug().
		Interface("vars", e.appliedVars).
		Str("gpu", string(e.gpuVendor)).
		Msg("applied gstreamer environment")

	return nil
}

// applyGPUSpecificEnv sets GPU vendor-specific environment variables.
func (e *EnvManager) applyGPUSpecificEnv() {
	// Only set LIBVA_DRIVER_NAME if not already set by user
	if os.Getenv("LIBVA_DRIVER_NAME") != "" {
		return
	}

	switch e.gpuVendor {
	case port.GPUVendorAMD:
		// AMD uses radeonsi driver (Mesa VA-API)
		e.setEnv("LIBVA_DRIVER_NAME", "radeonsi")

	case port.GPUVendorIntel:
		// Intel uses iHD (modern) or i965 (legacy)
		// iHD is preferred for Gen 8+ (Broadwell and newer)
		e.setEnv("LIBVA_DRIVER_NAME", "iHD")

	case port.GPUVendorNVIDIA:
		// NVIDIA uses nvidia-vaapi-driver
		e.setEnv("LIBVA_DRIVER_NAME", "nvidia")
		// NVIDIA benefits from EGL platform for better performance
		if os.Getenv("GST_GL_PLATFORM") == "" {
			e.setEnv("GST_GL_PLATFORM", "egl")
		}
	}
}

// setEnv sets an environment variable and tracks it.
func (e *EnvManager) setEnv(key, value string) {
	_ = os.Setenv(key, value) // Error ignored: setenv rarely fails
	e.appliedVars[key] = value
}

// GetAppliedVars returns a map of environment variables that were set.
func (e *EnvManager) GetAppliedVars() map[string]string {
	// Return a copy to prevent mutation
	result := make(map[string]string, len(e.appliedVars))
	for k, v := range e.appliedVars {
		result[k] = v
	}
	return result
}

// Ensure EnvManager implements port.GStreamerEnvManager
var _ port.GStreamerEnvManager = (*EnvManager)(nil)
