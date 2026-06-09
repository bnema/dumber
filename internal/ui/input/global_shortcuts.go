// Package input provides keyboard event handling and modal input mode management.
package input

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gio"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/rs/zerolog"
)

// GlobalShortcutHandler manages keyboard shortcuts that must work globally,
// even when WebView has focus. It uses GtkShortcutController with GTK_SHORTCUT_SCOPE_GLOBAL
// to intercept shortcuts before they reach the WebView.
type GlobalShortcutHandler struct {
	controller            *gtk.ShortcutController
	releaseController     *gtk.EventControllerKey
	window                *gtk.ApplicationWindow
	kbHandler             *KeyboardHandler
	onAction              ActionHandler
	ctx                   context.Context
	registered            map[KeyBinding]Action
	shortcutRefs          map[string]globalShortcutRegistration
	lastDispatchAt        map[Action]time.Time
	heldShortcuts         map[globalShortcutHoldKey]struct{}
	shortcutAction        *gtk.NamedAction
	globalAction          *gio.SimpleAction
	globalActionCb        func(gio.SimpleAction, uintptr)
	globalActionHandlerID uint
	keyReleasedCb         func(gtk.EventControllerKey, uint, uint, gdk.ModifierType)
	keyReleasedHandlerID  uint
	// generation is mutated only from the GTK main thread alongside controller replacement.
	generation uint64
}

// NewGlobalShortcutHandler creates a new global shortcut handler for shortcuts
// that need to work even when WebView has focus (like Alt+1-9 for tab switching).
func NewGlobalShortcutHandler(
	ctx context.Context,
	window *gtk.ApplicationWindow,
	workspace *entity.WorkspaceConfig,
	session *entity.SessionConfig,
	kbHandler *KeyboardHandler,
	onAction ActionHandler,
) *GlobalShortcutHandler {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating global shortcut handler")

	h := &GlobalShortcutHandler{
		controller:        gtk.NewShortcutController(),
		releaseController: gtk.NewEventControllerKey(),
		window:            window,
		kbHandler:         kbHandler,
		onAction:          onAction,
		ctx:               ctx,
		registered:        make(map[KeyBinding]Action),
		shortcutRefs:      make(map[string]globalShortcutRegistration),
		lastDispatchAt:    make(map[Action]time.Time),
		heldShortcuts:     make(map[globalShortcutHoldKey]struct{}),
	}

	if h.controller == nil {
		log.Error().Msg("failed to create shortcut controller")
		return nil
	}
	if h.releaseController == nil {
		log.Error().Msg("failed to create global shortcut release controller")
		return nil
	}

	h.configureGlobalControllers()
	h.registerDefaultGlobalShortcuts(log)

	if workspace != nil {
		actionMap := globalShortcutActionMap()

		for actionName, actionBinding := range workspace.Shortcuts.Actions {
			action, ok := actionMap[actionName]
			if !ok {
				continue
			}
			for _, keyStr := range actionBinding.Keys {
				binding, ok := ParseKeyString(keyStr)
				if !ok {
					log.Warn().Str("shortcut", keyStr).Str("action", string(action)).Msg("failed to parse global shortcut")
					continue
				}
				if h.registerShortcut(binding.Keyval, gdk.ModifierType(binding.Modifiers), action) {
					log.Trace().Str("shortcut", keyStr).Str("action", string(action)).Msg("registered global shortcut")
				}
			}
		}

		occupied := make(map[KeyBinding]Action, len(h.registered))
		for binding, action := range h.registered {
			occupied[binding] = action
		}
		for _, shortcut := range collectFloatingProfileShortcutsFromWorkspace(ctx, workspace, occupied) {
			if !h.registerShortcut(shortcut.Binding.Keyval, gdk.ModifierType(shortcut.Binding.Modifiers), shortcut.Action) {
				continue
			}
			if url, ok := ParseFloatingProfileAction(shortcut.Action); ok {
				log.Trace().
					Str("shortcut", formatBinding(shortcut.Binding)).
					Str("url", url).
					Msg("registered floating profile global shortcut")
			}
		}

		// Register all app-reserved shortcuts (mode activations, Ctrl+L, Ctrl+F, etc.)
		// so they work even when WebView has focus.
		shortcuts := NewShortcutSet(ctx, workspace, session)
		for binding, action := range shortcuts.Global {
			if _, exists := h.registered[binding]; exists {
				continue
			}
			h.registerShortcut(binding.Keyval, gdk.ModifierType(binding.Modifiers), action)
		}
	}

	// Attach to window
	window.AddController(&h.releaseController.EventController)
	window.AddController(&h.controller.EventController)

	log.Debug().
		Int("shortcuts", len(h.registered)).
		Msg("global shortcut handler created and attached")

	return h
}

