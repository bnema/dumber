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
	require.Contains(t, clipboardSelectionFetchBridgeJS, "isEditable(document.activeElement)")
	require.Contains(t, clipboardSelectionFetchBridgeJS, "sendFocusSync()")
}
