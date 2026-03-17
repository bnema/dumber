package input

// Manual Testing: Dead Key / Compose Support
//
// To verify native dead key support works correctly:
//
//  1. Set keyboard layout to "US International with dead keys"
//  2. Open dumber, navigate to a page with a text input (e.g. Google search)
//  3. Click into the text input field
//  4. Type: ' then e -- should produce e with acute accent
//  5. Type: " then u -- should produce u with diaeresis
//  6. Type: ` then a -- should produce a with grave accent
//  7. Type: ~ then n -- should produce n with tilde
//  8. Verify Ctrl+L still opens omnibox
//  9. Verify Alt+1-9 still switches tabs
//  10. Verify modal modes (Ctrl+T for tab mode) still work
//  11. In omnibox, verify long-press accent picker still works (hold 'e' for 400ms)
//  12. Verify Escape closes omnibox / exits modal modes
//  13. Verify session manager overlay captures all keyboard input

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/glib"
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
	workspace *entity.WorkspaceConfig
	session   *entity.SessionConfig

	// Action handler callback
	onAction ActionHandler
	// Optional routing callback that determines how a key should be handled.
	// Returns RouteHandleShortcuts (default), RoutePassToWidget (let focused
	// widget handle it), or RouteAccentDetection (long-press accent for GTK entries).
	routeKey func(KeyContext) KeyRoute
	// Optional accent handler for dead keys support
	accentHandler AccentHandler
	// Optional escape hook for app-level overlays
	onEscape func(ctx context.Context) bool

	// GTK controller (nil until attached)
	controller *gtk.EventControllerKey

	// Callback retention: must stay reachable by Go GC.
	keyPressedCb  func(gtk.EventControllerKey, uint, uint, gdk.ModifierType) bool
	keyReleasedCb func(gtk.EventControllerKey, uint, uint, gdk.ModifierType)

	ctx context.Context
	mu  sync.RWMutex
}

