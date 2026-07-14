package input

import (
	"context"
	"reflect"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/puregotk/v4/gtk"
)

func TestHoverHandlerSchedulesCallbackOnGLibAndRejectsStaleGeneration(t *testing.T) {
	h := NewHoverHandler(context.Background(), entity.PaneID("pane-1"))
	var scheduled func(uintptr) bool
	h.schedule = func(_ uint, fn func(uintptr) bool) uint {
		scheduled = fn
		return 7
	}

	var calls int
	h.SetOnEnter(func(entity.PaneID) { calls++ })
	h.handleEnter()
	first := scheduled
	h.handleEnter()
	if first(0) {
		t.Fatal("hover source must be one-shot")
	}
	if calls != 0 {
		t.Fatalf("stale hover callback ran %d times", calls)
	}
	if scheduled == nil {
		t.Fatal("current hover callback was not scheduled")
	}
	if scheduled(0) {
		t.Fatal("hover source must be one-shot")
	}
	if calls != 1 {
		t.Fatalf("current hover callback ran %d times, want 1", calls)
	}
}

func TestHoverHandlerDetachRemovesNativeControllerAndSignalsExactlyOnce(t *testing.T) {
	widget := &gtk.Widget{}
	controller := &gtk.EventControllerMotion{}
	h := NewHoverHandler(context.Background(), entity.PaneID("pane-1"))
	h.attachedWidget = widget
	h.motionCtrl = controller
	h.enterSignalID = 1
	h.leaveSignalID = 2
	h.motionSignalID = 3
	var disconnected []uint
	var removed int
	h.disconnectSignal = func(_ *gtk.EventControllerMotion, id uint) { disconnected = append(disconnected, id) }
	h.removeController = func(gotWidget *gtk.Widget, gotController *gtk.EventController) {
		if gotWidget != widget || gotController != &controller.EventController {
			t.Fatal("detach removed the wrong native controller")
		}
		removed++
	}

	h.Detach()
	h.Detach()

	if got, want := disconnected, []uint{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("disconnected signals = %v, want %v", got, want)
	}
	if removed != 1 {
		t.Fatalf("native controllers removed = %d, want 1", removed)
	}
	if h.attachedWidget != nil || h.motionCtrl != nil || h.enterCb != nil || h.leaveCb != nil || h.motionCb != nil {
		t.Fatal("detach must clear native and callback ownership")
	}
}

func TestHoverHandlerDetachInvalidatesScheduledCallbackAndRemovesSource(t *testing.T) {
	h := NewHoverHandler(context.Background(), entity.PaneID("pane-1"))
	var scheduled func(uintptr) bool
	var removed []uint
	h.schedule = func(_ uint, fn func(uintptr) bool) uint { scheduled = fn; return 9 }
	h.removeSource = func(id uint) bool { removed = append(removed, id); return true }
	h.SetOnEnter(func(entity.PaneID) { t.Fatal("detached hover callback ran") })
	h.handleEnter()
	h.Detach()
	if len(removed) != 1 || removed[0] != 9 {
		t.Fatalf("removed sources = %v, want [9]", removed)
	}
	if scheduled == nil || scheduled(0) {
		t.Fatal("detached callback must be rejected and one-shot")
	}
}
