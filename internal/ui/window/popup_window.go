package window

import (
	"context"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/rs/zerolog"
)

const (
	popupWindowDefaultWidth  = 900
	popupWindowDefaultHeight = 800
)

// PopupWindow is a lightweight top-level GTK host for native-required popup flows.
type PopupWindow struct {
	window  *gtk.ApplicationWindow
	content *gtk.Box
	logger  zerolog.Logger
}

func NewPopup(ctx context.Context, app *gtk.Application) (*PopupWindow, error) {
	log := logging.FromContext(ctx)
	pw := &PopupWindow{logger: log.With().Str("component", "popup-window").Logger()}

	pw.window = gtk.NewApplicationWindow(app)
	if pw.window == nil {
		return nil, ErrWindowCreationFailed
	}
	pw.window.SetDefaultSize(popupWindowDefaultWidth, popupWindowDefaultHeight)
	title := windowTitle
	pw.window.SetTitle(&title)

	pw.content = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if pw.content == nil {
		pw.window.Unref()
		return nil, ErrWidgetCreationFailed("popupWindow.content")
	}
	pw.content.SetHexpand(true)
	pw.content.SetVexpand(true)
	pw.content.SetVisible(true)
	pw.window.SetChild(&pw.content.Widget)
	return pw, nil
}

func (pw *PopupWindow) SetContent(widget *gtk.Widget) {
	if pw == nil || pw.content == nil {
		return
	}
	if widget == nil {
		return
	}
	widget.SetVisible(true)
	pw.content.Append(widget)
}

func (pw *PopupWindow) Show() {
	if pw == nil || pw.window == nil {
		return
	}
	pw.window.Present()
}

func (pw *PopupWindow) Close() {
	if pw == nil || pw.window == nil {
		return
	}
	pw.window.Close()
}

func (pw *PopupWindow) Destroy() {
	if pw == nil || pw.window == nil {
		return
	}
	pw.window.Destroy()
	pw.window = nil
	pw.content = nil
}

func (pw *PopupWindow) Window() *gtk.ApplicationWindow {
	if pw == nil {
		return nil
	}
	return pw.window
}

func (pw *PopupWindow) SetTitle(title string) {
	if pw == nil || pw.window == nil {
		return
	}
	pw.window.SetTitle(&title)
}
