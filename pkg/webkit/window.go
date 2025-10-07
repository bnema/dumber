package webkit

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Window wraps a GTK Window for the browser
type Window struct {
	win *gtk.Window
}

// NewWindow creates a new GTK window with the given title
func NewWindow(title string) (*Window, error) {
	InitMainThread()

	win := gtk.NewWindow()
	if win == nil {
		return nil, ErrWebViewNotInitialized
	}

	win.SetTitle(title)
	win.SetDefaultSize(1024, 768)

	return &Window{win: win}, nil
}

// SetTitle updates the window title
func (w *Window) SetTitle(title string) {
	if w == nil || w.win == nil {
		return
	}
	w.win.SetTitle(title)
}

// SetChild sets the child widget of the window
func (w *Window) SetChild(child gtk.Widgetter) {
	if w == nil || w.win == nil {
		return
	}
	w.win.SetChild(child)
}

// Show makes the window visible
func (w *Window) Show() {
	if w == nil || w.win == nil {
		return
	}
	w.win.Show()
}

// Present brings the window to the front
func (w *Window) Present() {
	if w == nil || w.win == nil {
		return
	}
	w.win.Present()
}

// Close destroys the window
func (w *Window) Close() {
	if w == nil || w.win == nil {
		return
	}
	w.win.Close()
}

// AsWindow returns the underlying gtk.Window for advanced operations
func (w *Window) AsWindow() *gtk.Window {
	if w == nil {
		return nil
	}
	return w.win
}
