package cef

import (
	"context"
	"os"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

const (
	dumberRenderStackEnvVar           = "DUMBER_RENDER_STACK"
	dumberRenderStackAllowSplitEnvVar = "DUMBER_RENDER_STACK_ALLOW_SPLIT"

	cefEngineType           = "cef"
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
//   - empty/default or vulkan-dmabuf: use GDK DMABUF presentation with GSK Vulkan and CEF ANGLE Vulkan
//   - auto: alias for vulkan-dmabuf
//   - legacy-gl: force the older GtkGLArea/OpenGL bridge for diagnostics
//
// Low-level env vars are treated as library-development escape hatches only.
// Dumber keeps the selected stack coherent by default and overwrites conflicts;
// set DUMBER_RENDER_STACK_ALLOW_SPLIT=1 to preserve explicit split-stack values.
func ApplyDefaultRenderStackEnvironment(ctx context.Context) string {
	return applyDefaultRenderStackEnvironment(ctx)
}

func applyDefaultRenderStackEnvironment(ctx context.Context) string {
	stack := normalizeRenderStack(ctx, os.Getenv(dumberRenderStackEnvVar))
	switch stack {
	case renderStackLegacyGL:
		setRenderStackEnv(ctx, gskRendererEnvVar, gskRendererOpenGL)
		setRenderStackEnv(ctx, cef2gtkBackendEnvVar, cef2gtkBackendGLArea)
		setRenderStackEnv(ctx, cef2gtkAngleBackendVar, cef2gtkAngleGLEGL)
	default:
		stack = renderStackVulkanDMABUF
		setRenderStackEnv(ctx, gskRendererEnvVar, gskRendererVulkan)
		setRenderStackEnv(ctx, cef2gtkBackendEnvVar, cef2gtkBackendGDKDMABUF)
		setRenderStackEnv(ctx, cef2gtkAngleBackendVar, cef2gtkAngleVulkan)
	}
	logRenderStackEnvironment(ctx, stack)
	return stack
}

func logRenderStackEnvironment(ctx context.Context, stack string) {
	if ctx == nil {
		ctx = context.Background()
	}
	logging.FromContext(ctx).Info().
		Str("render_stack", stack).
		Str("gsk_renderer", os.Getenv(gskRendererEnvVar)).
		Str("cef2gtk_backend", os.Getenv(cef2gtkBackendEnvVar)).
		Str("cef2gtk_angle_backend", os.Getenv(cef2gtkAngleBackendVar)).
		Bool("split_stack_allowed", envBoolEnabled(dumberRenderStackAllowSplitEnvVar)).
		Msg("cef: render stack environment configured")
}

// HardwareDecodeEnvironmentOptions carries config-derived CEF environment
// decisions from the composition root without making the CEF adapter import
// sibling configuration or environment adapters directly.
type HardwareDecodeEnvironmentOptions struct {
	EngineType               string
	HardwareDecodingDisabled bool
	RenderingEnvManager      port.RenderingEnvManager
}

// ApplyDefaultHardwareDecodeEnvironment maps Dumber's media config to CEF's
// developer-facing VAAPI switch env var before CEF command-line callbacks run.
// Existing explicit env values are preserved as low-level escape hatches.
func ApplyDefaultHardwareDecodeEnvironment(ctx context.Context, opts HardwareDecodeEnvironmentOptions) {
	if opts.EngineType != cefEngineType {
		return
	}
	if opts.HardwareDecodingDisabled {
		setEnvDefault(ctx, cefEnableVAAPIEnvVar, "0")
		return
	}
	setEnvDefault(ctx, cefEnableVAAPIEnvVar, "1")
	applyDefaultLIBVADriverEnvironment(ctx, opts.RenderingEnvManager)
}

func applyDefaultLIBVADriverEnvironment(ctx context.Context, manager port.RenderingEnvManager) {
	if strings.TrimSpace(os.Getenv("LIBVA_DRIVER_NAME")) != "" || manager == nil {
		return
	}
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

func normalizeRenderStack(ctx context.Context, value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", renderStackAuto, renderStackVulkanDMABUF:
		return renderStackVulkanDMABUF
	case renderStackLegacyGL:
		return renderStackLegacyGL
	default:
		if ctx == nil {
			ctx = context.Background()
		}
		logging.FromContext(ctx).Warn().
			Str("render_stack", value).
			Str("fallback", renderStackVulkanDMABUF).
			Msg("cef: unknown render stack, falling back to default")
		return renderStackVulkanDMABUF
	}
}

func setRenderStackEnv(ctx context.Context, key, value string) {
	if existing := strings.TrimSpace(os.Getenv(key)); existing != "" {
		if ctx == nil {
			ctx = context.Background()
		}
		if envBoolEnabled(dumberRenderStackAllowSplitEnvVar) {
			logging.FromContext(ctx).Warn().
				Str("key", key).
				Str("value", existing).
				Str("stack_value", value).
				Msg("cef: preserving explicit split render stack environment override")
			return
		}
		logging.FromContext(ctx).Warn().
			Str("key", key).
			Str("value", existing).
			Str("stack_value", value).
			Msg("cef: overriding low-level render stack environment to keep Dumber stack coherent")
	}
	_ = os.Setenv(key, value)
}

func setEnvDefault(ctx context.Context, key, value string) {
	if existing := strings.TrimSpace(os.Getenv(key)); existing != "" {
		if ctx == nil {
			ctx = context.Background()
		}
		logging.FromContext(ctx).Debug().
			Str("key", key).
			Str("value", existing).
			Msg("cef: preserving explicit environment override")
		return
	}
	_ = os.Setenv(key, value)
}
