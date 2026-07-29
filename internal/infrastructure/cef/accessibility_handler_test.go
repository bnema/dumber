package cef

import (
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubCEFValue struct{}

func (stubCEFValue) IsValid() bool                               { return true }
func (stubCEFValue) IsOwned() bool                               { return false }
func (stubCEFValue) IsReadOnly() bool                            { return true }
func (stubCEFValue) IsSame(purecef.Value) bool                   { return false }
func (stubCEFValue) IsEqual(purecef.Value) bool                  { return false }
func (stubCEFValue) Copy() purecef.Value                         { return stubCEFValue{} }
func (stubCEFValue) GetType() purecef.ValueType                  { return 0 }
func (stubCEFValue) GetBool() int32                              { return 0 }
func (stubCEFValue) GetInt() int32                               { return 0 }
func (stubCEFValue) GetDouble() float64                          { return 0 }
func (stubCEFValue) GetString() string                           { return "" }
func (stubCEFValue) GetBinary() purecef.BinaryValue              { return nil }
func (stubCEFValue) GetDictionary() purecef.DictionaryValue      { return nil }
func (stubCEFValue) GetList() purecef.ListValue                  { return nil }
func (stubCEFValue) SetNull() int32                              { return 0 }
func (stubCEFValue) SetBool(int32) int32                         { return 0 }
func (stubCEFValue) SetInt(int32) int32                          { return 0 }
func (stubCEFValue) SetDouble(float64) int32                     { return 0 }
func (stubCEFValue) SetString(string) int32                      { return 0 }
func (stubCEFValue) SetBinary(purecef.BinaryValue) int32         { return 0 }
func (stubCEFValue) SetDictionary(purecef.DictionaryValue) int32 { return 0 }
func (stubCEFValue) SetList(purecef.ListValue) int32             { return 0 }

func TestRenderHandlerExposesAccessibilityAtWrapTime(t *testing.T) {
	h := &dumberRenderHandler{a11y: newAccessibilityHandler(nil)}
	if h.GetAccessibilityHandler() == nil {
		t.Fatal("CEF would omit the accessibility override at wrap time")
	}
}

func TestRenderHandlerExposesAccessibilityWhenMainExists(t *testing.T) {
	// Must fail open (nil) when a11y was not installed, even if main exists —
	// never fall back to h.main.GetAccessibilityHandler().
	mainHandler := cefmocks.NewMockRenderHandler(t)
	h := &dumberRenderHandler{main: mainHandler, a11y: nil}
	assert.Nil(t, h.GetAccessibilityHandler())
	mainHandler.AssertNotCalled(t, "GetAccessibilityHandler")

	h.a11y = newAccessibilityHandler(nil)
	require.NotNil(t, h.GetAccessibilityHandler())
	mainHandler.AssertNotCalled(t, "GetAccessibilityHandler")
}

func TestNewDumberRenderHandlerInstallsNonNilAccessibilityHandler(t *testing.T) {
	require.Nil(t, newDumberRenderHandler(nil))

	wv := &WebView{viewBridge: &Cef2gtkAdapter{}}
	rh := newDumberRenderHandler(wv)
	require.NotNil(t, rh)
	require.NotNil(t, rh.GetAccessibilityHandler())

	handler, ok := rh.GetAccessibilityHandler().(*accessibilityHandler)
	require.True(t, ok)
	assert.Nil(t, handler.sink)
}

func TestAccessibilityHandler_NilSinkSkipsWriteJSON(t *testing.T) {
	calls := 0
	handler, ok := newAccessibilityHandler(nil).(*accessibilityHandler)
	require.True(t, ok)
	handler.writeJSON = func(purecef.Value, purecef.JsonWriterOptions) string {
		calls++
		return `{"unexpected":true}`
	}

	handler.OnAccessibilityTreeChange(stubCEFValue{})
	handler.OnAccessibilityLocationChange(stubCEFValue{})
	assert.Equal(t, 0, calls)
}

func TestAccessibilityHandler_NilValueIgnored(t *testing.T) {
	calls := 0
	var got []accessibilityPayload
	handler, ok := newAccessibilityHandler(func(p accessibilityPayload) bool {
		got = append(got, p)
		return true
	}).(*accessibilityHandler)
	require.True(t, ok)
	handler.writeJSON = func(purecef.Value, purecef.JsonWriterOptions) string {
		calls++
		return `{}`
	}

	handler.OnAccessibilityTreeChange(nil)
	handler.OnAccessibilityLocationChange(nil)
	assert.Equal(t, 0, calls)
	assert.Empty(t, got)
}

func TestAccessibilityHandler_TreeAndLocationWriteJSONOnce(t *testing.T) {
	var writeCalls []string
	var got []accessibilityPayload
	handler, ok := newAccessibilityHandler(func(p accessibilityPayload) bool {
		got = append(got, p)
		return true
	}).(*accessibilityHandler)
	require.True(t, ok)
	handler.writeJSON = func(value purecef.Value, options purecef.JsonWriterOptions) string {
		require.Equal(t, purecef.JsonWriterOptionsJsonWriterDefault, options)
		require.NotNil(t, value)
		writeCalls = append(writeCalls, "write")
		return `{"ok":true}`
	}

	handler.OnAccessibilityTreeChange(stubCEFValue{})
	handler.OnAccessibilityLocationChange(stubCEFValue{})

	require.Len(t, writeCalls, 2)
	require.Len(t, got, 2)
	assert.Equal(t, "tree", got[0].Kind)
	assert.Equal(t, `{"ok":true}`, got[0].JSON)
	assert.Equal(t, len(`{"ok":true}`), got[0].Bytes)
	assert.GreaterOrEqual(t, got[0].SerializeNanos, int64(0))
	assert.Equal(t, "location", got[1].Kind)
	assert.Equal(t, `{"ok":true}`, got[1].JSON)
	assert.Equal(t, len(`{"ok":true}`), got[1].Bytes)
	assert.GreaterOrEqual(t, got[1].SerializeNanos, int64(0))
}

func TestAccessibilityHandler_ProductionNoCaptureZeroWriteJSON(t *testing.T) {
	calls := 0

	// Factory/production path without DUMBER_A11Y_CAPTURE=1: worker stays nil,
	// sink is truly nil, WriteJson must not run.
	wv := &WebView{viewBridge: &Cef2gtkAdapter{}}
	rh := newDumberRenderHandler(wv)
	require.NotNil(t, rh)

	handler, ok := rh.GetAccessibilityHandler().(*accessibilityHandler)
	require.True(t, ok)
	require.Nil(t, handler.sink)
	handler.writeJSON = func(purecef.Value, purecef.JsonWriterOptions) string {
		calls++
		return `{}`
	}

	handler.OnAccessibilityTreeChange(stubCEFValue{})
	handler.OnAccessibilityLocationChange(stubCEFValue{})
	assert.Equal(t, 0, calls)
}
