// Package component provides UI components for the browser.
package component

import (
	"sync"
	"time"

	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/ui/layout"
)

const (
	// linkStatusShowDelayMs is the delay before showing the overlay to avoid flicker.
	linkStatusShowDelayMs = 100
	// linkStatusMaxChars is the maximum characters before truncation.
	linkStatusMaxChars = 80
)

// LinkStatusOverlay displays the destination URL when hovering over links.
// It appears in the bottom-left corner with a fade-in/fade-out transition.
type LinkStatusOverlay struct {
	factory   layout.WidgetFactory
	container layout.BoxWidget
	label     layout.LabelWidget

	pendingURI string // URI waiting to be shown after delay
	showTimer  uint   // GLib timer for delayed show
	visible    bool   // Whether overlay is currently visible (has .visible class)
	mu         sync.Mutex
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

	return &LinkStatusOverlay{
		factory:   factory,
		container: container,
		label:     label,
		visible:   false,
	}
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

	// Start delay timer before showing
	cb := glib.SourceFunc(func(_ uintptr) bool {
		l.mu.Lock()
		defer l.mu.Unlock()

		if l.pendingURI != "" {
			l.label.SetText(l.pendingURI)
			if !l.visible {
				l.visible = true
				l.container.AddCssClass("visible")
			}
		}
		l.showTimer = 0
		return false // Don't repeat
	})

	l.showTimer = glib.TimeoutAdd(uint(linkStatusShowDelayMs/time.Millisecond), &cb, 0)
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
	l.pendingURI = ""

	if l.visible {
		l.visible = false
		l.container.RemoveCssClass("visible")
	}
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
