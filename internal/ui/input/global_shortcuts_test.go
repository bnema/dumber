package input

import (
	"testing"
	"time"

	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gio"
	"github.com/bnema/puregotk/v4/gtk"
)

func TestShouldRegisterGTKGlobalShortcut_ExcludesPageModeActivation(t *testing.T) {
	if shouldRegisterGTKGlobalShortcut(ActionEnterPageMode) {
		t.Fatal("ActionEnterPageMode should stay off the GTK global shortcut controller")
	}
	if !shouldRegisterGTKGlobalShortcut(ActionOpenOmnibox) {
		t.Fatal("non-page-mode global shortcuts should remain registerable")
	}
}

func TestShouldIgnoreGlobalShortcutInMode_PageModeBlocksNonToggleActions(t *testing.T) {
	if !shouldIgnoreGlobalShortcutInMode(ModePage, ActionOpenOmnibox) {
		t.Fatal("Page mode should ignore app-global shortcuts like omnibox")
	}
	if shouldIgnoreGlobalShortcutInMode(ModeNormal, ActionOpenOmnibox) {
		t.Fatal("normal mode should not ignore app-global shortcuts")
	}
	if shouldIgnoreGlobalShortcutInMode(ModePage, ActionEnterPageMode) {
		t.Fatal("page mode toggle should not be blocked by the mode guard")
	}
}

func TestGlobalShortcutHandlerCallbackBudgetIsIndependentOfShortcutCount(t *testing.T) {
	h := &GlobalShortcutHandler{
		registered:   make(map[KeyBinding]Action),
		shortcutRefs: make(map[string]globalShortcutRegistration),
	}

	for i := 0; i < 57; i++ {
		binding := KeyBinding{Keyval: uint(gdk.KEY_1) + uint(i), Modifiers: Modifier(gdk.AltMaskValue)}
		action := ActionSwitchTabIndex1
		shortcutID := encodeGlobalShortcutID(action, binding, h.generation)
		h.registered[binding] = action
		h.shortcutRefs[shortcutID] = globalShortcutRegistration{action: action, binding: binding, generation: h.generation}
	}
	if got := h.estimatedPuregoCallbackBudget(); got != 0 {
		t.Fatalf("callback budget without retained GTK callbacks = %d, want 0", got)
	}

	h.globalActionCb = func(_ gio.SimpleAction, _ uintptr) {}
	h.keyReleasedCb = func(_ gtk.EventControllerKey, _ uint, _ uint, _ gdk.ModifierType) {}
	if got := h.estimatedPuregoCallbackBudget(); got != 2 {
		t.Fatalf("callback budget with 57 shortcuts = %d, want 2", got)
	}
}

func TestGlobalShortcutIDIncludesBinding(t *testing.T) {
	first := encodeGlobalShortcutID(ActionSwitchTabIndex1, KeyBinding{Keyval: uint(gdk.KEY_1), Modifiers: Modifier(gdk.AltMaskValue)}, 0)
	second := encodeGlobalShortcutID(ActionSwitchTabIndex1, KeyBinding{Keyval: uint(gdk.KEY_2), Modifiers: Modifier(gdk.AltMaskValue)}, 0)
	if first == second {
		t.Fatal("global shortcut IDs should distinguish bindings for the same action")
	}
}

func TestGlobalShortcutIDIncludesGeneration(t *testing.T) {
	binding := KeyBinding{Keyval: uint(gdk.KEY_1), Modifiers: Modifier(gdk.AltMaskValue)}
	first := encodeGlobalShortcutID(ActionSwitchTabIndex1, binding, 1)
	second := encodeGlobalShortcutID(ActionSwitchTabIndex1, binding, 2)
	if first == second {
		t.Fatal("global shortcut IDs should distinguish registrations across reload generations")
	}
}

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

