package cef

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClipboardSelectionFetchBridgeJS_CapturesInputAndTextareaSelections(t *testing.T) {
	require.Contains(t, clipboardSelectionFetchBridgeJS, "document.activeElement")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "selectionStart")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "selectionEnd")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "window.getSelection()")
}

func TestClipboardSelectionFetchBridgeJS_PostsTrustedFocusSyncRequests(t *testing.T) {
	require.Contains(t, clipboardSelectionFetchBridgeJS, "dumb:///api/focus-sync")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "X-Dumber-Bridge-Action")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "focus-sync")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "focusin")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "event.isTrusted === false")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "event && event.target")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "document.activeElement")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "isEditable(document.activeElement)")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "sendFocusSync()")
}

func TestClipboardSelectionFetchBridgeJS_PatchesAsyncClipboardAPIs(t *testing.T) {
	require.Contains(t, clipboardSelectionFetchBridgeJS, "navigator && navigator.clipboard")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "clipboardProto.writeText")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "clipboardObj.writeText")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "clipboardProto.write")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "clipboardObj.write")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "sendToClipboard(normalized)")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "mirrorClipboardItems(items)")
}
