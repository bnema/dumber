//go:build !webkit_cgo

package webkit

// Window represents a top-level application window hosting a WebView.
// Stub for future GTK application window management.
type Window struct {
	Title string
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
func (w *Window) SetChild(child uintptr) {
	_ = child
}
