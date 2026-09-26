package input

import (
	"github.com/bnema/dumber/internal/domain/vimkeys"
	"github.com/bnema/puregotk/v4/gdk"
)

// PageKeyForwarder receives canonical vimkeys strings ("j", "J", "<Escape>")
// while an in-page Vim interaction owns keyboard input.
type PageKeyForwarder func(key string)

// SetPageKeyCapture routes Vim Mode keys to fn until ClearPageKeyCapture or
// Vim Mode exits. Pending sequences are discarded so they cannot complete
// against keys meant for the page.
func (h *KeyboardHandler) SetPageKeyCapture(fn PageKeyForwarder) {
	h.ResetPendingSequence()
	h.mu.Lock()
	h.pageKeyCapture = fn
	h.mu.Unlock()
}

// ClearPageKeyCapture returns key handling to Vim Mode bindings.
func (h *KeyboardHandler) ClearPageKeyCapture() {
	h.mu.Lock()
	h.pageKeyCapture = nil
	h.mu.Unlock()
}

// PageKeyCaptureActive reports whether an in-page interaction owns keys.
func (h *KeyboardHandler) PageKeyCaptureActive() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.pageKeyCapture != nil
}

// forwardPageKey sends a Vim Mode key to the active page capture. Escape is
// forwarded and also releases the capture locally so a page that never
// reports completion cannot trap the keyboard.
func (h *KeyboardHandler) forwardPageKey(mode Mode, keyval uint, state gdk.ModifierType) bool {
	if mode != ModeVim {
		return false
	}
	h.mu.RLock()
	forward := h.pageKeyCapture
	h.mu.RUnlock()
	if forward == nil {
		return false
	}
	if isModifierKeyval(keyval) {
		return true
	}
	key, ok := KeyvalToVimKey(keyval, state)
	if !ok {
		return true
	}
	if key.Sym == "Esc" {
		// Any Escape ends the interaction, so the page must see a plain one.
		key.Mods = 0
		h.ClearPageKeyCapture()
	}
	forward(vimkeys.Sequence{key}.String())
	// Captured keys bypass dispatchAction, so keep Vim Mode's timeout alive.
	h.modal.ResetTimeout(h.ctx)
	return true
}

func isModifierKeyval(keyval uint) bool {
	switch keyval {
	case uint(gdk.KEY_Shift_L), uint(gdk.KEY_Shift_R),
		uint(gdk.KEY_Control_L), uint(gdk.KEY_Control_R),
		uint(gdk.KEY_Alt_L), uint(gdk.KEY_Alt_R),
		uint(gdk.KEY_Super_L), uint(gdk.KEY_Super_R),
		uint(gdk.KEY_Caps_Lock), uint(gdk.KEY_ISO_Level3_Shift):
		return true
	default:
		return false
	}
}
