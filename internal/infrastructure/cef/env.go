package cef

import (
	"os"
	"strconv"
	"strings"
)

const (
	cefExternalBeginFrameEnvVar   = "DUMBER_CEF_EXTERNAL_BEGIN_FRAME"
	cefWindowlessFrameRateEnvVar  = "DUMBER_CEF_WINDOWLESS_FRAME_RATE"
	cefEnableWebAuthnUnsafeEnvVar = "DUMBER_CEF_ENABLE_WEBAUTHN_UNSAFE"
	cefChromiumFlagsEnvVar        = "DUMBER_CEF_CHROMIUM_FLAGS"
	cefEnableVAAPIEnvVar          = "DUMBER_CEF_ENABLE_VAAPI"
	cefRenderNodeEnvVar           = "DUMBER_CEF_RENDER_NODE"
	cefRenderStallRecoveryEnvVar  = "DUMBER_CEF_RENDER_STALL_RECOVERY"
	cefRenderStallBacktraceEnvVar = "DUMBER_CEF_RENDER_STALL_BACKTRACE"
	cefScaleProbeEnvVar           = "DUMBER_CEF_SCALE_PROBE"
	cef2GTKTraceScaleEnvVar       = "PUREGO_CEF2GTK_TRACE_SCALE"
)

// envBoolEnabled returns true when the given environment variable is set
// to a truthy value ("1", "true", "yes", "on").
func envBoolEnabled(envVar string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envVar))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// externalBeginFrameEnabled reports whether CEF produces frames on BeginFrame
// ticks driven by the GTK frame clock instead of its own timer. It is opt-in:
// only an explicit true value enables it, and unset, empty or invalid values
// leave CEF's internal cadence in place.
func externalBeginFrameEnabled() bool {
	return envBoolEnabled(cefExternalBeginFrameEnvVar)
}

// windowlessFrameRateOverride returns an explicitly pinned OSR frame rate. A
// pinned rate takes precedence over the configured value and disables adaptive
// monitor-refresh polling, so a run can be compared at a single cadence.
func windowlessFrameRateOverride() (int32, bool) {
	raw := strings.TrimSpace(os.Getenv(cefWindowlessFrameRateEnvVar))
	if raw == "" {
		return 0, false
	}
	rate, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || rate <= 0 {
		return 0, false
	}
	return int32(rate), true
}

// windowlessFrameRatePinned reports whether the environment pins the OSR frame
// rate, which also suspends adaptive monitor-refresh polling.
func windowlessFrameRatePinned() bool {
	_, pinned := windowlessFrameRateOverride()
	return pinned
}

func cefWebAuthnUnsafeEnabled() bool {
	return envBoolEnabled(cefEnableWebAuthnUnsafeEnvVar)
}

func renderStallRecoveryEnabled() bool {
	return envBoolEnabled(cefRenderStallRecoveryEnvVar)
}

func renderStallBacktraceEnabled() bool {
	return envBoolEnabled(cefRenderStallBacktraceEnvVar)
}

func cefScaleProbeEnabled() bool {
	return envBoolEnabled(cefScaleProbeEnvVar) || os.Getenv(cef2GTKTraceScaleEnvVar) != ""
}
