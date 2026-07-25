package ui

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/ui/coordinator/content"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/bnema/puregotk/v4/gtk"
)

type popupStagingShell interface {
	SetContent(*gtk.Widget)
	DetachContent() *gtk.Widget
	Destroy()
}

type popupStagingHost struct {
	shell      popupStagingShell
	attached   *gtk.Widget
	once       sync.Once
	cleanupErr error
}

var newPopupStagingWindow = func(ctx context.Context, app *gtk.Application) (popupStagingShell, error) {
	return window.NewPopup(ctx, app)
}

func (h *popupStagingHost) cleanup() {
	if h == nil {
		return
	}
	h.once.Do(func() {
		if h.shell != nil {
			detached := h.shell.DetachContent()
			if h.attached != nil && detached == nil {
				h.cleanupErr = fmt.Errorf("popup staging content was not attached")
			}
			h.shell.Destroy()
		}
		h.attached = nil
	})
}

func (h *popupStagingHost) Detach() error {
	h.cleanup()
	return h.cleanupErr
}

func (h *popupStagingHost) Destroy() { h.cleanup() }

// stagePopup temporarily parents a WebKit popup widget in a hidden GTK shell.
// The shell is deliberately never shown or presented.
func (a *App) stagePopup(ctx context.Context, input content.StagePopupInput) (content.PopupStagingHost, error) {
	if a == nil || a.gtkApp == nil {
		return nil, fmt.Errorf("gtk application not available for popup staging")
	}
	if input.PopupWebView == nil {
		return nil, fmt.Errorf("popup staging webview is nil")
	}
	if a.contentCoord == nil {
		return nil, fmt.Errorf("content coordinator not available for popup staging")
	}

	shell, err := newPopupStagingWindow(ctx, a.gtkApp)
	if err != nil {
		return nil, err
	}
	widget := a.contentCoord.WrapWidget(ctx, input.PopupWebView)
	if widget == nil || widget.GtkWidget() == nil {
		shell.Destroy()
		return nil, fmt.Errorf("failed to wrap popup staging webview widget")
	}
	gtkWidget := widget.GtkWidget()
	widget.SetHexpand(true)
	widget.SetVexpand(true)
	shell.SetContent(gtkWidget)
	return &popupStagingHost{shell: shell, attached: gtkWidget}, nil
}

var _ content.PopupStagingHost = (*popupStagingHost)(nil)