func (h *GlobalShortcutHandler) configureGlobalControllers() {
	// Set global scope - this is the key to making shortcuts work
	// even when WebView has focus.
	h.controller.SetScope(gtk.ShortcutScopeGlobalValue)
	h.shortcutAction = gtk.NewNamedAction(globalShortcutActionFullName)
	h.globalAction = gio.NewSimpleAction(globalShortcutActionName, glib.NewVariantType("s"))
	if h.globalAction != nil {
		h.globalActionCb = func(_ gio.SimpleAction, parameter uintptr) {
			h.activateGlobalShortcut(parameter)
		}
		h.globalActionHandlerID = h.globalAction.ConnectActivate(&h.globalActionCb)
		h.window.AddAction(h.globalAction)
	}
	h.releaseController.SetPropagationPhase(gtk.PhaseCaptureValue)
	h.keyReleasedCb = func(_ gtk.EventControllerKey, keyval uint, keycode uint, _ gdk.ModifierType) {
		h.releaseHeldGlobalShortcuts(keyval, keycode)
	}
	h.keyReleasedHandlerID = h.releaseController.ConnectKeyReleased(&h.keyReleasedCb)
}

func (h *GlobalShortcutHandler) registerDefaultGlobalShortcuts(log *zerolog.Logger) {
	// Register Alt+1 through Alt+9 for tab switching.
	tabActions := []Action{
		ActionSwitchTabIndex1,
		ActionSwitchTabIndex2,
		ActionSwitchTabIndex3,
		ActionSwitchTabIndex4,
		ActionSwitchTabIndex5,
		ActionSwitchTabIndex6,
		ActionSwitchTabIndex7,
		ActionSwitchTabIndex8,
		ActionSwitchTabIndex9,
	}

	for i, action := range tabActions {
		keyval := uint(gdk.KEY_1) + uint(i) // KEY_1, KEY_2, ..., KEY_9
		h.registerShortcut(keyval, gdk.AltMaskValue, action)
		log.Trace().
			Uint("keyval", keyval).
			Str("action", string(action)).
			Msg("registered global shortcut")
	}

	// Alt+0 for tab 10.
	h.registerShortcut(uint(gdk.KEY_0), gdk.AltMaskValue, ActionSwitchTabIndex10)
	log.Trace().
		Uint("keyval", uint(gdk.KEY_0)).
		Str("action", string(ActionSwitchTabIndex10)).
		Msg("registered global shortcut")

	// Alt+Tab for switching to last active tab.
	h.registerShortcut(uint(gdk.KEY_Tab), gdk.AltMaskValue, ActionSwitchLastTab)
	log.Trace().
		Uint("keyval", uint(gdk.KEY_Tab)).
		Str("action", string(ActionSwitchLastTab)).
		Msg("registered global shortcut")

	// Ctrl+Shift+S for direct session manager access (needs global scope for WebView focus).
	h.registerShortcut(uint(gdk.KEY_s), gdk.ControlMaskValue|gdk.ShiftMaskValue, ActionOpenSessionManager)
	log.Trace().
		Uint("keyval", uint(gdk.KEY_s)).
		Str("action", string(ActionOpenSessionManager)).
		Msg("registered global shortcut")
}

