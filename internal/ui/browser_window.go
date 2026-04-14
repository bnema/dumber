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
	if bw := a.browserWindows[id]; bw != nil && bw.mainWindow == a.mainWindow {
		a.mainWindow = nil
	}
	delete(a.browserWindows, id)
}

func (a *App) OpenFreshWindow(ctx context.Context, url string) (*browserWindow, error) {
	dispatch := a.dispatchOnMainThread
	if dispatch == nil {
		dispatch = func(fn func()) { fn() }
	}

	var created *browserWindow
	var openErr error
	dispatch(func() {
		created, openErr = a.createBrowserWindow(ctx, url)
		if openErr != nil {
			return
		}
		a.registerBrowserWindow(created)
	})

	return created, openErr
}
