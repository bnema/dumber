package coordinator

import (
	"context"
	"strconv"
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTabCoordinator_CloseTargetEmptyDoesNotQuitGlobally verifies that
// closing the only tab in one window fires onCurrentWindowEmpty and does NOT
// call onQuit when another browser window still has tabs.
func TestTabCoordinator_CloseTargetEmptyDoesNotQuitGlobally(t *testing.T) {
	ctx := context.Background()

	// Two independent TabLists, each with one tab.
	firstTabs := entity.NewTabList()
	firstTabs.Add(entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first"))))

	secondTabs := entity.NewTabList()
	secondTabs.Add(entity.NewTab(entity.TabID("second-tab"), entity.WorkspaceID("ws-second"), entity.NewPane(entity.PaneID("pane-second"))))

	gen := counterIDGen()
	tabsUC := usecase.NewManageTabsUseCase(gen, nil)

	coord := NewTabCoordinator(ctx, TabCoordinatorConfig{
		TabsUC: tabsUC,
	})

	firstMainWindow := &window.MainWindow{}
	secondMainWindow := &window.MainWindow{}
	firstTarget := TabTarget{Tabs: firstTabs, MainWindow: firstMainWindow}
	secondTarget := TabTarget{Tabs: secondTabs, MainWindow: secondMainWindow}

	var (
		quitCalled        bool
		windowEmptyCalled bool
		windowEmptyTarget *window.MainWindow
	)

	coord.SetOnQuit(func() {
		quitCalled = true
	})
	coord.SetOnCurrentWindowEmpty(func(_ context.Context, target TabTarget) {
		windowEmptyCalled = true
		windowEmptyTarget = target.MainWindow
	})

	// Close the only tab in the first window.
	err := coord.Close(ctx, firstTarget)
	require.NoError(t, err, "Close on first target returned error")

	// onQuit must NOT be called (second window still has tabs).
	assert.False(t, quitCalled, "onQuit was called, but second window still has tabs")

	// onCurrentWindowEmpty must be called for the first window.
	assert.True(t, windowEmptyCalled, "onCurrentWindowEmpty was not called for the emptied window")
	assert.Same(t, firstMainWindow, windowEmptyTarget)

	// firstTabs must be empty.
	assert.Equal(t, 0, firstTabs.Count())

	// secondTabs must be unchanged (still has its tab).
	assert.Equal(t, 1, secondTabs.Count())
	assert.Equal(t, entity.TabID("second-tab"), secondTabs.ActiveTabID)

	// Closing the last tab globally (second window's only tab) should also fire window-empty.
	windowEmptyCalled = false
	windowEmptyTarget = nil
	err = coord.Close(ctx, secondTarget)
	require.NoError(t, err, "Close on second target returned error")

	// onQuit must still NOT be called (App decides when to quit).
	assert.False(t, quitCalled, "onQuit was called on last window; onQuit is no longer used by Close")

	// onCurrentWindowEmpty must be called for the second window.
	assert.True(t, windowEmptyCalled, "onCurrentWindowEmpty was not called for the last window")
	assert.Same(t, secondMainWindow, windowEmptyTarget)
}

// counterIDGen returns a function that generates sequential IDs.
func counterIDGen() func() string {
	var counter int
	return func() string {
		counter++
		return strconv.Itoa(counter)
	}
}

func TestTabCoordinator_SetMainWindowNilClearsCurrentTargetWindowPreservingTabs(t *testing.T) {
	tabs := entity.NewTabList()
	mainWindow := &window.MainWindow{}
	coord := NewTabCoordinator(context.Background(), TabCoordinatorConfig{
		Tabs:       tabs,
		MainWindow: mainWindow,
	})

	coord.SetMainWindow(nil)

	got := coord.CurrentTarget()
	if got.MainWindow != nil {
		t.Fatalf("current target main window = %p, want nil", got.MainWindow)
	}
	if got.Tabs != tabs {
		t.Fatalf("current target tabs = %p, want %p", got.Tabs, tabs)
	}
}

func TestTabCoordinator_SwitchByIndexUsesTargetTabListOnly(t *testing.T) {
	ctx := context.Background()

	// Create two independent TabLists, each with two tabs.
	firstTabs := entity.NewTabList()
	firstTabs.Add(entity.NewTab(entity.TabID("first-tab-1"), entity.WorkspaceID("ws-first-1"), entity.NewPane(entity.PaneID("pane-first-1"))))
	firstTabs.Add(entity.NewTab(entity.TabID("first-tab-2"), entity.WorkspaceID("ws-first-2"), entity.NewPane(entity.PaneID("pane-first-2"))))

	secondTabs := entity.NewTabList()
	secondTabs.Add(entity.NewTab(entity.TabID("second-tab-1"), entity.WorkspaceID("ws-second-1"), entity.NewPane(entity.PaneID("pane-second-1"))))
	secondTabs.Add(entity.NewTab(entity.TabID("second-tab-2"), entity.WorkspaceID("ws-second-2"), entity.NewPane(entity.PaneID("pane-second-2"))))

	gen := counterIDGen()
	tabsUC := usecase.NewManageTabsUseCase(gen, nil)

	// Create TabCoordinator without global tabs (target-driven).
	coord := NewTabCoordinator(ctx, TabCoordinatorConfig{
		TabsUC: tabsUC,
		// No Tabs, no MainWindow — target is provided per-call.
	})

	firstTarget := TabTarget{Tabs: firstTabs}
	secondTarget := TabTarget{Tabs: secondTabs}

	// Confirm initial active tab IDs are as expected (first tab in each list).
	if got := firstTabs.ActiveTabID; got != entity.TabID("first-tab-1") {
		t.Fatalf("firstTabs.ActiveTabID = %q, want %q", got, "first-tab-1")
	}
	if got := secondTabs.ActiveTabID; got != entity.TabID("second-tab-1") {
		t.Fatalf("secondTabs.ActiveTabID = %q, want %q", got, "second-tab-1")
	}

	// Switch second window to its second tab (index 1).
	err := coord.SwitchByIndex(ctx, secondTarget, 1)
	if err != nil {
		t.Fatalf("SwitchByIndex on second target failed: %v", err)
	}

	_ = firstTarget // first target is deliberately not passed to SwitchByIndex below

	// Assert firstTabs.ActiveTabID is unchanged.
	if got := firstTabs.ActiveTabID; got != entity.TabID("first-tab-1") {
		t.Fatalf("firstTabs.ActiveTabID = %q, want %q (should be unchanged)", got, "first-tab-1")
	}

	if got := secondTabs.PreviousActiveTabID; got != entity.TabID("second-tab-1") {
		t.Fatalf("secondTabs.PreviousActiveTabID = %q, want %q", got, "second-tab-1")
	}

	// Assert secondTabs.ActiveTabID is now the second window's tab 2.
	if got := secondTabs.ActiveTabID; got != entity.TabID("second-tab-2") {
		t.Fatalf("secondTabs.ActiveTabID = %q, want %q", got, "second-tab-2")
	}
}

func TestTabCoordinator_CreateAndSwitchCallbacksCarryTarget(t *testing.T) {
	ctx := context.Background()

	firstTabs := entity.NewTabList()
	firstTabs.Add(entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first"))))
	secondTabs := entity.NewTabList()
	secondTabs.Add(entity.NewTab(entity.TabID("second-tab-1"), entity.WorkspaceID("ws-second-1"), entity.NewPane(entity.PaneID("pane-second-1"))))
	secondTabs.Add(entity.NewTab(entity.TabID("second-tab-2"), entity.WorkspaceID("ws-second-2"), entity.NewPane(entity.PaneID("pane-second-2"))))

	coord := NewTabCoordinator(ctx, TabCoordinatorConfig{TabsUC: usecase.NewManageTabsUseCase(counterIDGen(), nil)})
	coord.SetCurrentTarget(TabTarget{Tabs: firstTabs})

	secondTarget := TabTarget{Tabs: secondTabs}
	var createdTarget TabTarget
	var switchedTargets []TabTarget
	coord.SetOnTabCreated(func(_ context.Context, target TabTarget, tab *entity.Tab) {
		createdTarget = target
	})
	coord.SetOnTabSwitched(func(_ context.Context, target TabTarget, tab *entity.Tab) {
		switchedTargets = append(switchedTargets, target)
	})

	if _, err := coord.Create(ctx, secondTarget, "https://example.com"); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if createdTarget.Tabs != secondTabs {
		t.Fatalf("created callback target tabs = %p, want %p", createdTarget.Tabs, secondTabs)
	}
	if len(switchedTargets) != 1 || switchedTargets[0].Tabs != secondTabs {
		t.Fatalf("create switch callback targets = %#v, want second target", switchedTargets)
	}

	switchedTargets = nil
	if err := coord.Switch(ctx, secondTarget, entity.TabID("second-tab-1")); err != nil {
		t.Fatalf("Switch returned error: %v", err)
	}
	if len(switchedTargets) != 1 || switchedTargets[0].Tabs != secondTabs {
		t.Fatalf("switch callback targets = %#v, want second target", switchedTargets)
	}
	if firstTabs.ActiveTabID != entity.TabID("first-tab") {
		t.Fatalf("first target active tab changed to %q", firstTabs.ActiveTabID)
	}
}

// TestTabCoordinator_SwitchByIndexCreatingCreatesMissingTabs verifies that
// SwitchByIndexCreating creates new tabs when the requested index exceeds
// the current tab count, and only modifies the specified target.
func TestTabCoordinator_SwitchByIndexCreatingCreatesMissingTabs(t *testing.T) {
	ctx := context.Background()

	firstTabs := entity.NewTabList()
	firstTabs.Add(entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first"))))

	secondTabs := entity.NewTabList()
	secondTabs.Add(entity.NewTab(entity.TabID("second-tab-1"), entity.WorkspaceID("ws-second-1"), entity.NewPane(entity.PaneID("pane-second-1"))))

	coord := NewTabCoordinator(ctx, TabCoordinatorConfig{TabsUC: usecase.NewManageTabsUseCase(counterIDGen(), nil)})

	secondTarget := TabTarget{Tabs: secondTabs}

	// Call SwitchByIndexCreating with index 3 (0-based), which is beyond the
	// current 1 tab. This should create 3 new tabs (indices 1, 2, 3) and
	// activate index 3.
	err := coord.SwitchByIndexCreating(ctx, secondTarget, 3, "about:blank")
	require.NoError(t, err, "SwitchByIndexCreating should not return error")

	// secondTabs should have 4 tabs now (1 existing + 3 created).
	require.Equal(t, 4, secondTabs.Count(), "second target should have 4 tabs after creating up to index 3")
	require.Greater(t, len(secondTabs.Tabs), 3, "secondTabs.Tabs must have at least 4 elements")
	assert.Equal(t, secondTabs.Tabs[3].ID, secondTabs.ActiveTabID, "active tab should be at index 3")

	// Verify created tabs/panes have URI "about:blank".
	for i := 1; i <= 3; i++ {
		require.Less(t, i, len(secondTabs.Tabs), "tab index %d out of range", i)
		require.NotNil(t, secondTabs.Tabs[i].Workspace, "tab %d workspace should not be nil", i)
		pane := secondTabs.Tabs[i].Workspace.ActivePane()
		require.NotNil(t, pane, "tab %d ActivePane should not be nil", i)
		require.NotNil(t, pane.Pane, "tab %d pane.Pane should not be nil", i)
		assert.Equal(t, "about:blank", pane.Pane.URI, "tab %d pane URI should be about:blank", i)
	}

	// firstTabs must be completely unchanged.
	assert.Equal(t, 1, firstTabs.Count(), "first target should still have 1 tab")
	assert.Equal(t, entity.TabID("first-tab"), firstTabs.ActiveTabID, "first target active tab should be unchanged")
}

// TestTabCoordinator_SwitchByIndexCreatingMissingURLError verifies that
// SwitchByIndexCreating returns an error when the initialURL is empty.
func TestTabCoordinator_SwitchByIndexCreatingMissingURLError(t *testing.T) {
	ctx := context.Background()

	targetTabs := entity.NewTabList()
	targetTabs.Add(entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1"))))

	coord := NewTabCoordinator(ctx, TabCoordinatorConfig{TabsUC: usecase.NewManageTabsUseCase(counterIDGen(), nil)})

	target := TabTarget{Tabs: targetTabs}

	// Call with empty URL — should return an error.
	err := coord.SwitchByIndexCreating(ctx, target, 1, "")
	require.Error(t, err, "SwitchByIndexCreating with empty URL should return error")

	// Tab count should remain 1.
	assert.Equal(t, 1, targetTabs.Count(), "tab count should remain 1 after error")
}