// registerShortcut creates and registers a single shortcut with the controller.
func (h *GlobalShortcutHandler) registerShortcut(keyval uint, modifiers gdk.ModifierType, action Action) bool {
	binding := KeyBinding{Keyval: keyval, Modifiers: Modifier(modifiers) & modifierMask}
	if existing, exists := h.registered[binding]; exists {
		logging.FromContext(h.ctx).Warn().
			Str("existing_action", string(existing)).
			Str("new_action", string(action)).
			Str("shortcut", formatBinding(binding)).
			Msg("global shortcut conflict, skipping")
		return false
	}

	if h.shortcutAction == nil {
		logging.FromContext(h.ctx).Error().
			Uint("keyval", keyval).
			Msg("global shortcut action is unavailable")
		return false
	}

	// Create trigger for this key combination.
	trigger := gtk.NewKeyvalTrigger(keyval, modifiers)
	if trigger == nil {
		logging.FromContext(h.ctx).Error().
			Uint("keyval", keyval).
			Msg("failed to create keyval trigger")
		return false
	}

	shortcut := gtk.NewShortcut(&trigger.ShortcutTrigger, &h.shortcutAction.ShortcutAction)
	if shortcut == nil {
		logging.FromContext(h.ctx).Error().
			Uint("keyval", keyval).
			Msg("failed to create shortcut")
		return false
	}

	shortcutID := encodeGlobalShortcutID(action, binding)
	shortcut.SetArguments(glib.NewVariantString(shortcutID))

	// Add to controller.
	h.controller.AddShortcut(shortcut)
	h.registered[binding] = action
	h.shortcutRefs[shortcutID] = globalShortcutRegistration{
		action:  action,
		binding: binding,
	}
	return true
}

func (h *GlobalShortcutHandler) activateGlobalShortcut(parameter uintptr) {
	log := logging.FromContext(h.ctx)
	registration, ok := h.globalShortcutRegistration(parameter)
	if !ok {
		log.Warn().Msg("global shortcut activated without registered argument")
		return
	}
	h.dispatchGlobalShortcut(registration.action, registration.binding, h.generation)
}

func (h *GlobalShortcutHandler) globalShortcutRegistration(parameter uintptr) (globalShortcutRegistration, bool) {
	if parameter == 0 || h.shortcutRefs == nil {
		return globalShortcutRegistration{}, false
	}
	variant := glib.VariantNewFromInternalPtr(parameter)
	if variant == nil {
		return globalShortcutRegistration{}, false
	}
	shortcutID := variant.GetString(nil)
	registration, ok := h.shortcutRefs[shortcutID]
	return registration, ok
}

func (h *GlobalShortcutHandler) dispatchGlobalShortcut(actionToDispatch Action, bindingForLog KeyBinding, generation uint64) bool {
	log := logging.FromContext(h.ctx)
	if h.isStaleGeneration(generation) {
		log.Trace().
			Str("action", string(actionToDispatch)).
			Str("shortcut", formatBinding(bindingForLog)).
			Uint64("callback_generation", generation).
			Uint64("current_generation", h.generation).
			Msg("stale global shortcut callback ignored")
		return true
	}
	if !h.isActiveWindowShortcutHandler() {
		log.Trace().
			Str("action", string(actionToDispatch)).
			Str("shortcut", formatBinding(bindingForLog)).
			Msg("inactive window global shortcut callback ignored")
		return false
	}
	eventInfo := h.inspectCurrentShortcutEvent()
	if !shouldDispatchGlobalShortcutEvent(eventInfo) {
		appendGlobalShortcutEventFields(log.Trace(), bindingForLog, actionToDispatch, eventInfo).
			Msg("global shortcut ignored without current key event")
		return true
	}
	if h.suppressHeldShortcut(actionToDispatch, eventInfo) {
		log.Trace().
			Str("action", string(actionToDispatch)).
			Str("shortcut", formatBinding(bindingForLog)).
			Msg("held global shortcut repeat suppressed")
		return true
	}
	if h.suppressRepeatedShortcut(actionToDispatch, time.Now()) {
		log.Trace().
			Str("action", string(actionToDispatch)).
			Str("shortcut", formatBinding(bindingForLog)).
			Msg("global shortcut repeat suppressed")
		return true
	}
	appendGlobalShortcutEventFields(log.Debug(), bindingForLog, actionToDispatch, eventInfo).
		Msg("global shortcut triggered")

	// Mode-enter/exit actions go through KeyboardHandler for modal state.
	if isModeAction(actionToDispatch) {
		if h.kbHandler != nil {
			h.kbHandler.DispatchAction(actionToDispatch)
			return true
		}
		log.Warn().
			Str("action", string(actionToDispatch)).
			Msg("mode action triggered but keyboard handler not set, falling through to default handler")
	}

	if h.onAction != nil {
		if err := h.onAction(h.ctx, actionToDispatch); err != nil {
			log.Error().
				Err(err).
				Str("action", string(actionToDispatch)).
				Msg("global shortcut action failed")
		}
	}
	return true
}

