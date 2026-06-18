package ui

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
)

func TestApp_EjectActivePaneToWindowRegistersTargetOwnership(t *testing.T) {
	app, sourceWindow, sourceTab := newEjectTestApp(t, stackedTab("source-tab-a", "source-ws-a", "pane-a", "pane-b", "pane-c"))
	// ActivePaneID is the source of truth. Keep ActiveStackIndex stale to cover
	// ejection after stack/UI state drift with more than two panes.
	sourceTab.Workspace.ActivePaneID = "pane-b"
	sourceTab.Workspace.Root.ActiveStackIndex = 0

	if err := app.EjectActivePaneToWindow(context.Background(), ""); err != nil {
		t.Fatalf("EjectActivePaneToWindow returned error: %v", err)
	}

	if got := len(app.browserWindows); got != 2 {
		t.Fatalf("browserWindows length = %d, want 2", got)
	}
	if app.browserWindows[sourceWindow.id] == nil {
		t.Fatal("source window should remain registered")
	}
	targetWindow := app.browserWindows["target-window"]
	if targetWindow == nil {
		t.Fatal("target window was not registered")
	}
	if got := targetWindow.tabs.Count(); got != 1 {
		t.Fatalf("target tab count = %d, want 1", got)
	}
	newTab := targetWindow.tabs.Tabs[0]
	if got := app.windowForTab[newTab.ID]; got != targetWindow {
		t.Fatalf("windowForTab[%s] = %p, want target %p", newTab.ID, got, targetWindow)
	}
	if got := newTab.Workspace.ActivePaneID; got != entity.PaneID("pane-b") {
		t.Fatalf("target active pane = %q, want pane-b", got)
	}
	if got := sourceTab.Workspace.FindPane("pane-b"); got != nil {
		t.Fatal("source workspace still contains moved pane")
	}
	if got := sourceTab.Workspace.ActivePaneID; got != entity.PaneID("pane-c") {
		t.Fatalf("source active pane = %q, want pane-c", got)
	}
	if got := sourceTab.Workspace.Root.ActiveStackIndex; got != 1 {
		t.Fatalf("source active stack index = %d, want 1", got)
	}
	if got := app.lastFocusedWindowID; got != targetWindow.id {
		t.Fatalf("lastFocusedWindowID = %q, want %q", got, targetWindow.id)
	}
}

func TestApp_EjectActivePaneToWindowRetainsSourceWindowOnRemainingActiveTab(t *testing.T) {
	sourceTab := entity.NewTab("source-tab", "source-ws", entity.NewPane("pane-a"))
	otherTab := entity.NewTab("other-tab", "other-ws", entity.NewPane("pane-b"))
	app, sourceWindow, _ := newEjectTestApp(t, sourceTab)
	sourceWindow.tabs.Add(otherTab)
	app.setBrowserWindowForTab(otherTab.ID, sourceWindow)
	sourceWindow.tabs.SetActive(sourceTab.ID)

	if err := app.EjectActivePaneToWindow(context.Background(), ""); err != nil {
		t.Fatalf("EjectActivePaneToWindow returned error: %v", err)
	}

	if app.browserWindows[sourceWindow.id] == nil {
		t.Fatal("source window should remain registered when another tab remains")
	}
	if got := sourceWindow.tabs.ActiveTabID; got != otherTab.ID {
		t.Fatalf("source active tab = %q, want %q", got, otherTab.ID)
	}
	if got := app.windowForTab[sourceTab.ID]; got != nil {
		t.Fatalf("closed source tab ownership remained: %p", got)
	}
	if got := app.windowForTab[otherTab.ID]; got != sourceWindow {
		t.Fatalf("remaining tab owner = %p, want source window %p", got, sourceWindow)
	}
}

func TestApp_EjectActivePaneToWindowRemovesEmptySourceWindow(t *testing.T) {
	sourceTab := entity.NewTab("source-tab", "source-ws", entity.NewPane("pane-a"))
	app, sourceWindow, _ := newEjectTestApp(t, sourceTab)

	if err := app.EjectActivePaneToWindow(context.Background(), ""); err != nil {
		t.Fatalf("EjectActivePaneToWindow returned error: %v", err)
	}

	if app.browserWindows[sourceWindow.id] != nil {
		t.Fatal("empty source window should be removed")
	}
	if got := len(app.browserWindows); got != 1 {
		t.Fatalf("browserWindows length = %d, want replacement target only", got)
	}
	if got := app.windowForTab[sourceTab.ID]; got != nil {
		t.Fatalf("source tab ownership remained after source tab closed: %p", got)
	}
	for _, bw := range app.browserWindows {
		if bw.tabs == nil || bw.tabs.Count() == 0 {
			t.Fatalf("registered zero-tab window remains: %#v", bw)
		}
	}
}

