package cef

import (
	"testing"

	cef2gtk "github.com/bnema/purego-cef2gtk"
)

func TestResolveCEFRenderStackPlan(t *testing.T) {
	tests := []struct {
		name      string
		stack     string
		wantStack cef2gtk.RenderStack
		wantAngle string
	}{
		{name: "vulkan", stack: "vulkan", wantStack: cef2gtk.RenderStackVulkan, wantAngle: "vulkan"},
		{name: "egl", stack: "egl", wantStack: cef2gtk.RenderStackEGL, wantAngle: "gl-egl"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := resolveCEFRenderStackPlan(tt.stack)
			if err != nil {
				t.Fatalf("resolveCEFRenderStackPlan(%q) error = %v", tt.stack, err)
			}
			if plan.Stack != tt.wantStack {
				t.Fatalf("Stack = %q, want %q", plan.Stack, tt.wantStack)
			}
			if plan.ANGLEBackend != tt.wantAngle {
				t.Fatalf("ANGLEBackend = %q, want %q", plan.ANGLEBackend, tt.wantAngle)
			}
		})
	}
}

func TestConfigureCommandLineWithRenderStackUsesPlan(t *testing.T) {
	plan, err := resolveCEFRenderStackPlan("egl")
	if err != nil {
		t.Fatalf("resolveCEFRenderStackPlan(egl) error = %v", err)
	}
	commandLine := newMutableCommandLineStub()

	configureCommandLineWithRenderStack(commandLine, plan)

	if got := commandLine.GetSwitchValue("use-angle"); got != "gl-egl" {
		t.Fatalf("use-angle = %q, want gl-egl", got)
	}
}
