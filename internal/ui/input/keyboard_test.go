package input

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsShortcutModified(t *testing.T) {
	tests := []struct {
		name     string
		mods     Modifier
		expected bool
	}{
		{"no modifier", ModNone, false},
		{"shift only", ModShift, false},
		{"ctrl", ModCtrl, true},
		{"alt", ModAlt, true},
		{"ctrl+shift", ModCtrl | ModShift, true},
		{"alt+shift", ModAlt | ModShift, true},
		{"ctrl+alt", ModCtrl | ModAlt, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsShortcutModified(tt.mods))
		})
	}
}

func TestIsTextInputKey(t *testing.T) {
	tests := []struct {
		name     string
		keyval   uint
		expected bool
	}{
		// Printable characters
		{"lowercase a", uint('a'), true},
		{"lowercase z", uint('z'), true},
		{"uppercase A", uint('A'), true},
		{"digit 1", uint('1'), true},
		{"space", 0x020, true},
		{"period", uint('.'), true},
		{"slash", uint('/'), true},
		{"apostrophe", 0x027, true},
		{"grave accent", 0x060, true},
		{"tilde", 0x07e, true},

		// Dead keys (used by US International layout)
		{"dead acute", 0xfe51, true},
		{"dead grave", 0xfe50, true},
		{"dead circumflex", 0xfe52, true},
		{"dead tilde", 0xfe53, true},
		{"dead diaeresis", 0xfe57, true},

		// Non-text keys (should NOT be treated as text)
		{"escape", 0xff1b, false},
		{"return", 0xff0d, false},
		{"tab", 0xff09, false},
		{"F1", 0xffbe, false},
		{"F12", 0xffc9, false},
		{"left arrow", 0xff51, false},
		{"right arrow", 0xff53, false},
		{"backspace", 0xff08, false},
		{"delete", 0xffff, false},
		{"home", 0xff50, false},
		{"end", 0xff57, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsTextInputKey(tt.keyval), "keyval 0x%x", tt.keyval)
		})
	}
}
