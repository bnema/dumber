package input

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
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
