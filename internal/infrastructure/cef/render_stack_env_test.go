package cef

import (
	"context"
	"os"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
)

func TestApplyDefaultRenderStackEnvironment_DefaultsToGDKDMABUFWithANGLEVulkan(t *testing.T) {
	t.Setenv(dumberRenderStackEnvVar, "")
	t.Setenv("GSK_RENDERER", "")
	t.Setenv("PUREGO_CEF2GTK_BACKEND", "")
	t.Setenv("PUREGO_CEF2GTK_ANGLE_BACKEND", "")

	got := applyDefaultRenderStackEnvironment(nil)

	if got != renderStackVulkanDMABUF {
		t.Fatalf("render stack = %q, want %q", got, renderStackVulkanDMABUF)
	}
	if got := os.Getenv("GSK_RENDERER"); got != "vulkan" {
		t.Fatalf("GSK_RENDERER = %q, want vulkan", got)
	}
	if got := os.Getenv("PUREGO_CEF2GTK_BACKEND"); got != "gdk-dmabuf" {
		t.Fatalf("PUREGO_CEF2GTK_BACKEND = %q, want gdk-dmabuf", got)
	}
	if got := os.Getenv("PUREGO_CEF2GTK_ANGLE_BACKEND"); got != "vulkan" {
		t.Fatalf("PUREGO_CEF2GTK_ANGLE_BACKEND = %q, want vulkan", got)
	}
}

func TestApplyDefaultRenderStackEnvironment_OverridesConflictingLowLevelDefaults(t *testing.T) {
	t.Setenv(dumberRenderStackEnvVar, "")
	t.Setenv(dumberRenderStackAllowSplitEnvVar, "")
	t.Setenv("GSK_RENDERER", "ngl")
	t.Setenv("PUREGO_CEF2GTK_BACKEND", "glarea")
	t.Setenv("PUREGO_CEF2GTK_ANGLE_BACKEND", "gl-egl")

	got := applyDefaultRenderStackEnvironment(nil)

	if got != renderStackVulkanDMABUF {
		t.Fatalf("render stack = %q, want %q", got, renderStackVulkanDMABUF)
	}
	if got := os.Getenv("GSK_RENDERER"); got != "vulkan" {
		t.Fatalf("GSK_RENDERER = %q, want coherent vulkan default", got)
	}
	if got := os.Getenv("PUREGO_CEF2GTK_BACKEND"); got != "gdk-dmabuf" {
		t.Fatalf("PUREGO_CEF2GTK_BACKEND = %q, want coherent gdk-dmabuf default", got)
	}
	if got := os.Getenv("PUREGO_CEF2GTK_ANGLE_BACKEND"); got != "vulkan" {
		t.Fatalf("PUREGO_CEF2GTK_ANGLE_BACKEND = %q, want coherent vulkan default", got)
	}
}

func TestApplyDefaultRenderStackEnvironment_AllowSplitPreservesExplicitLowLevelOverrides(t *testing.T) {
	t.Setenv(dumberRenderStackEnvVar, "")
	t.Setenv(dumberRenderStackAllowSplitEnvVar, "1")
	t.Setenv("GSK_RENDERER", "ngl")
	t.Setenv("PUREGO_CEF2GTK_BACKEND", "glarea")
	t.Setenv("PUREGO_CEF2GTK_ANGLE_BACKEND", "gl-egl")

	got := applyDefaultRenderStackEnvironment(nil)

	if got != renderStackVulkanDMABUF {
		t.Fatalf("render stack = %q, want %q", got, renderStackVulkanDMABUF)
	}
	if got := os.Getenv("GSK_RENDERER"); got != "ngl" {
		t.Fatalf("GSK_RENDERER = %q, want explicit ngl", got)
	}
	if got := os.Getenv("PUREGO_CEF2GTK_BACKEND"); got != "glarea" {
		t.Fatalf("PUREGO_CEF2GTK_BACKEND = %q, want explicit glarea", got)
	}
	if got := os.Getenv("PUREGO_CEF2GTK_ANGLE_BACKEND"); got != "gl-egl" {
		t.Fatalf("PUREGO_CEF2GTK_ANGLE_BACKEND = %q, want explicit gl-egl", got)
	}
}

