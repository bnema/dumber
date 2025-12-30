// Package env provides environment variable management for rendering subsystems.
package env

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// IsFlatpak returns true if the application is running inside a Flatpak sandbox.
// The Flatpak runtime provides all required libraries, so host pkg-config checks
// should be skipped when running in this environment.
func IsFlatpak() bool {
	_, err := os.Stat("/.flatpak-info")
	return err == nil
}

// IsPacman returns true if the application was installed via pacman (including AUR).
// Packages managed by pacman should not self-update; use pacman/AUR helpers instead.
// Detection uses `pacman -Qo` to check if the binary is owned by a package.
func IsPacman() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return false
	}

	// Check if pacman exists and if it owns this binary
	pacman, err := exec.LookPath("pacman")
	if err != nil {
		return false
	}

	// pacman -Qo returns 0 if the file is owned by a package
	cmd := exec.Command(pacman, "-Qo", resolved)
	return cmd.Run() == nil
}

// Manager implements port.RenderingEnvManager for configuring
// environment variables for GStreamer, WebKit, and GTK/GSK.
type Manager struct {
	appliedVars map[string]string
	gpuVendor   port.GPUVendor
}

// NewManager creates a new environment manager.
func NewManager() *Manager {
	return &Manager{
		appliedVars: make(map[string]string),
		gpuVendor:   port.GPUVendorUnknown,
	}
}

// DetectGPUVendor identifies the primary GPU vendor from system info.
func (m *Manager) DetectGPUVendor(ctx context.Context) port.GPUVendor {
	log := logging.FromContext(ctx)

	vendor := m.detectFromDRM()
	if vendor != port.GPUVendorUnknown {
		m.gpuVendor = vendor
		log.Debug().Str("vendor", string(vendor)).Msg("detected GPU vendor from DRM")
		return vendor
	}

	log.Debug().Msg("could not detect GPU vendor, using unknown")
	m.gpuVendor = port.GPUVendorUnknown
	return port.GPUVendorUnknown
}

// ApplyEnvironment sets all rendering environment variables.
// Must be called before GTK/WebKit initialization.
func (m *Manager) ApplyEnvironment(ctx context.Context, settings port.RenderingEnvSettings) error {
	log := logging.FromContext(ctx)
	m.appliedVars = make(map[string]string)

	m.applyGStreamerEnv(settings)
	m.applyGPUSpecificEnv()
	m.applyWebKitEnv(settings)
	m.applyGTKEnv(settings)
	m.applyDebugEnv(settings)
	m.applySkiaEnv(settings)

	log.Debug().
		Interface("vars", m.appliedVars).
		Str("gpu", string(m.gpuVendor)).
		Msg("applied rendering environment")

	return nil
}

func (m *Manager) applyGStreamerEnv(settings port.RenderingEnvSettings) {
	// VSync
	if settings.ForceVSync {
		m.setEnv("__GL_SYNC_TO_VBLANK", "1")
		m.setEnv("vblank_mode", "3")
	}

	// GL API
	switch strings.ToLower(settings.GLRenderingMode) {
	case "gles2":
		m.setEnv("GST_GL_API", "gles2")
	case "gl3":
		m.setEnv("GST_GL_API", "opengl3")
	case "none":
		m.setEnv("GST_GL_API", "none")
	case "auto", "":
		// Don't set - let GStreamer decide
	}

	// GStreamer debug level
	if settings.GStreamerDebugLevel > 0 {
		level := settings.GStreamerDebugLevel
		if level > 5 {
			level = 5
		}
		m.setEnv("GST_DEBUG", fmt.Sprintf("%d", level))
	}

	// NOTE: GST_BUFFER_SIZE, GST_QUEUE2_MAX_SIZE_BYTES, GST_QUEUE2_MAX_SIZE_TIME
	// are NOT valid GStreamer environment variables. They are element properties
	// that must be set programmatically on individual GStreamer elements.
	// WebKitGTK manages its own GStreamer pipeline internally.
}

