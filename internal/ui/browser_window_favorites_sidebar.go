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

// newFavoritesSidebarComponent constructs the native favorites sidebar. It is
// a package-level seam so headless tests can observe on-demand construction
// without instantiating GTK widgets.
var newFavoritesSidebarComponent = component.NewFavoritesSidebar

// ensureFavoritesSidebar constructs the favorites sidebar on first request.
// Construction is separated from mounting and shared visibility mutation:
// the caller mounts and reveals through mountNativeSidebar/showFavoritesSidebar.
// The component lifetime is detached from the caller's request context via
// context.WithoutCancel so a short-lived window-creation or shortcut context
// cannot cancel the sidebar's in-flight loads. It returns false when the
// window, dependencies, or native construction are unavailable.
func (bw *browserWindow) ensureFavoritesSidebar(ctx context.Context, a *App) bool {
	if bw == nil || a == nil || bw.mainWindow == nil {
		return false
	}
	if bw.favoritesSidebar != nil {
		return true
	}
	if a.deps == nil || a.deps.FavoritesUC == nil {
		return false
	}
	log := logging.FromContext(ctx)

	cfg := a.buildFavoritesSidebarConfig(bw)
	sidebar := newFavoritesSidebarComponent(context.WithoutCancel(ctx), cfg)
	if sidebar == nil {
		log.Warn().Msg("failed to create favorites sidebar")
		return false
	}

	bw.favoritesSidebar = sidebar
	bw.applySidebarWidthConfig(a)
	log.Debug().Msg("favorites sidebar initialized")
	return true
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
	if bw.sidebarVisible && bw.activeSidebarKind == nativeSidebarFavorites {
		a.hideAndRestoreFocusForBrowserWindow(bw)
		return nil
	}
	if !bw.ensureFavoritesSidebar(ctx, a) {
		return fmt.Errorf("favorites sidebar unavailable: native sidebar not initialized")
	}
	bw.toggleFavoritesSidebar()
	return nil
}

func (a *App) toggleCurrentPageFavoriteAction(ctx context.Context) error {
	if a == nil || a.deps == nil || a.deps.FavoritesUC == nil {
		return fmt.Errorf("favorites unavailable: usecase not configured")
	}
	bw := a.lastFocusedBrowserWindow()
	_, wv := a.activeWebViewForBrowserWindow(bw)
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
		a.showToastOnBrowserWindow(ctx, bw, result.Message, component.ToastSuccess,
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
