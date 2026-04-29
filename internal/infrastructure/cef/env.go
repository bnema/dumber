package cef

import (
	"os"
	"strings"
)

const (
	cefExternalBeginFrameEnvVar   = "DUMBER_CEF_EXTERNAL_BEGIN_FRAME"
	cefEnableWebAuthnUnsafeEnvVar = "DUMBER_CEF_ENABLE_WEBAUTHN_UNSAFE"
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

func externalBeginFrameEnabled() bool {
	return envBoolEnabled(cefExternalBeginFrameEnvVar)
}

func cefWebAuthnUnsafeEnabled() bool {
	return envBoolEnabled(cefEnableWebAuthnUnsafeEnvVar)
}
