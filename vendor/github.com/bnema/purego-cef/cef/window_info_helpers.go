package cef

import "github.com/bnema/purego-cef/internal/capi"

// WindowHandle mirrors cef_window_handle_t for callers that need to populate
// window-related fields in CEF structs without reaching into internal/capi.
type WindowHandle = capi.CEFWindowHandleT

// SetAsWindowless mirrors CefWindowInfo::SetAsWindowless for callers using the
// generated Go struct directly.
func SetAsWindowless(windowInfo *WindowInfo, parentWindow WindowHandle, sharedTexture bool) {
	if windowInfo == nil {
		return
	}
	windowInfo.ParentWindow = parentWindow
	windowInfo.WindowlessRenderingEnabled = 1
	windowInfo.SharedTextureEnabled = boolToInt32(sharedTexture)
}

func boolToInt32(v bool) int32 {
	if v {
		return 1
	}
	return 0
}
