package webkit

import (
	"fmt"

	"github.com/bnema/dumber/internal/logging"
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

	// Connect close-request signal to handle Cmd+Q and window close button
	// Returning true prevents the default close behavior and allows graceful shutdown
	win.ConnectCloseRequest(func() bool {
		logging.Info(fmt.Sprintf("[window] Close request received (Cmd+Q or close button) - initiating graceful shutdown"))
		// Quit the main loop gracefully, which will trigger cleanup
		QuitMainLoop()
		// Return true to prevent GTK from destroying the window immediately
		// The cleanup process will handle window destruction
		return true
	})

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
// Note: In GTK4, Show() is deprecated. Use SetVisible(true) instead.
func (w *Window) Show() {
	if w == nil || w.win == nil {
		return
	}
	w.win.SetVisible(true)
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

// SetDefaultSize sets the default window size
func (w *Window) SetDefaultSize(width, height int) {
	if w == nil || w.win == nil {
		return
	}
	w.win.SetDefaultSize(width, height)
}

// Destroy destroys the window
func (w *Window) Destroy() {
	if w == nil || w.win == nil {
		return
	}
	w.win.Destroy()
}

// InitializeGlobalShortcuts is defined in shortcuts.go and implemented there
