// Package input provides keyboard event handling and modal input mode management.
package input

// Hardware keycodes for the number row keys.
// These are XKB keycodes (evdev + 8) representing physical key positions,
// independent of keyboard layout. This enables shortcuts like Alt+1 to work
// on non-QWERTY layouts (AZERTY, QWERTZ, etc.) where the number keys produce
// different characters without Shift.
//
// For example, on French AZERTY:
//   - Physical "1" key produces "&" (keyval=ampersand)
//   - But the hardware keycode is still 10
//   - So Alt + physical "1" can be detected via keycode even though keyval differs
const (
	KeycodeDigit1 uint = 10 // Physical '1' key position
	KeycodeDigit2 uint = 11 // Physical '2' key position
	KeycodeDigit3 uint = 12 // Physical '3' key position
	KeycodeDigit4 uint = 13 // Physical '4' key position
	KeycodeDigit5 uint = 14 // Physical '5' key position
	KeycodeDigit6 uint = 15 // Physical '6' key position
	KeycodeDigit7 uint = 16 // Physical '7' key position
	KeycodeDigit8 uint = 17 // Physical '8' key position
	KeycodeDigit9 uint = 18 // Physical '9' key position
	KeycodeDigit0 uint = 19 // Physical '0' key position
)

// KeycodeToTabAction maps hardware keycodes for the number row to tab switch actions.
// This enables Alt+1-9/0 shortcuts to work on non-QWERTY keyboards by matching
// the physical key position rather than the translated keyval.
var KeycodeToTabAction = map[uint]Action{
	KeycodeDigit1: ActionSwitchTabIndex1,
	KeycodeDigit2: ActionSwitchTabIndex2,
	KeycodeDigit3: ActionSwitchTabIndex3,
	KeycodeDigit4: ActionSwitchTabIndex4,
	KeycodeDigit5: ActionSwitchTabIndex5,
	KeycodeDigit6: ActionSwitchTabIndex6,
	KeycodeDigit7: ActionSwitchTabIndex7,
	KeycodeDigit8: ActionSwitchTabIndex8,
	KeycodeDigit9: ActionSwitchTabIndex9,
	KeycodeDigit0: ActionSwitchTabIndex10,
}

// KeycodeToDigitIndex converts a hardware keycode to a digit index (0-9).
// Returns the index and true if the keycode is a number row key, or -1 and false otherwise.
// This is useful for components like omnibox that need Ctrl+1-9/0 shortcuts.
//
// Mapping:
//   - Keycode 10 (physical '1') -> index 0
//   - Keycode 11 (physical '2') -> index 1
//   - ...
//   - Keycode 18 (physical '9') -> index 8
//   - Keycode 19 (physical '0') -> index 9
func KeycodeToDigitIndex(keycode uint) (index int, ok bool) {
	if keycode >= KeycodeDigit1 && keycode <= KeycodeDigit0 {
		return int(keycode - KeycodeDigit1), true
	}
	return -1, false
}