// NewKeyboardHandler creates a new keyboard handler.
func NewKeyboardHandler(ctx context.Context, workspace *entity.WorkspaceConfig, session *entity.SessionConfig) *KeyboardHandler {
	log := logging.FromContext(ctx)

	log.Debug().Msg("creating keyboard handler")

	h := &KeyboardHandler{
		shortcuts: NewShortcutSet(ctx, workspace, session),
		modal:     NewModalState(ctx),
		workspace: workspace,
		session:   session,
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

// ReloadShortcuts rebuilds the shortcut set from new config values.
// This enables hot-reloading of keybindings without restarting.
func (h *KeyboardHandler) ReloadShortcuts(ctx context.Context, workspace *entity.WorkspaceConfig, session *entity.SessionConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()

	log := logging.FromContext(ctx)
	log.Debug().Msg("reloading shortcuts from config")

	h.shortcuts = NewShortcutSet(ctx, workspace, session)
	h.workspace = workspace
	h.session = session
}

// SetOnModeChange sets the callback for mode changes (for UI updates).
func (h *KeyboardHandler) SetOnModeChange(fn func(from, to Mode)) {
	h.modal.SetOnModeChange(func(from, to Mode) {
		// Switch controller phase: capture during modal, bubble for normal.
		// This ensures plain modal keys (s, h, etc.) reach the handler
		// before WebView, while normal typing still goes through WebKit IM.
		if to == ModeNormal {
			h.setControllerPhase(gtk.PhaseBubbleValue)
		} else if from == ModeNormal {
			h.setControllerPhase(gtk.PhaseCaptureValue)
		}
		// Forward to app-level callback
		if fn != nil {
			fn(from, to)
		}
	})
}

// setControllerPhase changes the propagation phase of the key controller.
// Capture phase during modal modes so plain keys reach the handler before WebView.
// Bubble phase in normal mode for WebKit IM/dead-key support.
//
// This is called synchronously from onModeChange so the phase switch takes
// effect before the next key event. The caller must ensure GTK thread safety;
// modal timeouts dispatch to the GTK main thread via glib.IdleAdd before
// calling ExitMode, so the onModeChange callback always runs on the GTK thread.
func (h *KeyboardHandler) setControllerPhase(phase gtk.PropagationPhase) {
	if h.controller == nil {
		return
	}
	h.controller.SetPropagationPhase(phase)
}

// SetRouteKey sets the callback that determines how each key event should be routed.
// The callback receives key context and returns a KeyRoute indicating whether the
// key should be handled by the shortcut system, passed to the focused widget
// (for IM/compose support), or routed through accent detection.
//
// If not set, all keys are routed through the shortcut system (RouteHandleShortcuts).
func (h *KeyboardHandler) SetRouteKey(fn func(KeyContext) KeyRoute) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.routeKey = fn
}

// SetAccentHandler sets the handler for long-press accent detection.
func (h *KeyboardHandler) SetAccentHandler(handler AccentHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.accentHandler = handler
}

// SetOnEscape sets an optional callback invoked for plain Escape in normal mode.
// Return true to consume the key and stop further handling.
func (h *KeyboardHandler) SetOnEscape(fn func(ctx context.Context) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onEscape = fn
}

// Mode returns the current input mode.
func (h *KeyboardHandler) Mode() Mode {
	return h.modal.Mode()
}

// AttachTo attaches the keyboard handler to a GTK window.
// The handler runs in the bubble phase so that WebKit's IM pipeline (including
// dead-key / compose sequences) processes key events first. App-level shortcuts
// are still intercepted because WebKit only consumes events it recognizes as
// text input; unhandled events bubble up to this controller.
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

	// Bubble phase: let WebKit IM handle key events first (dead keys, compose
	// sequences, input methods). Events not consumed by WebKit bubble up here
	// so app shortcuts still work.
	h.controller.SetPropagationPhase(gtk.PhaseBubbleValue)

	// Wire GTK main thread scheduler for modal timeouts. Timer goroutines
	// must dispatch ExitMode to the GTK thread because onModeChange may
	// call setControllerPhase (a GTK operation).
	h.modal.SetMainThreadScheduler(func(fn func()) {
		cb := glib.SourceFunc(func(_ uintptr) bool {
			fn()
			return false
		})
		glib.IdleAdd(&cb, 0)
	})

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
func (h *KeyboardHandler) handleKeyPress(keyval, keycode uint, state gdk.ModifierType) bool {
	log := logging.FromContext(h.ctx)

	h.mu.RLock()
	routeKey := h.routeKey
	accentHandler := h.accentHandler
	onEscape := h.onEscape
	h.mu.RUnlock()

	// Accent picker takes absolute priority when visible -- it has its own key controller
	if accentHandler != nil && accentHandler.IsPickerVisible() {
		return false
	}

	modifiers := Modifier(state) & modifierMask

	// Escape in normal mode: check app-level escape hook first
	if h.modal.Mode() == ModeNormal && keyval == uint(gdk.KEY_Escape) && modifiers == 0 {
		if onEscape != nil && onEscape(h.ctx) {
			return true
		}
	}

	// Determine routing for this key event
	route := RouteHandleShortcuts // default: process through shortcut system
	if routeKey != nil && h.modal.Mode() == ModeNormal {
		route = routeKey(KeyContext{
			Keyval:    keyval,
			Keycode:   keycode,
			Modifiers: modifiers,
		})
	}

	switch route {
	case RoutePassToWidget:
		// Let the focused widget handle this key directly (WebView IM, etc.)
		// No accent detection here -- WebView has its own JS-based accent detection.
		log.Trace().Uint("keyval", keyval).Msg("routing key to focused widget")
		return false

	case RouteAccentDetection:
		// Try long-press accent detection for GTK Entry widgets (omnibox, find bar).
		// If the accent handler doesn't consume the key, pass it to the widget.
		if h.tryAccentDetection(accentHandler, keyval, modifiers) {
			return true
		}
		log.Trace().Uint("keyval", keyval).Msg("accent detection declined, routing to widget")
		return false

	case RouteHandleShortcuts:
		// Fall through to shortcut processing below
	}

	// --- Shortcut processing path ---

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
// It is used only for the RouteAccentDetection route (GTK Entry widgets such as
// the omnibox and find bar). RoutePassToWidget does not call tryAccentDetection;
// WebView long-press accent behavior is handled entirely by the injected JS bridge
// (accentDetectionScript) and the Go-side accent key detection script, not here.
// Returns true if the key should be suppressed (i.e. RouteAccentDetection consumed it).
func (h *KeyboardHandler) tryAccentDetection(accentHandler AccentHandler, keyval uint, modifiers Modifier) bool {
	if accentHandler == nil {
		return false
	}
	// Only consider keys without Ctrl/Alt modifiers (Shift is OK for uppercase)
	if modifiers != 0 && modifiers != ModShift {
		return false
	}
	if char := KeyvalToRune(keyval); char != 0 {
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
	h.mu.RLock()
	workspace := h.workspace
	session := h.session
	h.mu.RUnlock()

	switch action {
	case ActionEnterTabMode:
		var ms int
		if workspace != nil {
			ms = workspace.TabMode.TimeoutMilliseconds
		}
		h.modal.EnterTabMode(h.ctx, time.Duration(ms)*time.Millisecond)
		return true
	case ActionEnterPaneMode:
		var ms int
		if workspace != nil {
			ms = workspace.PaneMode.TimeoutMilliseconds
		}
		h.modal.EnterPaneMode(h.ctx, time.Duration(ms)*time.Millisecond)
		return true
	case ActionEnterSessionMode:
		var ms int
		if session != nil {
			ms = session.SessionMode.TimeoutMilliseconds
		}
		h.modal.EnterSessionMode(h.ctx, time.Duration(ms)*time.Millisecond)
		return true
	case ActionEnterResizeMode:
		if h.modal.Mode() == ModeResize {
			h.modal.ExitMode(h.ctx)
			return true
		}
		var ms int
		if workspace != nil {
			ms = workspace.ResizeMode.TimeoutMilliseconds
		}
		h.modal.EnterResizeMode(h.ctx, time.Duration(ms)*time.Millisecond)
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
	h.mu.RLock()
	workspace := h.workspace
	h.mu.RUnlock()
	var ms int
	if workspace != nil {
		ms = workspace.TabMode.TimeoutMilliseconds
	}
	h.modal.EnterTabMode(h.ctx, time.Duration(ms)*time.Millisecond)
}

// EnterPaneMode programmatically enters pane mode.
// Useful for testing or programmatic mode changes.
func (h *KeyboardHandler) EnterPaneMode() {
	h.mu.RLock()
	workspace := h.workspace
	h.mu.RUnlock()
	var ms int
	if workspace != nil {
		ms = workspace.PaneMode.TimeoutMilliseconds
	}
	h.modal.EnterPaneMode(h.ctx, time.Duration(ms)*time.Millisecond)
}

// EnterSessionMode programmatically enters session mode.
// Useful for testing or programmatic mode changes.
func (h *KeyboardHandler) EnterSessionMode() {
	h.mu.RLock()
	session := h.session
	h.mu.RUnlock()
	var ms int
	if session != nil {
		ms = session.SessionMode.TimeoutMilliseconds
	}
	h.modal.EnterSessionMode(h.ctx, time.Duration(ms)*time.Millisecond)
}

// ExitMode programmatically exits modal mode.
// Useful for testing or programmatic mode changes.
func (h *KeyboardHandler) ExitMode() {
	h.modal.ExitMode(h.ctx)
}

// DispatchAction processes an action externally triggered (e.g., by a global
// shortcut). Mode-enter actions update modal state; other actions are forwarded
// to the registered action handler.
func (h *KeyboardHandler) DispatchAction(action Action) {
	mode := h.modal.Mode()
	h.dispatchAction(action, mode)
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
	if char := KeyvalToRune(keyval); char != 0 {
		accentHandler.OnKeyReleased(h.ctx, char)
	}
}

// KeyvalToRune converts a GTK keyval to a lowercase rune.
// Returns 0 if the keyval is not a printable character with potential accents.
func KeyvalToRune(keyval uint) rune {
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

// KeyRoute determines how a key event should be routed.
type KeyRoute int

const (
	// RouteHandleShortcuts means the key should be processed by the shortcut system.
	// This is the default route -- app-level shortcuts, modal mode keys, etc.
	RouteHandleShortcuts KeyRoute = iota

	// RoutePassToWidget means the key should propagate to the focused widget.
	// Used when overlays (session manager, tab picker) are visible, or when a
	// WebView should receive text input for native IM/compose/dead-key processing.
	// Long-press accent detection still runs first for accessibility: users with
	// motor disabilities may not be able to execute rapid compose sequences.
	RoutePassToWidget

	// RouteAccentDetection means the key should go through long-press accent
	// detection, then pass to the focused widget. Used for GTK Entry widgets
	// (omnibox, find bar) and as a universal accessibility fallback.
	RouteAccentDetection
)

// KeyContext provides key event information for routing decisions.
type KeyContext struct {
	Keyval    uint     // Translated key value
	Keycode   uint     // Hardware keycode
	Modifiers Modifier // Active modifiers (masked)
}

// IsShortcutModified returns true if the modifiers indicate a shortcut combination
// (Ctrl or Alt pressed), as opposed to plain text input (no modifier or Shift-only).
// Dead keys and compose sequences only use no-modifier or Shift, so this correctly
// identifies keys that should be intercepted vs passed through to the IM pipeline.
func IsShortcutModified(modifiers Modifier) bool {
	return modifiers&(ModCtrl|ModAlt) != 0
}

// IsTextInputKey returns true if the keyval represents a text input key
// (printable character or dead key) as opposed to a function/navigation key.
// This is used to determine whether a key should be passed through to the
// WebKit IM pipeline for compose/dead-key processing.
func IsTextInputKey(keyval uint) bool {
	// Dead keys used by compose sequences (US International, etc.)
	// GDK dead key range: KEY_dead_grave (0xfe50) to KEY_dead_greek (0xfe8c)
	if keyval >= 0xfe50 && keyval <= 0xfe8c {
		return true
	}

	// Printable ASCII range (space through tilde)
	if keyval >= 0x020 && keyval <= 0x07e {
		return true
	}
	// Latin-1 supplement (non-breaking space through ÿ)
	if keyval >= 0x0a0 && keyval <= 0x0ff {
		return true
	}
	// Extended Latin and other Unicode characters mapped in GDK keyval space
	if keyval >= 0x100 && keyval <= 0x20ff {
		return true
	}

	return false
}
