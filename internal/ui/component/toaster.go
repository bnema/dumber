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

// ToastPosition defines where the toast appears on screen.
type ToastPosition int

const (
	// ToastPositionTopLeft positions toast in top-left corner.
	ToastPositionTopLeft ToastPosition = iota
	// ToastPositionTopCenter positions toast at top center.
	ToastPositionTopCenter
	// ToastPositionTopRight positions toast in top-right corner.
	ToastPositionTopRight
	// ToastPositionBottomLeft positions toast in bottom-left corner.
	ToastPositionBottomLeft
	// ToastPositionBottomCenter positions toast at bottom center.
	ToastPositionBottomCenter
	// ToastPositionBottomRight positions toast in bottom-right corner.
	ToastPositionBottomRight
)

// ToastOptions configures toast appearance and behavior.
type ToastOptions struct {
	// Duration in milliseconds. 0 = persistent (no auto-dismiss), >0 = auto-dismiss after duration.
	Duration int
	// BackgroundColor overrides the default background color (CSS color value).
	// Empty string uses the level's default color.
	BackgroundColor string
	// TextColor overrides the default text color (CSS color value).
	// Empty string uses the default (auto-contrast with background).
	TextColor string
	// Position determines where the toast appears on screen.
	Position ToastPosition
}

// ToastOption is a functional option for configuring toast display.
type ToastOption func(*ToastOptions)

// WithDuration sets the auto-dismiss duration in milliseconds.
// Use 0 for persistent toasts that require manual dismissal.
func WithDuration(ms int) ToastOption {
	return func(o *ToastOptions) {
		o.Duration = ms
	}
}

// WithBackgroundColor sets a custom background color (CSS color value).
func WithBackgroundColor(color string) ToastOption {
	return func(o *ToastOptions) {
		o.BackgroundColor = color
	}
}

// WithTextColor sets a custom text color (CSS color value).
func WithTextColor(color string) ToastOption {
	return func(o *ToastOptions) {
		o.TextColor = color
	}
}

// WithPosition sets the toast position on screen.
func WithPosition(pos ToastPosition) ToastOption {
	return func(o *ToastOptions) {
		o.Position = pos
	}
}

// defaultToastOptions returns the default toast options.
func defaultToastOptions() ToastOptions {
	return ToastOptions{
		Duration: toastDismissTimeoutMs,
		Position: ToastPositionTopLeft,
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

	// Track current options for cleanup
	currentOpts     ToastOptions
	hasCustomStyle  bool
	customBgColor   string // Current custom background color
	customTextColor string // Current custom text color

	mu sync.Mutex
}

// NewToaster creates a new toaster component for overlay display.
// The toaster is positioned in the top-left corner with margin by default.
func NewToaster(factory layout.WidgetFactory) *Toaster {
	// Create container box for styling and positioning
	container := factory.NewBox(layout.OrientationHorizontal, 0)
	container.AddCssClass("toast")
	container.AddCssClass("toast-info") // Default level

	// Position in top-left with margin (default)
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
		currentOpts:  defaultToastOptions(),
	}
}

// Show displays a toast notification with the given message and level.
// If a toast is already visible, updates the text and resets the dismiss timer.
// Optional ToastOption arguments can customize duration, colors, and position.
func (t *Toaster) Show(ctx context.Context, message string, level ToastLevel, opts ...ToastOption) {
	log := logging.FromContext(ctx)

	// Build options from defaults and overrides
	options := defaultToastOptions()
	for _, opt := range opts {
		opt(&options)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Clear previous custom styles if any
	t.clearCustomStyle()

	// Update level CSS class if changed
	if t.currentLevel != level {
		t.container.RemoveCssClass(t.currentLevel.cssClass())
		t.container.AddCssClass(level.cssClass())
		t.currentLevel = level
	}

	// Apply custom colors if specified
	t.applyCustomStyle(options)

	// Apply position
	t.applyPosition(options.Position)

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

	// Start new dismiss timer (if duration > 0)
	if options.Duration > 0 {
		t.startDismissTimerWithDuration(ctx, options.Duration)
	}
	// Duration == 0 means persistent, no timer

	t.currentOpts = options

	log.Debug().
		Str("toast_message", message).
		Int("toast_level", int(level)).
		Int("duration", options.Duration).
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

	// Clear custom styles
	t.clearCustomStyle()

	t.visible = false
	t.container.SetVisible(false)
}

// applyPosition sets the widget alignment based on the position.
func (t *Toaster) applyPosition(pos ToastPosition) {
	var halign, valign gtk.Align

	switch pos {
	case ToastPositionTopLeft:
		halign = gtk.AlignStartValue
		valign = gtk.AlignStartValue
	case ToastPositionTopCenter:
		halign = gtk.AlignCenterValue
		valign = gtk.AlignStartValue
	case ToastPositionTopRight:
		halign = gtk.AlignEndValue
		valign = gtk.AlignStartValue
	case ToastPositionBottomLeft:
		halign = gtk.AlignStartValue
		valign = gtk.AlignEndValue
	case ToastPositionBottomCenter:
		halign = gtk.AlignCenterValue
		valign = gtk.AlignEndValue
	case ToastPositionBottomRight:
		halign = gtk.AlignEndValue
		valign = gtk.AlignEndValue
	default:
		halign = gtk.AlignStartValue
		valign = gtk.AlignStartValue
	}

	t.container.SetHalign(halign)
	t.container.SetValign(valign)
}

// applyCustomStyle applies custom background and text colors.
// Uses a CSS class that is styled dynamically by the theme manager.
func (t *Toaster) applyCustomStyle(opts ToastOptions) {
	if opts.BackgroundColor == "" && opts.TextColor == "" {
		return
	}

	// Add custom class to indicate custom styling is applied
	t.container.AddCssClass("toast-custom")
	t.hasCustomStyle = true

	// Store the colors for access by theme/CSS generation
	t.customBgColor = opts.BackgroundColor
	t.customTextColor = opts.TextColor

	// Remove level-based styling when using custom colors
	if opts.BackgroundColor != "" {
		t.container.RemoveCssClass(t.currentLevel.cssClass())
	}
}

// clearCustomStyle removes any custom styling applied.
func (t *Toaster) clearCustomStyle() {
	if t.hasCustomStyle {
		t.container.RemoveCssClass("toast-custom")
		// Remove any mode-specific classes
		t.container.RemoveCssClass("toast-pane-mode")
		t.container.RemoveCssClass("toast-tab-mode")
		t.container.RemoveCssClass("toast-session-mode")
		t.container.RemoveCssClass("toast-resize-mode")
		// Re-add level class if it was removed
		if t.customBgColor != "" {
			t.container.AddCssClass(t.currentLevel.cssClass())
		}
		t.hasCustomStyle = false
		t.customBgColor = ""
		t.customTextColor = ""
	}
}

// ApplyModeClass applies a mode-specific CSS class for styling.
// This uses the pre-defined CSS classes that reference CSS variables.
func (t *Toaster) ApplyModeClass(modeClass string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Clear any existing custom style first
	t.clearCustomStyle()

	// Remove level class and add mode class
	t.container.RemoveCssClass(t.currentLevel.cssClass())
	t.container.AddCssClass(modeClass)
	t.hasCustomStyle = true
	t.customBgColor = modeClass // Store class name for cleanup tracking
}

// startDismissTimerWithDuration starts the auto-dismiss timer with the given duration.
// Must be called with lock held.
func (t *Toaster) startDismissTimerWithDuration(ctx context.Context, durationMs int) {
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

	t.dismissTimer = glib.TimeoutAdd(uint(durationMs), &cb, 0)
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