func TestApp_EjectActivePaneToWindowSnapshotRetainsSourceWindowWhenTabsRemain(t *testing.T) {
	app, sourceWindow, sourceTab := newEjectTestApp(t, stackedTab("source-tab", "source-ws", "pane-a", "pane-b", "pane-c"))
	sourceTab.Workspace.ActivePaneID = "pane-b"

	if err := app.EjectActivePaneToWindow(context.Background(), ""); err != nil {
		t.Fatalf("EjectActivePaneToWindow returned error: %v", err)
	}

	windows, activeWindowIndex := app.GetWindowSnapshotState()
	if got := len(windows); got != 2 {
		t.Fatalf("snapshot window count = %d, want 2", got)
	}
	if got := activeWindowIndex; got != 1 {
		t.Fatalf("active window index = %d, want target index 1", got)
	}
	if got := windows[0].WindowID; got != entity.WindowID(sourceWindow.id) {
		t.Fatalf("first snapshot window = %q, want source %q", got, sourceWindow.id)
	}
	if got := windows[0].Tabs.Count(); got != 1 {
		t.Fatalf("source snapshot tab count = %d, want 1", got)
	}
	if windows[0].Tabs.Find(sourceTab.ID) == nil {
		t.Fatalf("source snapshot missing retained tab %q", sourceTab.ID)
	}
	assertTargetSnapshotContainsPane(t, windows[1], "pane-b")
}

func TestApp_EjectActivePaneToWindowHandlesThreePaneSplitTree(t *testing.T) {
	app, sourceWindow, sourceTab := newEjectTestApp(t, splitTreeTab("source-tab", "source-ws", "pane-a", "pane-b", "pane-c"))
	sourceTab.Workspace.ActivePaneID = "pane-c"

	if err := app.EjectActivePaneToWindow(context.Background(), ""); err != nil {
		t.Fatalf("EjectActivePaneToWindow returned error: %v", err)
	}

	if got := len(app.browserWindows); got != 2 {
		t.Fatalf("browserWindows length = %d, want source and target", got)
	}
	if app.browserWindows[sourceWindow.id] == nil {
		t.Fatal("source window should remain registered")
	}
	if got := sourceTab.Workspace.PaneCount(); got != 2 {
		t.Fatalf("source pane count = %d, want 2", got)
	}
	if sourceTab.Workspace.FindPane("pane-a") == nil || sourceTab.Workspace.FindPane("pane-b") == nil {
		t.Fatal("source workspace missing remaining split panes")
	}
	if got := sourceTab.Workspace.FindPane("pane-c"); got != nil {
		t.Fatal("source workspace still contains moved split pane")
	}
	targetWindow := app.browserWindows["target-window"]
	if targetWindow == nil || targetWindow.tabs.Count() != 1 {
		t.Fatalf("target window/tabs not created correctly: %#v", targetWindow)
	}
	newTab := targetWindow.tabs.Tabs[0]
	if got := newTab.Workspace.ActivePaneID; got != entity.PaneID("pane-c") {
		t.Fatalf("target active pane = %q, want pane-c", got)
	}
	if got := app.windowForTab[newTab.ID]; got != targetWindow {
		t.Fatalf("target owner = %p, want target window %p", got, targetWindow)
	}
}

func TestApp_EjectActivePaneToWindowSnapshotReplacesEmptySourceWindow(t *testing.T) {
	sourceTab := entity.NewTab("source-tab", "source-ws", entity.NewPane("pane-a"))
	app, _, _ := newEjectTestApp(t, sourceTab)

	if err := app.EjectActivePaneToWindow(context.Background(), ""); err != nil {
		t.Fatalf("EjectActivePaneToWindow returned error: %v", err)
	}

	windows, activeWindowIndex := app.GetWindowSnapshotState()
	if got := len(windows); got != 1 {
		t.Fatalf("snapshot window count = %d, want replacement target only", got)
	}
	if got := activeWindowIndex; got != 0 {
		t.Fatalf("active window index = %d, want target index 0", got)
	}
	assertTargetSnapshotContainsPane(t, windows[0], "pane-a")
}

