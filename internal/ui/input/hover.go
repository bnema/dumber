package input

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gobject"
	"github.com/bnema/puregotk/v4/gtk"
)

const (
	// HoverFocusDelay is the delay before switching focus on hover.
	HoverFocusDelay = 150 * time.Millisecond
)

// HoverCallback is called when a pane should receive focus from hover.
type HoverCallback func(paneID entity.PaneID)

// MotionCallback is called when intentional mouse movement is detected.
type MotionCallback func()

// HoverHandler handles mouse hover events for focus-follows-mouse behavior.
// It uses a debounce timer to avoid rapid focus switches.
type HoverHandler struct {
	attachedWidget *gtk.Widget
	motionCtrl     *gtk.EventControllerMotion
	enterSignalID  uint
	leaveSignalID  uint
	motionSignalID uint
	paneID         entity.PaneID
	onEnter        HoverCallback
	onMotion       MotionCallback

	// Callback retention: must stay reachable by Go GC.
	enterCb  func(gtk.EventControllerMotion, float64, float64)
	leaveCb  func(gtk.EventControllerMotion)
	motionCb func(gtk.EventControllerMotion, float64, float64)

	timerMu          sync.Mutex
	sourceID         uint
	generation       uint64
	detached         bool
	schedule         func(uint, func(uintptr) bool) uint
	removeSource     func(uint) bool
	disconnectSignal func(*gtk.EventControllerMotion, uint)
	removeController func(*gtk.Widget, *gtk.EventController)

	ctx context.Context
}

// NewHoverHandler creates a new hover handler for a specific pane.
func NewHoverHandler(ctx context.Context, paneID entity.PaneID) *HoverHandler {
	log := logging.FromContext(ctx)
	log.Debug().Str("pane_id", string(paneID)).Msg("creating hover handler")

	return &HoverHandler{
		ctx:              ctx,
		paneID:           paneID,
		schedule:         scheduleHoverSource,
		removeSource:     glib.SourceRemove,
		disconnectSignal: disconnectHoverSignal,
		removeController: removeHoverController,
	}
}

// SetOnEnter sets the callback for when the pane should receive focus.
func (h *HoverHandler) SetOnEnter(fn HoverCallback) {
	h.onEnter = fn
}

// SetOnMotion sets the callback for intentional mouse movement.
// This fires on actual mouse motion within the pane, not on synthetic
// enter/leave events from GTK widget rearrangement.
func (h *HoverHandler) SetOnMotion(fn MotionCallback) {
	h.onMotion = fn
}

// AttachTo attaches the hover handler to a GTK widget.
func (h *HoverHandler) AttachTo(widget *gtk.Widget) {
	log := logging.FromContext(h.ctx)

	if widget == nil {
		log.Error().Msg("cannot attach hover handler to nil widget")
		return
	}

	// Reattachment must not leave a controller owned by the previous widget.
	if h.motionCtrl != nil || h.attachedWidget != nil {
		h.Detach()
	}
	h.timerMu.Lock()
	h.detached = false
	h.attachedWidget = widget
	h.timerMu.Unlock()

	h.motionCtrl = gtk.NewEventControllerMotion()
	if h.motionCtrl == nil {
		log.Error().Msg("failed to create motion controller")
		return
	}

	// Connect enter handler with debounce (retain callbacks to prevent GC)
	h.enterCb = func(_ gtk.EventControllerMotion, _ float64, _ float64) {
		h.handleEnter()
	}
	h.enterSignalID = h.motionCtrl.ConnectEnter(&h.enterCb)

	// Connect leave handler to cancel pending focus
	h.leaveCb = func(_ gtk.EventControllerMotion) {
		h.handleLeave()
	}
	h.leaveSignalID = h.motionCtrl.ConnectLeave(&h.leaveCb)

	// Connect motion handler to detect intentional mouse movement.
	// Unlike enter/leave, motion only fires when the cursor physically moves
	// inside the widget — not on synthetic events from widget rearrangement.
	h.motionCb = func(_ gtk.EventControllerMotion, _ float64, _ float64) {
		if h.onMotion != nil {
			h.onMotion()
		}
	}
	h.motionSignalID = h.motionCtrl.ConnectMotion(&h.motionCb)

	// Add controller to widget
	widget.AddController(&h.motionCtrl.EventController)

	log.Debug().Str("pane_id", string(h.paneID)).Msg("hover handler attached to widget")
}