func TestGlobalShortcutHandlerDetachForDestroyClearsRetainedState(t *testing.T) {
	h := &GlobalShortcutHandler{
		registered:            make(map[KeyBinding]Action),
		shortcutRefs:          make(map[string]globalShortcutRegistration),
		lastDispatchAt:        make(map[Action]time.Time),
		heldShortcuts:         make(map[globalShortcutHoldKey]struct{}),
		globalActionCb:        func(_ gio.SimpleAction, _ uintptr) {},
		globalActionHandlerID: 1,
		keyReleasedCb:         func(_ gtk.EventControllerKey, _ uint, _ uint, _ gdk.ModifierType) {},
		keyReleasedHandlerID:  2,
	}

	h.DetachForDestroy()

	if h.controller != nil {
		t.Fatal("controller should be cleared during destroy detach")
	}
	if h.releaseController != nil {
		t.Fatal("releaseController should be cleared during destroy detach")
	}
	if h.shortcutAction != nil {
		t.Fatal("shortcutAction should be cleared during destroy detach")
	}
	if h.globalAction != nil {
		t.Fatal("globalAction should be cleared during destroy detach")
	}
	if h.globalActionCb != nil {
		t.Fatal("globalActionCb should be cleared during destroy detach")
	}
	if h.keyReleasedCb != nil {
		t.Fatal("keyReleasedCb should be cleared during destroy detach")
	}
	if h.globalActionHandlerID != 0 {
		t.Fatal("globalActionHandlerID should be reset during destroy detach")
	}
	if h.keyReleasedHandlerID != 0 {
		t.Fatal("keyReleasedHandlerID should be reset during destroy detach")
	}
	if h.registered != nil {
		t.Fatal("registered shortcuts should be cleared during destroy detach")
	}
	if h.shortcutRefs != nil {
		t.Fatal("shortcut refs should be cleared during destroy detach")
	}
	if h.lastDispatchAt != nil {
		t.Fatal("lastDispatchAt should be cleared during destroy detach")
	}
	if h.heldShortcuts != nil {
		t.Fatal("heldShortcuts should be cleared during destroy detach")
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
	h := &GlobalShortcutHandler{heldShortcuts: make(map[globalShortcutHoldKey]struct{})}
	info := globalShortcutEventInfo{hasCurrentEvent: true, eventType: gdk.KeyPressValue, eventKeyval: uint('f'), eventKeycode: 41}

	if h.suppressHeldShortcut(ActionToggleFloatingPane, info) {
		t.Fatal("first held global shortcut dispatch was suppressed")
	}
	if !h.suppressHeldShortcut(ActionToggleFloatingPane, info) {
		t.Fatal("held global shortcut repeat after cooldown was not suppressed")
	}

	h.releaseHeldGlobalShortcuts(uint('x'), 0)
	if !h.suppressHeldShortcut(ActionToggleFloatingPane, info) {
		t.Fatal("unrelated key release re-armed held global shortcut")
	}

	h.releaseHeldGlobalShortcuts(uint('f'), 0)
	if h.suppressHeldShortcut(ActionToggleFloatingPane, info) {
		t.Fatal("matching key release did not re-arm held global shortcut")
	}
}

func TestGlobalShortcutHandlerSuppressesHeldModeAction(t *testing.T) {
	h := &GlobalShortcutHandler{heldShortcuts: make(map[globalShortcutHoldKey]struct{})}
	info := globalShortcutEventInfo{hasCurrentEvent: true, eventType: gdk.KeyPressValue, eventKeyval: uint('p'), eventKeycode: 33}

	if h.suppressHeldShortcut(ActionEnterPaneMode, info) {
		t.Fatal("first mode shortcut dispatch was suppressed")
	}
	if !h.suppressHeldShortcut(ActionEnterPaneMode, info) {
		t.Fatal("held mode shortcut repeat was not suppressed")
	}
}

func TestGlobalShortcutHandlerSuppressesHeldFloatingProfileAction(t *testing.T) {
	h := &GlobalShortcutHandler{heldShortcuts: make(map[globalShortcutHoldKey]struct{})}
	info := globalShortcutEventInfo{hasCurrentEvent: true, eventType: gdk.KeyPressValue, eventKeyval: uint('o'), eventKeycode: 32}
	action := NewFloatingProfileAction("work", "https://example.com")

	if h.suppressHeldShortcut(action, info) {
		t.Fatal("first floating-profile shortcut dispatch was suppressed")
	}
	if !h.suppressHeldShortcut(action, info) {
		t.Fatal("held floating-profile shortcut repeat was not suppressed")
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