const (
	globalShortcutActionName     = "dumber-global-shortcut"
	globalShortcutActionFullName = "win." + globalShortcutActionName
)

type globalShortcutRegistration struct {
	action  Action
	binding KeyBinding
}

func encodeGlobalShortcutID(action Action, binding KeyBinding) string {
	return fmt.Sprintf("%s\x1f%d\x1f%d", action, binding.Keyval, binding.Modifiers)
}

func (h *GlobalShortcutHandler) estimatedPuregoCallbackBudget() int {
	if h == nil {
		return 0
	}
	callbacks := 0
	if h.globalActionCb != nil {
		callbacks++
	}
	if h.keyReleasedCb != nil {
		callbacks++
	}
	return callbacks
}

// globalShortcutActionMap returns a fresh map of workspace action names to global shortcut actions.
type globalShortcutHoldKey struct {
	keyval  uint
	keycode uint
}

type globalShortcutEventInfo struct {
	hasCurrentEvent        bool
	eventType              gdk.EventType
	eventKeyval            uint
	eventKeycode           uint
	eventLayout            uint
	eventLevel             uint
	eventState             gdk.ModifierType
	eventConsumedModifiers gdk.ModifierType
	eventTime              uint32
	hasDevice              bool
	deviceName             string
	deviceSource           gdk.InputSource
	controllerState        gdk.ModifierType
	controllerTime         uint32
}

func (h *GlobalShortcutHandler) inspectCurrentShortcutEvent() globalShortcutEventInfo {
	var info globalShortcutEventInfo
	if h == nil || h.controller == nil {
		return info
	}

	info.controllerState = h.controller.GetCurrentEventState()
	info.controllerTime = h.controller.GetCurrentEventTime()
	if device := h.controller.GetCurrentEventDevice(); device != nil {
		info.hasDevice = true
		info.deviceName = device.GetName()
		info.deviceSource = device.GetSource()
	}

	event := h.controller.GetCurrentEvent()
	if event == nil {
		return info
	}
	info.hasCurrentEvent = true
	info.eventType = event.GetEventType()
	info.eventState = event.GetModifierState()
	info.eventTime = event.GetTime()
	if !info.hasDevice {
		if device := event.GetDevice(); device != nil {
			info.hasDevice = true
			info.deviceName = device.GetName()
			info.deviceSource = device.GetSource()
		}
	}

	switch info.eventType {
	case gdk.KeyPressValue, gdk.KeyReleaseValue:
		keyEvent := gdk.KeyEventNewFromInternalPtr(event.GoPointer())
		info.eventKeyval = keyEvent.GetKeyval()
		info.eventKeycode = keyEvent.GetKeycode()
		info.eventLayout = keyEvent.GetLayout()
		info.eventLevel = keyEvent.GetLevel()
		info.eventConsumedModifiers = keyEvent.GetConsumedModifiers()
	}

	return info
}

func shouldDispatchGlobalShortcutEvent(info globalShortcutEventInfo) bool {
	return info.hasCurrentEvent && info.eventType == gdk.KeyPressValue
}

