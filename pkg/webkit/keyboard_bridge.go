package webkit

import (
	"fmt"
	"log"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Keyboard shortcut modifier string constants
const (
	modifierCmdOrCtrl = "cmdorctrl"
	modifierShift     = "shift"
	modifierAlt       = "alt"
)

// GDK key constants for special keys
const (
	gdkKeyLeft  = 0xff51
	gdkKeyRight = 0xff53
	gdkKeyUp    = 0xff52
	gdkKeyDown  = 0xff54
	gdkKeyF12   = 0xffbe
	gdkKeyPlus  = 0x2b
	gdkKeyEqual = 0x3d
	gdkKeyMinus = 0x2d
)

// Key name constants for shortcut mapping
const (
	keyP          = "p"
	keyL          = "l"
	keyF          = "f"
	keyC          = "c"
	keyW          = "w"
	keyR          = "r"
	keyArrowLeft  = "arrowleft"
	keyArrowRight = "arrowright"
	keyArrowUp    = "arrowup"
	keyArrowDown  = "arrowdown"
	keyF12        = "F12"
	keyPlus       = "plus"
	keyMinus      = "minus"
	keyZero       = "0"
)

// ShortcutRouting defines how a keyboard shortcut should be handled by the bridge
type ShortcutRouting struct {
	forwardToJS bool   // Should dispatch dumber:key event to JavaScript
	blockWebKit bool   // Should block WebKit's default handling (return true from handler)
	description string // Human-readable description for debugging
}

// shortcutRoutingTable maps normalized shortcuts to their routing behavior
// This centralizes all routing decisions in one place, avoiding magic strings and scattered logic
var shortcutRoutingTable = map[string]ShortcutRouting{
	// Workspace pane navigation - handled by GTK window-level shortcuts only
	"alt+arrowleft":  {forwardToJS: false, blockWebKit: true, description: "Navigate to left pane (GTK handler)"},
	"alt+arrowright": {forwardToJS: false, blockWebKit: true, description: "Navigate to right pane (GTK handler)"},
	"alt+arrowup":    {forwardToJS: false, blockWebKit: true, description: "Navigate to upper pane (GTK handler)"},
	"alt+arrowdown":  {forwardToJS: false, blockWebKit: true, description: "Navigate to lower pane (GTK handler)"},

	// Pane mode - forward to JavaScript but block WebKit's print dialog
	"cmdorctrl+p": {forwardToJS: true, blockWebKit: true, description: "Enter pane mode (JS handler + block print dialog)"},
}

// defaultRouting defines the default behavior for shortcuts not in the routing table
var defaultRouting = ShortcutRouting{
	forwardToJS: true,  // Forward all shortcuts to JS by default
	blockWebKit: false, // Allow WebKit to handle (for reload, devtools, zoom, etc.)
	description: "Default: forward to JS, allow WebKit",
}

// AttachKeyboardBridge attaches an EventControllerKey to bridge keyboard events to JavaScript
// This is critical for allowing JavaScript KeyboardService to receive keyboard events
// that would otherwise be consumed by GTK shortcuts
func (w *WebView) AttachKeyboardBridge() {
	if w == nil || w.view == nil {
		return
	}

	// Create event controller for keyboard
	keyController := gtk.NewEventControllerKey()

	// Set to capture phase so we see events before WebView processes them
	keyController.SetPropagationPhase(gtk.PhaseCapture)

	// Connect to key-pressed signal
	keyController.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		// Build normalized shortcut string
		var parts []string

		ctrl := state.Has(gdk.ControlMask)
		alt := state.Has(gdk.AltMask)
		shift := state.Has(gdk.ShiftMask)

		// Build modifier prefix using switch on modifier combination
		switch {
		case ctrl && shift:
			parts = append(parts, modifierCmdOrCtrl, modifierShift)
		case ctrl && !shift:
			parts = append(parts, modifierCmdOrCtrl)
		case alt:
			parts = append(parts, modifierAlt)
		case shift:
			parts = append(parts, modifierShift)
		default:
			// No modifiers - only forward if we match a key below
		}

		// Map keyval to key name
		var keyName string
		switch keyval {
		case 'p', 'P':
			keyName = keyP
		case 'l', 'L':
			keyName = keyL
		case 'f', 'F':
			keyName = keyF
		case 'c', 'C':
			keyName = keyC
		case 'w', 'W':
			keyName = keyW
		case 'r', 'R':
			keyName = keyR
		case gdkKeyLeft:
			keyName = keyArrowLeft
		case gdkKeyRight:
			keyName = keyArrowRight
		case gdkKeyUp:
			keyName = keyArrowUp
		case gdkKeyDown:
			keyName = keyArrowDown
		case gdkKeyF12:
			keyName = keyF12
		case gdkKeyPlus, gdkKeyEqual:
			keyName = keyPlus
		case gdkKeyMinus:
			keyName = keyMinus
		case '0':
			keyName = keyZero
		default:
			// Only forward shortcuts with modifiers
			if !ctrl && !alt && !shift {
				return false
			}
			return false
		}

		if keyName != "" {
			parts = append(parts, keyName)
		}

		shortcut := ""
		if len(parts) > 0 {
			shortcut = parts[0]
			for i := 1; i < len(parts); i++ {
				shortcut += "+" + parts[i]
			}
		}

		if shortcut != "" {
			// Look up routing behavior from table, use default if not found
			routing, exists := shortcutRoutingTable[shortcut]
			if !exists {
				routing = defaultRouting
			}

			// Dispatch to JavaScript if routing says to
			if routing.forwardToJS {
				script := fmt.Sprintf(`document.dispatchEvent(new CustomEvent('dumber:key',{detail:{shortcut:'%s'}}));`, shortcut)
				if err := w.InjectScript(script); err != nil {
					log.Printf("[keyboard-bridge] Failed to dispatch key event: %v", err)
				}
				log.Printf("[keyboard-bridge] Forwarded shortcut to JS: %s", shortcut)
			} else {
				log.Printf("[keyboard-bridge] Shortcut %s: %s (not forwarded to JS)", shortcut, routing.description)
			}

			// Return routing decision to block or allow WebKit
			return routing.blockWebKit
		}

		// No shortcut matched - allow WebKit to handle normally
		return false
	})

	// Attach controller to WebView
	w.view.AddController(keyController)
	log.Printf("[keyboard-bridge] EventControllerKey attached to WebView ID %d", w.id)
}
