package cef2gtk

import (
	"github.com/bnema/purego-cef/cef"

	"github.com/bnema/purego-cef2gtk/internal/cefadapter"
)

// CommandLineOptions configures CEF command-line setup for the GTK bridge.
type CommandLineOptions struct {
	// RenderStackPlan provides the resolved render stack. When empty,
	// ConfigureCommandLine defaults to the Vulkan stack unless the diagnostic
	// PUREGO_CEF2GTK_ANGLE_BACKEND environment override is set.
	RenderStackPlan RenderStackPlan
}

// ConfigureCommandLine configures the CEF command line for Wayland accelerated
// rendering. It is intended to be called from App.OnBeforeCommandLineProcessing.
func ConfigureCommandLine(commandLine cef.CommandLine, opts CommandLineOptions) {
	cefadapter.ConfigureWaylandGPUCommandLineWithOptions(commandLine, cefadapter.CommandLineOptions{
		ANGLEBackend: opts.RenderStackPlan.ANGLEBackend,
	})
}