func appendGlobalShortcutEventFields(evt *zerolog.Event, binding KeyBinding, action Action, info globalShortcutEventInfo) *zerolog.Event {
	if evt == nil {
		return nil
	}
	evt = evt.
		Str("action", string(action)).
		Str("shortcut", formatBinding(binding)).
		Uint("shortcut_keyval", binding.Keyval).
		Str("shortcut_key", formatKeyval(binding.Keyval)).
		Str("shortcut_modifiers", formatModifierMask(gdk.ModifierType(binding.Modifiers))).
		Str("controller_modifiers", formatModifierMask(info.controllerState)).
		Uint32("controller_event_time", info.controllerTime).
		Bool("has_current_event", info.hasCurrentEvent)
	if info.hasCurrentEvent {
		evt = evt.
			Str("event_type", formatEventType(info.eventType)).
			Int("event_type_value", int(info.eventType)).
			Str("event_modifiers", formatModifierMask(info.eventState)).
			Uint32("event_time", info.eventTime)
		if info.eventKeyval != 0 {
			evt = evt.
				Uint("event_keyval", info.eventKeyval).
				Str("event_key", formatKeyval(info.eventKeyval))
		}
		if info.eventKeycode != 0 {
			evt = evt.Uint("event_keycode", info.eventKeycode)
		}
		if info.eventLayout != 0 {
			evt = evt.Uint("event_layout", info.eventLayout)
		}
		if info.eventLevel != 0 {
			evt = evt.Uint("event_level", info.eventLevel)
		}
		if info.eventConsumedModifiers != 0 {
			evt = evt.Str("event_consumed_modifiers", formatModifierMask(info.eventConsumedModifiers))
		}
	}
	if info.hasDevice {
		evt = evt.
			Str("event_device", info.deviceName).
			Int("event_device_source", int(info.deviceSource))
	}
	return evt
}

func globalShortcutActionMap() map[string]Action {
	return map[string]Action{
		"toggle_floating_pane":        ActionToggleFloatingPane,
		"toggle-floating-pane":        ActionToggleFloatingPane,
		"consume_or_expel_left":       ActionConsumeOrExpelLeft,
		"consume-or-expel-left":       ActionConsumeOrExpelLeft,
		"consume_or_expel_right":      ActionConsumeOrExpelRight,
		"consume-or-expel-right":      ActionConsumeOrExpelRight,
		"consume_or_expel_up":         ActionConsumeOrExpelUp,
		"consume-or-expel-up":         ActionConsumeOrExpelUp,
		"consume_or_expel_down":       ActionConsumeOrExpelDown,
		"consume-or-expel-down":       ActionConsumeOrExpelDown,
		"toggle_history_systemview":   ActionToggleHistorySystemView,
		"toggle-history-systemview":   ActionToggleHistorySystemView,
		"toggle_favorites_systemview": ActionToggleFavoritesSystemView,
		"toggle-favorites-systemview": ActionToggleFavoritesSystemView,
		"toggle_config_systemview":    ActionToggleConfigSystemView,
		"toggle-config-systemview":    ActionToggleConfigSystemView,
	}
}

// ReloadShortcuts rebuilds global shortcuts from a new config.
// It replaces the GTK shortcut controller to ensure stale shortcuts
// are removed.
func (h *GlobalShortcutHandler) ReloadShortcuts(ctx context.Context, workspace *entity.WorkspaceConfig, session *entity.SessionConfig) {
	log := logging.FromContext(ctx)
	h.ctx = ctx
	h.generation++

	if h.window == nil || h.controller == nil {
		return
	}

	// Remove old controller from window
	h.window.RemoveController(&h.controller.EventController)

	// Create fresh controller
	h.controller = gtk.NewShortcutController()
	if h.controller == nil {
		log.Error().Msg("failed to create shortcut controller during reload")
		return
	}
	h.controller.SetScope(gtk.ShortcutScopeGlobalValue)
	h.registered = make(map[KeyBinding]Action)
	h.shortcutRefs = make(map[string]globalShortcutRegistration)
	h.lastDispatchAt = make(map[Action]time.Time)
	h.heldShortcuts = make(map[globalShortcutHoldKey]struct{})

	h.registerDefaultGlobalShortcuts(log)

	// Re-register config-driven shortcuts
	if workspace != nil {
		actionMap := globalShortcutActionMap()

		for actionName, actionBinding := range workspace.Shortcuts.Actions {
			action, ok := actionMap[actionName]
			if !ok {
				continue
			}
			for _, keyStr := range actionBinding.Keys {
				binding, ok := ParseKeyString(keyStr)
				if !ok {
					continue
				}
				h.registerShortcut(binding.Keyval, gdk.ModifierType(binding.Modifiers), action)
			}
		}

		occupied := make(map[KeyBinding]Action, len(h.registered))
		for binding, action := range h.registered {
			occupied[binding] = action
		}
		for _, shortcut := range collectFloatingProfileShortcutsFromWorkspace(ctx, workspace, occupied) {
			if h.registerShortcut(shortcut.Binding.Keyval, gdk.ModifierType(shortcut.Binding.Modifiers), shortcut.Action) {
				if url, ok := ParseFloatingProfileAction(shortcut.Action); ok {
					log.Trace().Str("shortcut", formatBinding(shortcut.Binding)).Str("url", url).Msg("registered floating profile global shortcut")
				}
			}
		}

		shortcuts := NewShortcutSet(ctx, workspace, session)
		for binding, action := range shortcuts.Global {
			if _, exists := h.registered[binding]; exists {
				continue
			}
			h.registerShortcut(binding.Keyval, gdk.ModifierType(binding.Modifiers), action)
		}
	}

	// Attach new controller to window
	h.window.AddController(&h.controller.EventController)

	log.Debug().
		Int("shortcuts", len(h.registered)).
		Msg("global shortcuts reloaded")
}

