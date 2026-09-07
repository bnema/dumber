package ui

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	domainurl "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/shared/syncdispatch"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/focus"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/bnema/puregotk/v4/glib"
)

type nativeSidebarKind string

const (
	nativeSidebarNone      nativeSidebarKind = ""
	nativeSidebarHistory   nativeSidebarKind = "history"
	nativeSidebarFavorites nativeSidebarKind = "favorites"
)

type browserWindow struct {
	id                     string
	initialURL             string
	tabs                   *entity.TabList // per-window tab list (single source of truth for tab state)
	mainWindow             *window.MainWindow
	appToaster             *component.Toaster
	modeToaster            *component.Toaster
	touchpadNavIndicator   *component.TouchpadNavigationIndicator
	borderMgr              *focus.BorderManager
	sessionManager         *component.SessionManager
	tabPicker              *component.TabPicker
	tabPickerWidget        layout.Widget
	tabPickerPaneID        entity.PaneID
	insertAccentUC         *usecase.InsertAccentUseCase
	accentPicker           *component.AccentPicker
	keyboardHandler        *input.KeyboardHandler
	globalShortcutHandler  *input.GlobalShortcutHandler
	permissionDialog       port.PermissionDialogPresenter
	webrtcIndicator        *component.WebRTCPermissionIndicator
	historySidebar         *component.HistorySidebar
	favoritesSidebar       *component.FavoritesSidebar
	historySidebarReloader historySidebarReloader
	sidebarVisible         bool
	activeSidebarKind      nativeSidebarKind

	// vimModePaneID tracks this window's pane-local Vim Mode accent/pulse owner.
	vimModePaneID entity.PaneID
	// vimNavigationHighlightedWebViews holds every WebView with a semantic Vim
	// navigation target that must be cleared when this window leaves Vim Mode.
	vimNavigationHighlightedWebViews   []port.WebView
	vimNavigationHighlightedWebViewIDs map[port.WebViewID]struct{}
}

func (bw *browserWindow) detachInputForDestroy() {
	if bw == nil {
		return
	}
	if bw.keyboardHandler != nil {
		bw.keyboardHandler.DetachForDestroy()
	}
	if bw.globalShortcutHandler != nil {
		bw.globalShortcutHandler.DetachForDestroy()
	}
}

func (bw *browserWindow) teardownForDestroy() {
	if bw == nil {
		return
	}
	bw.detachInputForDestroy()
	bw.clearShellState()
}

func (bw *browserWindow) clearShellState() {
	if bw == nil {
		return
	}
	// Destroy the history sidebar before releasing the reference so its
	// context, debounce timer, callbacks, and in-flight goroutines are
	// cleaned up before the window itself is torn down.
	if bw.historySidebar != nil {
		bw.historySidebar.Destroy()
	}
	if bw.favoritesSidebar != nil {
		bw.favoritesSidebar.Destroy()
	}
	if bw.touchpadNavIndicator != nil {
		bw.touchpadNavIndicator.Destroy()
	}
	bw.appToaster = nil
	bw.modeToaster = nil
	bw.touchpadNavIndicator = nil
	bw.borderMgr = nil
	bw.sessionManager = nil
	bw.tabPicker = nil
	bw.tabPickerWidget = nil
	bw.tabPickerPaneID = ""
	bw.insertAccentUC = nil
	bw.accentPicker = nil
	bw.keyboardHandler = nil
	bw.globalShortcutHandler = nil
	bw.permissionDialog = nil
	bw.webrtcIndicator = nil
	bw.historySidebar = nil
	bw.favoritesSidebar = nil
	bw.historySidebarReloader = nil
	bw.activeSidebarKind = nativeSidebarNone
	bw.vimModePaneID = ""
	bw.vimNavigationHighlightedWebViews = nil
	bw.vimNavigationHighlightedWebViewIDs = nil
}