func (m *Manager) applyGPUSpecificEnv() {
	if os.Getenv("LIBVA_DRIVER_NAME") != "" {
		return
	}

	switch m.gpuVendor {
	case port.GPUVendorAMD:
		m.setEnv("LIBVA_DRIVER_NAME", "radeonsi")
	case port.GPUVendorIntel:
		m.setEnv("LIBVA_DRIVER_NAME", "iHD")
	case port.GPUVendorNVIDIA:
		m.setEnv("LIBVA_DRIVER_NAME", "nvidia")
		if os.Getenv("GST_GL_PLATFORM") == "" {
			m.setEnv("GST_GL_PLATFORM", "egl")
		}
	}
}

func (m *Manager) applyWebKitEnv(settings port.RenderingEnvSettings) {
	if settings.DisableDMABufRenderer {
		m.setEnv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
	}
	if settings.ForceCompositingMode {
		m.setEnv("WEBKIT_FORCE_COMPOSITING_MODE", "1")
	}
	if settings.DisableCompositingMode {
		m.setEnv("WEBKIT_DISABLE_COMPOSITING_MODE", "1")
	}
}

func (m *Manager) applyGTKEnv(settings port.RenderingEnvSettings) {
	renderer := strings.ToLower(strings.TrimSpace(settings.GSKRenderer))
	if renderer != "" && renderer != "auto" {
		m.setEnv("GSK_RENDERER", renderer)
	}

	var gskDisable []string
	if settings.DisableMipmaps {
		gskDisable = append(gskDisable, "mipmap")
	}
	if len(gskDisable) > 0 {
		m.setEnv("GSK_GPU_DISABLE", strings.Join(gskDisable, ","))
	}

	var gdkDebug []string
	if settings.PreferGL {
		gdkDebug = append(gdkDebug, "gl-prefer-gl")
	}
	if settings.DebugFrames {
		gdkDebug = append(gdkDebug, "frames")
	}
	if len(gdkDebug) > 0 {
		m.setEnv("GDK_DEBUG", strings.Join(gdkDebug, ","))
	}
}

func (m *Manager) applyDebugEnv(settings port.RenderingEnvSettings) {
	if settings.ShowFPS {
		m.setEnv("WEBKIT_SHOW_FPS", "1")
	}
	if settings.SampleMemory {
		m.setEnv("WEBKIT_SAMPLE_MEMORY", "1")
	}
}

func (m *Manager) applySkiaEnv(settings port.RenderingEnvSettings) {
	// CPU painting threads: only set if > 0 and env var not already set
	if settings.SkiaCPUPaintingThreads > 0 && os.Getenv("WEBKIT_SKIA_CPU_PAINTING_THREADS") == "" {
		m.setEnv("WEBKIT_SKIA_CPU_PAINTING_THREADS", fmt.Sprintf("%d", settings.SkiaCPUPaintingThreads))
	}

	// GPU painting threads: only set if >= 0 (0 is valid: disables GPU tile painting)
	if settings.SkiaGPUPaintingThreads >= 0 && os.Getenv("WEBKIT_SKIA_GPU_PAINTING_THREADS") == "" {
		m.setEnv("WEBKIT_SKIA_GPU_PAINTING_THREADS", fmt.Sprintf("%d", settings.SkiaGPUPaintingThreads))
	}

	// CPU rendering: only set if true and env var not already set
	if settings.SkiaEnableCPURendering && os.Getenv("WEBKIT_SKIA_ENABLE_CPU_RENDERING") == "" {
		m.setEnv("WEBKIT_SKIA_ENABLE_CPU_RENDERING", "1")
	}
}

func (m *Manager) setEnv(key, value string) {
	_ = os.Setenv(key, value)
	m.appliedVars[key] = value
}

// GetAppliedVars returns a copy of applied environment variables.
func (m *Manager) GetAppliedVars() map[string]string {
	result := make(map[string]string, len(m.appliedVars))
	for k, v := range m.appliedVars {
		result[k] = v
	}
	return result
}

func (*Manager) detectFromDRM() port.GPUVendor {
	const drmBase = "/sys/class/drm"
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

var _ port.RenderingEnvManager = (*Manager)(nil)
