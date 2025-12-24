// Package input provides keyboard event handling and modal input mode management.
package input

import (
	"context"

	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// GlobalShortcutHandler manages keyboard shortcuts that must work globally,
// even when WebView has focus. It uses GtkShortcutController with GTK_SHORTCUT_SCOPE_GLOBAL
// to intercept shortcuts before they reach the WebView.
type GlobalShortcutHandler struct {
	controller *gtk.ShortcutController
	onAction   ActionHandler
	ctx        context.Context

	// Keep references to callbacks to prevent GC from collecting them
	callbacks []gtk.ShortcutFunc
}

// NewGlobalShortcutHandler creates a new global shortcut handler for shortcuts
// that need to work even when WebView has focus (like Alt+1-9 for tab switching).
func NewGlobalShortcutHandler(
	ctx context.Context,
	window *gtk.ApplicationWindow,
	onAction ActionHandler,
) *GlobalShortcutHandler {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating global shortcut handler")

	h := &GlobalShortcutHandler{
		controller: gtk.NewShortcutController(),
		onAction:   onAction,
		ctx:        ctx,
		callbacks:  make([]gtk.ShortcutFunc, 0),
	}

	if h.controller == nil {
		log.Error().Msg("failed to create shortcut controller")
		return nil
	}

	// Set global scope - this is the key to making shortcuts work
	// even when WebView has focus
	h.controller.SetScope(gtk.ShortcutScopeGlobalValue)

	// Register Alt+1 through Alt+9 for tab switching
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

	// Alt+0 for tab 10
	h.registerShortcut(uint(gdk.KEY_0), gdk.AltMaskValue, ActionSwitchTabIndex10)
	log.Trace().
		Uint("keyval", uint(gdk.KEY_0)).
		Str("action", string(ActionSwitchTabIndex10)).
		Msg("registered global shortcut")

	// Alt+Tab for switching to last active tab
	h.registerShortcut(uint(gdk.KEY_Tab), gdk.AltMaskValue, ActionSwitchLastTab)
	log.Trace().
		Uint("keyval", uint(gdk.KEY_Tab)).
		Str("action", string(ActionSwitchLastTab)).
		Msg("registered global shortcut")

	// Attach to window
	window.AddController(&h.controller.EventController)

	log.Debug().
		Int("shortcuts", len(h.callbacks)).
		Msg("global shortcut handler created and attached")

	return h
}

// registerShortcut creates and registers a single shortcut with the controller.
func (h *GlobalShortcutHandler) registerShortcut(keyval uint, modifiers gdk.ModifierType, action Action) {
	// Create trigger for this key combination
	trigger := gtk.NewKeyvalTrigger(keyval, modifiers)
	if trigger == nil {
		logging.FromContext(h.ctx).Error().
			Uint("keyval", keyval).
			Msg("failed to create keyval trigger")
		return
	}

	// Create callback action
	// We need to capture the action in the closure
	actionToDispatch := action
	callback := gtk.ShortcutFunc(func(_ uintptr, _ *glib.Variant, _ uintptr) bool {
		log := logging.FromContext(h.ctx)
		log.Debug().
			Str("action", string(actionToDispatch)).
			Msg("global shortcut triggered")

		if h.onAction != nil {
			if err := h.onAction(h.ctx, actionToDispatch); err != nil {
				log.Error().
					Err(err).
					Str("action", string(actionToDispatch)).
					Msg("global shortcut action failed")
			}
		}
		return true // Event consumed
	})

	// Store callback reference to prevent GC
	h.callbacks = append(h.callbacks, callback)

	// Create the shortcut action
	shortcutAction := gtk.NewCallbackAction(&callback, 0, nil)
	if shortcutAction == nil {
		logging.FromContext(h.ctx).Error().
			Uint("keyval", keyval).
			Msg("failed to create callback action")
		return
	}

	// Create the shortcut combining trigger and action
	shortcut := gtk.NewShortcut(&trigger.ShortcutTrigger, &shortcutAction.ShortcutAction)
	if shortcut == nil {
		logging.FromContext(h.ctx).Error().
			Uint("keyval", keyval).
			Msg("failed to create shortcut")
		return
	}

	// Add to controller
	h.controller.AddShortcut(shortcut)
}

// Detach removes the global shortcut handler from the window.
// Note: GTK handles cleanup when the widget is destroyed,
// but we clear our references here.
func (h *GlobalShortcutHandler) Detach() {
	h.controller = nil
	h.callbacks = nil
}