func (bw *browserWindow) initChrome(ctx context.Context, a *App) {
	if bw == nil || a == nil || a.widgetFactory == nil || bw.mainWindow == nil {
		return
	}

	bw.initToasterOverlay(a)
	bw.initTouchpadNavigationIndicator()
	bw.initBorderOverlay(a)
	bw.initAccentPicker(ctx, a)
	bw.initSessionManager(ctx, a)
	bw.initTabPicker(ctx, a)
	bw.initHistorySidebar(ctx, a)
	bw.initFavoritesSidebar(ctx, a)
}

func (bw *browserWindow) initToasterOverlay(a *App) {
	if bw == nil || a == nil || a.widgetFactory == nil || bw.mainWindow == nil {
		return
	}

	bw.appToaster = component.NewToaster(a.widgetFactory)
	if bw.appToaster != nil {
		if widget := bw.appToaster.Widget(); widget != nil {
			if gtkWidget := widget.GtkWidget(); gtkWidget != nil {
				bw.mainWindow.AddOverlay(gtkWidget)
			}
		}
	}

	bw.modeToaster = component.NewToaster(a.widgetFactory)
	if bw.modeToaster != nil {
		if widget := bw.modeToaster.Widget(); widget != nil {
			if gtkWidget := widget.GtkWidget(); gtkWidget != nil {
				bw.mainWindow.AddOverlay(gtkWidget)
			}
		}
	}
}

func (bw *browserWindow) initTouchpadNavigationIndicator() {
	if bw == nil || bw.mainWindow == nil {
		return
	}
	bw.touchpadNavIndicator = component.NewTouchpadNavigationIndicator()
	if bw.touchpadNavIndicator == nil {
		return
	}
	if widget := bw.touchpadNavIndicator.Widget(); widget != nil {
		bw.mainWindow.AddNonMeasuringOverlay(widget)
	}
}

func (bw *browserWindow) initBorderOverlay(a *App) {
	if bw == nil || a == nil || a.widgetFactory == nil || bw.mainWindow == nil {
		return
	}

	bw.borderMgr = focus.NewBorderManager(a.widgetFactory)
	if bw.borderMgr == nil {
		return
	}
	if widget := bw.borderMgr.Widget(); widget != nil {
		if gtkWidget := widget.GtkWidget(); gtkWidget != nil {
			bw.mainWindow.AddOverlay(gtkWidget)
		}
	}
}

func (bw *browserWindow) initAccentPicker(ctx context.Context, a *App) {
	log := logging.FromContext(ctx)
	if bw == nil || a == nil || a.widgetFactory == nil || bw.mainWindow == nil || a.deps == nil {
		log.Debug().Msg("widget factory, deps, or main window not available, skipping accent picker")
		return
	}

	bw.accentPicker = component.NewAccentPicker(a.widgetFactory)
	if bw.accentPicker == nil {
		log.Warn().Msg("failed to create accent picker")
		return
	}
	if widget := bw.accentPicker.Widget(); widget != nil {
		if gtkWidget := widget.GtkWidget(); gtkWidget != nil {
			bw.mainWindow.AddOverlay(gtkWidget)
		}
	}

	a.accentFocusProvider = a.deps.AccentFocusProvider
	bw.insertAccentUC = usecase.NewInsertAccentUseCase(
		a.accentFocusProvider,
		bw.accentPicker,
		func(fn func()) {
			cb := glib.SourceFunc(func(_ uintptr) bool {
				fn()
				return false
			})
			glib.IdleAdd(&cb, 0)
		},
	)

	log.Debug().Msg("accent picker initialized")
}

