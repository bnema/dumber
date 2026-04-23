package cef

import (
	"testing"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/require"
)

type testPopupOpenerDictionaryValue struct {
	strings map[string]string
	bools   map[string]int32
}

func (v testPopupOpenerDictionaryValue) IsValid() bool                        { return true }
func (v testPopupOpenerDictionaryValue) IsOwned() bool                        { return true }
func (v testPopupOpenerDictionaryValue) IsReadOnly() bool                     { return false }
func (v testPopupOpenerDictionaryValue) IsSame(purecef.DictionaryValue) bool  { return false }
func (v testPopupOpenerDictionaryValue) IsEqual(purecef.DictionaryValue) bool { return false }
func (v testPopupOpenerDictionaryValue) Copy(int32) purecef.DictionaryValue   { return v }
func (v testPopupOpenerDictionaryValue) GetSize() int                         { return len(v.strings) + len(v.bools) }
func (v testPopupOpenerDictionaryValue) Clear() int32                         { return 1 }
func (v testPopupOpenerDictionaryValue) HasKey(key string) bool {
	_, ok := v.strings[key]
	if ok {
		return true
	}
	_, ok = v.bools[key]
	return ok
}
func (v testPopupOpenerDictionaryValue) GetKeys(purecef.StringList) int32             { return 0 }
func (v testPopupOpenerDictionaryValue) Remove(string) int32                          { return 1 }
func (v testPopupOpenerDictionaryValue) GetType(string) purecef.ValueType             { return 0 }
func (v testPopupOpenerDictionaryValue) GetValue(string) purecef.Value                { return nil }
func (v testPopupOpenerDictionaryValue) GetBool(key string) int32                     { return v.bools[key] }
func (v testPopupOpenerDictionaryValue) GetInt(string) int32                          { return 0 }
func (v testPopupOpenerDictionaryValue) GetDouble(string) float64                     { return 0 }
func (v testPopupOpenerDictionaryValue) GetString(key string) string                  { return v.strings[key] }
func (v testPopupOpenerDictionaryValue) GetBinary(string) purecef.BinaryValue         { return nil }
func (v testPopupOpenerDictionaryValue) GetDictionary(string) purecef.DictionaryValue { return nil }
func (v testPopupOpenerDictionaryValue) GetList(string) purecef.ListValue             { return nil }
func (v testPopupOpenerDictionaryValue) SetValue(string, purecef.Value) int32         { return 1 }
func (v testPopupOpenerDictionaryValue) SetNull(string) int32                         { return 1 }
func (v testPopupOpenerDictionaryValue) SetBool(string, int32) int32                  { return 1 }
func (v testPopupOpenerDictionaryValue) SetInt(string, int32) int32                   { return 1 }
func (v testPopupOpenerDictionaryValue) SetDouble(string, float64) int32              { return 1 }
func (v testPopupOpenerDictionaryValue) SetString(string, string) int32               { return 1 }
func (v testPopupOpenerDictionaryValue) SetBinary(string, purecef.BinaryValue) int32  { return 1 }
func (v testPopupOpenerDictionaryValue) SetDictionary(string, purecef.DictionaryValue) int32 {
	return 1
}
func (v testPopupOpenerDictionaryValue) SetList(string, purecef.ListValue) int32 { return 1 }

type testPopupOpenerV8Context struct {
	valid      bool
	evalCode   string
	evalURL    string
	enterCalls int
	exitCalls  int
}

func (c *testPopupOpenerV8Context) GetTaskRunner() purecef.TaskRunner { return nil }
func (c *testPopupOpenerV8Context) IsValid() bool                     { return c.valid }
func (c *testPopupOpenerV8Context) GetBrowser() purecef.Browser       { return nil }
func (c *testPopupOpenerV8Context) GetFrame() purecef.Frame           { return nil }
func (c *testPopupOpenerV8Context) GetGlobal() purecef.V8Value        { return nil }
func (c *testPopupOpenerV8Context) Enter() int32 {
	c.enterCalls++
	return 1
}
func (c *testPopupOpenerV8Context) Exit() int32 {
	c.exitCalls++
	return 1
}
func (c *testPopupOpenerV8Context) IsSame(that purecef.V8Context) bool { return c == that }
func (c *testPopupOpenerV8Context) Eval(code string, scriptURL string, _ int32, _, _ unsafe.Pointer) int32 {
	c.evalCode = code
	c.evalURL = scriptURL
	return 1
}
func (c *testPopupOpenerV8Context) RawPointer() unsafe.Pointer { return nil }
func (c *testPopupOpenerV8Context) Release()                   {}

