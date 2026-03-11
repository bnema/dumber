package webkit

import (
	"testing"

	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/stretchr/testify/assert"
)

// TestAccentDetectionScriptInjectionMode verifies that the accent detection
// script is configured for all-frames injection (not top-frame only).
// This ensures the script runs in iframes as well as the top-level document.
func TestAccentDetectionScriptInjectionMode(t *testing.T) {
	assert.Equal(t,
		webkit.UserContentInjectAllFramesValue,
		accentDetectionInjectionMode,
		"accent detection script must inject into all frames, not just the top frame",
	)
}
