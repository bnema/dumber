package cef

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func testClipboardSelectionFetchBridgeJS() string {
	return buildClipboardSelectionFetchBridgeJS("test-bridge-nonce")
}

func TestClipboardSelectionFetchBridgeJS_CapturesInputAndTextareaSelections(t *testing.T) {
	script := testClipboardSelectionFetchBridgeJS()

	require.Contains(t, script, "document.activeElement")
	require.Contains(t, script, "selectionStart")
	require.Contains(t, script, "selectionEnd")
	require.Contains(t, script, "window.getSelection()")
	require.Contains(t, script, "type !== 'password'")
}

func TestClipboardSelectionFetchBridgeJS_PostsTrustedFocusSyncRequests(t *testing.T) {
	script := testClipboardSelectionFetchBridgeJS()

	require.Contains(t, script, "dumb:///api/focus-sync")
	require.Contains(t, script, "X-Dumber-Bridge-Action")
	require.Contains(t, script, "X-Dumber-Bridge-Nonce")
	require.Contains(t, script, "focus-sync")
	require.Contains(t, script, "focusin")
	require.Contains(t, script, "event.isTrusted === false")
	require.Contains(t, script, "event && event.target")
	require.Contains(t, script, "document.activeElement")
	require.Contains(t, script, "isEditable(document.activeElement)")
	require.Contains(t, script, "sendFocusSync()")
}

func TestClipboardSelectionFetchBridgeJS_HandlesTrustedClipboardEvents(t *testing.T) {
	script := testClipboardSelectionFetchBridgeJS()

	require.Contains(t, script, "dumb:///api/clipboard-set")
	require.Contains(t, script, "X-Dumber-Body")
	require.Contains(t, script, "X-Dumber-Bridge-Nonce")
	require.Contains(t, script, "document.addEventListener('copy', function(event)")
	require.Contains(t, script, "document.addEventListener('cut', function(event)")
	require.Contains(t, script, "event.isTrusted === false")
}

func TestClipboardSelectionFetchBridgeJS_PatchesAsyncClipboardAPIs(t *testing.T) {
	script := testClipboardSelectionFetchBridgeJS()

	require.Contains(t, script, "navigator && navigator.clipboard")
	require.Contains(t, script, "clipboardProto.writeText")
	require.Contains(t, script, "clipboardObj.writeText")
	require.Contains(t, script, "clipboardProto.write")
	require.Contains(t, script, "clipboardObj.write")
	require.Contains(t, script, "sendToClipboard(normalized)")
	require.Contains(t, script, "mirrorClipboardItems(items)")
	require.NotContains(t, script, "__DUMBER_BRIDGE_NONCE__")
	require.Contains(t, script, "test-bridge-nonce")
}
