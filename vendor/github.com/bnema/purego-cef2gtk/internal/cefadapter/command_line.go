package cefadapter

import (
	"os"
	"strings"

	"github.com/bnema/purego-cef/cef"
)

const (
	ozonePlatformSwitch  = "ozone-platform"
	useAngleSwitch       = "use-angle"
	useGLSwitch          = "use-gl"
	enableFeaturesSwitch = "enable-features"

	angleBackendEnvVar = "PUREGO_CEF2GTK_ANGLE_BACKEND"
)

// CommandLineOptions configures CEF command-line switches required for Wayland
// accelerated rendering without adding software fallbacks.
type CommandLineOptions struct {
	// ANGLEBackend selects Chromium's ANGLE backend. Empty defaults to vulkan
	// unless PUREGO_CEF2GTK_ANGLE_BACKEND is set as a diagnostic override.
	ANGLEBackend string
}

// ConfigureWaylandGPUCommandLine configures CEF command-line switches required
// for Wayland accelerated rendering without adding software fallbacks.
func ConfigureWaylandGPUCommandLine(commandLine cef.CommandLine) {
	ConfigureWaylandGPUCommandLineWithOptions(commandLine, CommandLineOptions{})
}

// ConfigureWaylandGPUCommandLineWithOptions configures CEF command-line switches
// using typed options while preserving PUREGO_CEF2GTK_ANGLE_BACKEND as a
// diagnostic override when ANGLEBackend is empty.
func ConfigureWaylandGPUCommandLineWithOptions(commandLine cef.CommandLine, opts CommandLineOptions) {
	if commandLine == nil {
		return
	}
	angleBackend, angleBackendExplicit := resolveAngleBackend(opts.ANGLEBackend)
	if angleBackendExplicit {
		commandLine.RemoveSwitch(useAngleSwitch)
		commandLine.AppendSwitchWithValue(useAngleSwitch, angleBackend)
	} else if !commandLine.HasSwitch(useAngleSwitch) {
		commandLine.AppendSwitchWithValue(useAngleSwitch, angleBackend)
	}

	effectiveAngleBackend := normalizeExistingAngleBackend(commandLine.GetSwitchValue(useAngleSwitch))
	if effectiveAngleBackend == "none" {
		commandLine.RemoveSwitch(useGLSwitch)
	} else {
		if normalizeExistingAngleBackend(commandLine.GetSwitchValue(useGLSwitch)) != "angle" {
			commandLine.RemoveSwitch(useGLSwitch)
			commandLine.AppendSwitchWithValue(useGLSwitch, "angle")
		}
		if effectiveAngleBackend == "vulkan" {
			features := mergeCommaTokens(
				commandLine.GetSwitchValue(enableFeaturesSwitch),
				"Vulkan,DefaultANGLEVulkan,VulkanFromANGLE",
			)
			commandLine.RemoveSwitch(enableFeaturesSwitch)
			commandLine.AppendSwitchWithValue(enableFeaturesSwitch, features)
		}
	}
	if !commandLine.HasSwitch(ozonePlatformSwitch) {
		commandLine.AppendSwitchWithValue(ozonePlatformSwitch, "wayland")
	}
}

func resolveAngleBackend(option string) (backend string, explicit bool) {
	if strings.TrimSpace(option) != "" {
		return normalizeAngleBackend(option, "vulkan"), true
	}
	if value, explicit := os.LookupEnv(angleBackendEnvVar); explicit {
		return normalizeAngleBackend(value, "gl-egl"), true
	}
	return "vulkan", false
}

func normalizeAngleBackend(value, fallback string) string {
	normalized := normalizeExistingAngleBackend(value)
	switch normalized {
	case "vulkan", "gl-egl", "none":
		return normalized
	default:
		return fallback
	}
}

func normalizeExistingAngleBackend(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func mergeCommaTokens(existing, required string) string {
	seen := make(map[string]bool)
	out := make([]string, 0)
	for _, list := range []string{existing, required} {
		for _, token := range strings.Split(list, ",") {
			token = strings.TrimSpace(token)
			if token == "" || seen[token] {
				continue
			}
			seen[token] = true
			out = append(out, token)
		}
	}
	return strings.Join(out, ",")
}
