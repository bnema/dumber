package cef2gtk

import (
	"github.com/bnema/purego-cef/cef"

	"github.com/bnema/purego-cef2gtk/internal/cefadapter"
)

// WindowInfoOptions configures CEF window-info setup for the GTK bridge.
type WindowInfoOptions struct {
	// Parent is the native parent window handle passed to CEF.
	Parent uintptr
}

// ConfigureWindowInfo configures CEF window info for accelerated windowless
// rendering with shared textures.
func ConfigureWindowInfo(info *cef.WindowInfo, opts WindowInfoOptions) {
	cefadapter.ConfigureAcceleratedWindowInfo(info, opts.Parent)
}
