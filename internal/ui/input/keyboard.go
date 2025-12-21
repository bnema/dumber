package input

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// ActionHandler is called when a keyboard action is triggered.
// It receives the context and the action to perform.
// Return an error if the action fails.
type ActionHandler func(ctx context.Context, action Action) error

// KeyboardHandler processes keyboard events and dispatches actions.
// It manages modal input modes and routes key events to the appropriate handlers.
type KeyboardHandler struct {
	shortcuts *ShortcutSet
	modal     *ModalState
	cfg       *config.WorkspaceConfig

	// Action handler callback
	onAction ActionHandler
	// Optional bypass check (e.g., omnibox visible)
	shouldBypass func() bool

	// GTK controller (nil until attached)
	controller *gtk.EventControllerKey

	ctx context.Context
	mu  sync.RWMutex
}

// NewKeyboardHandler creates a new keyboard handler.
func NewKeyboardHandler(
	ctx context.Context,
	cfg *config.WorkspaceConfig,
) *KeyboardHandler {
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

	// Connect key pressed handler
	keyPressedCb := func(_ gtk.EventControllerKey, keyval uint, _ uint, state gdk.ModifierType) bool {
		return h.handleKeyPress(keyval, state)
	}
	h.controller.ConnectKeyPressed(&keyPressedCb)

	// Add controller to window
	window.AddController(&h.controller.EventController)

	log.Debug().Msg("keyboard handler attached to window")
}

// Detach removes the keyboard handler.
// Note: GTK handles cleanup when the widget is destroyed,
// but we clear our reference here.
func (h *KeyboardHandler) Detach() {
	h.controller = nil
}

// handleKeyPress processes a key press event.
// Returns true if the event was handled and should not propagate further.
func (h *KeyboardHandler) handleKeyPress(keyval uint, state gdk.ModifierType) bool {
	h.mu.RLock()
	shouldBypass := h.shouldBypass
	h.mu.RUnlock()
	if shouldBypass != nil && shouldBypass() {
		return false
	}

	// Build KeyBinding from event
	modifiers := Modifier(state) & modifierMask
	binding := KeyBinding{
		Keyval:    keyval,
		Modifiers: modifiers,
	}

	mode := h.modal.Mode()

	// Look up the action for this key binding
	action, found := h.shortcuts.Lookup(binding, mode)

	if !found {
		if mode != ModeNormal {
			// In modal mode, consume unrecognized keys to prevent WebView handling
			return true
		}
		// In normal mode, let the key pass through to WebView
		return false
	}

	// Handle special mode actions first
	switch action {
	case ActionEnterTabMode:
		timeout := time.Duration(h.cfg.TabMode.TimeoutMilliseconds) * time.Millisecond
		h.modal.EnterTabMode(h.ctx, timeout)
		return true

	case ActionEnterPaneMode:
		timeout := time.Duration(h.cfg.PaneMode.TimeoutMilliseconds) * time.Millisecond
		h.modal.EnterPaneMode(h.ctx, timeout)
		return true

	case ActionExitMode:
		h.modal.ExitMode(h.ctx)
		return true
	}

	// Dispatch action to handler
	h.mu.RLock()
	handler := h.onAction
	h.mu.RUnlock()

	if handler != nil {
		if err := handler(h.ctx, action); err != nil {
			log := logging.FromContext(h.ctx)
			log.Error().
				Err(err).
				Str("action", string(action)).
				Msg("action handler error")
		}
	}

	// Auto-exit modal mode after certain actions
	if mode != ModeNormal && ShouldAutoExitMode(action) {
		h.modal.ExitMode(h.ctx)
	}

	return true // Consumed the key
}

// EnterTabMode programmatically enters tab mode.
// Useful for testing or programmatic mode changes.
func (h *KeyboardHandler) EnterTabMode() {
	timeout := time.Duration(h.cfg.TabMode.TimeoutMilliseconds) * time.Millisecond
	h.modal.EnterTabMode(h.ctx, timeout)
}

// EnterPaneMode programmatically enters pane mode.
// Useful for testing or programmatic mode changes.
func (h *KeyboardHandler) EnterPaneMode() {
	timeout := time.Duration(h.cfg.PaneMode.TimeoutMilliseconds) * time.Millisecond
	h.modal.EnterPaneMode(h.ctx, timeout)
}

// ExitMode programmatically exits modal mode.
// Useful for testing or programmatic mode changes.
func (h *KeyboardHandler) ExitMode() {
	h.modal.ExitMode(h.ctx)
}
