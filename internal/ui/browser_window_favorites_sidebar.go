package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
)

// initFavoritesSidebar creates the native favorites sidebar when favorites are configured.
func (bw *browserWindow) initFavoritesSidebar(ctx context.Context, a *App) {
	if bw == nil || a == nil || bw.mainWindow == nil || a.deps == nil || a.deps.FavoritesUC == nil {
		return
	}
	log := logging.FromContext(ctx)

	cfg := a.buildFavoritesSidebarConfig(bw)
	sidebar := component.NewFavoritesSidebar(ctx, cfg)
	if sidebar == nil {
		log.Warn().Msg("failed to create favorites sidebar")
		return
	}

	bw.favoritesSidebar = sidebar
	bw.favoritesSidebar.Hide()
	bw.applySidebarWidthConfig(a)
	log.Debug().Msg("favorites sidebar initialized")
}

//nolint:dupl // Favorites and history sidebar configs intentionally mirror each other for separate component types.
func (a *App) buildFavoritesSidebarConfig(bw *browserWindow) component.FavoritesSidebarConfig {
	var favoritesUC port.FavoritesSidebarFavorites
	if a.deps != nil {
		favoritesUC = a.deps.FavoritesUC
	}
	return component.FavoritesSidebarConfig{
		FavoritesUC: favoritesUC,
		OnNavigate: func(navCtx context.Context, url string) error {
			return a.navigateFavoritesSidebarSelection(navCtx, bw, url)
		},
		OnNavigateKeepOpen: func(navCtx context.Context, url string) error {
			return a.navigateFavoritesSidebarSelection(navCtx, bw, url)
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

func (a *App) navigateFavoritesSidebarSelection(ctx context.Context, bw *browserWindow, url string) error {
	if a == nil || bw == nil || !a.hasBrowserWindow(bw) {
		return nil
	}
	return a.navigateFromBrowserWindow(ctx, bw, url)
}

func (bw *browserWindow) toggleFavoritesSidebar() {
	if bw == nil || bw.favoritesSidebar == nil {
		return
	}
	if bw.sidebarVisible && bw.activeSidebarKind == nativeSidebarFavorites {
		bw.hideFavoritesSidebar()
		return
	}
	bw.showFavoritesSidebar()
}

func (bw *browserWindow) showFavoritesSidebar() {
	if bw == nil || bw.favoritesSidebar == nil || bw.mainWindow == nil {
		return
	}
	bw.mountNativeSidebar(nativeSidebarFavorites)
	bw.favoritesSidebar.Show()
	bw.mainWindow.SetSidebarVisible(true)
	bw.sidebarVisible = true
	bw.activeSidebarKind = nativeSidebarFavorites
}

func (bw *browserWindow) hideFavoritesSidebar() {
	if bw == nil || bw.favoritesSidebar == nil || bw.mainWindow == nil {
		return
	}
	bw.favoritesSidebar.Hide()
	bw.mainWindow.SetSidebarVisible(false)
	bw.sidebarVisible = false
	if bw.activeSidebarKind == nativeSidebarFavorites {
		bw.activeSidebarKind = nativeSidebarNone
	}
}

func (a *App) toggleFavoritesSidebarAction(ctx context.Context) error {
	bw := a.lastFocusedBrowserWindow()
	if bw == nil {
		return fmt.Errorf("favorites sidebar unavailable: no focused browser window")
	}
	if bw.favoritesSidebar == nil {
		return fmt.Errorf("favorites sidebar unavailable: native sidebar not initialized")
	}
	if bw.sidebarVisible && bw.activeSidebarKind == nativeSidebarFavorites {
		a.hideAndRestoreFocusForBrowserWindow(bw)
		return nil
	}
	bw.toggleFavoritesSidebar()
	return nil
}

func (a *App) toggleCurrentPageFavoriteAction(ctx context.Context) error {
	if a == nil || a.deps == nil || a.deps.FavoritesUC == nil {
		return fmt.Errorf("favorites unavailable: usecase not configured")
	}
	_, wv := a.activeWebViewForBrowserWindow(a.lastFocusedBrowserWindow())
	if wv == nil {
		return fmt.Errorf("favorites unavailable: no active webview")
	}
	uri := strings.TrimSpace(wv.URI())
	if uri == "" {
		return fmt.Errorf("favorites unavailable: active page has no URI")
	}
	result, err := a.deps.FavoritesUC.Toggle(ctx, uri, wv.Title())
	if err != nil {
		return err
	}
	if result != nil && strings.TrimSpace(result.Message) != "" {
		a.showToastOnLastFocusedBrowserWindow(ctx, result.Message, component.ToastSuccess,
			component.WithDuration(component.ToastBriefDurationMs),
			component.WithPosition(component.ToastPositionBottomRight),
		)
	}
	a.reloadVisibleFavoritesSidebars("current-page-favorite-toggle")
	return nil
}

func (a *App) reloadVisibleFavoritesSidebars(reason string) {
	if a == nil {
		return
	}
	for _, bw := range a.browserWindows {
		if bw == nil || bw.favoritesSidebar == nil {
			continue
		}
		bw.favoritesSidebar.RequestReloadIfVisible(reason)
	}
}