func TestDecodePopupOpenerRenderMetadata(t *testing.T) {
	metadata, ok := decodePopupOpenerRenderMetadata(testPopupOpenerDictionaryValue{
		strings: map[string]string{
			popupOpenerExtraInfoParentURIKey:   "https://example.com/login",
			popupOpenerExtraInfoBridgeNonceKey: "bridge-nonce",
		},
		bools: map[string]int32{
			popupOpenerExtraInfoEnabledKey: 1,
		},
	})

	require.True(t, ok)
	require.Equal(t, "https://example.com/login", metadata.ParentURI)
	require.Equal(t, "bridge-nonce", metadata.BridgeNonce)
}

func TestPopupOpenerRenderProcessHandler_OnContextCreatedInstallsBridgeForConfiguredMainFrame(t *testing.T) {
	h := newPopupOpenerRenderProcessHandler()
	browser := cefmocks.NewMockBrowser(t)
	browser.EXPECT().GetIdentifier().Return(int32(42)).Twice()
	ctx := &testPopupOpenerV8Context{valid: true}

	h.OnBrowserCreated(browser, testPopupOpenerDictionaryValue{
		strings: map[string]string{
			popupOpenerExtraInfoParentURIKey:   "https://example.com/login",
			popupOpenerExtraInfoBridgeNonceKey: "bridge-nonce",
		},
		bools: map[string]int32{
			popupOpenerExtraInfoEnabledKey: 1,
		},
	})
	h.OnContextCreated(browser, stubFrame{main: true, url: "https://example.com/popup"}, ctx)

	require.Equal(t, 1, ctx.enterCalls)
	require.Equal(t, 1, ctx.exitCalls)
	require.Contains(t, ctx.evalCode, "popup-opener-navigate")
	require.Contains(t, ctx.evalCode, "popup-opener-post-message")
	require.Contains(t, ctx.evalCode, "https://example.com/login")
	require.Contains(t, ctx.evalCode, "bridge-nonce")
	require.Equal(t, popupOpenerRenderScriptURL, ctx.evalURL)
}

func TestPopupOpenerRenderProcessHandler_OnContextCreatedSkipsWithoutMetadata(t *testing.T) {
	h := newPopupOpenerRenderProcessHandler()
	browser := cefmocks.NewMockBrowser(t)
	browser.EXPECT().GetIdentifier().Return(int32(7)).Once()
	ctx := &testPopupOpenerV8Context{valid: true}

	h.OnContextCreated(browser, stubFrame{main: true, url: "https://example.com/popup"}, ctx)

	require.Zero(t, ctx.enterCalls)
	require.Empty(t, ctx.evalCode)
}

func TestEnablePopupOpenerBridge_SyncsPendingCreateExtraInfo(t *testing.T) {
	oldBuilder := popupOpenerRenderExtraInfoBuilder
	popupOpenerRenderExtraInfoBuilder = func(parentURI, bridgeNonce string) purecef.DictionaryValue {
		require.Equal(t, "https://example.com/login", parentURI)
		require.NotEmpty(t, bridgeNonce)
		return testPopupOpenerDictionaryValue{strings: map[string]string{"ok": "1"}}
	}
	t.Cleanup(func() {
		popupOpenerRenderExtraInfoBuilder = oldBuilder
	})

	parent := &WebView{}
	parent.updateURI("https://example.com/login")
	windowInfo := purecef.NewWindowInfo()
	wv := &WebView{pendingCreate: &pendingBrowserCreate{windowInfo: &windowInfo}}

	wv.EnablePopupOpenerBridge(parent, false)

	require.NotNil(t, wv.pendingCreate.extraInfo)
	require.NotEmpty(t, wv.bridgeNonce)
}
