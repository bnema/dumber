package ui

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/window"
)

// initHistorySidebar creates and mounts the history sidebar into the
// browser window's sidebar container. The sidebar is hidden by default.
func (bw *browserWindow) initHistorySidebar(ctx context.Context, a *App) {
	if bw == nil || a == nil || bw.mainWindow == nil || a.deps == nil || a.deps.HistoryUC == nil {
		return
	}
	log := logging.FromContext(ctx)

	cfg := a.buildHistorySidebarConfig(ctx, bw)

	sidebar := component.NewHistorySidebar(ctx, cfg)
	if sidebar == nil {
		log.Warn().Msg("failed to create history sidebar")
		return
	}

	bw.historySidebar = sidebar
	bw.sidebarVisible = false

	// Mount into the main window's sidebar box
	bw.mainWindow.SetSidebarWidget(sidebar.Widget())

	// Apply sidebar width from config, falling back to the default 320px.
	// The width is clamped to [280, 380] by SetSidebarWidth internally.
	bw.applySidebarWidthConfig(a)

	log.Debug().Msg("history sidebar initialized")
}

// buildHistorySidebarConfig constructs the HistorySidebarConfig for the given
// browser window. Extracted from initHistorySidebar for testability.
func (a *App) buildHistorySidebarConfig(ctx context.Context, bw *browserWindow) component.HistorySidebarConfig {
	var historyUC port.HistorySidebarHistory
	if a.deps != nil {
		historyUC = a.deps.HistoryUC
	}

	return component.HistorySidebarConfig{
		HistoryUC: historyUC,
		OnNavigate: func(navCtx context.Context, url string) error {
			return a.navigateHistorySidebarSelection(navCtx, bw, url)
		},
		OnNavigateKeepOpen: func(navCtx context.Context, url string) error {
			return a.navigateHistorySidebarSelection(navCtx, bw, url)
		},
		OnOpenInNewPane: func(splitCtx context.Context, url string) error {
			if a.wsCoord == nil || !a.hasBrowserWindow(bw) {
				return nil
			}
			a.activateBrowserWindow(bw)
			return a.wsCoord.SplitWithURL(splitCtx, usecase.SplitRight, url)
		},
		OnClose: func() {
			a.hideAndRestoreFocusForBrowserWindow(bw)
		},
	}
}

func (a *App) navigateHistorySidebarSelection(ctx context.Context, bw *browserWindow, url string) error {
	if a == nil || bw == nil || !a.hasBrowserWindow(bw) {
		return nil
	}
	return a.navigateFromBrowserWindow(ctx, bw, url)
}

// toggleHistorySidebar toggles sidebar visibility. An optional width config
// can be provided and is applied when showing the sidebar.
func (bw *browserWindow) toggleHistorySidebar(widthCfg ...window.SidebarWidthConfig) {
	if bw == nil || bw.historySidebar == nil {
		return
	}

	if bw.sidebarVisible {
		bw.hideHistorySidebar()
	} else {
		bw.showHistorySidebar(widthCfg...)
	}
}

// showHistorySidebar makes the sidebar visible and grabs focus for the search
// entry. An optional width config can be provided to override the default width.
func (bw *browserWindow) showHistorySidebar(widthCfg ...window.SidebarWidthConfig) {
	if bw == nil || bw.historySidebar == nil || bw.mainWindow == nil {
		return
	}
	// Apply width config if provided
	if len(widthCfg) > 0 {
		bw.mainWindow.SetSidebarWidth(widthCfg[0])
	}
	bw.historySidebar.Show()
	bw.mainWindow.SetSidebarVisible(true)
	bw.sidebarVisible = true
}

// hideHistorySidebar hides the sidebar. Callers should also restore focus
// to the active content pane after calling this.
func (bw *browserWindow) hideHistorySidebar() {
	if bw == nil || bw.historySidebar == nil || bw.mainWindow == nil {
		return
	}
	bw.historySidebar.Hide()
	bw.mainWindow.SetSidebarVisible(false)
	bw.sidebarVisible = false
}

// historySidebarWidthConfig extracts the config-backed sidebar width and
// applies it via the MainWindow.SetSidebarWidth path. It is called during
// initialization and can be reused if config is reloaded at runtime.
func historySidebarWidthConfig(widthPx int) window.SidebarWidthConfig {
	cfg := window.SidebarDefaultWidth()
	if widthPx > 0 {
		cfg.WidthPx = widthPx
	}
	return cfg
}

// applySidebarWidthConfig applies the config-backed sidebar width to the
// main window's sidebar.
func (bw *browserWindow) applySidebarWidthConfig(a *App) {
	if bw == nil || bw.mainWindow == nil || a == nil || a.deps == nil || a.deps.Config == nil {
		return
	}
	bw.mainWindow.SetSidebarWidth(historySidebarWidthConfig(a.deps.Config.SidebarWidth))
}

// toggleHistorySidebarAction is the keyboard-action handler for toggling the
// history sidebar on the last focused browser window.
func (a *App) toggleHistorySidebarAction(ctx context.Context) error {
	bw := a.lastFocusedBrowserWindow()
	if bw == nil {
		return fmt.Errorf("history sidebar unavailable: no focused browser window")
	}
	if bw.historySidebar == nil {
		return fmt.Errorf("history sidebar unavailable: native sidebar not initialized")
	}
	bw.toggleHistorySidebar()
	return nil
}
