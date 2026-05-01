package cef

import (
	"context"
	"os"
	"testing"
)

func TestApplyDefaultRenderStackEnvironment_DefaultsToGDKDMABUFWithANGLEGL(t *testing.T) {
	t.Setenv(dumberRenderStackEnvVar, "")
	t.Setenv("GSK_RENDERER", "")
	t.Setenv("PUREGO_CEF2GTK_BACKEND", "")
	t.Setenv("PUREGO_CEF2GTK_ANGLE_BACKEND", "")

	got := applyDefaultRenderStackEnvironment(context.Background())

	if got != renderStackVulkanDMABUF {
		t.Fatalf("render stack = %q, want %q", got, renderStackVulkanDMABUF)
	}
	if got := os.Getenv("GSK_RENDERER"); got != "vulkan" {
		t.Fatalf("GSK_RENDERER = %q, want vulkan", got)
	}
	if got := os.Getenv("PUREGO_CEF2GTK_BACKEND"); got != "gdk-dmabuf" {
		t.Fatalf("PUREGO_CEF2GTK_BACKEND = %q, want gdk-dmabuf", got)
	}
	if got := os.Getenv("PUREGO_CEF2GTK_ANGLE_BACKEND"); got != "gl-egl" {
		t.Fatalf("PUREGO_CEF2GTK_ANGLE_BACKEND = %q, want gl-egl", got)
	}
}

func TestApplyDefaultRenderStackEnvironment_PreservesExplicitLowLevelOverrides(t *testing.T) {
	t.Setenv(dumberRenderStackEnvVar, "")
	t.Setenv("GSK_RENDERER", "ngl")
	t.Setenv("PUREGO_CEF2GTK_BACKEND", "glarea")
	t.Setenv("PUREGO_CEF2GTK_ANGLE_BACKEND", "gl-egl")

	got := applyDefaultRenderStackEnvironment(context.Background())

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

	got := applyDefaultRenderStackEnvironment(context.Background())

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