func TestApp_EjectActivePaneToWindowTargetUICreationFailureRestoresState(t *testing.T) {
	app, sourceWindow, sourceTab := newEjectTestApp(t, stackedTab("source-tab", "source-ws", "pane-a", "pane-b"))
	app.workspaceViewCreateOverride = nil
	sourceTab.Workspace.ActivePaneID = "pane-b"

	err := app.EjectActivePaneToWindow(context.Background(), "")
	if err == nil {
		t.Fatal("EjectActivePaneToWindow returned nil error, want target UI creation error")
	}

	if got := len(app.browserWindows); got != 1 {
		t.Fatalf("browserWindows length = %d, want restored source only", got)
	}
	if app.browserWindows[sourceWindow.id] != sourceWindow {
		t.Fatal("source window should remain registered after rollback")
	}
	if app.browserWindows["target-window"] != nil {
		t.Fatal("target window should be removed after rollback")
	}
	if got := sourceWindow.tabs.Count(); got != 1 {
		t.Fatalf("source tab count = %d, want restored source tab", got)
	}
	restoredTab := sourceWindow.tabs.Find(sourceTab.ID)
	if restoredTab == nil {
		t.Fatal("source tab missing after rollback")
	}
	if restoredTab.Workspace.FindPane("pane-a") == nil || restoredTab.Workspace.FindPane("pane-b") == nil {
		t.Fatal("source workspace panes were not restored after rollback")
	}
	if got := app.windowForTab[sourceTab.ID]; got != sourceWindow {
		t.Fatalf("source tab owner = %p, want source window %p", got, sourceWindow)
	}
	if got := app.windowForTabCount(); got != 1 {
		t.Fatalf("windowForTab length = %d, want restored source only", got)
	}
}

func TestApp_EjectActivePaneToWindowUsecaseFailureRollsBackTargetWindow(t *testing.T) {
	app, _, sourceTab := newEjectTestApp(t, stackedTab("source-tab", "source-ws", "pane-a", "pane-b"))
	sourceTab.Workspace.ActivePaneID = "missing-pane"

	if err := app.EjectActivePaneToWindow(context.Background(), ""); err != nil {
		t.Fatalf("EjectActivePaneToWindow returned error: %v", err)
	}

	if got := len(app.browserWindows); got != 1 {
		t.Fatalf("browserWindows length = %d, want unchanged source only", got)
	}
	if app.browserWindows["target-window"] != nil {
		t.Fatal("target window should not be registered after failure/no-op")
	}
	if got := app.windowForTabCount(); got != 1 {
		t.Fatalf("windowForTab length = %d, want unchanged source only", got)
	}
}

func TestApp_EjectActivePaneToWindowNoopsWhenRequestedPaneIsNotActiveWorkspacePane(t *testing.T) {
	app, _, sourceTab := newEjectTestApp(t, stackedTab("source-tab", "source-ws", "pane-a", "pane-b"))
	sourceTab.Workspace.ActivePaneID = "pane-a"

	if err := app.EjectActivePaneToWindow(context.Background(), "floating-pane:source-tab:default"); err != nil {
		t.Fatalf("EjectActivePaneToWindow returned error: %v", err)
	}

	if got := len(app.browserWindows); got != 1 {
		t.Fatalf("browserWindows length = %d, want unchanged source only", got)
	}
	if sourceTab.Workspace.FindPane("pane-a") == nil || sourceTab.Workspace.FindPane("pane-b") == nil {
		t.Fatal("source workspace changed after mismatched requested pane no-op")
	}
}

func TestApp_EjectActivePaneToWindowNoopsForInternalSystemPane(t *testing.T) {
	pane := entity.NewPane("pane-a")
	pane.URI = "dumb://history"
	app, _, sourceTab := newEjectTestApp(t, entity.NewTab("source-tab", "source-ws", pane))

	if err := app.EjectActivePaneToWindow(context.Background(), ""); err != nil {
		t.Fatalf("EjectActivePaneToWindow returned error: %v", err)
	}

	if got := len(app.browserWindows); got != 1 {
		t.Fatalf("browserWindows length = %d, want unchanged source only", got)
	}
	if sourceTab.Workspace.FindPane("pane-a") == nil {
		t.Fatal("source system pane was moved")
	}
}

