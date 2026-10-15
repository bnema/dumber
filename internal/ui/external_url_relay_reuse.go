package ui

import (
	"context"

	"github.com/bnema/dumber/internal/domain/entity"
	domainurl "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/logging"
)

// openRelayURLOnMainThread opens a relayed external URL, reusing the pane from
// a recent identical request when it still exists. Must run on the main thread.
func (a *App) openRelayURLOnMainThread(ctx context.Context, url string, cfg entity.RuntimeConfigSnapshot) error {
	if a.reuseRelayPane(ctx, url) {
		return nil
	}
	if err := a.openExternalURLOnMainThread(ctx, url, cfg); err != nil {
		return err
	}
	a.recordRelayPane(url)
	return nil
}

// reuseRelayPane navigates and focuses the pane previously opened for url.
// It reports false when there is nothing reusable and a new pane should be created.
func (a *App) reuseRelayPane(ctx context.Context, url string) bool {
	paneID, ok := a.externalRelayPanes.Lookup(url, func(id entity.PaneID) bool {
		return a.browserWindowForPane(id) != nil
	})
	if !ok {
		return false
	}
	log := logging.FromContext(ctx)

	bw := a.browserWindowForPane(paneID)
	var tab *entity.Tab
	if bw != nil && bw.tabs != nil {
		for _, candidate := range bw.tabs.Tabs {
			if candidate != nil && candidate.Workspace != nil && candidate.Workspace.FindPane(paneID) != nil {
				tab = candidate
				break
			}
		}
	}
	if tab == nil || a.navCoord == nil || a.contentCoord == nil {
		a.externalRelayPanes.Forget(url)
		return false
	}

	// Reload the URL first: when the pane cannot be navigated, fall back to
	// creating a fresh pane instead of focusing a dead one.
	if err := a.navCoord.NavigateWebView(ctx, url, paneID, a.contentCoord.GetWebView(paneID)); err != nil {
		log.Warn().Err(err).
			Str("pane_id", string(paneID)).
			Str("url_host", logging.SafeURLHost(url)).
			Msg("ui: external URL relay pane could not be navigated; opening a new pane")
		a.externalRelayPanes.Forget(url)
		return false
	}

	a.focusRelayPane(ctx, bw, tab, paneID)
	log.Debug().
		Str("pane_id", string(paneID)).
		Str("url_host", logging.SafeURLHost(url)).
		Msg("ui: external URL reused relay pane")
	return true
}

func (a *App) focusRelayPane(ctx context.Context, bw *browserWindow, tab *entity.Tab, paneID entity.PaneID) {
	if bw.tabs.ActiveTabID != tab.ID {
		if a.tabCoord != nil {
			if err := a.tabCoord.Switch(ctx, a.tabTargetForBrowserWindow(bw), tab.ID); err != nil {
				logging.FromContext(ctx).Warn().Err(err).
					Str("tab_id", string(tab.ID)).
					Msg("ui: failed to switch to relay pane tab")
			}
		} else {
			bw.tabs.SetActive(tab.ID)
			if bw.mainWindow != nil && bw.mainWindow.TabBar() != nil {
				bw.mainWindow.TabBar().SetActive(tab.ID)
			}
			a.switchWorkspaceView(ctx, tab.ID)
		}
	}

	wsView := a.workspaceViews[tab.ID]
	if a.wsCoord != nil {
		a.wsCoord.FocusPaneByID(ctx, tab.Workspace, wsView, paneID)
	} else {
		tab.Workspace.ActivePaneID = paneID
	}

	// A floating pane over the tab would hide the relay pane and keep the
	// active-pane override pointing at itself.
	a.hideVisibleFloatingSessions(ctx, tab.ID, nil)

	if bw.mainWindow != nil {
		bw.mainWindow.Show()
	}
	a.activateBrowserWindow(bw)
	a.syncFloatingFocus()
	if wsView != nil {
		wsView.FocusPane(paneID)
	}
}

// recordRelayPane remembers the pane that was just created for url. After a
// successful placement the new pane is the active pane of the last-focused
// window's active tab (tabbed, split, stacked, and fresh window alike).
func (a *App) recordRelayPane(url string) {
	ws := a.activeWorkspaceForBrowserWindow(a.lastFocusedBrowserWindow())
	if ws == nil {
		return
	}
	active := ws.ActivePane()
	if active == nil || active.Pane == nil || domainurl.RelayURLKey(active.Pane.URI) != domainurl.RelayURLKey(url) {
		return
	}
	a.externalRelayPanes.Record(url, active.Pane.ID)
}
