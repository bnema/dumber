package input

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/rs/zerolog"
)

// ActionHandler is called when a keyboard action is triggered.
// It receives the context and the action to perform.
// Return an error if the action fails.
type ActionHandler func(ctx context.Context, action Action) error

// AccentHandler handles long-press accent detection.
// Called for character keys that may have accent variants.
type AccentHandler interface {
	// OnKeyPressed is called when a key is pressed.
	// Returns true if the accent handler is handling this key (picker visible or will show).
	OnKeyPressed(ctx context.Context, char rune, shiftHeld bool) bool
	// OnKeyReleased is called when a key is released.
	OnKeyReleased(ctx context.Context, char rune)
	// IsPickerVisible returns true if the accent picker is currently visible.
	IsPickerVisible() bool
	// Cancel cancels any pending long-press detection.
	Cancel(ctx context.Context)
}

// KeyboardHandler processes keyboard events and dispatches actions.
// It manages modal input modes and routes key events to the appropriate handlers.
type KeyboardHandler struct {
	shortcuts *ShortcutSet
	modal     *ModalState
	cfg       *config.Config

	// Action handler callback
	onAction ActionHandler
	// Optional bypass check (e.g., omnibox visible)
	shouldBypass func() bool
	// Optional accent handler for dead keys support
	accentHandler AccentHandler

	// GTK controller (nil until attached)
	controller *gtk.EventControllerKey

	// Callback retention: must stay reachable by Go GC.
	keyPressedCb  func(gtk.EventControllerKey, uint, uint, gdk.ModifierType) bool
	keyReleasedCb func(gtk.EventControllerKey, uint, uint, gdk.ModifierType)

	ctx context.Context
	mu  sync.RWMutex
}

// NewKeyboardHandler creates a new keyboard handler.
func NewKeyboardHandler(ctx context.Context, cfg *config.Config) *KeyboardHandler {
	log := logging.FromContext(ctx)

	log.Debug().Msg("creating keyboard handler")

	h := &KeyboardHandler{
		shortcuts: NewShortcutSet(ctx, cfg),
		modal:     NewModalState(ctx),
		cfg:       cfg,
		ctx:       ctx,
	}

	return h
}

// SetOnAction sets the callback for when actions are triggered.
func (h *KeyboardHandler) SetOnAction(fn ActionHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onAction = fn
}

// ReloadShortcuts rebuilds the shortcut set from the new config.
// This enables hot-reloading of keybindings without restarting.
func (h *KeyboardHandler) ReloadShortcuts(ctx context.Context, cfg *config.Config) {
	h.mu.Lock()
	defer h.mu.Unlock()

	log := logging.FromContext(ctx)
	log.Debug().Msg("reloading shortcuts from config")

	h.shortcuts = NewShortcutSet(ctx, cfg)
	h.cfg = cfg
}

// SetOnModeChange sets the callback for mode changes (for UI updates).
func (h *KeyboardHandler) SetOnModeChange(fn func(from, to Mode)) {
	h.modal.SetOnModeChange(fn)
}

// SetShouldBypassInput sets a hook to bypass keyboard handling entirely.
// When true, events propagate to focused widgets instead.
func (h *KeyboardHandler) SetShouldBypassInput(fn func() bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.shouldBypass = fn
}

// SetAccentHandler sets the handler for long-press accent detection.
func (h *KeyboardHandler) SetAccentHandler(handler AccentHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.accentHandler = handler
}

// Mode returns the current input mode.
func (h *KeyboardHandler) Mode() Mode {
	return h.modal.Mode()
}

// AttachTo attaches the keyboard handler to a GTK window.
// The handler will intercept key events in the capture phase.
func (h *KeyboardHandler) AttachTo(window *gtk.ApplicationWindow) {
	log := logging.FromContext(h.ctx)

	if window == nil {
		log.Error().Msg("cannot attach keyboard handler to nil window")
		return
	}

	h.controller = gtk.NewEventControllerKey()
	if h.controller == nil {
		log.Error().Msg("failed to create event controller key")
		return
	}

	// Set capture phase to intercept events before WebView gets them
	h.controller.SetPropagationPhase(gtk.PhaseCaptureValue)

	// Connect key pressed handler (retain callback to prevent GC).
	// The callback receives: keyval (translated key), keycode (hardware key position), state (modifiers)
	h.keyPressedCb = func(_ gtk.EventControllerKey, keyval uint, keycode uint, state gdk.ModifierType) bool {
		return h.handleKeyPress(keyval, keycode, state)
	}
	h.controller.ConnectKeyPressed(&h.keyPressedCb)

	// Connect key released handler for accent detection.
	h.keyReleasedCb = func(_ gtk.EventControllerKey, keyval uint, _ uint, _ gdk.ModifierType) {
		h.handleKeyRelease(keyval)
	}
	h.controller.ConnectKeyReleased(&h.keyReleasedCb)

	// Add controller to window
	window.AddController(&h.controller.EventController)

	log.Debug().Msg("keyboard handler attached to window")
}

