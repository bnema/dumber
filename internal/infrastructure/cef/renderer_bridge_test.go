package cef

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRendererBridgeExtensionJS_DoesNotExposeWritableGlobalDispatch(t *testing.T) {
	require.NotContains(t, rendererBridgeExtensionJS, "window.__dumberNativeBridge")
	require.NotContains(t, rendererBridgeExtensionJS, "DumberBridgeDispatch =")
	require.NotContains(t, rendererBridgeExtensionJS, "window.DumberBridgeDispatch")
	require.NotContains(t, rendererBridgeExtensionJS, "window.__dumberBridgeAction")
	require.NotContains(t, rendererBridgeExtensionJS, "window.__dumberBridgePayload")
}

func TestRendererBridgeExtensionJS_UsesNativeDispatchInExtensionScope(t *testing.T) {
	require.Contains(t, rendererBridgeExtensionJS, "native function Dispatch(action, payload);")
	require.Contains(t, rendererBridgeExtensionJS, "return Dispatch(action, payload == null ? '' : String(payload));")
	require.Contains(t, rendererBridgeExtensionJS, "send('bridge_ready',")
}

func TestRendererBridgeExtensionJS_EncodesTrustedSuccessSemantics(t *testing.T) {
	require.Contains(t, rendererBridgeExtensionJS, "if (!e.isTrusted) return;")
	require.Contains(t, rendererBridgeExtensionJS, "setTimeout(function() {")
	require.NotContains(t, rendererBridgeExtensionJS, "document.execCommand = function")
	require.Contains(t, rendererBridgeExtensionJS, "return result;")
}

func TestRendererBridgeSourceDoesNotNeedGoLinkname(t *testing.T) {
	src, err := os.ReadFile("renderer_bridge.go")
	require.NoError(t, err)
	require.NotContains(t, string(src), "go:linkname")
}

func TestCEFContentInjectorSourceDoesNotKeepDeprecatedClipboardBridgeConstants(t *testing.T) {
	src, err := os.ReadFile("content_injector.go")
	require.NoError(t, err)
	require.NotContains(t, string(src), "autoCopySelectionBridgeJS")
	require.NotContains(t, string(src), "clipboardCopyBridgeJS")
	require.NotContains(t, string(src), "editableFocusBridgeJS")
}

func TestDecodeRendererBridgeExplicitTextCopyPayload(t *testing.T) {
	req, err := decodeRendererBridgeExplicitTextCopyPayload([]byte(`{"text":"copied text","action":"cut"}`))
	require.NoError(t, err)
	require.Equal(t, "copied text", req.Text)
	require.Equal(t, "cut", req.Action)
}
