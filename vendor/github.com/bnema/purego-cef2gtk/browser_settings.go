package cef2gtk

import "github.com/bnema/purego-cef/cef"

// BrowserSettingsOptions configures CEF browser settings used by the GTK bridge.
type BrowserSettingsOptions struct {
	// WindowlessFrameRate is CEF's maximum frame rate for off-screen rendering.
	// Values <= 0 leave the caller's setting unchanged.
	WindowlessFrameRate int32
}

// ConfigureBrowserSettings applies purego-cef2gtk browser-setting options.
func ConfigureBrowserSettings(settings *cef.BrowserSettings, opts BrowserSettingsOptions) {
	if settings == nil {
		return
	}
	if opts.WindowlessFrameRate > 0 {
		settings.WindowlessFrameRate = opts.WindowlessFrameRate
	}
}
