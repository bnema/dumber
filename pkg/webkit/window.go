//go:build !webkit_cgo

package webkit

import "sync"

// Window represents a top-level application window hosting a WebView.
// Stub for future GTK application window management.
type Window struct {
	Title string
	child uintptr
	mu    sync.Mutex
}

// NewWindow constructs a Window. In real implementation, this would create a GTK window.
func NewWindow(title string) (*Window, error) {
	_ = title
	return &Window{Title: title}, ErrNotImplemented
}

// SetTitle updates the window title (no-op in non-CGO build).
func (w *Window) SetTitle(title string) {
	w.Title = title
}

// SetChild is a stubbed window child setter for non-CGO builds.
// Simulates GTK4 behavior where setting child to 0 unparents the current child.
func (w *Window) SetChild(child uintptr) {
	if w == nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// GTK4 behavior: Setting child to 0 removes and potentially invalidates the current child
	if child == 0 && w.child != 0 {
		// GTK4 automatically unparents the current child but keeps the widget alive
		widgetMu.Lock()
		if stub, ok := widgetState[w.child]; ok {
			stub.parent = 0
		}
		widgetMu.Unlock()
	}

	// GTK4 validation: if child is not 0, it must be a valid widget
	if child != 0 {
		widgetMu.Lock()
		childStub, childExists := widgetState[child]
		if !childExists || childStub.isDestroyed {
			widgetMu.Unlock()
			panic("GTK-CRITICAL simulation: gtk_window_set_child: assertion 'GTK_IS_WIDGET (widget)' failed - child is invalid")
		}
		childStub.parent = 1 // Simplified window handle
		widgetMu.Unlock()
	}

	w.child = child
}
