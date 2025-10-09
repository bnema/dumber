package webkit

import (
	"testing"
)

// TestShortcutRoutingTable verifies the routing table configuration
func TestShortcutRoutingTable(t *testing.T) {
	tests := []struct {
		shortcut    string
		expectEntry bool
		forwardToJS bool
		blockWebKit bool
		description string
	}{
		{
			shortcut:    "alt+arrowup",
			expectEntry: true,
			forwardToJS: false,
			blockWebKit: true,
			description: "Navigate to upper pane (callback handler)",
		},
		{
			shortcut:    "alt+arrowdown",
			expectEntry: true,
			forwardToJS: false,
			blockWebKit: true,
			description: "Navigate to lower pane (callback handler)",
		},
		{
			shortcut:    "alt+arrowleft",
			expectEntry: true,
			forwardToJS: false,
			blockWebKit: true,
			description: "Navigate to left pane (callback handler)",
		},
		{
			shortcut:    "alt+arrowright",
			expectEntry: true,
			forwardToJS: false,
			blockWebKit: true,
			description: "Navigate to right pane (callback handler)",
		},
		{
			shortcut:    "cmdorctrl+p",
			expectEntry: true,
			forwardToJS: true,
			blockWebKit: true,
			description: "Enter pane mode (JS handler + block print dialog)",
		},
		{
			shortcut:    "cmdorctrl+l",
			expectEntry: false, // Not in table, should use default
			forwardToJS: true,
			blockWebKit: false,
			description: "Default routing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.shortcut, func(t *testing.T) {
			routing, exists := shortcutRoutingTable[tt.shortcut]

			if tt.expectEntry {
				if !exists {
					t.Errorf("Expected routing entry for %s but not found", tt.shortcut)
					return
				}

				if routing.forwardToJS != tt.forwardToJS {
					t.Errorf("forwardToJS: got %v, want %v", routing.forwardToJS, tt.forwardToJS)
				}

				if routing.blockWebKit != tt.blockWebKit {
					t.Errorf("blockWebKit: got %v, want %v", routing.blockWebKit, tt.blockWebKit)
				}

				if routing.description != tt.description {
					t.Errorf("description: got %q, want %q", routing.description, tt.description)
				}
			} else {
				if exists {
					t.Errorf("Expected no routing entry for %s but found one", tt.shortcut)
				}
			}
		})
	}
}

// TestDefaultRouting verifies the default routing behavior
func TestDefaultRouting(t *testing.T) {
	if defaultRouting.forwardToJS != true {
		t.Errorf("defaultRouting.forwardToJS: got %v, want true", defaultRouting.forwardToJS)
	}

	if defaultRouting.blockWebKit != false {
		t.Errorf("defaultRouting.blockWebKit: got %v, want false", defaultRouting.blockWebKit)
	}
}

// TestWorkspaceNavigationShortcuts verifies workspace navigation shortcuts block WebKit
func TestWorkspaceNavigationShortcuts(t *testing.T) {
	workspaceShortcuts := []string{
		"alt+arrowup",
		"alt+arrowdown",
		"alt+arrowleft",
		"alt+arrowright",
	}

	for _, shortcut := range workspaceShortcuts {
		t.Run(shortcut, func(t *testing.T) {
			routing, exists := shortcutRoutingTable[shortcut]

			if !exists {
				t.Fatalf("Workspace navigation shortcut %s not found in routing table", shortcut)
			}

			// These shortcuts must block WebKit to prevent page scrolling
			if !routing.blockWebKit {
				t.Errorf("Workspace navigation shortcut %s must block WebKit (got blockWebKit=%v)",
					shortcut, routing.blockWebKit)
			}

			// These shortcuts should NOT forward to JS (handled by callback)
			if routing.forwardToJS {
				t.Errorf("Workspace navigation shortcut %s should not forward to JS (got forwardToJS=%v)",
					shortcut, routing.forwardToJS)
			}
		})
	}
}

// TestConvertToGtkFormat tests the key format conversion
func TestConvertToGtkFormat(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ctrl+l", "<Control>l"},
		{"ctrl+plus", "<Control>plus"},
		{"ctrl+shift+c", "<Control><Shift>c"},
		{"alt+arrowleft", "<Alt>Left"},
		{"alt+arrowright", "<Alt>Right"},
		{"alt+arrowup", "<Alt>Up"},
		{"alt+arrowdown", "<Alt>Down"},
		{"cmdorctrl+p", "<Control>p"},
		{"F12", "F12"},
		{"ctrl+0", "<Control>0"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := convertToGtkFormat(tt.input)
			if result != tt.expected {
				t.Errorf("convertToGtkFormat(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestKeyNameConstants verifies key name constants are properly defined
func TestKeyNameConstants(t *testing.T) {
	expectedKeys := map[string]string{
		"keyP":          "p",
		"keyL":          "l",
		"keyF":          "f",
		"keyC":          "c",
		"keyW":          "w",
		"keyR":          "r",
		"keyArrowLeft":  "arrowleft",
		"keyArrowRight": "arrowright",
		"keyArrowUp":    "arrowup",
		"keyArrowDown":  "arrowdown",
		"keyF12":        "F12",
		"keyPlus":       "plus",
		"keyMinus":      "minus",
		"keyZero":       "0",
	}

	// Verify constants exist and have expected values
	keys := map[string]string{
		"keyP":          keyP,
		"keyL":          keyL,
		"keyF":          keyF,
		"keyC":          keyC,
		"keyW":          keyW,
		"keyR":          keyR,
		"keyArrowLeft":  keyArrowLeft,
		"keyArrowRight": keyArrowRight,
		"keyArrowUp":    keyArrowUp,
		"keyArrowDown":  keyArrowDown,
		"keyF12":        keyF12,
		"keyPlus":       keyPlus,
		"keyMinus":      keyMinus,
		"keyZero":       keyZero,
	}

	for name, expected := range expectedKeys {
		if actual, ok := keys[name]; !ok {
			t.Errorf("Constant %s not found", name)
		} else if actual != expected {
			t.Errorf("Constant %s: got %q, want %q", name, actual, expected)
		}
	}
}
