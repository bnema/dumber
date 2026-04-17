package webkit

import (
	"testing"

	"github.com/bnema/puregotk/v4/webkit"
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

func TestExplicitCopyScriptCapturesClipboardOperations(t *testing.T) {
	script := buildExplicitCopyScript()

	assert.Contains(t, script, "explicit_text_copy")
	assert.Contains(t, script, "document.addEventListener('copy'")
	assert.Contains(t, script, "document.addEventListener('cut'")
	assert.Contains(t, script, "document.execCommand")
	assert.Contains(t, script, "navigator.clipboard.writeText")
}

func TestExplicitCopyScriptReadsInputAndTextareaSelectionFirst(t *testing.T) {
	script := buildExplicitCopyScript()

	assert.Contains(t, script, "document.activeElement")
	assert.Contains(t, script, "selectionStart")
	assert.Contains(t, script, "selectionEnd")
	assert.Contains(t, script, "INPUT")
	assert.Contains(t, script, "TEXTAREA")
}
