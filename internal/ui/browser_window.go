package ui

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/window"
)

type browserWindow struct {
	id                    string
	initialURL            string
	activeTabID           entity.TabID
	mainWindow            *window.MainWindow
	keyboardHandler       *input.KeyboardHandler
	globalShortcutHandler *input.GlobalShortcutHandler
	permissionDialog      port.PermissionDialogPresenter
	webrtcIndicator       *component.WebRTCPermissionIndicator
}

func (a *App) registerBrowserWindow(bw *browserWindow) {
	if bw == nil {
		return
	}
	if a.browserWindows == nil {
		a.browserWindows = make(map[string]*browserWindow)
	}
	a.browserWindows[bw.id] = bw
}

func (a *App) removeBrowserWindow(id string) {
	if id == "" || a.browserWindows == nil {
		return
	}
	for tabID, bw := range a.windowForTab {
		if bw != nil && bw.id == id {
			delete(a.windowForTab, tabID)
		}
	}
	if bw := a.browserWindows[id]; bw != nil && bw.mainWindow == a.mainWindow {
		delete(a.browserWindows, id)
		for _, remaining := range a.browserWindows {
			if remaining != nil && remaining.mainWindow != nil {
				a.activateBrowserWindow(remaining)
				return
			}
		}
		a.mainWindow = nil
		a.keyboardHandler = nil
		a.globalShortcutHandler = nil
		if a.tabCoord != nil {
			a.tabCoord.SetMainWindow(nil)
		}
		return
	}
	delete(a.browserWindows, id)
}

func (a *App) OpenFreshWindow(ctx context.Context, url string) error {
	dispatch := a.dispatchOnMainThread
	if dispatch == nil {
		dispatch = func(fn func()) { fn() }
	}

	var openErr error
	dispatch(func() {
		openErr = a.openFreshWindow(ctx, url)
	})

	return openErr
}
