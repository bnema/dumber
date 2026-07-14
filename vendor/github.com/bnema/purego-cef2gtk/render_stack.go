package cef2gtk

import (
	"fmt"
	"os"
	"strings"
)

// RenderStack selects a coherent accelerated CEF/GTK GPU render stack.
type RenderStack string

const (
	// RenderStackVulkan uses GDK DMABUF presentation with ANGLE Vulkan and GSK Vulkan.
	RenderStackVulkan RenderStack = "vulkan"
	// RenderStackEGL uses GtkGLArea presentation with ANGLE GL/EGL and GSK OpenGL.
	RenderStackEGL RenderStack = "egl"
)

// RenderStackPlan is the low-level plan for a selected accelerated render stack.
type RenderStackPlan struct {
	Stack           RenderStack
	Backend         Backend
	ANGLEBackend    string
	GSKRenderer     string
	OSRBackingScale string
}

// ResolveRenderStack resolves a public render-stack choice to the low-level
// purego-cef2gtk options and process-global GTK/CEF hints it requires.
func ResolveRenderStack(stack RenderStack) (RenderStackPlan, error) {
	switch normalizeRenderStack(stack) {
	case "", RenderStackVulkan:
		return RenderStackPlan{
			Stack:           RenderStackVulkan,
			Backend:         BackendGDKDMABUF,
			ANGLEBackend:    "vulkan",
			GSKRenderer:     "vulkan",
			OSRBackingScale: "auto",
		}, nil
	case RenderStackEGL:
		return RenderStackPlan{
			Stack:           RenderStackEGL,
			Backend:         BackendGLArea,
			ANGLEBackend:    "gl-egl",
			GSKRenderer:     "opengl",
			OSRBackingScale: "off",
		}, nil
	default:
		return RenderStackPlan{}, fmt.Errorf("unsupported render stack %q", stack)
	}
}

// ConfigureRenderStackEnvironment applies process-global GTK/OSR environment
// hints from a resolved render stack plan. Call it before GTK is initialized
// when the host application wants purego-cef2gtk to own the coherent stack
// setup instead of setting those variables itself.
func ConfigureRenderStackEnvironment(plan RenderStackPlan) {
	if strings.TrimSpace(plan.GSKRenderer) != "" {
		_ = os.Setenv("GSK_RENDERER", plan.GSKRenderer)
	}
	if strings.TrimSpace(plan.OSRBackingScale) != "" {
		_ = os.Setenv(osrBackingScaleEnvVar, plan.OSRBackingScale)
	}
}

func normalizeRenderStack(stack RenderStack) RenderStack {
	return RenderStack(strings.ToLower(strings.TrimSpace(string(stack))))
}
