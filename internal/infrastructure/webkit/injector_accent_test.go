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

func TestAccentDetectionScriptTracksLastFocusedEditableElement(t *testing.T) {
	assert.Contains(t,
		accentDetectionScript,
		"window.__dumber_lastEditableEl = e.target",
		"accent detection script must track the last focused editable element",
	)
	assert.Contains(t,
		accentDetectionScript,
		"document.addEventListener('focusin'",
		"accent detection script must register a focusin listener",
	)
}
