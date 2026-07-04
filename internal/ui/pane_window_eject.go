package ui

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
)

func (a *App) createEmptyBrowserWindow(ctx context.Context) (*browserWindow, error) {
	created, err := a.createBrowserWindow(ctx, "")
	if err != nil {
		return nil, err
	}
	a.registerBrowserWindow(created)
	return created, nil
}

func (a *App) cleanupEjectTargetWindow(bw *browserWindow) {
	if bw == nil {
		return
	}
	if bw.tabs != nil {
		for _, tab := range bw.tabs.Tabs {
			if tab == nil {
				continue
			}
			delete(a.windowForTab, tab.ID)
			delete(a.workspaceViews, tab.ID)
			if a.tabs != nil {
				a.tabs.Remove(tab.ID)
			}
		}
	}
	a.removeBrowserWindow(bw.id)
	if bw.mainWindow != nil {
		bw.mainWindow.Destroy()
	}
}

type ejectRollbackSnapshot struct {
	sourceTabs     *entity.TabList
	sourceSnapshot *entity.TabList
	targetTabs     *entity.TabList
	targetSnapshot *entity.TabList
	globalSnapshot *entity.TabList
	windowForTab   map[entity.TabID]*browserWindow
	workspaceViews map[entity.TabID]*component.WorkspaceView
}

func (a *App) captureEjectRollbackSnapshot(sourceTabs, targetTabs *entity.TabList) ejectRollbackSnapshot {
	return ejectRollbackSnapshot{
		sourceTabs:     sourceTabs,
		sourceSnapshot: sourceTabs.Snapshot(),
		targetTabs:     targetTabs,
		targetSnapshot: targetTabs.Snapshot(),
		globalSnapshot: a.tabs.Snapshot(),
		windowForTab:   cloneWindowForTabMap(a.windowForTab),
		workspaceViews: cloneWorkspaceViewsMap(a.workspaceViews),
	}
}

func (a *App) restoreEjectSnapshots(snapshot ejectRollbackSnapshot) {
	if snapshot.sourceTabs != nil {
		snapshot.sourceTabs.ReplaceFrom(snapshot.sourceSnapshot)
	}
	if snapshot.targetTabs != nil {
		snapshot.targetTabs.ReplaceFrom(snapshot.targetSnapshot)
	}
	if a.tabs != nil {
		a.tabs.ReplaceFrom(snapshot.globalSnapshot)
	}
	a.windowForTab = snapshot.windowForTab
	a.workspaceViews = snapshot.workspaceViews
}

func (a *App) rollbackEjectTargetUIFailure(targetWindow *browserWindow, snapshot ejectRollbackSnapshot) error {
	a.restoreEjectSnapshots(snapshot)
	a.cleanupEjectTargetWindow(targetWindow)
	return fmt.Errorf("failed to create target workspace view")
}

func cloneWindowForTabMap(source map[entity.TabID]*browserWindow) map[entity.TabID]*browserWindow {
	cloned := make(map[entity.TabID]*browserWindow, len(source))
	maps.Copy(cloned, source)
	return cloned
}

func cloneWorkspaceViewsMap(source map[entity.TabID]*component.WorkspaceView) map[entity.TabID]*component.WorkspaceView {
	cloned := make(map[entity.TabID]*component.WorkspaceView, len(source))
	maps.Copy(cloned, source)
	return cloned
}

func (a *App) activePaneEjectSource(requestedPaneID entity.PaneID) (*browserWindow, *entity.TabList, *entity.Tab, entity.PaneID) {
	sourceWindow := a.lastFocusedBrowserWindow()
	sourceTabs := a.tabListForBrowserWindow(sourceWindow)
	sourceTab := a.activeTabForBrowserWindow(sourceWindow)
	if sourceWindow == nil || sourceTabs == nil || sourceTab == nil || sourceTab.Workspace == nil {
		return nil, nil, nil, ""
	}
	sourcePaneID := sourceTab.Workspace.ActivePaneID
	if requestedPaneID != "" && requestedPaneID != sourcePaneID {
		return nil, nil, nil, ""
	}
	sourceNode := sourceTab.Workspace.FindPane(sourcePaneID)
	if sourcePaneID == "" || sourceNode == nil || sourceNode.Pane == nil || !isEjectableWorkspacePane(sourceNode.Pane) {
		return nil, nil, nil, ""
	}
	return sourceWindow, sourceTabs, sourceTab, sourcePaneID
}

func isEjectableWorkspacePane(pane *entity.Pane) bool {
	if pane == nil {
		return false
	}
	return pane.WindowType == entity.WindowMain && !strings.HasPrefix(pane.URI, "dumb://")
}

