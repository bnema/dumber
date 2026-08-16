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
	script := buildAccentDetectionScript("")
	assert.Contains(t,
		script,
		"window.__dumber_lastEditableEl = e.target",
		"accent detection script must track the last focused editable element",
	)
	assert.Contains(t,
		script,
		"document.addEventListener('focusin'",
		"accent detection script must register a focusin listener",
	)
	assert.Contains(t, script, "el.isContentEditable")
	assert.Contains(t, script, "closest('[contenteditable]')")
}

func TestAccentDetectionScriptPostsEditableFocusChangedMessages(t *testing.T) {
	script := buildAccentDetectionScript("")
	assert.Contains(t, script, "editable_focus_changed")
	assert.Contains(t, script, "document.addEventListener('focusout'")
	assert.Contains(t, script, `const editableFocusToken = "";`)
	assert.Contains(t, script, `payload: { editable: editable, token: editableFocusToken }`)
	assert.Contains(t, script, "let lastEditableFocusState = null;")
	assert.Contains(t, script, "if (lastEditableFocusState === editable) return;")
	assert.Contains(t, script, `postEditableFocus(true)`)
	assert.Contains(t, script, `postEditableFocus(false)`)
	assert.Contains(t, script, `e && e.isTrusted === false`)
	assert.Contains(t, script, `document.activeElement`)
	assert.Contains(t, script, `window.__dumber_lastEditableEl = document.activeElement`)
}

func TestBuildAccentDetectionScript_EmptyTokenReturnsBeforeHandlerLookup(t *testing.T) {
	empty := buildAccentDetectionScript("")
	assert.Contains(t, empty, "if (!editableFocusToken) return;")
	nonEmpty := buildAccentDetectionScript("abc123")
	assert.Contains(t, nonEmpty, `const editableFocusToken = "abc123";`)
	assert.Contains(t, nonEmpty, "if (!editableFocusToken) return;")
	assert.Contains(t, nonEmpty, `postEditableFocus(true)`)
	assert.Contains(t, nonEmpty, "window.webkit.messageHandlers.dumber.postMessage")
}

func TestExplicitCopyScriptCapturesClipboardOperations(t *testing.T) {
	script := buildExplicitCopyScript()

	assert.Contains(t, script, "explicit_text_copy")
	assert.Contains(t, script, "document.addEventListener('copy'")
	assert.Contains(t, script, "document.addEventListener('cut'")
	assert.Contains(t, script, "document.execCommand")
	assert.Contains(t, script, "navigator.clipboard.writeText")
	assert.Contains(t, script, "navigator.userActivation")
	assert.Contains(t, script, "var userActivated = hasUserActivation()")
	assert.Contains(t, script, "if (userActivated)")
	assert.Contains(t, script, "try {")
	assert.Contains(t, script, "finally {")
	assert.Contains(t, script, "pendingCommand === command")
}

func TestExplicitCopyScriptReadsInputAndTextareaSelectionFirst(t *testing.T) {
	script := buildExplicitCopyScript()

	assert.Contains(t, script, "document.activeElement")
	assert.Contains(t, script, "selectionStart")
	assert.Contains(t, script, "selectionEnd")
	assert.Contains(t, script, "INPUT")
	assert.Contains(t, script, "TEXTAREA")
	assert.Contains(t, script, "type || '').toLowerCase()")
	assert.Contains(t, script, "case 'password':")
}
