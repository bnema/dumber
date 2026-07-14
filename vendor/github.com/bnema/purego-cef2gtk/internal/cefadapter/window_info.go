package cefadapter

import "github.com/bnema/purego-cef/cef"

// ConfigureAcceleratedWindowInfo configures CEF window info for windowless
// accelerated rendering with shared textures.
func ConfigureAcceleratedWindowInfo(info *cef.WindowInfo, parent uintptr) {
	if info == nil {
		return
	}
	cef.SetAsWindowless(info, cef.WindowHandle(parent), true)
}