// Detach removes the global shortcut handler from the window.
// Note: GTK handles cleanup when the widget is destroyed,
// but we clear our references here.
func (h *GlobalShortcutHandler) Detach() {
	h.generation++
	if h.window != nil && h.controller != nil {
		h.window.RemoveController(&h.controller.EventController)
	}
	if h.releaseController != nil && h.keyReleasedHandlerID != 0 {
		h.releaseController.DisconnectSignal(h.keyReleasedHandlerID)
	}
	if h.window != nil && h.releaseController != nil {
		h.window.RemoveController(&h.releaseController.EventController)
	}
	if h.globalAction != nil && h.globalActionHandlerID != 0 {
		h.globalAction.DisconnectSignal(h.globalActionHandlerID)
	}
	if h.window != nil && h.globalAction != nil {
		h.window.RemoveAction(globalShortcutActionName)
	}
	h.controller = nil
	h.releaseController = nil
	h.shortcutAction = nil
	h.globalAction = nil
	h.globalActionCb = nil
	h.globalActionHandlerID = 0
	h.keyReleasedCb = nil
	h.keyReleasedHandlerID = 0
	h.registered = nil
	h.shortcutRefs = nil
	h.lastDispatchAt = nil
	h.heldShortcuts = nil
}

const globalShortcutRepeatSuppressWindow = 250 * time.Millisecond

func (h *GlobalShortcutHandler) isStaleGeneration(generation uint64) bool {
	return h == nil || generation != h.generation
}

func (h *GlobalShortcutHandler) isActiveWindowShortcutHandler() bool {
	return h != nil && h.controller != nil && h.window != nil && h.window.IsActive()
}

func (h *GlobalShortcutHandler) suppressHeldShortcut(action Action, info globalShortcutEventInfo) bool {
	if h == nil || !isHeldGlobalShortcutSuppressed(action) || !info.hasCurrentEvent {
		return false
	}
	if h.heldShortcuts == nil {
		// Defensive: if hold tracking is unavailable during detach/reload,
		// suppress rather than risk dispatching a stale global callback.
		return true
	}
	key := globalShortcutHoldKey{keyval: normalizeKeyval(info.eventKeyval), keycode: info.eventKeycode}
	if key.keyval == 0 && key.keycode == 0 {
		return false
	}
	if _, ok := h.heldShortcuts[key]; ok {
		return true
	}
	h.heldShortcuts[key] = struct{}{}
	return false
}

func (h *GlobalShortcutHandler) releaseHeldGlobalShortcuts(keyval, keycode uint) {
	if h == nil || len(h.heldShortcuts) == 0 {
		return
	}
	keyval = normalizeKeyval(keyval)
	for key := range h.heldShortcuts {
		if (keyval != 0 && key.keyval == keyval) || (keycode != 0 && key.keycode == keycode) {
			delete(h.heldShortcuts, key)
		}
	}
}

func (h *GlobalShortcutHandler) suppressRepeatedShortcut(action Action, now time.Time) bool {
	if h == nil {
		return false
	}
	if !isRepeatedGlobalShortcutSuppressed(action) {
		if _, ok := ParseFloatingProfileTarget(action); !ok {
			return false
		}
	}
	if h.lastDispatchAt == nil {
		// Detached/reloading handlers clear dispatch state; consume one-shot shortcuts
		// rather than letting a stale GTK callback repeat destructive actions.
		return true
	}
	if last, ok := h.lastDispatchAt[action]; ok && now.Sub(last) < globalShortcutRepeatSuppressWindow {
		return true
	}
	h.lastDispatchAt[action] = now
	return false
}

