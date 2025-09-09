//go:build !webkit_cgo

package webkit

import "fmt"

// RegisterKeyboardShortcut binds a global or window-scoped accelerator to a callback.
func (w *WebView) RegisterKeyboardShortcut(accel string, callback func()) error {
	if w == nil || w.destroyed {
		return ErrNotImplemented
	}
	if accel == "" || callback == nil {
		return fmt.Errorf("invalid shortcut registration")
	}
	// Placeholder: in real impl, bind GTK accelerator; for now, store for tests.
	if shortcuts := getShortcutRegistry(w); shortcuts != nil {
		shortcuts[accel] = callback
	}
	return nil
}

// internal registry per view while no GTK binding exists.
type shortcutRegistry map[string]func()

var viewShortcuts = make(map[*WebView]shortcutRegistry)

func getShortcutRegistry(w *WebView) shortcutRegistry {
	reg, ok := viewShortcuts[w]
	if !ok {
		reg = make(shortcutRegistry)
		viewShortcuts[w] = reg
	}
	return reg
}

// No layout-specific remaps in non-CGO build either.