func TestApplyDefaultRenderStackEnvironment_LegacyGLUsesGLArea(t *testing.T) {
	t.Setenv(dumberRenderStackEnvVar, "legacy-gl")
	t.Setenv("GSK_RENDERER", "")
	t.Setenv("PUREGO_CEF2GTK_BACKEND", "")
	t.Setenv("PUREGO_CEF2GTK_ANGLE_BACKEND", "")

	got := applyDefaultRenderStackEnvironment(nil)

	if got != renderStackLegacyGL {
		t.Fatalf("render stack = %q, want %q", got, renderStackLegacyGL)
	}
	if got := os.Getenv("PUREGO_CEF2GTK_BACKEND"); got != "glarea" {
		t.Fatalf("PUREGO_CEF2GTK_BACKEND = %q, want glarea", got)
	}
	if got := os.Getenv("PUREGO_CEF2GTK_ANGLE_BACKEND"); got != "gl-egl" {
		t.Fatalf("PUREGO_CEF2GTK_ANGLE_BACKEND = %q, want gl-egl", got)
	}
}

func TestNormalizeRenderStackUnknownFallsBackToGDKDMABUF(t *testing.T) {
	got := normalizeRenderStack(nil, "vulkan-dmabuff")

	if got != renderStackVulkanDMABUF {
		t.Fatalf("render stack = %q, want %q", got, renderStackVulkanDMABUF)
	}
}

func TestApplyDefaultHardwareDecodeEnvironment_DefaultsCEFToVAAPIForAuto(t *testing.T) {
	t.Setenv(cefEnableVAAPIEnvVar, "")

	ApplyDefaultHardwareDecodeEnvironment(context.Background(), HardwareDecodeEnvironmentOptions{EngineType: cefEngineType})

	if got := os.Getenv(cefEnableVAAPIEnvVar); got != "1" {
		t.Fatalf("%s = %q, want 1", cefEnableVAAPIEnvVar, got)
	}
}

func TestApplyDefaultHardwareDecodeEnvironment_PreservesExplicitLIBVADriver(t *testing.T) {
	t.Setenv(cefEnableVAAPIEnvVar, "")
	t.Setenv("LIBVA_DRIVER_NAME", "custom-driver")

	ApplyDefaultHardwareDecodeEnvironment(context.Background(), HardwareDecodeEnvironmentOptions{
		EngineType:          cefEngineType,
		RenderingEnvManager: stubRenderingEnvManager{},
	})

	if got := os.Getenv("LIBVA_DRIVER_NAME"); got != "custom-driver" {
		t.Fatalf("LIBVA_DRIVER_NAME = %q, want explicit custom-driver", got)
	}
}

func TestApplyDefaultHardwareDecodeEnvironment_DisablesCEFVAAPIWhenMediaDisabled(t *testing.T) {
	t.Setenv(cefEnableVAAPIEnvVar, "")

	ApplyDefaultHardwareDecodeEnvironment(context.Background(), HardwareDecodeEnvironmentOptions{
		EngineType:               cefEngineType,
		HardwareDecodingDisabled: true,
	})

	if got := os.Getenv(cefEnableVAAPIEnvVar); got != "0" {
		t.Fatalf("%s = %q, want 0", cefEnableVAAPIEnvVar, got)
	}
}

func TestApplyDefaultHardwareDecodeEnvironment_PreservesExplicitCEFVAAPIOverride(t *testing.T) {
	t.Setenv(cefEnableVAAPIEnvVar, "0")

	ApplyDefaultHardwareDecodeEnvironment(context.Background(), HardwareDecodeEnvironmentOptions{EngineType: cefEngineType})

	if got := os.Getenv(cefEnableVAAPIEnvVar); got != "0" {
		t.Fatalf("%s = %q, want explicit 0", cefEnableVAAPIEnvVar, got)
	}
}

type stubRenderingEnvManager struct{}

func (stubRenderingEnvManager) DetectGPUVendor(context.Context) port.GPUVendor {
	return port.GPUVendorAMD
}
func (stubRenderingEnvManager) GetAppliedVars() map[string]string { return nil }
