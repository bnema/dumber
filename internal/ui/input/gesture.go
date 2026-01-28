package input

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// DirectNavigator allows direct back/forward navigation on a WebView.
// This is used to call GoBack/GoForward directly from gesture handlers
// without going through callback chains, preserving user gesture context.
type DirectNavigator interface {
	GoBackDirect()
	GoForwardDirect()
}

// gtkEventSequenceClaimed matches GTK_EVENT_SEQUENCE_CLAIMED (value 1)
// Used to tell GTK we've handled the gesture and to stop propagating it.
const gtkEventSequenceClaimed = gtk.EventSequenceClaimedValue

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

	// Action handler callback (fallback if no direct navigator)
	onAction ActionHandler

	// Direct navigator for calling GoBack/GoForward without callback chains.
	// This preserves user gesture context like Epiphany does.
	navigator DirectNavigator

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
// This is a fallback if no DirectNavigator is set.
func (h *GestureHandler) SetOnAction(fn ActionHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onAction = fn
}

// SetNavigator sets a direct navigator for back/forward operations.
// When set, GoBack/GoForward are called directly without callback chains,
// preserving user gesture context.
func (h *GestureHandler) SetNavigator(nav DirectNavigator) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.navigator = nav
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

	log := logging.FromContext(h.ctx)

	// Try direct navigation first (preserves user gesture context)
	h.mu.RLock()
	nav := h.navigator
	handler := h.onAction
	h.mu.RUnlock()

	switch button {
	case mouseButtonBack:
		if nav != nil {
			log.Debug().Uint("button", button).Msg("gesture: direct go back")
			nav.GoBackDirect()
		} else if handler != nil {
			handler(h.ctx, ActionGoBack)
		}
	case mouseButtonForward:
		if nav != nil {
			log.Debug().Uint("button", button).Msg("gesture: direct go forward")
			nav.GoForwardDirect()
		} else if handler != nil {
			handler(h.ctx, ActionGoForward)
		}
	default:
		// Not a navigation button, ignore
		return
	}

	// Claim the gesture sequence to stop event propagation.
	// This matches Epiphany's behavior: gtk_gesture_set_state(gesture, GTK_EVENT_SEQUENCE_CLAIMED)
	// Without this, GTK may continue propagating the event which can interfere with WebKit.
	h.clickGesture.Gesture.SetState(gtkEventSequenceClaimed)
}

// Detach removes the gesture handler.
// Note: GTK handles cleanup when the widget is destroyed,
// but we clear our reference here.
func (h *GestureHandler) Detach() {
	h.clickGesture = nil
	h.pressedCb = nil
}