func newEjectTestApp(t *testing.T, sourceTab *entity.Tab) (*App, *browserWindow, *entity.Tab) {
	t.Helper()
	sourceTabs := entity.NewTabList()
	sourceTabs.Add(sourceTab)
	sourceWindow := &browserWindow{id: "source-window", tabs: sourceTabs}
	ids := []string{"new-tab", "new-workspace"}
	idIndex := 0
	app := &App{
		tabs:                entity.NewTabList(),
		browserWindows:      map[string]*browserWindow{sourceWindow.id: sourceWindow},
		browserWindowOrder:  []string{sourceWindow.id},
		lastFocusedWindowID: sourceWindow.id,
		windowForTab:        map[entity.TabID]*browserWindow{sourceTab.ID: sourceWindow},
		workspaceViews:      map[entity.TabID]*component.WorkspaceView{},
		extractPaneToTabListUC: usecase.NewExtractPaneToTabListUseCase(func() string {
			if idIndex >= len(ids) {
				return "extra-id"
			}
			id := ids[idIndex]
			idIndex++
			return id
		}),
		browserWindowFactory: func(context.Context, string) (*browserWindow, error) {
			return &browserWindow{id: "target-window", tabs: entity.NewTabList()}, nil
		},
	}
	app.workspaceViewCreateOverride = func(_ context.Context, tab *entity.Tab) bool {
		if tab == nil {
			return false
		}
		app.workspaceViews[tab.ID] = &component.WorkspaceView{}
		return true
	}
	app.tabs.Add(sourceTab)
	return app, sourceWindow, sourceTab
}

func stackedTab(tabID entity.TabID, workspaceID entity.WorkspaceID, paneIDs ...entity.PaneID) *entity.Tab {
	children := make([]*entity.PaneNode, 0, len(paneIDs))
	stack := &entity.PaneNode{ID: "stack", IsStacked: true}
	for _, paneID := range paneIDs {
		child := &entity.PaneNode{ID: string(paneID) + "-node", Pane: entity.NewPane(paneID), Parent: stack}
		children = append(children, child)
	}
	stack.Children = children
	workspace := &entity.Workspace{ID: workspaceID, Root: stack, ActivePaneID: paneIDs[0]}
	return &entity.Tab{ID: tabID, Workspace: workspace}
}

func splitTreeTab(tabID entity.TabID, workspaceID entity.WorkspaceID, paneAID, paneBID, paneCID entity.PaneID) *entity.Tab {
	paneA := entity.NewPane(paneAID)
	paneB := entity.NewPane(paneBID)
	paneC := entity.NewPane(paneCID)

	leftSplit := &entity.PaneNode{ID: "left-split", SplitDir: entity.SplitVertical, SplitRatio: 0.5}
	leafA := &entity.PaneNode{ID: string(paneAID) + "-node", Pane: paneA, Parent: leftSplit}
	leafB := &entity.PaneNode{ID: string(paneBID) + "-node", Pane: paneB, Parent: leftSplit}
	leftSplit.Children = []*entity.PaneNode{leafA, leafB}

	root := &entity.PaneNode{ID: "root-split", SplitDir: entity.SplitHorizontal, SplitRatio: 0.5}
	leafC := &entity.PaneNode{ID: string(paneCID) + "-node", Pane: paneC, Parent: root}
	leftSplit.Parent = root
	root.Children = []*entity.PaneNode{leftSplit, leafC}

	workspace := &entity.Workspace{ID: workspaceID, Root: root, ActivePaneID: paneAID}
	return &entity.Tab{ID: tabID, Workspace: workspace}
}

func assertTargetSnapshotContainsPane(t *testing.T, window entity.WindowTabListState, paneID entity.PaneID) {
	t.Helper()
	if got := window.WindowID; got != entity.WindowID("target-window") {
		t.Fatalf("target snapshot window ID = %q, want target-window", got)
	}
	if got := window.Tabs.Count(); got != 1 {
		t.Fatalf("target snapshot tab count = %d, want 1", got)
	}
	newTab := window.Tabs.Tabs[0]
	if newTab.Workspace == nil || newTab.Workspace.FindPane(paneID) == nil {
		t.Fatalf("target snapshot missing moved pane %q", paneID)
	}
}

func (a *App) windowForTabCount() int { return len(a.windowForTab) }
