// Package component provides UI components for the browser.
package component

import (
	"context"
	"fmt"
	"sync"

	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/layout"
)

const (
	// Default auto-dismiss timeout in milliseconds.
	toastDismissTimeoutMs = 1500
)

// ToastLevel indicates the visual style of a toast notification.
type ToastLevel int

const (
	// ToastInfo is for informational messages (accent color).
	ToastInfo ToastLevel = iota
	// ToastSuccess is for success confirmations (green).
	ToastSuccess
	// ToastWarning is for warning messages (yellow).
	ToastWarning
	// ToastError is for error messages (red).
	ToastError
)

// cssClass returns the CSS class for this toast level.
func (l ToastLevel) cssClass() string {
	switch l {
	case ToastInfo:
		return "toast-info"
	case ToastSuccess:
		return "toast-success"
	case ToastWarning:
		return "toast-warning"
	case ToastError:
		return "toast-error"
	default:
		return "toast-info"
	}
}

// Toaster displays toast notifications in an overlay.
// It supports different notification levels and auto-dismissal.
// When a new toast is shown while one is already visible, the text
// is updated in-place and the dismiss timer is reset (spam protection).
type Toaster struct {
	factory      layout.WidgetFactory
	container    layout.BoxWidget   // Outer container for positioning
	label        layout.LabelWidget // Message text
	currentLevel ToastLevel
	visible      bool
	dismissTimer uint // GLib timer source ID

	mu sync.Mutex
}

// NewToaster creates a new toaster component for overlay display.
// The toaster is positioned in the top-left corner with margin.
func NewToaster(factory layout.WidgetFactory) *Toaster {
	// Create container box for styling and positioning
	container := factory.NewBox(layout.OrientationHorizontal, 0)
	container.AddCssClass("toast")
	container.AddCssClass("toast-info") // Default level

	// Position in top-left with margin
	container.SetHalign(gtk.AlignStartValue)
	container.SetValign(gtk.AlignStartValue)

	// Don't expand to fill space
	container.SetHexpand(false)
	container.SetVexpand(false)

	// Don't intercept pointer events - let clicks pass through
	container.SetCanTarget(false)
	container.SetCanFocus(false)

	// Hidden by default
	container.SetVisible(false)

	// Create label for message text
	label := factory.NewLabel("")
	label.SetCanTarget(false)
	label.SetCanFocus(false)
	container.Append(label)

	return &Toaster{
		factory:      factory,
		container:    container,
		label:        label,
		currentLevel: ToastInfo,
		visible:      false,
	}
}

// Show displays a toast notification with the given message and level.
// If a toast is already visible, updates the text and resets the dismiss timer.
func (t *Toaster) Show(ctx context.Context, message string, level ToastLevel) {
	log := logging.FromContext(ctx)

	t.mu.Lock()
	defer t.mu.Unlock()

	// Update level CSS class if changed
	if t.currentLevel != level {
		t.container.RemoveCssClass(t.currentLevel.cssClass())
		t.container.AddCssClass(level.cssClass())
		t.currentLevel = level
	}

	// Update message text
	t.label.SetText(message)

	// Cancel existing timer if any
	if t.dismissTimer != 0 {
		glib.SourceRemove(t.dismissTimer)
		t.dismissTimer = 0
	}

	// Show the toast if not already visible
	if !t.visible {
		t.visible = true
		t.container.SetVisible(true)
	}

	// Start new dismiss timer
	t.startDismissTimer(ctx)

	log.Debug().
		Str("toast_message", message).
		Int("toast_level", int(level)).
		Msg("toast shown")
}

// ShowZoom displays a zoom level toast (convenience method).
// Formats the zoom percentage with a % suffix.
func (t *Toaster) ShowZoom(ctx context.Context, zoomPercent int) {
	message := fmt.Sprintf("%d%%", zoomPercent)
	t.Show(ctx, message, ToastInfo)
}

// Hide manually dismisses the toast.
func (t *Toaster) Hide() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.hide()
}

// hide performs the actual hide operation (must be called with lock held).
func (t *Toaster) hide() {
	if !t.visible {
		return
	}

	// Cancel timer if running
	if t.dismissTimer != 0 {
		glib.SourceRemove(t.dismissTimer)
		t.dismissTimer = 0
	}

	t.visible = false
	t.container.SetVisible(false)
}

// startDismissTimer starts the auto-dismiss timer.
// Must be called with lock held.
func (t *Toaster) startDismissTimer(ctx context.Context) {
	log := logging.FromContext(ctx)

	cb := glib.SourceFunc(func(_ uintptr) bool {
		t.mu.Lock()
		defer t.mu.Unlock()

		// Only hide if timer hasn't been replaced
		t.hide()
		t.dismissTimer = 0

		log.Debug().Msg("toast auto-dismissed")
		return false // Don't repeat
	})

	t.dismissTimer = glib.TimeoutAdd(toastDismissTimeoutMs, &cb, 0)
}

// Widget returns the underlying widget for embedding in overlays.
func (t *Toaster) Widget() layout.Widget {
	return t.container
}

// IsVisible returns whether the toast is currently visible.
func (t *Toaster) IsVisible() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.visible
}
