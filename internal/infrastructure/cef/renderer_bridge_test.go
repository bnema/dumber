package cef

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testBridgeProcessMessage struct {
	name string
	args *testBridgeListValue
}

type testBridgeListValue struct {
	values map[int]string
}

func newTestBridgeProcessMessage(name string, editable bool) *testBridgeProcessMessage {
	msg := &testBridgeProcessMessage{name: name, args: &testBridgeListValue{values: map[int]string{}}}
	msg.args.SetString(0, rendererBridgeActionEditableFocusChanged)
	if editable {
		msg.args.SetString(1, "1")
	} else {
		msg.args.SetString(1, "0")
	}
	return msg
}

func (m *testBridgeProcessMessage) IsValid() bool                                     { return true }
func (m *testBridgeProcessMessage) IsReadOnly() bool                                  { return false }
func (m *testBridgeProcessMessage) Copy() purecef.ProcessMessage                      { return m }
func (m *testBridgeProcessMessage) GetName() string                                   { return m.name }
func (m *testBridgeProcessMessage) GetArgumentList() purecef.ListValue                { return m.args }
func (m *testBridgeProcessMessage) GetSharedMemoryRegion() purecef.SharedMemoryRegion { return nil }

func (v *testBridgeListValue) IsValid() bool                      { return true }
func (v *testBridgeListValue) IsOwned() bool                      { return true }
func (v *testBridgeListValue) IsReadOnly() bool                   { return false }
func (v *testBridgeListValue) IsSame(that purecef.ListValue) bool { return v == that }
func (v *testBridgeListValue) IsEqual(that purecef.ListValue) bool {
	return v == that
}
func (v *testBridgeListValue) Copy() purecef.ListValue         { return v }
func (v *testBridgeListValue) SetSize(_ int) int32             { return 0 }
func (v *testBridgeListValue) GetSize() int                    { return len(v.values) }
func (v *testBridgeListValue) Clear() int32                    { v.values = map[int]string{}; return 0 }
func (v *testBridgeListValue) Remove(index int) int32          { delete(v.values, index); return 0 }
func (v *testBridgeListValue) GetType(_ int) purecef.ValueType { return 0 }
func (v *testBridgeListValue) GetValue(_ int) purecef.Value    { return nil }
func (v *testBridgeListValue) GetBool(_ int) int32             { return 0 }
func (v *testBridgeListValue) GetInt(_ int) int32              { return 0 }
func (v *testBridgeListValue) GetDouble(_ int) float64         { return 0 }
func (v *testBridgeListValue) GetString(index int) string {
	if v.values == nil {
		return ""
	}
	return v.values[index]
}
func (v *testBridgeListValue) GetBinary(_ int) purecef.BinaryValue         { return nil }
func (v *testBridgeListValue) GetDictionary(_ int) purecef.DictionaryValue { return nil }
func (v *testBridgeListValue) GetList(_ int) purecef.ListValue             { return nil }
func (v *testBridgeListValue) SetValue(_ int, _ purecef.Value) int32       { return 0 }
func (v *testBridgeListValue) SetNull(_ int) int32                         { return 0 }
func (v *testBridgeListValue) SetBool(_ int, _ int32) int32                { return 0 }
func (v *testBridgeListValue) SetInt(_ int, _ int32) int32                 { return 0 }
func (v *testBridgeListValue) SetDouble(_ int, _ float64) int32            { return 0 }
func (v *testBridgeListValue) SetString(index int, value string) int32 {
	if v.values == nil {
		v.values = map[int]string{}
	}
	v.values[index] = value
	return 0
}
func (v *testBridgeListValue) SetBinary(_ int, _ purecef.BinaryValue) int32         { return 0 }
func (v *testBridgeListValue) SetDictionary(_ int, _ purecef.DictionaryValue) int32 { return 0 }
func (v *testBridgeListValue) SetList(_ int, _ purecef.ListValue) int32             { return 0 }

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

func TestRendererBridgePackageDoesNotNeedGoLinkname(t *testing.T) {
	files := parseCEFPackageFiles(t)
	for _, file := range files {
		for _, group := range file.Comments {
			for _, comment := range group.List {
				require.NotContains(t, comment.Text, "go:linkname")
			}
		}
	}
}

func TestCEFPackageDoesNotKeepDeprecatedClipboardBridgeConstants(t *testing.T) {
	files := parseCEFPackageFiles(t)
	for _, banned := range []string{"autoCopySelectionBridgeJS", "clipboardCopyBridgeJS", "editableFocusBridgeJS"} {
		require.False(t, packageDeclaresIdentifier(files, banned), "unexpected deprecated identifier %q", banned)
	}
}

func TestDecodeRendererBridgeExplicitTextCopyPayload(t *testing.T) {
	req, err := decodeRendererBridgeExplicitTextCopyPayload([]byte(`{"text":"copied text","action":"cut"}`))
	require.NoError(t, err)
	require.Equal(t, "copied text", req.Text)
	require.Equal(t, "cut", req.Action)
}

func parseCEFPackageFiles(t *testing.T) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		require.NoError(t, err)
		if file.Name != nil && file.Name.Name == "cef" {
			files = append(files, file)
		}
	}
	require.NotEmpty(t, files)
	return files
}

func packageDeclaresIdentifier(files []*ast.File, name string) bool {
	for _, file := range files {
		found := false
		ast.Inspect(file, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if ok && ident.Name == name {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func TestRendererBridgeProcessHandler_OnFocusedNodeChanged_ReportsEditableState(t *testing.T) {
	oldFactory := newRendererBridgeProcessMessage
	t.Cleanup(func() { newRendererBridgeProcessMessage = oldFactory })
	newRendererBridgeProcessMessage = func(name string) purecef.ProcessMessage {
		return newTestBridgeProcessMessage(name, true)
	}

	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().SendProcessMessage(purecef.ProcessIDPidBrowser, mock.Anything).Run(func(_ purecef.ProcessID, message purecef.ProcessMessage) {
		require.Equal(t, rendererBridgeMessageName, message.GetName())
		args := message.GetArgumentList()
		require.Equal(t, "editable_focus_changed", args.GetString(0))
		require.Equal(t, "1", args.GetString(1))
	}).Once()

	(&rendererBridgeProcessHandler{}).OnFocusedNodeChanged(nil, frame, stubEditableDomnode{editable: true})
}