func (bw *browserWindow) initSessionManager(ctx context.Context, a *App) {
	log := logging.FromContext(ctx)
	if bw == nil || a == nil || a.deps == nil || bw.mainWindow == nil {
		log.Debug().Msg("deps or main window not available, skipping session manager")
		return
	}
	runtimeCfg := a.runtimeConfigSnapshot().UI

	var listSessionsUC *usecase.ListSessionsUseCase
	var deleteSessionUC *usecase.DeleteSessionUseCase
	if a.deps.SessionRepo != nil && a.deps.SessionStateRepo != nil {
		listSessionsUC = usecase.NewListSessionsUseCase(
			a.deps.SessionRepo,
			a.deps.SessionStateRepo,
		)
		deleteSessionUC = usecase.NewDeleteSessionUseCase(
			a.deps.SessionStateRepo,
			a.deps.SessionRepo,
		)
	}

	bw.sessionManager = component.NewSessionManager(ctx, component.SessionManagerConfig{
		ListSessionsUC:  listSessionsUC,
		DeleteSessionUC: deleteSessionUC,
		CurrentSession:  a.deps.CurrentSessionID,
		UIScale:         runtimeCfg.DefaultUIScale,
		OnClose: func() {
			log.Debug().Msg("session manager closed")
		},
		OnOpen: func(sessionID entity.SessionID) {
			log.Info().Str("session_id", string(sessionID)).Msg("session restoration requested")
			if spawner := a.deps.SessionSpawner; spawner == nil {
				log.Warn().Str("session_id", string(sessionID)).Msg("session spawner not available")
				return
			} else if err := spawner.SpawnWithSession(sessionID); err != nil {
				log.Error().Err(err).Str("session_id", string(sessionID)).Msg("failed to spawn session")
			}
		},
		OnToast: func(ctx context.Context, message string, level component.ToastLevel) {
			if bw.appToaster != nil {
				bw.appToaster.Show(ctx, message, level)
			}
		},
	})

	if bw.sessionManager == nil {
		log.Warn().Msg("failed to create session manager")
		return
	}
	if widget := bw.sessionManager.Widget(); widget != nil {
		bw.mainWindow.AddOverlay(widget)
	}
	log.Debug().Msg("session manager initialized")
}

func (bw *browserWindow) initTabPicker(ctx context.Context, a *App) {
	log := logging.FromContext(ctx)
	if bw == nil || a == nil || a.deps == nil {
		log.Debug().Msg("deps/config not available, skipping tab picker")
		return
	}
	runtimeCfg := a.runtimeConfigSnapshot().UI

	bw.tabPicker = component.NewTabPicker(ctx, component.TabPickerConfig{
		UIScale: runtimeCfg.DefaultUIScale,
		OnClose: func() {
			log.Debug().Msg("tab picker closed")
		},
		OnSelect: func(item component.TabPickerItem) {
			cb := glib.SourceFunc(func(_ uintptr) bool {
				var targetID entity.TabID
				if !item.IsNew {
					targetID = item.TabID
				}
				if err := a.moveActivePaneToTabFromBrowserWindow(ctx, bw, targetID); err != nil {
					log.Warn().Err(err).Msg("move pane to tab failed")
				}
				return false
			})
			glib.IdleAdd(&cb, 0)
		},
	})

	if bw.tabPicker == nil {
		log.Warn().Msg("failed to create tab picker")
		return
	}

	log.Debug().Msg("tab picker initialized")
}

func (bw *browserWindow) ensureTabs() {
	if bw != nil && bw.tabs == nil {
		bw.tabs = entity.NewTabList()
	}
}

func (a *App) hasBrowserWindow(target *browserWindow) bool {
	if a == nil || target == nil || a.browserWindows == nil {
		return false
	}
	bw, ok := a.browserWindows[target.id]
	return ok && bw == target
}

func (a *App) registerBrowserWindow(bw *browserWindow) {
	if bw == nil {
		return
	}
	bw.ensureTabs()
	if a.browserWindows == nil {
		a.browserWindows = make(map[string]*browserWindow)
	}
	a.browserWindows[bw.id] = bw

	// Track registration order, guarding against duplicates.
	if slices.Contains(a.browserWindowOrder, bw.id) {
		return
	}
	a.browserWindowOrder = append(a.browserWindowOrder, bw.id)
	a.residencyNoteWindowsChanged()
}

