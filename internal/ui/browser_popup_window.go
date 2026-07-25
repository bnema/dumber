package ui

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/coordinator/content"
)

var showPopupBrowserWindow = func(bw *browserWindow) {
	if bw != nil && bw.mainWindow != nil {
		bw.mainWindow.Show()
	}
}

// openPopupBrowserWindow adopts the exact related WebView created for a
// browsing-context request into a complete Dumber browser window.
func (a *App) openPopupBrowserWindow(ctx context.Context, input content.BrowserWindowInput) (content.BrowserWindowResult, error) {
	if input.PopupPane == nil || input.PopupWebView == nil {
		return content.BrowserWindowResult{}, fmt.Errorf("popup browser window requires pane and webview")
	}
	bw, err := a.createEmptyBrowserWindow(ctx)
	if err != nil {
		return content.BrowserWindowResult{}, err
	}
	tab, err := a.tabCoord.CreateWithPane(
		ctx,
		a.ensureTabTargetForBrowserWindow(bw),
		input.PopupPane,
		input.PopupWebView,
		input.TargetURI,
	)
	if err != nil {
		a.cleanupEjectTargetWindow(bw)
		return content.BrowserWindowResult{}, err
	}
	a.setBrowserWindowForTab(tab.ID, bw)
	a.activateBrowserWindow(bw)

	var showOnce sync.Once
	show := func() {
		showOnce.Do(func() { showPopupBrowserWindow(bw) })
	}
	if lifecycle, ok := input.PopupWebView.(port.PopupLifecycleCapable); ok {
		lifecycle.SetOnClose(func() { a.closeAdoptedPopupTab(context.Background(), bw, tab.ID) })
		if !input.Ready {
			lifecycle.SetOnReadyToShow(show)
			return content.BrowserWindowResult{WindowID: bw.id}, nil
		}
	}
	show()
	return content.BrowserWindowResult{WindowID: bw.id}, nil
}

func (a *App) closeAdoptedPopupTab(ctx context.Context, bw *browserWindow, tabID entity.TabID) {
	if a == nil || bw == nil || bw.tabs == nil || bw.tabs.Find(tabID) == nil {
		return
	}
	bw.tabs.SetActive(tabID)
	_ = a.tabCoord.Close(ctx, a.ensureTabTargetForBrowserWindow(bw))
}
