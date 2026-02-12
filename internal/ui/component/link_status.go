// Package component provides UI components for the browser.
package component

import (
	"sync"

	"github.com/bnema/dumber/internal/ui/mainloop"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/ui/layout"
)

const (
	// linkStatusShowDelayMs is the delay before showing the overlay to avoid flicker.
	linkStatusShowDelayMs = 100
	// linkStatusAutoHideMs is the delay before auto-hiding the overlay (e.g., during fullscreen video).
	linkStatusAutoHideMs = 10000
	// linkStatusMaxChars is the maximum characters before truncation.
	linkStatusMaxChars = 80
)

// LinkStatusOverlay displays the destination URL when hovering over links.
// It appears in the bottom-left corner with a fade-in/fade-out transition.
type LinkStatusOverlay struct {
	factory   layout.WidgetFactory
	container layout.BoxWidget
	label     layout.LabelWidget

	pendingURI    string // URI waiting to be shown after delay
	showTimer     uint   // GLib timer for delayed show
	autoHideTimer uint   // GLib timer for auto-hide after timeout
	visible       bool   // Whether overlay is currently visible (has .visible class)
	showTimerCb   glib.SourceFunc
	autoHideCb    glib.SourceFunc
	coalescer     *mainloop.Coalescer
	mu            sync.Mutex
}

// NewLinkStatusOverlay creates a new link status overlay component.
// The overlay is positioned at bottom-left with fade transitions via CSS.
func NewLinkStatusOverlay(factory layout.WidgetFactory) *LinkStatusOverlay {
	// Create container box for styling and positioning
	container := factory.NewBox(layout.OrientationHorizontal, 0)
	container.AddCssClass("link-status")

	// Position at bottom-left
	container.SetHalign(gtk.AlignStartValue)
	container.SetValign(gtk.AlignEndValue)

	// Don't expand to fill space
	container.SetHexpand(false)
	container.SetVexpand(false)

	// Don't intercept pointer events - let clicks pass through
	container.SetCanTarget(false)
	container.SetCanFocus(false)

	// Create label for URL text
	label := factory.NewLabel("")
	label.SetCanTarget(false)
	label.SetCanFocus(false)
	label.SetEllipsize(layout.EllipsizeMiddle) // Truncate in middle for URLs
	label.SetMaxWidthChars(linkStatusMaxChars)
	container.Append(label)

	overlay := &LinkStatusOverlay{
		factory:   factory,
		container: container,
		label:     label,
		visible:   false,
	}
	overlay.coalescer = mainloop.NewCoalescer(func(fn func()) {
		var cb glib.SourceFunc = func(uintptr) bool {
			fn()
			return false
		}
		glib.IdleAdd(&cb, 0)
	})
	overlay.showTimerCb = func(_ uintptr) bool {
		overlay.mu.Lock()
		defer overlay.mu.Unlock()

		if overlay.pendingURI != "" {
			overlay.label.SetText(overlay.pendingURI)
			if !overlay.visible {
				overlay.visible = true
				overlay.container.AddCssClass("visible")
			}
			overlay.resetAutoHideTimerLocked()
		}
		overlay.showTimer = 0
		return false
	}
	overlay.autoHideCb = func(_ uintptr) bool {
		overlay.mu.Lock()
		defer overlay.mu.Unlock()

		overlay.autoHideTimer = 0
		overlay.hide()
		return false
	}

	return overlay
}

// Show displays the link status overlay with the given URI.
// If uri is empty, hides the overlay instead.
// Uses a small delay to avoid flicker during rapid mouse movement.
func (l *LinkStatusOverlay) Show(uri string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Cancel any pending show timer
	if l.showTimer != 0 {
		glib.SourceRemove(l.showTimer)
		l.showTimer = 0
	}

	if uri == "" {
		l.hide()
		return
	}

	l.pendingURI = uri

	// Coalesce rapid hover events before scheduling delay timer.
	l.coalescer.Post("link-status-show-delay", func() {
		l.mu.Lock()
		defer l.mu.Unlock()

		if l.pendingURI == "" {
			return
		}
		if l.showTimer != 0 {
			glib.SourceRemove(l.showTimer)
		}
		l.showTimer = glib.TimeoutAdd(linkStatusShowDelayMs, &l.showTimerCb, 0)
	})
}

// Hide manually hides the link status overlay.
func (l *LinkStatusOverlay) Hide() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hide()
}

// hide performs the actual hide operation (must be called with lock held).
func (l *LinkStatusOverlay) hide() {
	// Cancel pending show timer
	if l.showTimer != 0 {
		glib.SourceRemove(l.showTimer)
		l.showTimer = 0
	}
	// Cancel auto-hide timer
	if l.autoHideTimer != 0 {
		glib.SourceRemove(l.autoHideTimer)
		l.autoHideTimer = 0
	}
	l.pendingURI = ""

	if l.visible {
		l.visible = false
		l.container.RemoveCssClass("visible")
	}
}

// resetAutoHideTimerLocked starts or resets the auto-hide timer (must be called with lock held).
func (l *LinkStatusOverlay) resetAutoHideTimerLocked() {
	// Cancel existing auto-hide timer
	if l.autoHideTimer != 0 {
		glib.SourceRemove(l.autoHideTimer)
		l.autoHideTimer = 0
	}

	// Coalesce timer resets while pointer moves quickly across links.
	l.coalescer.Post("link-status-auto-hide", func() {
		l.mu.Lock()
		defer l.mu.Unlock()

		if l.pendingURI == "" {
			return
		}
		if l.autoHideTimer != 0 {
			glib.SourceRemove(l.autoHideTimer)
		}
		l.autoHideTimer = glib.TimeoutAdd(linkStatusAutoHideMs, &l.autoHideCb, 0)
	})
}

// Widget returns the underlying widget for embedding in overlays.
func (l *LinkStatusOverlay) Widget() layout.Widget {
	return l.container
}

// IsVisible returns whether the overlay is currently visible.
func (l *LinkStatusOverlay) IsVisible() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.visible
}

// Cleanup cancels any pending timers and clears state.
// Must be called before removing the overlay from the UI.
func (l *LinkStatusOverlay) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Cancel pending show timer to prevent use-after-free
	if l.showTimer != 0 {
		glib.SourceRemove(l.showTimer)
		l.showTimer = 0
	}
	// Cancel auto-hide timer
	if l.autoHideTimer != 0 {
		glib.SourceRemove(l.autoHideTimer)
		l.autoHideTimer = 0
	}
	if l.coalescer != nil {
		l.coalescer.Destroy()
		l.coalescer = nil
	}
	l.pendingURI = ""
	l.visible = false
}