func (a *App) releaseTabWorkspace(ctx context.Context, tab *entity.Tab) {
	if tab == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	a.releaseFloatingSessionsForTab(ctx, tab.ID)
	if a.contentCoord != nil && tab.Workspace != nil {
		for _, pane := range tab.Workspace.AllPanes() {
			if pane == nil {
				continue
			}
			a.contentCoord.ReleaseWebView(ctx, pane.ID)
		}
	}
	delete(a.workspaceViews, tab.ID)
	delete(a.windowForTab, tab.ID)
	if a.tabs != nil && a.tabs.Find(tab.ID) != nil {
		a.tabs.Remove(tab.ID)
	}
}

func (a *App) removeBrowserWindow(id string) {
	if id == "" || a.browserWindows == nil {
		return
	}
	removed := a.browserWindows[id]
	wasMainWindow := removed != nil && removed.mainWindow == a.mainWindow

	a.removeBrowserWindowOrder(id)
	a.releaseNativePopupsForBrowserWindow(id)
	a.releaseBrowserWindowTabs(context.Background(), id, removed)
	if removed != nil {
		removed.teardownForDestroy()
	}
	if a.contentCoord != nil {
		a.contentCoord.ClearPopupNamedContextsForWindow(id)
	}
	delete(a.browserWindows, id)

	fallback := a.deterministicBrowserWindowFallback()
	a.updateLastFocusedWindowAfterRemoval(id, fallback)
	if wasMainWindow {
		a.clearMainBrowserWindowAfterRemoval(fallback)
	}
	a.residencyAfterWindowRemoved()
}

func (a *App) releaseNativePopupsForBrowserWindow(windowID string) {
	for popupID, popup := range a.nativePopupWindows {
		if popup != nil && popup.parentWindowID == windowID {
			a.releaseNativePopupWindow(popupID, nativePopupReleaseDestroy)
		}
	}
}

func (a *App) removeBrowserWindowOrder(id string) {
	for i, wid := range a.browserWindowOrder {
		if wid == id {
			a.browserWindowOrder = append(a.browserWindowOrder[:i], a.browserWindowOrder[i+1:]...)
			return
		}
	}
}

func (a *App) releaseBrowserWindowTabs(ctx context.Context, windowID string, bw *browserWindow) {
	ownedTabs := a.collectBrowserWindowTabs(windowID, bw)
	for _, tabID := range sortedTabIDs(ownedTabs) {
		a.releaseTabWorkspace(ctx, ownedTabs[tabID])
	}
	a.cleanupStaleTabMappingsForWindow(windowID)
}

func (a *App) collectBrowserWindowTabs(windowID string, bw *browserWindow) map[entity.TabID]*entity.Tab {
	ownedTabs := make(map[entity.TabID]*entity.Tab)
	if bw != nil && bw.tabs != nil {
		for _, tab := range bw.tabs.Tabs {
			if tab != nil {
				ownedTabs[tab.ID] = tab
			}
		}
	}
	for tabID, owner := range a.windowForTab {
		if owner == nil || owner.id != windowID {
			continue
		}
		if tab := tabFromBrowserWindow(owner, tabID); tab != nil {
			ownedTabs[tabID] = tab
			continue
		}
		if a.tabs != nil {
			ownedTabs[tabID] = a.tabs.Find(tabID)
		}
	}
	return ownedTabs
}

func tabFromBrowserWindow(bw *browserWindow, tabID entity.TabID) *entity.Tab {
	if bw == nil || bw.tabs == nil {
		return nil
	}
	return bw.tabs.Find(tabID)
}

func sortedTabIDs(tabs map[entity.TabID]*entity.Tab) []entity.TabID {
	ids := make([]entity.TabID, 0, len(tabs))
	for tabID := range tabs {
		ids = append(ids, tabID)
	}
	slices.Sort(ids)
	return ids
}

func (a *App) cleanupStaleTabMappingsForWindow(windowID string) {
	for tabID, bw := range a.windowForTab {
		if bw == nil || bw.id != windowID {
			continue
		}
		delete(a.workspaceViews, tabID)
		delete(a.windowForTab, tabID)
		if a.tabs != nil && a.tabs.Find(tabID) != nil {
			a.tabs.Remove(tabID)
		}
	}
}

