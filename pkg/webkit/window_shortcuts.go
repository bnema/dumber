//go:build webkit_cgo

package webkit

import (
	"log"
	"strings"
	"unsafe"
)

// No C imports needed - using Go wrapper functions from shortcuts_cgo.go

// WindowShortcuts manages global shortcuts at the window level
type WindowShortcuts struct {
	window     *Window
	controller uintptr // *C.GtkShortcutController
	callbacks  map[string]uintptr
}

// InitializeGlobalShortcuts sets up the GTK4 global shortcut controller
func (w *Window) InitializeGlobalShortcuts() *WindowShortcuts {
	if w.win == nil {
		log.Printf("[window-shortcuts] Cannot initialize - invalid window")
		return nil
	}

	controller := CreateGlobalShortcutController(uintptr(unsafe.Pointer(w.win)))
	if controller == 0 {
		log.Printf("[window-shortcuts] Failed to create shortcut controller")
		return nil
	}

	ws := &WindowShortcuts{
		window:     w,
		controller: controller,
		callbacks:  make(map[string]uintptr),
	}

	log.Printf("[window-shortcuts] Global shortcut controller initialized")
	return ws
}

// RegisterGlobalShortcut registers a shortcut at the window level with global scope
func (ws *WindowShortcuts) RegisterGlobalShortcut(key string, callback func()) error {
	if ws == nil || ws.controller == 0 {
		return ErrNotImplemented
	}

	// Convert key format (e.g., "ctrl+l" -> "<Control>l")
	gtkKey := convertToGtkFormat(key)
	if gtkKey == "" {
		log.Printf("[window-shortcuts] Invalid key format: %s", key)
		return ErrNotImplemented
	}

	// Register callback and get handle
	handle := registerWindowShortcutCallback(callback)
	if handle == 0 {
		log.Printf("[window-shortcuts] Failed to register callback for %s", key)
		return ErrNotImplemented
	}

	// Store handle for cleanup
	ws.callbacks[key] = handle

	// Register with GTK using Go wrapper function
	if err := AddShortcutToController(ws.controller, gtkKey, handle); err != nil {
		log.Printf("[window-shortcuts] Failed to add shortcut to controller: %v", err)
		unregisterWindowShortcutCallback(handle)
		delete(ws.callbacks, key)
		return err
	}

	// Only add Alt+Arrow and Ctrl+W shortcuts to global blocking registry
	// All other window shortcuts should bubble up to GTK for proper handling
	if strings.HasPrefix(key, "alt+Arrow") {
		RegisterGlobalShortcutWithHandle(key, handle)
	} else if key == "ctrl+w" {
		// Ctrl+W should also have callback handle for consistent pane closing
		RegisterGlobalShortcutWithHandle(key, handle)
	}
	// Other shortcuts (Ctrl+L, Ctrl+F, zoom) are NOT registered for blocking

	log.Printf("[window-shortcuts] Registered global shortcut: %s -> %s", key, gtkKey)
	return nil
}

// Cleanup unregisters all shortcuts and cleans up resources
func (ws *WindowShortcuts) Cleanup() {
	if ws == nil {
		return
	}

	// Unregister all callbacks
	for key, handle := range ws.callbacks {
		unregisterWindowShortcutCallback(handle)
		log.Printf("[window-shortcuts] Unregistered shortcut: %s", key)
	}

	ws.callbacks = nil
	ws.controller = 0
}

// convertToGtkFormat converts common key formats to GTK shortcut format
func convertToGtkFormat(key string) string {
	// Convert from electron/webkit format to GTK format
	key = strings.ToLower(key)

	// Handle cmdorctrl -> Control
	key = strings.ReplaceAll(key, "cmdorctrl+", "ctrl+")
	key = strings.ReplaceAll(key, "cmd+", "ctrl+")

	// Build GTK format
	var parts []string
	components := strings.Split(key, "+")

	for i, part := range components {
		if i == len(components)-1 {
			// Last part is the key
			switch part {
			case "f12":
				parts = append(parts, "F12")
			case "f5":
				parts = append(parts, "F5")
			case "l":
				parts = append(parts, "l")
			case "f":
				parts = append(parts, "f")
			case "r":
				parts = append(parts, "r")
			case "c":
				parts = append(parts, "c")
			case "left", "arrowleft":
				parts = append(parts, "Left")
			case "right", "arrowright":
				parts = append(parts, "Right")
			case "up", "arrowup":
				parts = append(parts, "Up")
			case "down", "arrowdown":
				parts = append(parts, "Down")
			default:
				parts = append(parts, part)
			}
		} else {
			// Modifier
			switch part {
			case "ctrl":
				parts = append(parts, "<Control>")
			case "shift":
				parts = append(parts, "<Shift>")
			case "alt":
				parts = append(parts, "<Alt>")
			case "super":
				parts = append(parts, "<Super>")
			}
		}
	}

	return strings.Join(parts, "")
}
