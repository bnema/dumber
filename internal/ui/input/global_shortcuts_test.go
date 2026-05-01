package input

import (
	"testing"
	"time"
)

func TestGlobalShortcutHandlerSuppressesRepeatedOneShotActions(t *testing.T) {
	h := &GlobalShortcutHandler{lastDispatchAt: make(map[Action]time.Time)}
	now := time.Unix(100, 0)

	if h.suppressRepeatedShortcut(ActionZoomReset, now) {
		t.Fatal("first zoom reset dispatch was suppressed")
	}
	if !h.suppressRepeatedShortcut(ActionZoomReset, now.Add(globalShortcutRepeatSuppressWindow/2)) {
		t.Fatal("repeated zoom reset dispatch inside suppression window was not suppressed")
	}
	if h.suppressRepeatedShortcut(ActionZoomReset, now.Add(globalShortcutRepeatSuppressWindow)) {
		t.Fatal("zoom reset dispatch at suppression boundary was suppressed")
	}
}

func TestGlobalShortcutHandlerAllowsRepeatingNavigationActions(t *testing.T) {
	h := &GlobalShortcutHandler{lastDispatchAt: make(map[Action]time.Time)}
	now := time.Unix(100, 0)

	if h.suppressRepeatedShortcut(ActionFocusRight, now) {
		t.Fatal("first focus-right dispatch was suppressed")
	}
	if h.suppressRepeatedShortcut(ActionFocusRight, now.Add(time.Millisecond)) {
		t.Fatal("focus-right repeat was suppressed")
	}
}

func TestGlobalShortcutHandlerSuppressesOneShotActionsAfterDetach(t *testing.T) {
	h := &GlobalShortcutHandler{
		registered:     make(map[KeyBinding]Action),
		lastDispatchAt: make(map[Action]time.Time),
	}

	if h.suppressRepeatedShortcut(ActionZoomReset, time.Unix(100, 0)) {
		t.Fatal("one-shot shortcut was suppressed before detach")
	}
	h.Detach()
	if !h.suppressRepeatedShortcut(ActionZoomReset, time.Unix(100, 0)) {
		t.Fatal("one-shot shortcut was not suppressed with detached handler state")
	}
}

func TestGlobalShortcutHandlerGenerationMarksOldCallbacksStale(t *testing.T) {
	h := &GlobalShortcutHandler{}
	generation := h.generation

	if h.isStaleGeneration(generation) {
		t.Fatal("current generation was marked stale")
	}
	h.generation++
	if !h.isStaleGeneration(generation) {
		t.Fatal("old generation was not marked stale")
	}
}
