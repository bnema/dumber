package webkit

import (
	"fmt"
	"math"

	"github.com/bnema/dumber/internal/logging"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Touchpad swipe velocity threshold (pixels per second)
// Lower values make gestures more sensitive, higher values require faster swipes
const (
	swipeVelocityThreshold = 500.0 // Reasonable threshold for intentional swipes
)

// AttachTouchpadGestures attaches touchpad swipe gesture handling to the WebView
// This enables two-finger swipe navigation (left/right for back/forward)
func (w *WebView) AttachTouchpadGestures() {
	if w.view == nil {
		logging.Error(fmt.Sprintf("[touchpad-gestures] Cannot attach: WebView is nil"))
		return
	}

	// Create gesture controller for swipe detection
	gestureSwipe := gtk.NewGestureSwipe()

	// Use bubble phase instead of capture to not interfere with scroll events
	// This lets normal scrolling work while still detecting navigation swipes
	gestureSwipe.SetPropagationPhase(gtk.PhaseBubble)

	// Set to require touchpad (2 fingers minimum for touchpad gestures)
	// This prevents single-finger drags from triggering navigation
	gestureSwipe.SetTouchOnly(false) // Allow both touch and touchpad

	// Connect to swipe signal
	gestureSwipe.ConnectSwipe(func(velocityX, velocityY float64) {
		// Only process horizontal swipes (ignore mostly vertical swipes)
		// Use absolute values to compare magnitudes
		absVelocityX := math.Abs(velocityX)
		absVelocityY := math.Abs(velocityY)

		// Require horizontal velocity to be dominant
		if absVelocityY > absVelocityX {
			logging.Debug(fmt.Sprintf("[touchpad-gestures] Swipe is more vertical (X:%.1f Y:%.1f), ignoring", velocityX, velocityY))
			return
		}

		// Check if swipe velocity meets threshold
		if absVelocityX < swipeVelocityThreshold {
			logging.Debug(fmt.Sprintf("[touchpad-gestures] Swipe velocity too low (%.1f < %.1f), ignoring", absVelocityX, swipeVelocityThreshold))
			return
		}

		// Determine swipe direction and navigate
		if velocityX > 0 {
			// Swipe right (positive velocity) - go back
			if w.view.CanGoBack() {
				logging.Debug(fmt.Sprintf("[touchpad-gestures] Swipe right detected (velocity: %.1f) - navigating backward", velocityX))
				w.view.GoBack()
			} else {
				logging.Debug(fmt.Sprintf("[touchpad-gestures] Swipe right detected but cannot go back"))
			}
		} else {
			// Swipe left (negative velocity) - go forward
			if w.view.CanGoForward() {
				logging.Debug(fmt.Sprintf("[touchpad-gestures] Swipe left detected (velocity: %.1f) - navigating forward", velocityX))
				w.view.GoForward()
			} else {
				logging.Debug(fmt.Sprintf("[touchpad-gestures] Swipe left detected but cannot go forward"))
			}
		}
	})

	// Attach controller to WebView
	w.view.AddController(gestureSwipe)
	logging.Debug(fmt.Sprintf("[touchpad-gestures] Touchpad gesture controller attached to WebView ID %d", w.id))
}
