package cef

import (
	"fmt"

	cef2gtk "github.com/bnema/purego-cef2gtk"

	"github.com/bnema/dumber/internal/application/port"
)

func resolveCEFRenderStackPlan(stack string) (cef2gtk.RenderStackPlan, error) {
	plan, err := cef2gtk.ResolveRenderStack(cef2gtk.RenderStack(stack))
	if err != nil {
		return cef2gtk.RenderStackPlan{}, fmt.Errorf("resolve CEF render stack: %w", err)
	}
	return plan, nil
}

// ApplyConfiguredRenderStackEnvironment applies Dumber's CEF render-stack
// process environment before GTK/libadwaita initialize.
func ApplyConfiguredRenderStackEnvironment(stack string, logger port.Logger) cef2gtk.RenderStackPlan {
	plan, err := resolveCEFRenderStackPlan(stack)
	if err != nil {
		logWarn(logger, "cef: invalid render stack, falling back to Vulkan", port.Field("error", err.Error()))
		plan, _ = cef2gtk.ResolveRenderStack(cef2gtk.RenderStackVulkan)
	}
	cef2gtk.ConfigureRenderStackEnvironment(plan)
	logInfo(logger, "cef: render stack environment configured",
		port.Field("render_stack", string(plan.Stack)),
		port.Field("render_backend", plan.Backend.String()),
		port.Field("angle_backend", plan.ANGLEBackend),
		port.Field("gsk_renderer", plan.GSKRenderer),
		port.Field("osr_backing_scale", plan.OSRBackingScale),
	)
	return plan
}