func (a *App) updateLastFocusedWindowAfterRemoval(removedID string, fallback *browserWindow) {
	if a.lastFocusedWindowID != removedID {
		return
	}
	if fallback != nil {
		a.lastFocusedWindowID = fallback.id
		return
	}
	a.lastFocusedWindowID = ""
}

func (a *App) clearMainBrowserWindowAfterRemoval(fallback *browserWindow) {
	a.clearResizeModeBorder()
	if fallback != nil {
		a.activateBrowserWindow(fallback)
		return
	}
	a.lastFocusedWindowID = ""
	a.mainWindow = nil
	a.keyboardHandler = nil
	a.globalShortcutHandler = nil
	if a.tabCoord != nil {
		a.tabCoord.SetCurrentTarget(coordinator.TabTarget{})
	}
}

func (a *App) cleanupTransientBrowserWindowForDestroy(bw *browserWindow) {
	if bw == nil {
		return
	}
	bw.teardownForDestroy()
	if a.contentCoord != nil {
		a.contentCoord.ClearPopupNamedContextsForWindow(bw.id)
	}
	fallback := a.deterministicBrowserWindowFallback()
	if a.lastFocusedWindowID == bw.id {
		if fallback != nil {
			a.lastFocusedWindowID = fallback.id
		} else {
			a.lastFocusedWindowID = ""
		}
	}
	if bw.mainWindow == a.mainWindow {
		a.clearResizeModeBorder()
		if fallback != nil {
			a.activateBrowserWindow(fallback)
			return
		}
		a.lastFocusedWindowID = ""
		a.mainWindow = nil
		a.keyboardHandler = nil
		a.globalShortcutHandler = nil
		if a.tabCoord != nil {
			a.tabCoord.SetCurrentTarget(coordinator.TabTarget{})
		}
	}
}

func (a *App) deterministicBrowserWindowFallback() *browserWindow {
	if a.browserWindows == nil {
		return nil
	}
	for _, id := range a.browserWindowOrder {
		bw := a.browserWindows[id]
		if id == "" || bw == nil || bw.mainWindow == nil {
			continue
		}
		return bw
	}
	ids := make([]string, 0, len(a.browserWindows))
	for id, bw := range a.browserWindows {
		if id == "" || bw == nil || bw.mainWindow == nil {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return nil
	}
	return a.browserWindows[ids[0]]
}

func (a *App) dispatchWindowURLWork(ctx context.Context, label, url string, work func(context.Context) error) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("url_host", logging.SafeURLHost(url)).Str("dispatch_label", label).Msg("ui: window URL dispatch requested")

	dispatch := a.dispatchOnMainThread
	if dispatch == nil {
		dispatch = func(label string, fn func()) syncdispatch.SyncDispatchResult {
			if fn != nil {
				fn()
			}
			return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchInline}
		}
	}

	var workErr error
	var windowCountBefore, windowCountAfter int
	var hasTabCoord, hasTabsUC bool
	result := dispatch(label, func() {
		windowCountBefore = len(a.browserWindows)
		hasTabCoord = a.tabCoord != nil
		hasTabsUC = a.tabsUC != nil
		log.Debug().
			Str("url_host", logging.SafeURLHost(url)).
			Str("dispatch_label", label).
			Int("window_count_before", windowCountBefore).
			Bool("has_tab_coord", hasTabCoord).
			Bool("has_tabs_uc", hasTabsUC).
			Msg("ui: window URL main-thread work started")
		workErr = work(ctx)
		windowCountAfter = len(a.browserWindows)
	})
	if !result.Completed() {
		log.Warn().
			Str("url_host", logging.SafeURLHost(url)).
			Str("dispatch_label", label).
			Dur("elapsed", result.Elapsed).
			Str("dispatch_status", string(result.Status)).
			Msg("ui: window URL skipped after main-thread dispatch did not complete")
		return fmt.Errorf("main thread dispatch did not complete: %s", result.Status)
	}
	if workErr != nil {
		log.Warn().Err(workErr).
			Str("url_host", logging.SafeURLHost(url)).
			Str("dispatch_label", label).
			Dur("elapsed", result.Elapsed).
			Str("dispatch_status", string(result.Status)).
			Int("window_count_after", windowCountAfter).
			Msg("ui: window URL operation failed")
		return workErr
	}
	log.Debug().
		Str("url_host", logging.SafeURLHost(url)).
		Str("dispatch_label", label).
		Dur("elapsed", result.Elapsed).
		Str("dispatch_status", string(result.Status)).
		Int("window_count_after", windowCountAfter).
		Msg("ui: window URL operation completed")
	return nil
}

