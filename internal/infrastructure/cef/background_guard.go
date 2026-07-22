package cef

import (
	"fmt"

	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
)

// opaqueWhiteBackground is Chromium's standard windowless canvas fallback. It is
// baked into every BrowserSettings.BackgroundColor so pages that paint nothing
// get an opaque white canvas (never a theme-dark one). A zero value must not be
// used: in windowless/OSR mode a zero-alpha background enables transparent
// painting, which this engine does not want.
const opaqueWhiteBackground uint32 = 0xFFFFFFFF

// flashGuardCSSClassPrefix scopes each WebView's transient background provider to
// its own render widget so guards from different tabs never collide.
const flashGuardCSSClassPrefix = "dumber-cef-flash-guard-"

// colorByteMask isolates a single 8-bit channel from a packed ARGB value.
const colorByteMask = 0xFF

// applyBackgroundFlashGuard paints the theme background color behind the CEF
// render widget until the first frame is presented. It is the presentation-layer
// replacement for the former dark BrowserSettings.BackgroundColor: Chromium's
// canvas is now always opaque white, so the anti-white-flash lives here instead.
// The bridge widget (a GtkPicture/GtkGLArea) renders its CSS background until an
// opaque frame texture covers it, at which point markFirstFramePainted tears the
// guard down. Must run on the GTK main thread.
func (wv *WebView) applyBackgroundFlashGuard(widget *gtk.Widget) {
	if wv == nil || widget == nil {
		return
	}
	argb := wv.backgroundColor
	if argb == 0 {
		return // No theme color configured; nothing to guard against.
	}
	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}

	a := float64((argb>>24)&colorByteMask) / colorScale
	r := (argb >> 16) & colorByteMask
	g := (argb >> 8) & colorByteMask
	b := argb & colorByteMask
	class := fmt.Sprintf("%s%d", flashGuardCSSClassPrefix, uint64(wv.id))
	css := fmt.Sprintf(".%s { background-color: rgba(%d, %d, %d, %.4f); }", class, r, g, b, a)

	provider := gtk.NewCssProvider()
	if provider == nil {
		return
	}
	provider.LoadFromString(css)

	wv.bgGuardMu.Lock()
	defer wv.bgGuardMu.Unlock()
	if wv.firstFramePainted.Load() {
		return // First frame already arrived before the guard could be installed.
	}
	widget.AddCssClass(class)
	gtk.StyleContextAddProviderForDisplay(display, provider, uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION))
	wv.bgGuardProvider = provider
	wv.bgGuardWidget = widget
	wv.bgGuardClass = class
}

// markFirstFramePainted records that the first CEF frame has been presented and
// schedules teardown of the background flash guard so later frames render
// normally. Safe to call from any thread and idempotent.
func (wv *WebView) markFirstFramePainted() {
	if wv == nil || !wv.firstFramePainted.CompareAndSwap(false, true) {
		return
	}
	wv.runOnGTK(func() {
		wv.removeBackgroundFlashGuard()
	})
}

// removeBackgroundFlashGuard detaches the transient theme-background provider and
// clears the render widget's guard class. Must run on the GTK main thread. It is
// safe to call when no guard is installed.
func (wv *WebView) removeBackgroundFlashGuard() {
	if wv == nil {
		return
	}
	wv.bgGuardMu.Lock()
	provider := wv.bgGuardProvider
	widget := wv.bgGuardWidget
	class := wv.bgGuardClass
	wv.bgGuardProvider = nil
	wv.bgGuardWidget = nil
	wv.bgGuardClass = ""
	wv.bgGuardMu.Unlock()

	if provider == nil {
		return
	}
	if widget != nil && class != "" {
		widget.RemoveCssClass(class)
	}
	if display := gdk.DisplayGetDefault(); display != nil {
		gtk.StyleContextRemoveProviderForDisplay(display, provider)
	}
}
