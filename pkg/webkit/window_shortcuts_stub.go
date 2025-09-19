//go:build !webkit_cgo

package webkit

import "log"

// WindowShortcuts stub implementation for non-CGO builds
type WindowShortcuts struct {
	window *Window
}

// InitializeGlobalShortcuts returns a stub implementation for non-CGO builds
func (w *Window) InitializeGlobalShortcuts() *WindowShortcuts {
	log.Printf("[window-shortcuts] Stub implementation - global shortcuts not available without webkit_cgo")
	return &WindowShortcuts{
		window: w,
	}
}

// RegisterGlobalShortcut stub - always returns nil for non-CGO builds
func (ws *WindowShortcuts) RegisterGlobalShortcut(key string, callback func()) error {
	log.Printf("[window-shortcuts] Stub: would register %s (webkit_cgo required for actual functionality)", key)
	return nil
}

// Cleanup stub implementation
func (ws *WindowShortcuts) Cleanup() {
	log.Printf("[window-shortcuts] Stub cleanup")
}

// Stub implementations for Go wrapper functions
func CreateGlobalShortcutController(window uintptr) uintptr {
	return 0
}

func AddShortcutToController(controller uintptr, key string, handle uintptr) error {
	return nil
}

func registerWindowShortcutCallback(cb func()) uintptr {
	return 0
}

func unregisterWindowShortcutCallback(id uintptr) {
}