func (a *App) OpenExternalURL(ctx context.Context, url string) error {
	url = domainurl.Normalize(url)
	return a.dispatchWindowURLWork(ctx, "ui.open_external_url", url, func(ctx context.Context) error {
		return a.openExternalURLOnMainThread(ctx, url, a.runtimeConfigSnapshot())
	})
}

func (a *App) OpenFreshWindow(ctx context.Context, url string) error {
	url = domainurl.Normalize(url)
	return a.dispatchWindowURLWork(ctx, "ui.open_fresh_window", url, func(ctx context.Context) error {
		return a.openFreshWindow(ctx, url)
	})
}

func (a *App) openExternalURLOnMainThread(ctx context.Context, url string, cfg entity.RuntimeConfigSnapshot) error {
	externalLinks := cfg.UI.Workspace.ExternalLinks
	if externalLinks.Behavior == entity.ExternalLinkBehaviorWindowed || externalLinks.Behavior == "" {
		return a.openFreshWindow(ctx, url)
	}

	err := a.openExternalURLInFocusedWindow(ctx, url, externalLinks)
	if err == nil {
		return nil
	}
	logging.FromContext(ctx).Warn().
		Err(err).
		Str("url_host", logging.SafeURLHost(url)).
		Str("behavior", string(externalLinks.Behavior)).
		Msg("ui: external URL placement failed; opening fresh window")
	return a.openFreshWindow(ctx, url)
}

func (a *App) openExternalURLInFocusedWindow(ctx context.Context, url string, cfg entity.ExternalLinksConfig) error {
	target, activeTab, err := a.externalURLTarget()
	if err != nil {
		return err
	}

	switch cfg.Behavior {
	case entity.ExternalLinkBehaviorTabbed:
		if err := a.createExternalURLTab(ctx, target, url); err != nil {
			return err
		}
	case entity.ExternalLinkBehaviorSplit:
		if err := a.placeExternalURLInPane(ctx, activeTab.Workspace, url, func(ctx context.Context) error {
			return a.wsCoord.SplitWithURLWithoutOmnibox(ctx, externalLinkSplitDirection(cfg.Placement), url)
		}); err != nil {
			return err
		}
	case entity.ExternalLinkBehaviorStacked:
		if err := a.placeExternalURLInPane(ctx, activeTab.Workspace, url, func(ctx context.Context) error {
			return a.wsCoord.StackPaneWithURLWithoutOmnibox(ctx, url)
		}); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported external URL behavior %q", cfg.Behavior)
	}

	if target.mainWindow != nil {
		target.mainWindow.Show()
	}
	a.activateBrowserWindow(target)
	return nil
}

func (a *App) externalURLTarget() (*browserWindow, *entity.Tab, error) {
	target := a.lastFocusedBrowserWindow()
	if target == nil || target.id == "" || a.browserWindows[target.id] != target {
		return nil, nil, fmt.Errorf("last-focused browser window is missing or stale")
	}
	activeTab := a.activeTabForBrowserWindow(target)
	if activeTab == nil || activeTab.Workspace == nil || activeTab.Workspace.ActivePane() == nil {
		return nil, nil, fmt.Errorf("last-focused browser window has no active tab and pane")
	}
	return target, activeTab, nil
}

