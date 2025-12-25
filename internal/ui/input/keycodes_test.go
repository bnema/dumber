package input

import (
	"fmt"
	"testing"
)

// TestKeycodeConstants verifies the hardware keycode constants are correct.
// These are XKB keycodes (evdev + 8) for the number row keys.
func TestKeycodeConstants(t *testing.T) {
	tests := []struct {
		name     string
		keycode  uint
		expected uint
	}{
		{"KeycodeDigit1", KeycodeDigit1, 10},
		{"KeycodeDigit2", KeycodeDigit2, 11},
		{"KeycodeDigit3", KeycodeDigit3, 12},
		{"KeycodeDigit4", KeycodeDigit4, 13},
		{"KeycodeDigit5", KeycodeDigit5, 14},
		{"KeycodeDigit6", KeycodeDigit6, 15},
		{"KeycodeDigit7", KeycodeDigit7, 16},
		{"KeycodeDigit8", KeycodeDigit8, 17},
		{"KeycodeDigit9", KeycodeDigit9, 18},
		{"KeycodeDigit0", KeycodeDigit0, 19},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.keycode != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.keycode, tt.expected)
			}
		})
	}
}

// TestKeycodeToTabActionMapping verifies all number row keycodes map to the correct tab actions.
func TestKeycodeToTabActionMapping(t *testing.T) {
	tests := []struct {
		keycode        uint
		expectedAction Action
	}{
		{KeycodeDigit1, ActionSwitchTabIndex1},
		{KeycodeDigit2, ActionSwitchTabIndex2},
		{KeycodeDigit3, ActionSwitchTabIndex3},
		{KeycodeDigit4, ActionSwitchTabIndex4},
		{KeycodeDigit5, ActionSwitchTabIndex5},
		{KeycodeDigit6, ActionSwitchTabIndex6},
		{KeycodeDigit7, ActionSwitchTabIndex7},
		{KeycodeDigit8, ActionSwitchTabIndex8},
		{KeycodeDigit9, ActionSwitchTabIndex9},
		{KeycodeDigit0, ActionSwitchTabIndex10},
	}

	for _, tt := range tests {
		t.Run(string(tt.expectedAction), func(t *testing.T) {
			action, ok := KeycodeToTabAction[tt.keycode]
			if !ok {
				t.Errorf("keycode %d not found in KeycodeToTabAction map", tt.keycode)
				return
			}
			if action != tt.expectedAction {
				t.Errorf("KeycodeToTabAction[%d] = %s, want %s", tt.keycode, action, tt.expectedAction)
			}
		})
	}
}

// TestKeycodeToTabActionCompleteness verifies the map contains exactly 10 entries.
func TestKeycodeToTabActionCompleteness(t *testing.T) {
	expectedCount := 10
	if len(KeycodeToTabAction) != expectedCount {
		t.Errorf("KeycodeToTabAction has %d entries, want %d", len(KeycodeToTabAction), expectedCount)
	}
}

// TestKeycodeToTabActionNoUnexpectedKeycodes verifies only valid keycodes are in the map.
func TestKeycodeToTabActionNoUnexpectedKeycodes(t *testing.T) {
	validKeycodes := map[uint]bool{
		KeycodeDigit1: true,
		KeycodeDigit2: true,
		KeycodeDigit3: true,
		KeycodeDigit4: true,
		KeycodeDigit5: true,
		KeycodeDigit6: true,
		KeycodeDigit7: true,
		KeycodeDigit8: true,
		KeycodeDigit9: true,
		KeycodeDigit0: true,
	}

	for keycode := range KeycodeToTabAction {
		if !validKeycodes[keycode] {
			t.Errorf("unexpected keycode %d in KeycodeToTabAction map", keycode)
		}
	}
}

// European keyboard layout test scenarios.
// These test the concept that hardware keycodes are layout-independent.
//
// On all these layouts, the physical "1" key has hardware keycode 10,
// regardless of what character it produces:
//
// | Layout      | Physical "1" key produces | Keycode |
// |-------------|---------------------------|---------|
// | QWERTY (US) | 1                         | 10      |
// | QWERTY (UK) | 1                         | 10      |
// | AZERTY (FR) | &                         | 10      |
// | AZERTY (BE) | &                         | 10      |
// | QWERTZ (DE) | 1                         | 10      |
// | QWERTZ (CH) | 1                         | 10      |
// | Dvorak      | 1                         | 10      |