// Detach removes the keyboard handler.
// Note: GTK handles cleanup when the widget is destroyed,
// but we clear our reference here.
func (h *KeyboardHandler) Detach() {
	h.controller = nil
	h.keyPressedCb = nil
	h.keyReleasedCb = nil
}

// handleKeyPress processes a key press event.
// Returns true if the event was handled and should not propagate further.
// Parameters:
//   - keyval: the translated key value (depends on keyboard layout)
//   - keycode: the hardware keycode (physical key position, layout-independent)
//   - state: modifier keys state
func (h *KeyboardHandler) handleKeyPress(keyval, keycode uint, state gdk.ModifierType) bool {
	log := logging.FromContext(h.ctx)

	h.mu.RLock()
	shouldBypass := h.shouldBypass
	accentHandler := h.accentHandler
	h.mu.RUnlock()

	// Check if accent picker is visible - it takes priority
	if accentHandler != nil && accentHandler.IsPickerVisible() {
		return false // Let the accent picker handle the key via its own controller
	}

	modifiers := Modifier(state) & modifierMask

	// Check accent detection first - works in both normal and bypass mode
	// This enables accent picker for omnibox (GTK SearchEntry) as well as WebView
	if h.tryAccentDetection(accentHandler, keyval, modifiers) {
		return true
	}

	if shouldBypass != nil && shouldBypass() {
		log.Debug().Uint("keyval", keyval).Uint("keycode", keycode).Uint("state", uint(state)).Msg("keyboard handler bypassed")
		return false
	}

	// Normalize uppercase letters for consistent binding lookup
	keyval = normalizeKeyval(keyval)

	binding := KeyBinding{Keyval: keyval, Modifiers: modifiers}
	mode := h.modal.Mode()
	action, found := h.lookupAction(log, binding, mode, modifiers, keycode)

	if !found {
		return mode != ModeNormal // Consume unrecognized keys in modal mode
	}

	return h.dispatchAction(action, mode)
}

// tryAccentDetection starts long-press detection for accent-eligible keys.
// Returns true if the key should be suppressed (blocked from reaching the WebView).
func (h *KeyboardHandler) tryAccentDetection(accentHandler AccentHandler, keyval uint, modifiers Modifier) bool {
	if h.modal.Mode() != ModeNormal || accentHandler == nil {
		return false
	}
	// Only consider keys without Ctrl/Alt modifiers (Shift is OK for uppercase)
	if modifiers != 0 && modifiers != ModShift {
		return false
	}
	if char := keyvalToRune(keyval); char != 0 {
		return accentHandler.OnKeyPressed(h.ctx, char, modifiers == ModShift)
	}
	return false
}

// normalizeKeyval converts uppercase A-Z to lowercase for consistent binding lookup.
func normalizeKeyval(keyval uint) uint {
	if keyval >= uint('A') && keyval <= uint('Z') {
		return keyval + (uint('a') - uint('A'))
	}
	return keyval
}

// dispatchAction dispatches the action and handles mode-related logic.
func (h *KeyboardHandler) dispatchAction(action Action, mode Mode) bool {
	if h.handleModeAction(action) {
		return true
	}

	h.mu.RLock()
	handler := h.onAction
	h.mu.RUnlock()

	if handler != nil {
		if err := handler(h.ctx, action); err != nil {
			log := logging.FromContext(h.ctx)
			log.Error().Err(err).Str("action", string(action)).Msg("action handler error")
		}
	}

	if mode == ModeResize && isResizeAction(action) {
		h.modal.ResetTimeout(h.ctx)
	}

	if mode != ModeNormal && ShouldAutoExitMode(action) {
		h.modal.ExitMode(h.ctx)
	}

	return true // Consumed the key
}