func (a *App) createExternalURLTab(ctx context.Context, target *browserWindow, url string) error {
	if a.tabCoord == nil {
		return fmt.Errorf("tab coordinator not available")
	}
	created, err := a.tabCoord.Create(ctx, a.tabTargetForBrowserWindow(target), url)
	if err != nil {
		a.rollbackExternalTabCreation(ctx, target, created)
		return err
	}
	if created == nil || created.Workspace == nil || created.Workspace.ActivePane() == nil ||
		created.Workspace.ActivePane().Pane == nil || created.Workspace.ActivePane().Pane.URI != url ||
		a.activeTabForBrowserWindow(target) != created || a.workspaceViews[created.ID] == nil {
		a.rollbackExternalTabCreation(ctx, target, created)
		return fmt.Errorf("created tab did not become an active UI target")
	}
	return nil
}

func (a *App) placeExternalURLInPane(ctx context.Context, ws *entity.Workspace, url string, place func(context.Context) error) error {
	if a.wsCoord == nil {
		return fmt.Errorf("workspace coordinator not available")
	}
	before := ws.ActivePaneID
	if err := place(ctx); err != nil {
		a.rollbackExternalPaneCreation(ctx, ws, before)
		return err
	}
	if err := verifyExternalPaneCreation(ws, before, url); err != nil {
		a.rollbackExternalPaneCreation(ctx, ws, before)
		return err
	}
	return nil
}

func (a *App) rollbackExternalPaneCreation(ctx context.Context, ws *entity.Workspace, previous entity.PaneID) {
	if a.panesUC == nil {
		logging.FromContext(ctx).Warn().Msg("ui: cannot roll back external URL pane; panes use case is unavailable")
		return
	}
	if ws == nil || ws.ActivePaneID == "" || ws.ActivePaneID == previous {
		return
	}
	created := ws.ActivePane()
	if created == nil || created.Pane == nil {
		return
	}
	createdID := created.Pane.ID
	if _, err := a.panesUC.Close(ctx, ws, created); err != nil {
		logging.FromContext(ctx).Warn().Err(err).Msg("ui: failed to roll back external URL pane")
		return
	}
	ws.ActivePaneID = previous
	if a.contentCoord != nil {
		a.contentCoord.ReleaseWebView(ctx, createdID)
	}
	for tabID, view := range a.workspaceViews {
		owner := a.browserWindowForTab(tabID)
		if owner == nil || owner.tabs == nil || view == nil {
			continue
		}
		tab := owner.tabs.Find(tabID)
		if tab == nil || tab.Workspace == nil || tab.Workspace.ID != ws.ID {
			continue
		}
		if err := view.Rebuild(ctx); err != nil {
			logging.FromContext(ctx).Warn().Err(err).Msg("ui: failed to rebuild after external URL pane rollback")
		}
		break
	}
}

func (a *App) rollbackExternalTabCreation(ctx context.Context, target *browserWindow, created *entity.Tab) {
	if target == nil || created == nil {
		return
	}
	if tabs := a.tabListForBrowserWindow(target); tabs != nil {
		tabs.Remove(created.ID)
	}
	if target.mainWindow != nil && target.mainWindow.TabBar() != nil {
		target.mainWindow.TabBar().RemoveTab(created.ID)
	}
	a.releaseTabWorkspace(ctx, created)
}

func externalLinkSplitDirection(placement entity.ExternalLinkPlacement) usecase.SplitDirection {
	switch placement {
	case entity.ExternalLinkPlacementLeft:
		return usecase.SplitLeft
	case entity.ExternalLinkPlacementTop:
		return usecase.SplitUp
	case entity.ExternalLinkPlacementBottom:
		return usecase.SplitDown
	default:
		return usecase.SplitRight
	}
}

func verifyExternalPaneCreation(ws *entity.Workspace, previous entity.PaneID, url string) error {
	if ws == nil || ws.ActivePaneID == "" || ws.ActivePaneID == previous {
		return fmt.Errorf("external URL pane was not activated")
	}
	active := ws.ActivePane()
	if active == nil || active.Pane == nil || domainurl.Normalize(active.Pane.URI) != domainurl.Normalize(url) {
		return fmt.Errorf("external URL pane was not created with the requested URL")
	}
	return nil
}
