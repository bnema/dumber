// Package component provides UI components for the browser.
package component

import (
	"sync"

	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/ui/layout"
)

const (
	// Animation step size - how much to increment per frame
	progressStep = 0.02
	// Animation interval in milliseconds
	progressIntervalMs = 16 // ~60fps
)

// ProgressBar displays a slim loading progress indicator at the bottom of a pane.
// Uses native GtkProgressBar with "osd" styling for overlay appearance.
// Implements smooth animation by incrementing towards the target value.
type ProgressBar struct {
	progressBar layout.ProgressBarWidget

	visible        bool
	currentValue   float64 // Current displayed value
	targetValue    float64 // Target value to animate towards
	animationTimer uint    // Timer source ID for animation

	mu sync.Mutex
}

// NewProgressBar creates a new progress bar component using the widget factory.
func NewProgressBar(factory layout.WidgetFactory) *ProgressBar {
	progressBar := factory.NewProgressBar()

	// Add "osd" class for on-screen-display overlay styling (like Epiphany)
	progressBar.AddCssClass("osd")

	// Position at bottom, full width
	progressBar.SetValign(gtk.AlignEndValue)
	progressBar.SetHalign(gtk.AlignFillValue)
	progressBar.SetHexpand(true)

	// Don't intercept pointer events - let clicks pass through to WebView
	progressBar.SetCanTarget(false)
	progressBar.SetCanFocus(false)

	// Hidden by default
	progressBar.SetVisible(false)

	return &ProgressBar{
		progressBar:  progressBar,
		visible:      false,
		currentValue: 0,
		targetValue:  0,
	}
}

// SetProgress sets the target progress value and starts smooth animation.
// progress should be between 0.0 and 1.0.
func (pb *ProgressBar) SetProgress(progress float64) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	// Clamp progress to valid range
	if progress < 0 {
		progress = 0
	} else if progress > 1 {
		progress = 1
	}

	pb.targetValue = progress

	// For large jumps (>0.3) or completion, update immediately
	diff := progress - pb.currentValue
	if diff > 0.3 || progress >= 1.0 {
		pb.currentValue = progress
		pb.progressBar.SetFraction(progress)
		return
	}

	// Start animation if not already running
	if pb.animationTimer == 0 {
		pb.startAnimation()
	}
}

// startAnimation begins the smooth progress animation.
// Must be called with lock held.
func (pb *ProgressBar) startAnimation() {
	var cb glib.SourceFunc
	cb = func(_ uintptr) bool {
		pb.mu.Lock()
		defer pb.mu.Unlock()

		// Check if we've reached the target
		if pb.currentValue >= pb.targetValue {
			pb.animationTimer = 0
			return false // Stop the timer
		}

		// Increment towards target
		pb.currentValue += progressStep
		if pb.currentValue > pb.targetValue {
			pb.currentValue = pb.targetValue
		}

		pb.progressBar.SetFraction(pb.currentValue)

		// Continue animation if not at target
		return pb.currentValue < pb.targetValue
	}

	pb.animationTimer = glib.TimeoutAdd(progressIntervalMs, &cb, 0)
}

// Show makes the progress bar visible.
func (pb *ProgressBar) Show() {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if !pb.visible {
		pb.visible = true
		pb.progressBar.SetVisible(true)
	}
}

// Hide makes the progress bar invisible and resets state.
func (pb *ProgressBar) Hide() {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if pb.visible {
		pb.visible = false
		pb.progressBar.SetVisible(false)

		// Stop any running animation
		if pb.animationTimer != 0 {
			glib.SourceRemove(pb.animationTimer)
			pb.animationTimer = 0
		}

		// Reset values
		pb.currentValue = 0
		pb.targetValue = 0
		pb.progressBar.SetFraction(0)
	}
}

// IsVisible returns whether the progress bar is currently visible.
func (pb *ProgressBar) IsVisible() bool {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	return pb.visible
}

// Widget returns the underlying widget for embedding in overlays.
func (pb *ProgressBar) Widget() layout.Widget {
	return pb.progressBar
}