// TestEuropeanLayoutKeycodes documents and verifies the keycode behavior
// expected for various European keyboard layouts.
func TestEuropeanLayoutKeycodes(t *testing.T) {
	// This test documents that our solution works for all these layouts
	// because we use hardware keycodes, not keyvals.
	//
	// The keycode for the physical number row is always 10-19,
	// regardless of what character the key produces.

	layouts := []struct {
		name        string
		description string
		// Physical key position (1-9, 0) always maps to keycode 10-19
		// The keyval differs but we don't care - we use keycode
	}{
		{
			name:        "QWERTY_US",
			description: "US English - 1234567890 produce digits directly",
		},
		{
			name:        "QWERTY_UK",
			description: "UK English - 1234567890 produce digits directly",
		},
		{
			name:        "AZERTY_FR",
			description: "French - &é\"'(-è_çà produce symbols, Shift for digits",
		},
		{
			name:        "AZERTY_BE",
			description: "Belgian - &é\"'(§è!çà produce symbols, Shift for digits",
		},
		{
			name:        "QWERTZ_DE",
			description: "German - 1234567890 produce digits directly (symbols on Shift)",
		},
		{
			name:        "QWERTZ_CH",
			description: "Swiss German - 1234567890 produce digits directly",
		},
		{
			name:        "QWERTZ_AT",
			description: "Austrian - 1234567890 produce digits directly",
		},
		{
			name:        "Dvorak",
			description: "Dvorak - number row same as QWERTY",
		},
		{
			name:        "Colemak",
			description: "Colemak - number row same as QWERTY",
		},
		{
			name:        "Portuguese_PT",
			description: "Portuguese - 1234567890 produce digits directly",
		},
		{
			name:        "Spanish_ES",
			description: "Spanish - 1234567890 produce digits directly",
		},
		{
			name:        "Italian_IT",
			description: "Italian - 1234567890 produce digits directly",
		},
		{
			name:        "Nordic",
			description: "Nordic layouts (SE/NO/DK/FI) - 1234567890 produce digits directly",
		},
	}

	for _, layout := range layouts {
		t.Run(layout.name, func(t *testing.T) {
			// For all layouts, pressing Alt + physical "1" key should:
			// 1. Have hardware keycode 10 (KeycodeDigit1)
			// 2. Map to ActionSwitchTabIndex1

			action, ok := KeycodeToTabAction[KeycodeDigit1]
			if !ok {
				t.Errorf("[%s] keycode 10 (physical '1') not mapped", layout.name)
				return
			}
			if action != ActionSwitchTabIndex1 {
				t.Errorf("[%s] keycode 10 maps to %s, want %s",
					layout.name, action, ActionSwitchTabIndex1)
			}

			// Document the layout for clarity
			t.Logf("Layout %s: %s - keycode fallback will work", layout.name, layout.description)
		})
	}
}

// TestAZERTYSpecificScenario tests the specific scenario from issue #25.
// On French AZERTY, pressing Alt+1 produces Alt+& (ampersand) because
// the digit 1 requires Shift. Our keycode fallback handles this.
func TestAZERTYSpecificScenario(t *testing.T) {
	// Scenario: User on French AZERTY keyboard presses Alt + physical "1" key
	//
	// What happens:
	// 1. GTK receives: keyval=ampersand (0x26), keycode=10, modifiers=Alt
	// 2. keyval lookup for "Alt+&" fails (no such shortcut defined)
	// 3. Keycode fallback: keycode 10 with Alt modifier
	// 4. KeycodeToTabAction[10] = ActionSwitchTabIndex1
	// 5. Tab 1 is activated

	// The keycode for physical "1" on ALL layouts
	physicalKey1Keycode := uint(10)

	// Verify our mapping handles this
	action, ok := KeycodeToTabAction[physicalKey1Keycode]
	if !ok {
		t.Fatal("keycode 10 should be mapped for AZERTY support")
	}
	if action != ActionSwitchTabIndex1 {
		t.Errorf("keycode 10 should map to ActionSwitchTabIndex1, got %s", action)
	}
}

// TestNonNumberRowKeycodes verifies that non-number-row keycodes don't accidentally match.
func TestNonNumberRowKeycodes(t *testing.T) {
	// Some keycodes that should NOT be in our map
	invalidKeycodes := []uint{
		0,  // Invalid
		1,  // Escape
		9,  // Before number row
		20, // After number row (minus key)
		24, // Q key
		38, // A key
		50, // Shift
		64, // Alt
	}

	for _, keycode := range invalidKeycodes {
		if _, ok := KeycodeToTabAction[keycode]; ok {
			t.Errorf("keycode %d should not be in KeycodeToTabAction map", keycode)
		}
	}
}

// TestKeycodeToDigitIndex verifies the keycode to digit index conversion.
func TestKeycodeToDigitIndex(t *testing.T) {
	tests := []struct {
		keycode       uint
		expectedIndex int
		expectedOk    bool
	}{
		{KeycodeDigit1, 0, true},
		{KeycodeDigit2, 1, true},
		{KeycodeDigit3, 2, true},
		{KeycodeDigit4, 3, true},
		{KeycodeDigit5, 4, true},
		{KeycodeDigit6, 5, true},
		{KeycodeDigit7, 6, true},
		{KeycodeDigit8, 7, true},
		{KeycodeDigit9, 8, true},
		{KeycodeDigit0, 9, true},
		// Invalid keycodes
		{0, -1, false},
		{9, -1, false},  // Before number row
		{20, -1, false}, // After number row
		{64, -1, false}, // Alt key
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("keycode_%d", tt.keycode), func(t *testing.T) {
			index, ok := KeycodeToDigitIndex(tt.keycode)
			if ok != tt.expectedOk {
				t.Errorf("KeycodeToDigitIndex(%d) ok = %v, want %v", tt.keycode, ok, tt.expectedOk)
			}
			if index != tt.expectedIndex {
				t.Errorf("KeycodeToDigitIndex(%d) index = %d, want %d", tt.keycode, index, tt.expectedIndex)
			}
		})
	}
}

// TestKeycodeToDigitIndexForOmnibox verifies the function works for omnibox Ctrl+1-9/0 shortcuts.
func TestKeycodeToDigitIndexForOmnibox(t *testing.T) {
	// Simulate omnibox usage: Ctrl+1 should select first item (index 0)
	// Ctrl+0 should select 10th item (index 9)

	// Physical "1" key -> index 0 (first omnibox result)
	index, ok := KeycodeToDigitIndex(KeycodeDigit1)
	if !ok || index != 0 {
		t.Errorf("Ctrl+1 should map to index 0, got index=%d ok=%v", index, ok)
	}

	// Physical "0" key -> index 9 (10th omnibox result)
	index, ok = KeycodeToDigitIndex(KeycodeDigit0)
	if !ok || index != 9 {
		t.Errorf("Ctrl+0 should map to index 9, got index=%d ok=%v", index, ok)
	}
}