func (a *App) EjectActivePaneToWindow(ctx context.Context, requestedPaneID entity.PaneID) error {
	if a.extractPaneToTabListUC == nil {
		return nil
	}

	sourceWindow, sourceTabs, sourceTab, sourcePaneID := a.activePaneEjectSource(requestedPaneID)
	if sourceWindow == nil {
		return nil
	}

	targetWindow, err := a.createEmptyBrowserWindow(ctx)
	if err != nil {
		return err
	}
	targetTabs := a.ensureTabListForBrowserWindow(targetWindow)
	if targetTabs == nil {
		a.cleanupEjectTargetWindow(targetWindow)
		return fmt.Errorf("target tab list is required")
	}

	rollbackSnapshot := a.captureEjectRollbackSnapshot(sourceTabs, targetTabs)

	out, err := a.extractPaneToTabListUC.Execute(usecase.ExtractPaneToTabListInput{
		SourceTabs:   sourceTabs,
		SourceTabID:  sourceTab.ID,
		SourcePaneID: sourcePaneID,
		TargetTabs:   targetTabs,
	})
	if err != nil {
		a.cleanupEjectTargetWindow(targetWindow)
		return err
	}
	if out == nil || out.NewTab == nil {
		a.cleanupEjectTargetWindow(targetWindow)
		return nil
	}

	a.setBrowserWindowForTab(out.NewTab.ID, targetWindow)
	a.syncDerivedGlobalTabMirror(out.NewTab)
	if a.tabs != nil {
		a.tabs.SetActive(out.NewTab.ID)
	}
	a.ensureTargetTabUI(ctx, out.NewTab, targetWindow)
	if a.workspaceViews[out.NewTab.ID] == nil {
		return a.rollbackEjectTargetUIFailure(targetWindow, rollbackSnapshot)
	}

	if out.SourceTabClosed {
		a.removeSourceTabUI(sourceTab.ID, sourceWindow)
		delete(a.windowForTab, sourceTab.ID)
		a.switchSourceWindowToActiveTab(ctx, sourceWindow, sourceTabs)
	} else {
		a.rebuildAndAttachWorkspace(ctx, sourceTab.ID, sourceTab)
	}

	a.rebuildAndAttachWorkspace(ctx, out.NewTab.ID, out.NewTab)
	if a.contentCoord != nil && out.NewTab.Workspace != nil {
		a.contentCoord.SyncWebViewViewport(ctx, out.NewTab.Workspace.ActivePaneID, "eject-pane-to-window")
	}

	a.updateEjectWindowChrome(ctx, sourceWindow, targetWindow, targetTabs, out.NewTab.ID)

	if out.SourceTabClosed && sourceTabs.Count() == 0 {
		a.removeEmptySourceBrowserWindow(sourceWindow)
	}

	targetWindows := a.orderedBrowserWindowsForSnapshot()
	a.replaceGlobalTabsFromRuntimeWindows(targetWindows, targetWindow)

	a.activateBrowserWindow(targetWindow)
	a.switchWorkspaceView(ctx, out.NewTab.ID)
	if targetWindow.mainWindow != nil {
		targetWindow.mainWindow.Show()
	}
	a.MarkDirty()
	return nil
}

func (a *App) updateEjectWindowChrome(
	ctx context.Context,
	sourceWindow, targetWindow *browserWindow,
	targetTabs *entity.TabList,
	newTabID entity.TabID,
) {
	targetTabs.SetActive(newTabID)
	if tabBar := a.tabBarForBrowserWindow(targetWindow); tabBar != nil {
		tabBar.SetActive(newTabID)
	}
	if a.tabCoord != nil {
		a.tabCoord.UpdateBarVisibility(ctx, a.tabTargetForBrowserWindow(sourceWindow))
		a.tabCoord.UpdateBarVisibility(ctx, a.tabTargetForBrowserWindow(targetWindow))
	}
	a.updateBrowserWindowTabBarVisibility(sourceWindow)
	a.updateBrowserWindowTabBarVisibility(targetWindow)
}

func (a *App) switchSourceWindowToActiveTab(ctx context.Context, bw *browserWindow, tabs *entity.TabList) {
	if bw == nil || tabs == nil || tabs.Count() == 0 {
		return
	}
	activeTab := tabs.ActiveTab()
	if activeTab == nil {
		return
	}
	if tabBar := a.tabBarForBrowserWindow(bw); tabBar != nil {
		tabBar.SetActive(activeTab.ID)
	}
	a.switchWorkspaceView(ctx, activeTab.ID)
}

func (a *App) removeEmptySourceBrowserWindow(bw *browserWindow) {
	if bw == nil || a.tabCountForBrowserWindow(bw) != 0 {
		return
	}
	a.removeBrowserWindow(bw.id)
	if bw.mainWindow != nil {
		bw.mainWindow.Destroy()
	}
}

func (a *App) orderedBrowserWindowsForSnapshot() []*browserWindow {
	windows := make([]*browserWindow, 0, len(a.browserWindows))
	for _, id := range a.windowOrder() {
		if bw := a.browserWindows[id]; bw != nil {
			windows = append(windows, bw)
		}
	}
	return windows
}
