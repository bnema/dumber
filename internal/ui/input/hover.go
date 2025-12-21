package input

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

const (
	// HoverFocusDelay is the delay before switching focus on hover.
	HoverFocusDelay = 150 * time.Millisecond
)

// HoverCallback is called when a pane should receive focus from hover.
type HoverCallback func(paneID entity.PaneID)

// HoverHandler handles mouse hover events for focus-follows-mouse behavior.
// It uses a debounce timer to avoid rapid focus switches.
type HoverHandler struct {
	motionCtrl *gtk.EventControllerMotion
	paneID     entity.PaneID
	onEnter    HoverCallback

	timer    *time.Timer
	timerMu  sync.Mutex
	canceled bool

	ctx context.Context
}

// NewHoverHandler creates a new hover handler for a specific pane.
func NewHoverHandler(ctx context.Context, paneID entity.PaneID) *HoverHandler {
	log := logging.FromContext(ctx)
	log.Debug().Str("pane_id", string(paneID)).Msg("creating hover handler")

	return &HoverHandler{
		ctx:    ctx,
		paneID: paneID,
	}
}

// SetOnEnter sets the callback for when the pane should receive focus.
func (h *HoverHandler) SetOnEnter(fn HoverCallback) {
	h.onEnter = fn
}

// AttachTo attaches the hover handler to a GTK widget.
func (h *HoverHandler) AttachTo(widget *gtk.Widget) {
	log := logging.FromContext(h.ctx)

	if widget == nil {
		log.Error().Msg("cannot attach hover handler to nil widget")
		return
	}

	h.motionCtrl = gtk.NewEventControllerMotion()
	if h.motionCtrl == nil {
		log.Error().Msg("failed to create motion controller")
		return
	}

	// Connect enter handler with debounce
	enterCb := func(_ gtk.EventControllerMotion, _ float64, _ float64) {
		h.handleEnter()
	}
	h.motionCtrl.ConnectEnter(&enterCb)

	// Connect leave handler to cancel pending focus
	leaveCb := func(_ gtk.EventControllerMotion) {
		h.handleLeave()
	}
	h.motionCtrl.ConnectLeave(&leaveCb)

	// Add controller to widget
	widget.AddController(&h.motionCtrl.EventController)

	log.Debug().Str("pane_id", string(h.paneID)).Msg("hover handler attached to widget")
}

// handleEnter processes the mouse enter event with debouncing.
func (h *HoverHandler) handleEnter() {
	h.timerMu.Lock()
	defer h.timerMu.Unlock()

	// Cancel any existing timer
	if h.timer != nil {
		h.timer.Stop()
	}

	h.canceled = false

	// Start new debounce timer
	h.timer = time.AfterFunc(HoverFocusDelay, func() {
		h.timerMu.Lock()
		canceled := h.canceled
		h.timerMu.Unlock()

		if canceled {
			return
		}

		if h.onEnter != nil {
			h.onEnter(h.paneID)
		}
	})
}

// handleLeave cancels any pending focus switch.
func (h *HoverHandler) handleLeave() {
	h.timerMu.Lock()
	defer h.timerMu.Unlock()

	h.canceled = true
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
}

// Detach removes the hover handler.
func (h *HoverHandler) Detach() {
	h.timerMu.Lock()
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
	h.timerMu.Unlock()

	h.motionCtrl = nil
}