func (h *KeyboardHandler) lookupAction(
	log *zerolog.Logger,
	binding KeyBinding,
	mode Mode,
	modifiers Modifier,
	keycode uint,
) (Action, bool) {
	action, found := h.shortcuts.Lookup(binding, mode)

	// Fallback: check hardware keycode for Alt+number shortcuts on non-QWERTY layouts.
	if !found && mode == ModeNormal && modifiers == ModAlt {
		if keycodeAction, ok := KeycodeToTabAction[keycode]; ok {
			action = keycodeAction
			found = true
			if log != nil {
				log.Debug().
					Uint("keycode", keycode).
					Str("action", string(action)).
					Msg("tab switch via hardware keycode fallback (non-QWERTY layout)")
			}
		}
	}

	if !found && mode == ModeNormal && (modifiers == ModAlt || modifiers == (ModAlt|ModShift)) {
		if bracketActions, ok := KeycodeToBracketAction[keycode]; ok {
			if modifiers == (ModAlt | ModShift) {
				action = bracketActions.WithShift
			} else {
				action = bracketActions.NoShift
			}
			found = action != ""
			if found && log != nil {
				log.Debug().
					Uint("keycode", keycode).
					Str("action", string(action)).
					Msg("consume/expel via hardware keycode fallback (non-QWERTY layout)")
			}
		}
	}

	return action, found
}

func isResizeAction(action Action) bool {
	switch action {
	case ActionResizeIncreaseLeft,
		ActionResizeIncreaseRight,
		ActionResizeIncreaseUp,
		ActionResizeIncreaseDown,
		ActionResizeDecreaseLeft,
		ActionResizeDecreaseRight,
		ActionResizeDecreaseUp,
		ActionResizeDecreaseDown,
		ActionResizeIncrease,
		ActionResizeDecrease:
		return true
	default:
		return false
	}
}

func (h *KeyboardHandler) handleModeAction(action Action) bool {
	switch action {
	case ActionEnterTabMode:
		timeout := time.Duration(h.cfg.Workspace.TabMode.TimeoutMilliseconds) * time.Millisecond
		h.modal.EnterTabMode(h.ctx, timeout)
		return true
	case ActionEnterPaneMode:
		timeout := time.Duration(h.cfg.Workspace.PaneMode.TimeoutMilliseconds) * time.Millisecond
		h.modal.EnterPaneMode(h.ctx, timeout)
		return true
	case ActionEnterSessionMode:
		timeout := time.Duration(h.cfg.Session.SessionMode.TimeoutMilliseconds) * time.Millisecond
		h.modal.EnterSessionMode(h.ctx, timeout)
		return true
	case ActionEnterResizeMode:
		if h.modal.Mode() == ModeResize {
			h.modal.ExitMode(h.ctx)
			return true
		}
		timeout := time.Duration(h.cfg.Workspace.ResizeMode.TimeoutMilliseconds) * time.Millisecond
		h.modal.EnterResizeMode(h.ctx, timeout)
		return true
	case ActionExitMode:
		h.modal.ExitMode(h.ctx)
		return true
	default:
		return false
	}
}

// EnterTabMode programmatically enters tab mode.
// Useful for testing or programmatic mode changes.
func (h *KeyboardHandler) EnterTabMode() {
	timeout := time.Duration(h.cfg.Workspace.TabMode.TimeoutMilliseconds) * time.Millisecond
	h.modal.EnterTabMode(h.ctx, timeout)
}

// EnterPaneMode programmatically enters pane mode.
// Useful for testing or programmatic mode changes.
func (h *KeyboardHandler) EnterPaneMode() {
	timeout := time.Duration(h.cfg.Workspace.PaneMode.TimeoutMilliseconds) * time.Millisecond
	h.modal.EnterPaneMode(h.ctx, timeout)
}

// EnterSessionMode programmatically enters session mode.
// Useful for testing or programmatic mode changes.
func (h *KeyboardHandler) EnterSessionMode() {
	timeout := time.Duration(h.cfg.Session.SessionMode.TimeoutMilliseconds) * time.Millisecond
	h.modal.EnterSessionMode(h.ctx, timeout)
}

// ExitMode programmatically exits modal mode.
// Useful for testing or programmatic mode changes.
func (h *KeyboardHandler) ExitMode() {
	h.modal.ExitMode(h.ctx)
}

// handleKeyRelease processes a key release event for accent detection.
func (h *KeyboardHandler) handleKeyRelease(keyval uint) {
	h.mu.RLock()
	accentHandler := h.accentHandler
	h.mu.RUnlock()

	if accentHandler == nil {
		return
	}

	// Convert keyval to rune for the accent handler
	if char := keyvalToRune(keyval); char != 0 {
		accentHandler.OnKeyReleased(h.ctx, char)
	}
}

// keyvalToRune converts a GTK keyval to a lowercase rune.
// Returns 0 if the keyval is not a printable character with potential accents.
func keyvalToRune(keyval uint) rune {
	// Handle lowercase a-z
	if keyval >= uint('a') && keyval <= uint('z') {
		return rune(keyval)
	}
	// Handle uppercase A-Z (convert to lowercase for accent lookup)
	if keyval >= uint('A') && keyval <= uint('Z') {
		return rune(keyval + ('a' - 'A'))
	}
	return 0
}
