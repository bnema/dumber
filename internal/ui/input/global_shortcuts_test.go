package input

import (
	"testing"
	"time"

	"github.com/bnema/puregotk/v4/gdk"
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

func TestGlobalShortcutHandlerAllowsRepeatingFocusNavigationActions(t *testing.T) {
	h := &GlobalShortcutHandler{lastDispatchAt: make(map[Action]time.Time)}
	now := time.Unix(100, 0)

	if h.suppressRepeatedShortcut(ActionFocusRight, now) {
		t.Fatal("first focus-right dispatch was suppressed")
	}
	if h.suppressRepeatedShortcut(ActionFocusRight, now.Add(time.Millisecond)) {
		t.Fatal("focus-right repeat was suppressed")
	}
}

func TestGlobalShortcutHandlerSuppressesRepeatedBrowserNavigationActions(t *testing.T) {
	h := &GlobalShortcutHandler{lastDispatchAt: make(map[Action]time.Time)}
	now := time.Unix(100, 0)

	if h.suppressRepeatedShortcut(ActionGoBack, now) {
		t.Fatal("first go-back dispatch was suppressed")
	}
	if !h.suppressRepeatedShortcut(ActionGoBack, now.Add(time.Millisecond)) {
		t.Fatal("repeated go-back dispatch was not suppressed")
	}
}

func TestGlobalShortcutHandlerSuppressesRepeatedTabIndexSwitchActions(t *testing.T) {
	actions := []Action{
		ActionSwitchTabIndex1,
		ActionSwitchTabIndex2,
		ActionSwitchTabIndex3,
		ActionSwitchTabIndex4,
		ActionSwitchTabIndex5,
		ActionSwitchTabIndex6,
		ActionSwitchTabIndex7,
		ActionSwitchTabIndex8,
		ActionSwitchTabIndex9,
		ActionSwitchTabIndex10,
	}

	for _, action := range actions {
		t.Run(string(action), func(t *testing.T) {
			h := &GlobalShortcutHandler{lastDispatchAt: make(map[Action]time.Time)}
			now := time.Unix(100, 0)

			if h.suppressRepeatedShortcut(action, now) {
				t.Fatalf("first %s dispatch was suppressed", action)
			}
			if !h.suppressRepeatedShortcut(action, now.Add(time.Millisecond)) {
				t.Fatalf("repeated %s dispatch was not suppressed", action)
			}
		})
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

func TestGlobalShortcutHandlerReturnsInactiveForNilOrDetached(t *testing.T) {
	var nilHandler *GlobalShortcutHandler
	if nilHandler.isActiveWindowShortcutHandler() {
		t.Fatal("nil handler should return inactive")
	}
	if (&GlobalShortcutHandler{}).isActiveWindowShortcutHandler() {
		t.Fatal("handler with nil window should return inactive")
	}

	h := &GlobalShortcutHandler{
		registered:     make(map[KeyBinding]Action),
		lastDispatchAt: make(map[Action]time.Time),
	}
	h.Detach()
	if h.isActiveWindowShortcutHandler() {
		t.Fatal("detached handler with nil controller should return inactive")
	}
}

func TestGlobalShortcutHandlerSuppressesRepeatedAdditionalOneShotUIActions(t *testing.T) {
	actions := []Action{
		ActionPrintPage,
		ActionReload,
		ActionHardReload,
		ActionToggleFullscreen,
		ActionToggleHistorySystemView,
		ActionOpenOmnibox,
		ActionOpenFind,
		ActionFindNext,
		ActionFindPrev,
		ActionOpenDevTools,
		ActionToggleFloatingPane,
		ActionToggleFavoritesSystemView,
		ActionToggleConfigSystemView,
		ActionCopyURL,
		ActionConsumeOrExpelLeft,
		ActionConsumeOrExpelRight,
		ActionConsumeOrExpelUp,
		ActionConsumeOrExpelDown,
	}

	for _, action := range actions {
		t.Run(string(action), func(t *testing.T) {
			h := &GlobalShortcutHandler{lastDispatchAt: make(map[Action]time.Time)}
			now := time.Unix(100, 0)

			if h.suppressRepeatedShortcut(action, now) {
				t.Fatalf("first %s dispatch was suppressed", action)
			}
			if !h.suppressRepeatedShortcut(action, now.Add(time.Millisecond)) {
				t.Fatalf("repeated %s dispatch was not suppressed", action)
			}
		})
	}
}

func TestGlobalShortcutHandlerSuppressesRepeatedFloatingProfileAction(t *testing.T) {
	h := &GlobalShortcutHandler{lastDispatchAt: make(map[Action]time.Time)}
	now := time.Unix(100, 0)
	action := NewFloatingProfileAction("work", "https://example.com")

	if h.suppressRepeatedShortcut(action, now) {
		t.Fatal("first floating-profile dispatch was suppressed")
	}
	if !h.suppressRepeatedShortcut(action, now.Add(time.Millisecond)) {
		t.Fatal("repeated floating-profile dispatch was not suppressed")
	}
}

func TestGlobalShortcutHandlerSuppressesActionSwitchLastTab(t *testing.T) {
	if !isRepeatedGlobalShortcutSuppressed(ActionSwitchLastTab) {
		t.Fatal("ActionSwitchLastTab should be suppressed")
	}

	h := &GlobalShortcutHandler{lastDispatchAt: make(map[Action]time.Time)}
	now := time.Unix(100, 0)

	if h.suppressRepeatedShortcut(ActionSwitchLastTab, now) {
		t.Fatal("first ActionSwitchLastTab dispatch was suppressed")
	}
	if !h.suppressRepeatedShortcut(ActionSwitchLastTab, now.Add(time.Millisecond)) {
		t.Fatal("repeated ActionSwitchLastTab dispatch was not suppressed")
	}
}

func TestGlobalShortcutHandlerSuppressesHeldGlobalShortcutUntilRelease(t *testing.T) {
	h := &GlobalShortcutHandler{heldShortcuts: make(map[globalShortcutHoldKey]time.Time)}
	info := globalShortcutEventInfo{hasCurrentEvent: true, eventType: gdk.KeyPressValue, eventKeyval: uint('f'), eventKeycode: 41}

	if h.suppressHeldShortcut(ActionToggleFloatingPane, info, time.Unix(100, 0)) {
		t.Fatal("first held global shortcut dispatch was suppressed")
	}
	if !h.suppressHeldShortcut(ActionToggleFloatingPane, info, time.Unix(101, 0)) {
		t.Fatal("held global shortcut repeat after cooldown was not suppressed")
	}

	h.releaseHeldGlobalShortcuts(uint('x'), 0)
	if !h.suppressHeldShortcut(ActionToggleFloatingPane, info, time.Unix(102, 0)) {
		t.Fatal("unrelated key release re-armed held global shortcut")
	}

	h.releaseHeldGlobalShortcuts(uint('f'), 0)
	if h.suppressHeldShortcut(ActionToggleFloatingPane, info, time.Unix(103, 0)) {
		t.Fatal("matching key release did not re-arm held global shortcut")
	}
}

func TestGlobalShortcutHandlerSuppressesHeldModeAction(t *testing.T) {
	h := &GlobalShortcutHandler{heldShortcuts: make(map[globalShortcutHoldKey]time.Time)}
	info := globalShortcutEventInfo{hasCurrentEvent: true, eventType: gdk.KeyPressValue, eventKeyval: uint('p'), eventKeycode: 33}

	if h.suppressHeldShortcut(ActionEnterPaneMode, info, time.Unix(100, 0)) {
		t.Fatal("first mode shortcut dispatch was suppressed")
	}
	if !h.suppressHeldShortcut(ActionEnterPaneMode, info, time.Unix(101, 0)) {
		t.Fatal("held mode shortcut repeat was not suppressed")
	}
}

func TestShouldDispatchGlobalShortcutEventRequiresCurrentKeyEvent(t *testing.T) {
	if shouldDispatchGlobalShortcutEvent(globalShortcutEventInfo{}) {
		t.Fatal("global shortcut without current event should not dispatch")
	}
	if shouldDispatchGlobalShortcutEvent(globalShortcutEventInfo{hasCurrentEvent: true, eventType: gdk.KeyReleaseValue}) {
		t.Fatal("global shortcut from key-release event should not dispatch")
	}
	if !shouldDispatchGlobalShortcutEvent(globalShortcutEventInfo{hasCurrentEvent: true, eventType: gdk.KeyPressValue}) {
		t.Fatal("global shortcut from key-press event should dispatch")
	}
}

func TestFormatModifierMask(t *testing.T) {
	got := formatModifierMask(gdk.ControlMaskValue | gdk.ShiftMaskValue | gdk.ModifierType(0x2000))
	if got != "ctrl+shift+0x2000" {
		t.Fatalf("unexpected modifier mask: %q", got)
	}
	if formatModifierMask(0) != "none" {
		t.Fatal("zero modifier mask should render as none")
	}
}

func TestFormatEventType(t *testing.T) {
	if got := formatEventType(gdk.KeyPressValue); got != "key-press" {
		t.Fatalf("unexpected key press event type: %q", got)
	}
	if got := formatEventType(gdk.EventType(99)); got != "event-99" {
		t.Fatalf("unexpected fallback event type: %q", got)
	}
}
