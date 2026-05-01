package cef

import (
	"context"
	"os"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	renderenv "github.com/bnema/dumber/internal/infrastructure/env"
	"github.com/bnema/dumber/internal/logging"
)

const (
	dumberRenderStackEnvVar = "DUMBER_RENDER_STACK"

	renderStackAuto         = "auto"
	renderStackVulkanDMABUF = "vulkan-dmabuf"
	renderStackLegacyGL     = "legacy-gl"
	cef2gtkBackendEnvVar    = "PUREGO_CEF2GTK_BACKEND"
	cef2gtkAngleBackendVar  = "PUREGO_CEF2GTK_ANGLE_BACKEND"
	gskRendererEnvVar       = "GSK_RENDERER"
	cef2gtkBackendGDKDMABUF = "gdk-dmabuf"
	cef2gtkBackendGLArea    = "glarea"
	cef2gtkAngleVulkan      = "vulkan"
	cef2gtkAngleGLEGL       = "gl-egl"
	gskRendererVulkan       = "vulkan"
	gskRendererOpenGL       = "opengl"
)

// ApplyDefaultRenderStackEnvironment configures the process environment for
// Dumber's default CEF presentation stack before GTK/libadwaita initialize.
//
// DUMBER_RENDER_STACK accepts:
//   - auto or empty: use the GPU-first GDK DMABUF stack
//   - vulkan-dmabuf: force GDK DMABUF presentation with GSK Vulkan and CEF ANGLE Vulkan
//   - legacy-gl: force the older GtkGLArea/OpenGL bridge for diagnostics
//
// Low-level env vars remain diagnostic escape hatches: when explicitly set,
// they are preserved instead of overwritten.
func ApplyDefaultRenderStackEnvironment(ctx context.Context) string {
	return applyDefaultRenderStackEnvironment(ctx)
}

func applyDefaultRenderStackEnvironment(ctx context.Context) string {
	switch normalizeRenderStack(os.Getenv(dumberRenderStackEnvVar)) {
	case renderStackLegacyGL:
		setEnvDefault(ctx, gskRendererEnvVar, gskRendererOpenGL)
		setEnvDefault(ctx, cef2gtkBackendEnvVar, cef2gtkBackendGLArea)
		setEnvDefault(ctx, cef2gtkAngleBackendVar, cef2gtkAngleGLEGL)
		return renderStackLegacyGL
	default:
		setEnvDefault(ctx, gskRendererEnvVar, gskRendererVulkan)
		setEnvDefault(ctx, cef2gtkBackendEnvVar, cef2gtkBackendGDKDMABUF)
		setEnvDefault(ctx, cef2gtkAngleBackendVar, cef2gtkAngleVulkan)
		return renderStackVulkanDMABUF
	}
}

// ApplyDefaultHardwareDecodeEnvironment maps Dumber's media config to CEF's
// developer-facing VAAPI switch env var before CEF command-line callbacks run.
// Existing explicit env values are preserved as low-level escape hatches.
func ApplyDefaultHardwareDecodeEnvironment(ctx context.Context, cfg *config.Config) {
	if cfg == nil || cfg.Engine.ResolveEngineType() != config.EngineTypeCEF {
		return
	}
	if cfg.Media.HardwareDecodingMode == config.HardwareDecodingDisable {
		setEnvDefault(ctx, cefEnableVAAPIEnvVar, "0")
		return
	}
	setEnvDefault(ctx, cefEnableVAAPIEnvVar, "1")
	applyDefaultLIBVADriverEnvironment(ctx)
}

func applyDefaultLIBVADriverEnvironment(ctx context.Context) {
	if strings.TrimSpace(os.Getenv("LIBVA_DRIVER_NAME")) != "" {
		return
	}
	manager := renderenv.NewManager()
	switch manager.DetectGPUVendor(ctx) {
	case port.GPUVendorAMD:
		setEnvDefault(ctx, "LIBVA_DRIVER_NAME", "radeonsi")
	case port.GPUVendorIntel:
		setEnvDefault(ctx, "LIBVA_DRIVER_NAME", "iHD")
	case port.GPUVendorNVIDIA:
		if ctx == nil {
			ctx = context.Background()
		}
		logging.FromContext(ctx).Warn().
			Msg("cef: not defaulting LIBVA_DRIVER_NAME for NVIDIA; Chromium/CEF VAAPI support is driver-dependent")
	}
}

func normalizeRenderStack(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", renderStackAuto, renderStackVulkanDMABUF:
		return renderStackVulkanDMABUF
	case renderStackLegacyGL:
		return renderStackLegacyGL
	default:
		return renderStackVulkanDMABUF
	}
}

func setEnvDefault(ctx context.Context, key, value string) {
	if existing := strings.TrimSpace(os.Getenv(key)); existing != "" {
		if ctx == nil {
			ctx = context.Background()
		}
		logging.FromContext(ctx).Debug().
			Str("key", key).
			Str("value", existing).
			Msg("cef: preserving explicit render stack environment override")
		return
	}
	_ = os.Setenv(key, value)
}
