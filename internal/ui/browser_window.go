package ui

import (
	"context"

	"github.com/bnema/dumber/internal/ui/window"
)

type browserWindow struct {
	id         string
	initialURL string
	mainWindow *window.MainWindow
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
				a.mainWindow = remaining.mainWindow
				if a.tabCoord != nil {
					a.tabCoord.SetMainWindow(remaining.mainWindow)
				}
				return
			}
		}
		a.mainWindow = nil
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
