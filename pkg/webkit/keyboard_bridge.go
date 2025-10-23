package webkit

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Keyboard shortcut modifier string constants
const (
	modifierCtrl  = "ctrl"
	modifierShift = "shift"
	modifierAlt   = "alt"
)

// GDK key constants for special keys
const (
	gdkKeyLeft   = 0xff51
	gdkKeyRight  = 0xff53
	gdkKeyUp     = 0xff52
	gdkKeyDown   = 0xff54
	gdkKeyEscape = 0xff1b
	gdkKeyF12    = 0xffbe
	gdkKeyPlus   = 0x2b
	gdkKeyEqual  = 0x3d
	gdkKeyMinus  = 0x2d
)

// Key name constants for shortcut mapping
const (
	keyP          = "p"
	keyL          = "l"
	keyR          = "r"
	keyD          = "d"
	keyU          = "u"
	keyF          = "f"
	keyC          = "c"
	keyW          = "w"
	keyX          = "x"
	keyS          = "s"
	keyEscape     = "escape"
	keyArrowLeft  = "arrowleft"
	keyArrowRight = "arrowright"
	keyArrowUp    = "arrowup"
	keyArrowDown  = "arrowdown"
	keyF12        = "F12"
	keyPlus       = "plus"
	keyMinus      = "minus"
	keyZero       = "0"
	altmod        = "alt"
)

// Pane mode action constants
const (
	actionClose      = "close"
	actionSplitLeft  = "split-left"
	actionSplitRight = "split-right"
	actionSplitDown  = "split-down"
	actionSplitUp    = "split-up"
	actionStack      = "stack"
	actionExit       = "exit"
	actionEnter      = "enter"
)

// Navigation direction constants
const (
	directionLeft  = "left"
	directionRight = "right"
	directionUp    = "up"
	directionDown  = "down"
)

// Map shortcuts to actions (unified for all keyboard handling)
var shortcutActions = map[string]string{
	// Pane mode actions (no modifier)
	keyX:          actionClose,
	keyS:          actionStack,
	keyL:          actionSplitLeft,
	keyR:          actionSplitRight,
	keyD:          actionSplitDown,
	keyU:          actionSplitUp,
	keyArrowLeft:  actionSplitLeft,
	keyArrowRight: actionSplitRight,
	keyArrowDown:  actionSplitDown,
	keyArrowUp:    actionSplitUp,

	// Navigation (alt modifier)
	altmod + "+" + keyArrowLeft:  directionLeft,
	altmod + "+" + keyArrowRight: directionRight,
	altmod + "+" + keyArrowDown:  directionDown,
	altmod + "+" + keyArrowUp:    directionUp,
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
		// Check if pane mode is active - block ALL keys except pane mode actions
		w.mu.RLock()
		paneModeChecker := w.isPaneModeActive
		w.mu.RUnlock()

		isPaneModeActive := paneModeChecker != nil && paneModeChecker()

		// Build normalized shortcut string
		var parts []string

		ctrl := state.Has(gdk.ControlMask)
		alt := state.Has(gdk.AltMask)
		shift := state.Has(gdk.ShiftMask)

		// Build modifier prefix using switch on modifier combination
		switch {
		case ctrl && shift:
			parts = append(parts, modifierCtrl, modifierShift)
		case ctrl && !shift:
			parts = append(parts, modifierCtrl)
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
		case 'r', 'R':
			keyName = keyR
		case 'd', 'D':
			keyName = keyD
		case 'u', 'U':
			keyName = keyU
		case 'f', 'F':
			keyName = keyF
		case 'c', 'C':
			keyName = keyC
		case 'w', 'W':
			keyName = keyW
		case 'x', 'X':
			keyName = keyX
		case 's', 'S':
			keyName = keyS
		case gdkKeyLeft:
			keyName = keyArrowLeft
		case gdkKeyRight:
			keyName = keyArrowRight
		case gdkKeyUp:
			keyName = keyArrowUp
		case gdkKeyDown:
			keyName = keyArrowDown
		case gdkKeyEscape:
			keyName = keyEscape
		case gdkKeyF12:
			keyName = keyF12
		case gdkKeyPlus, gdkKeyEqual:
			keyName = keyPlus
		case gdkKeyMinus:
			keyName = keyMinus
		case '0':
			keyName = keyZero
		default:
			// For unknown keys, let WebView handle them
			// This allows web apps to receive Ctrl+C, Ctrl+V, Ctrl+A, etc.
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
			// Special case: Ctrl+P enters pane mode
			if shortcut == modifierCtrl+"+"+keyP {
				w.mu.RLock()
				handler := w.onPaneModeShortcut
				w.mu.RUnlock()
				if handler != nil && handler(actionEnter) {
					log.Printf("[keyboard-bridge] Pane mode: enter")
					return true
				}
				return true // Block even if no handler
			}

			// Special case: Escape exits active mode
			if shortcut == keyEscape {
				w.mu.RLock()
				handler := w.onPaneModeShortcut
				w.mu.RUnlock()
				if handler != nil && handler(actionExit) {
					log.Printf("[keyboard-bridge] Escape: exited mode")
					return true
				}
				return false // Pass through to DOM if no active mode
			}

			// Check if shortcut maps to an action
			if action, exists := shortcutActions[shortcut]; exists {
				// Navigation actions (alt+arrow)
				if action == directionLeft || action == directionRight || action == directionUp || action == directionDown {
					w.mu.RLock()
					handler := w.onWorkspaceNavigation
					w.mu.RUnlock()
					if handler != nil && handler(action) {
						log.Printf("[keyboard-bridge] Navigation: %s", action)
						return true
					}
					return true // Block anyway to prevent scrolling
				}

				// Pane mode actions (x, l, r, d, u, arrows)
				w.mu.RLock()
				handler := w.onPaneModeShortcut
				w.mu.RUnlock()
				if handler != nil && handler(action) {
					log.Printf("[keyboard-bridge] Pane mode action: %s", action)
					return true
				}
				return false // Not in pane mode, pass through
			}

			// Pane mode active: block all other shortcuts
			if isPaneModeActive {
				log.Printf("[keyboard-bridge] Blocking '%s' during pane mode", shortcut)
				return true
			}

			// Forward to JavaScript using proper JSON marshaling to prevent any potential injection
			shortcutJSON, err := json.Marshal(shortcut)
			if err != nil {
				log.Printf("[keyboard-bridge] Failed to marshal shortcut: %v", err)
				return false
			}
			script := fmt.Sprintf(`document.dispatchEvent(new CustomEvent('dumber:key',{detail:{shortcut:%s}}));`, shortcutJSON)
			if err := w.InjectScript(script); err != nil {
				log.Printf("[keyboard-bridge] Failed to dispatch: %v", err)
			}
			return false // Allow WebKit to handle
		}

		// Pane mode active: block ALL non-shortcut keys
		if isPaneModeActive {
			log.Printf("[keyboard-bridge] Blocking key during pane mode: keyval=%d", keyval)
			return true // Block all other keys
		}

		// No shortcut matched and not in pane mode - allow WebKit to handle normally
		return false
	})

	// Attach controller to WebView
	w.view.AddController(keyController)
	log.Printf("[keyboard-bridge] EventControllerKey attached to WebView ID %d", w.id)
}