// scheduleHoverSource schedules a one-shot callback on GTK's owning GLib main
// context. GTK state must never be touched by a Go timer goroutine.
func scheduleHoverSource(delay uint, fn func(uintptr) bool) uint {
	cb := glib.SourceFunc(fn)
	return glib.TimeoutAdd(delay, &cb, 0)
}

// handleEnter processes the mouse enter event with debouncing.
func (h *HoverHandler) handleEnter() {
	h.timerMu.Lock()
	if h.detached {
		h.timerMu.Unlock()
		return
	}
	h.cancelSourceLocked()
	h.generation++
	generation := h.generation
	schedule := h.schedule
	h.sourceID = schedule(uint(HoverFocusDelay.Milliseconds()), func(_ uintptr) bool {
		h.timerMu.Lock()
		if h.detached || h.generation != generation {
			h.timerMu.Unlock()
			return false
		}
		h.sourceID = 0
		callback := h.onEnter
		paneID := h.paneID
		h.timerMu.Unlock()

		if callback != nil {
			callback(paneID)
		}
		return false
	})
	h.timerMu.Unlock()
}

func (h *HoverHandler) cancelSourceLocked() {
	if h.sourceID == 0 {
		return
	}
	h.removeSource(h.sourceID)
	h.sourceID = 0
}

// handleLeave cancels any pending focus switch.
func (h *HoverHandler) handleLeave() {
	h.Cancel()
}

// Cancel cancels any pending focus switch.
// This can be called externally to cancel hover when keyboard navigation occurs.
func (h *HoverHandler) Cancel() {
	h.timerMu.Lock()
	defer h.timerMu.Unlock()

	h.generation++
	h.cancelSourceLocked()
}

// Detach removes the hover handler and invalidates callbacks already queued on
// the GLib main context. The generation check is required because removing a
// source can race with dispatch.
func (h *HoverHandler) Detach() {
	h.timerMu.Lock()
	h.detached = true
	h.generation++
	h.cancelSourceLocked()
	h.onEnter = nil
	h.onMotion = nil
	widget := h.attachedWidget
	controller := h.motionCtrl
	enterID := h.enterSignalID
	leaveID := h.leaveSignalID
	motionID := h.motionSignalID
	h.attachedWidget = nil
	h.motionCtrl = nil
	h.enterSignalID = 0
	h.leaveSignalID = 0
	h.motionSignalID = 0
	h.enterCb = nil
	h.leaveCb = nil
	h.motionCb = nil
	disconnectSignal := h.disconnectSignal
	removeController := h.removeController
	h.timerMu.Unlock()

	if controller == nil {
		return
	}
	if disconnectSignal != nil {
		for _, id := range []uint{enterID, leaveID, motionID} {
			if id != 0 {
				disconnectSignal(controller, id)
			}
		}
	}
	if widget != nil && removeController != nil {
		removeController(widget, &controller.EventController)
	}
}

func disconnectHoverSignal(controller *gtk.EventControllerMotion, id uint) {
	if controller == nil || id == 0 || controller.GoPointer() == 0 {
		return
	}
	gobject.SignalHandlerDisconnect(gobject.ObjectNewFromInternalPtr(controller.GoPointer()), id)
}

func removeHoverController(widget *gtk.Widget, controller *gtk.EventController) {
	if widget != nil && controller != nil {
		widget.RemoveController(controller)
	}
}
