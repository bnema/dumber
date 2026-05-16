package cef

import (
	"context"
	"os"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
)

const (
	dumberRenderStackEnvVar           = "DUMBER_RENDER_STACK"
	dumberRenderStackAllowSplitEnvVar = "DUMBER_RENDER_STACK_ALLOW_SPLIT"

	cefEngineType                = "cef"
	renderStackAuto              = "auto"
	renderStackVulkanDMABUF      = "vulkan-dmabuf"
	renderStackLegacyGL          = "legacy-gl"
	cef2gtkBackendEnvVar         = "PUREGO_CEF2GTK_BACKEND"
	cef2gtkAngleBackendVar       = "PUREGO_CEF2GTK_ANGLE_BACKEND"
	cef2gtkOSRBackingScaleEnvVar = "PUREGO_CEF2GTK_OSR_BACKING_SCALE"
	gskRendererEnvVar            = "GSK_RENDERER"
	cef2gtkBackendGDKDMABUF      = "gdk-dmabuf"
	cef2gtkBackendGLArea         = "glarea"
	cef2gtkAngleVulkan           = "vulkan"
	cef2gtkAngleGLEGL            = "gl-egl"
	gskRendererVulkan            = "vulkan"
	gskRendererOpenGL            = "opengl"
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
func ApplyDefaultRenderStackEnvironment(logger port.Logger) string {
	return applyDefaultRenderStackEnvironment(logger)
}

func applyDefaultRenderStackEnvironment(logger port.Logger) string {
	stack := normalizeRenderStack(logger, os.Getenv(dumberRenderStackEnvVar))
	switch stack {
	case renderStackLegacyGL:
		setRenderStackEnv(logger, gskRendererEnvVar, gskRendererOpenGL)
		setRenderStackEnv(logger, cef2gtkBackendEnvVar, cef2gtkBackendGLArea)
		setRenderStackEnv(logger, cef2gtkAngleBackendVar, cef2gtkAngleGLEGL)
	default:
		stack = renderStackVulkanDMABUF
		setRenderStackEnv(logger, gskRendererEnvVar, gskRendererVulkan)
		setRenderStackEnv(logger, cef2gtkBackendEnvVar, cef2gtkBackendGDKDMABUF)
		setRenderStackEnv(logger, cef2gtkAngleBackendVar, cef2gtkAngleVulkan)
		setEnvDefault(logger, cef2gtkOSRBackingScaleEnvVar, "auto")
	}
	logRenderStackEnvironment(logger, stack)
	return stack
}

func logRenderStackEnvironment(logger port.Logger, stack string) {
	logInfo(logger, "cef: render stack environment configured",
		port.Field("render_stack", stack),
		port.Field("gsk_renderer", os.Getenv(gskRendererEnvVar)),
		port.Field("cef2gtk_backend", os.Getenv(cef2gtkBackendEnvVar)),
		port.Field("cef2gtk_angle_backend", os.Getenv(cef2gtkAngleBackendVar)),
		port.Field("cef2gtk_osr_backing_scale", os.Getenv(cef2gtkOSRBackingScaleEnvVar)),
		port.Field("split_stack_allowed", envBoolEnabled(dumberRenderStackAllowSplitEnvVar)),
	)
}

// HardwareDecodeEnvironmentOptions carries config-derived CEF environment
// decisions from the composition root without making the CEF adapter import
// sibling configuration or environment adapters directly.
type HardwareDecodeEnvironmentOptions struct {
	EngineType               string
	HardwareDecodingDisabled bool
	RenderingEnvManager      port.RenderingEnvManager
	Logger                   port.Logger
}

// ApplyDefaultHardwareDecodeEnvironment maps Dumber's media config to CEF's
// developer-facing VAAPI switch env var before CEF command-line callbacks run.
// Existing explicit env values are preserved as low-level escape hatches.
func ApplyDefaultHardwareDecodeEnvironment(ctx context.Context, opts HardwareDecodeEnvironmentOptions) {
	if opts.EngineType != cefEngineType {
		return
	}
	if opts.HardwareDecodingDisabled {
		setEnvDefault(opts.Logger, cefEnableVAAPIEnvVar, "0")
		return
	}
	setEnvDefault(opts.Logger, cefEnableVAAPIEnvVar, "1")
	applyDefaultLIBVADriverEnvironment(ctx, opts.RenderingEnvManager, opts.Logger)
}

func applyDefaultLIBVADriverEnvironment(ctx context.Context, manager port.RenderingEnvManager, logger port.Logger) {
	if strings.TrimSpace(os.Getenv("LIBVA_DRIVER_NAME")) != "" || manager == nil {
		return
	}
	switch manager.DetectGPUVendor(ctx) {
	case port.GPUVendorAMD:
		setEnvDefault(logger, "LIBVA_DRIVER_NAME", "radeonsi")
	case port.GPUVendorIntel:
		setEnvDefault(logger, "LIBVA_DRIVER_NAME", "iHD")
	case port.GPUVendorNVIDIA:
		logWarn(logger, "cef: not defaulting LIBVA_DRIVER_NAME for NVIDIA; Chromium/CEF VAAPI support is driver-dependent")
	}
}

func normalizeRenderStack(logger port.Logger, value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", renderStackAuto, renderStackVulkanDMABUF:
		return renderStackVulkanDMABUF
	case renderStackLegacyGL:
		return renderStackLegacyGL
	default:
		logWarn(logger, "cef: unknown render stack, falling back to default",
			port.Field("render_stack", value),
			port.Field("fallback", renderStackVulkanDMABUF),
		)
		return renderStackVulkanDMABUF
	}
}

func setRenderStackEnv(logger port.Logger, key, value string) {
	if existing := strings.TrimSpace(os.Getenv(key)); existing != "" {
		fields := []port.LogField{
			port.Field("key", key),
			port.Field("value", existing),
			port.Field("stack_value", value),
		}
		if envBoolEnabled(dumberRenderStackAllowSplitEnvVar) {
			logWarn(logger, "cef: preserving explicit split render stack environment override", fields...)
			return
		}
		logWarn(logger, "cef: overriding low-level render stack environment to keep Dumber stack coherent", fields...)
	}
	_ = os.Setenv(key, value)
}

func setEnvDefault(logger port.Logger, key, value string) {
	if existing := strings.TrimSpace(os.Getenv(key)); existing != "" {
		logDebug(logger, "cef: preserving explicit environment override",
			port.Field("key", key),
			port.Field("value", existing),
		)
		return
	}
	_ = os.Setenv(key, value)
}

func logDebug(logger port.Logger, msg string, fields ...port.LogField) {
	if logger != nil {
		logger.Debug(msg, fields...)
	}
}

func logInfo(logger port.Logger, msg string, fields ...port.LogField) {
	if logger != nil {
		logger.Info(msg, fields...)
	}
}

func logWarn(logger port.Logger, msg string, fields ...port.LogField) {
	if logger != nil {
		logger.Warn(msg, fields...)
	}
}
