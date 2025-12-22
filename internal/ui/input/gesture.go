package input

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

const (
	// Mouse button constants for X11/Wayland
	mouseButtonBack    = 8 // Side button - back
	mouseButtonForward = 9 // Side button - forward
)

// GestureHandler handles mouse button gestures for navigation.
// It recognizes mouse buttons 8 (back) and 9 (forward) on WebView widgets.
type GestureHandler struct {
	clickGesture *gtk.GestureClick

	// Callback retention: must stay reachable by Go GC.
	pressedCb func(gtk.GestureClick, int, float64, float64)

	// Action handler callback
	onAction ActionHandler

	ctx context.Context
	mu  sync.RWMutex
}

// NewGestureHandler creates a new gesture handler.
func NewGestureHandler(ctx context.Context) *GestureHandler {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating gesture handler")

	return &GestureHandler{
		ctx: ctx,
	}
}

// SetOnAction sets the callback for when navigation actions are triggered.
func (h *GestureHandler) SetOnAction(fn ActionHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onAction = fn
}

// AttachTo attaches the gesture handler to a GTK widget.
// Typically called with a WebView widget to enable mouse button navigation.
func (h *GestureHandler) AttachTo(widget *gtk.Widget) {
	log := logging.FromContext(h.ctx)

	if widget == nil {
		log.Error().Msg("cannot attach gesture handler to nil widget")
		return
	}

	// Create click gesture that listens to all buttons
	h.clickGesture = gtk.NewGestureClick()
	if h.clickGesture == nil {
		log.Error().Msg("failed to create gesture click")
		return
	}

	// Listen to all mouse buttons (not just primary)
	h.clickGesture.SetButton(0)

	// Connect pressed handler (retain callback to prevent GC)
	h.pressedCb = func(_ gtk.GestureClick, nPress int, _ float64, _ float64) {
		h.handlePressed(nPress)
	}
	h.clickGesture.ConnectPressed(&h.pressedCb)

	// Add controller to widget
	widget.AddController(&h.clickGesture.EventController)

	log.Debug().Msg("gesture handler attached to widget")
}

// handlePressed processes a mouse button press event.
func (h *GestureHandler) handlePressed(nPress int) {
	// Only handle single clicks for navigation
	if nPress != 1 {
		return
	}

	// Get the button that was pressed
	button := h.clickGesture.GetCurrentButton()

	var action Action
	switch button {
	case mouseButtonBack:
		action = ActionGoBack
	case mouseButtonForward:
		action = ActionGoForward
	default:
		// Not a navigation button, ignore
		return
	}

	// Dispatch action to handler
	h.mu.RLock()
	handler := h.onAction
	h.mu.RUnlock()

	if handler != nil {
		if err := handler(h.ctx, action); err != nil {
			log := logging.FromContext(h.ctx)
			log.Error().
				Err(err).
				Str("action", string(action)).
				Uint("button", button).
				Msg("gesture action handler error")
		}
	}
}

// Detach removes the gesture handler.
// Note: GTK handles cleanup when the widget is destroyed,
// but we clear our reference here.
func (h *GestureHandler) Detach() {
	h.clickGesture = nil
	h.pressedCb = nil
}
