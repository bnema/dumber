package webkit

import (
	"fmt"

	"github.com/bnema/dumber/internal/logging"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Mouse button constants for navigation
// Standard button numbers (consistent across Wayland and GTK)
const (
	mouseButtonBack    = 8 // Side button (back)
	mouseButtonForward = 9 // Side button (forward)
)

// AttachMouseGestures attaches mouse button gesture handling to the WebView
// This enables hardware mouse button navigation (back/forward buttons)
func (w *WebView) AttachMouseGestures() {
	if w.view == nil {
		logging.Error(fmt.Sprintf("[mouse-gestures] Cannot attach: WebView is nil"))
		return
	}

	// Create gesture controller for mouse clicks
	gestureClick := gtk.NewGestureClick()

	// Listen to all mouse buttons (0 = any button)
	gestureClick.SetButton(0)

	// Set to capture phase to intercept before WebView processes
	gestureClick.SetPropagationPhase(gtk.PhaseCapture)

	// Connect to pressed signal
	gestureClick.ConnectPressed(func(nPress int, x, y float64) {
		// Get which button was pressed
		button := gestureClick.CurrentButton()

		// Only handle single clicks (not double/triple clicks)
		if nPress != 1 {
			return
		}

		switch button {
		case mouseButtonBack:
			if w.view.CanGoBack() {
				logging.Debug(fmt.Sprintf("[mouse-gestures] Back button clicked - navigating backward"))
				w.view.GoBack()
			} else {
				logging.Debug(fmt.Sprintf("[mouse-gestures] Back button clicked but cannot go back"))
			}

		case mouseButtonForward:
			if w.view.CanGoForward() {
				logging.Debug(fmt.Sprintf("[mouse-gestures] Forward button clicked - navigating forward"))
				w.view.GoForward()
			} else {
				logging.Debug(fmt.Sprintf("[mouse-gestures] Forward button clicked but cannot go forward"))
			}
		}
	})

	// Attach controller to WebView
	w.view.AddController(gestureClick)

	logging.Debug(fmt.Sprintf("[mouse-gestures] Mouse gesture controller attached to WebView ID %d", w.id))
}
