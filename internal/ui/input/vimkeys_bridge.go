package input

import (
	"strings"

	"github.com/bnema/dumber/internal/domain/vimkeys"
	"github.com/bnema/puregotk/v4/gdk"
)

// namedKeyvalSym maps a GDK named keyval onto a canonical vimkeys symbol.
// Immutable switch mirrors Phase 1 parser quality (no mutable package map).
func namedKeyvalSym(keyval uint) (string, bool) {
	switch keyval {
	case uint(gdk.KEY_Escape):
		return "Esc", true
	case uint(gdk.KEY_Return), uint(gdk.KEY_KP_Enter):
		return "CR", true
	case uint(gdk.KEY_Tab), uint(gdk.KEY_ISO_Left_Tab):
		return "Tab", true
	case uint(gdk.KEY_BackSpace):
		return "BackSpace", true
	case uint(gdk.KEY_space):
		return "Space", true
	case uint(gdk.KEY_Left):
		return "Left", true
	case uint(gdk.KEY_Right):
		return "Right", true
	case uint(gdk.KEY_Up):
		return "Up", true
	case uint(gdk.KEY_Down):
		return "Down", true
	case uint(gdk.KEY_Home):
		return "Home", true
	case uint(gdk.KEY_End):
		return "End", true
	case uint(gdk.KEY_Page_Up):
		return "PageUp", true
	case uint(gdk.KEY_Page_Down):
		return "PageDown", true
	case uint(gdk.KEY_Delete):
		return "Delete", true
	default:
		return "", false
	}
}

// KeyvalToVimKey converts a GDK key event into a vimkeys.Key.
// Named keys keep modifiers. Printable non-letter glyphs drop Shift because the
// glyph already encodes it; Ctrl and Alt are preserved.
func KeyvalToVimKey(keyval uint, state gdk.ModifierType) (vimkeys.Key, bool) {
	mods := vimModsFromState(state)
	if sym, ok := namedKeyvalSym(keyval); ok {
		return vimkeys.Key{Sym: sym, Mods: mods}, true
	}

	r := rune(keyvalToUnicode(keyval))
	if r == 0 || r < 0x20 {
		return vimkeys.Key{}, false
	}
	if r >= 'A' && r <= 'Z' {
		return vimkeys.Key{Sym: strings.ToLower(string(r)), Mods: mods}, true
	}
	if r >= 'a' && r <= 'z' {
		return vimkeys.Key{Sym: string(r), Mods: mods}, true
	}
	return vimkeys.Key{Sym: string(r), Mods: mods &^ vimkeys.ModShift}, true
}

func vimModsFromState(state gdk.ModifierType) vimkeys.Mods {
	var mods vimkeys.Mods
	if state&gdk.ControlMaskValue != 0 {
		mods |= vimkeys.ModCtrl
	}
	if state&gdk.ShiftMaskValue != 0 {
		mods |= vimkeys.ModShift
	}
	if state&gdk.AltMaskValue != 0 {
		mods |= vimkeys.ModAlt
	}
	return mods
}

// keyvalToUnicode mirrors gdk_keyval_to_unicode for Latin-1 graphic keysyms
// (identity mapping) and falls back to GDK for the remaining keyvals.
func keyvalToUnicode(keyval uint) uint32 {
	// ASCII graphic Latin-1 keysyms share their Unicode code points.
	if keyval >= 0x20 && keyval <= 0x7e {
		return uint32(keyval)
	}
	return gdk.KeyvalToUnicode(keyval)
}