func isHeldGlobalShortcutSuppressed(action Action) bool {
	if isRepeatedGlobalShortcutSuppressed(action) || isModeAction(action) {
		return true
	}
	_, ok := ParseFloatingProfileTarget(action)
	return ok
}

func isRepeatedGlobalShortcutSuppressed(action Action) bool {
	switch action {
	case ActionGoBack,
		ActionGoForward,
		ActionZoomReset,
		ActionReload,
		ActionHardReload,
		ActionPrintPage,
		ActionOpenOmnibox,
		ActionOpenFind,
		ActionFindNext,
		ActionFindPrev,
		ActionOpenDevTools,
		ActionToggleFullscreen,
		ActionToggleFloatingPane,
		ActionToggleHistorySystemView,
		ActionToggleFavoritesSystemView,
		ActionToggleConfigSystemView,
		ActionCopyURL,
		ActionConsumeOrExpelLeft,
		ActionConsumeOrExpelRight,
		ActionConsumeOrExpelUp,
		ActionConsumeOrExpelDown,
		ActionClosePane,
		ActionCloseTab,
		ActionQuit,
		ActionOpenSessionManager,
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
		ActionSwitchLastTab:
		return true
	default:
		return false
	}
}

func formatBinding(binding KeyBinding) string {
	parts := make([]string, 0, 3)
	if binding.Modifiers&ModCtrl != 0 {
		parts = append(parts, "ctrl")
	}
	if binding.Modifiers&ModShift != 0 {
		parts = append(parts, "shift")
	}
	if binding.Modifiers&ModAlt != 0 {
		parts = append(parts, "alt")
	}
	parts = append(parts, formatKeyval(binding.Keyval))
	return strings.Join(parts, "+")
}

func formatKeyval(keyval uint) string {
	if keyval == 0 {
		return ""
	}
	keyName := gdk.KeyvalName(keyval)
	if keyName == "" {
		return fmt.Sprintf("0x%x", keyval)
	}
	return strings.ToLower(keyName)
}

func formatModifierMask(modifiers gdk.ModifierType) string {
	if modifiers == 0 {
		return "none"
	}
	parts := make([]string, 0, 4)
	if modifiers&gdk.ControlMaskValue != 0 {
		parts = append(parts, "ctrl")
	}
	if modifiers&gdk.ShiftMaskValue != 0 {
		parts = append(parts, "shift")
	}
	if modifiers&gdk.AltMaskValue != 0 {
		parts = append(parts, "alt")
	}
	remaining := modifiers &^ (gdk.ControlMaskValue | gdk.ShiftMaskValue | gdk.AltMaskValue)
	if remaining != 0 {
		parts = append(parts, fmt.Sprintf("0x%x", uint(remaining)))
	}
	return strings.Join(parts, "+")
}

func formatEventType(eventType gdk.EventType) string {
	switch eventType {
	case gdk.KeyPressValue:
		return "key-press"
	case gdk.KeyReleaseValue:
		return "key-release"
	case gdk.ButtonPressValue:
		return "button-press"
	case gdk.ButtonReleaseValue:
		return "button-release"
	case gdk.MotionNotifyValue:
		return "motion"
	case gdk.ScrollValue:
		return "scroll"
	case gdk.TouchBeginValue:
		return "touch-begin"
	case gdk.TouchUpdateValue:
		return "touch-update"
	case gdk.TouchEndValue:
		return "touch-end"
	case gdk.FocusChangeValue:
		return "focus-change"
	case gdk.DeleteValue:
		return "delete"
	default:
		return fmt.Sprintf("event-%d", int(eventType))
	}
}

func isModeAction(action Action) bool {
	switch action {
	case ActionEnterTabMode, ActionEnterPaneMode, ActionEnterSessionMode, ActionEnterResizeMode, ActionExitMode:
		return true
	default:
		return false
	}
}
