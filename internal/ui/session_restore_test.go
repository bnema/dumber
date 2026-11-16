package ui

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/stretchr/testify/require"
)

func TestSessionManagerImportsNewWindowsWithoutSpawning(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint("creationFailure=", fail), func(t *testing.T) {
			ctx := context.Background()
			sessions := repomocks.NewMockSessionRepository(t)
			ended := time.Now()
			sessions.EXPECT().FindByID(ctx, entity.SessionID("saved")).Return(&entity.Session{ID: "saved", EndedAt: &ended}, nil)
			state := entity.SnapshotFromWindowTabLists("saved", []entity.WindowTabListState{{Tabs: entity.NewTabList()}, {Tabs: entity.NewTabList()}}, 1, ended)
			oldTabs := entity.NewTabList()
			oldTab := entity.NewTab("old-tab", "old-workspace", entity.NewPane("old-pane"))
			oldTabs.Add(oldTab)
			old := &browserWindow{id: "old", tabs: oldTabs}
			snapshot := portmocks.NewMockSnapshotService(t)
			if !fail {
				snapshot.EXPECT().MarkDirty().Once()
			}
			app := &App{
				deps:          &Dependencies{CurrentSessionID: "live", SessionRepo: sessions, SessionStateRepo: mockSessionStateRepoWithSnapshot(t, "saved", state)},
				runtimeConfig: runtimeConfigStateForTest(&config.Config{}),
				tabs:          entity.NewTabList(), browserWindows: map[string]*browserWindow{"old": old}, browserWindowOrder: []string{"old"},
				lastFocusedWindowID: "old", windowForTab: map[entity.TabID]*browserWindow{oldTab.ID: old}, snapshotService: snapshot,
			}
			app.tabs.Add(oldTab)
			count := 0
			app.browserWindowFactory = func(context.Context, string) (*browserWindow, error) {
				count++
				if fail && count == 2 {
					return nil, errors.New("window unavailable")
				}
				return &browserWindow{id: fmt.Sprintf("new-%d", count)}, nil
			}
			notified := false
			sessionManagerOpenHandler(ctx, func(id entity.SessionID) error { return app.restoreSessionInNewWindows(ctx, id) }, func(_ context.Context, message string, level component.ToastLevel) {
				notified = true
				if fail {
					require.Contains(t, message, "window unavailable")
					require.Equal(t, component.ToastWarning, level)
				} else {
					require.Equal(t, "Session restored", message)
				}
			})("saved")
			require.True(t, notified)
			require.Same(t, old, app.browserWindows["old"])
			require.Same(t, oldTab, old.tabs.Find(oldTab.ID))
			require.Same(t, old, app.windowForTab[oldTab.ID])
			require.NotNil(t, app.tabs.Find(oldTab.ID))
			require.Equal(t, entity.SessionID("live"), app.deps.CurrentSessionID)
			if fail {
				require.Len(t, app.browserWindows, 1)
			} else {
				require.Equal(t, []string{"old", "new-1", "new-2"}, app.browserWindowOrder)
				require.Equal(t, "new-2", app.lastFocusedWindowID)
			}
		})
	}
}

func TestImportedWindowTabListsPreserveURLsAndExistingOwnership(t *testing.T) {
	savedTabs := entity.NewTabList()
	pane := entity.NewPane("saved-pane")
	pane.URI = "https://www.google.com/"
	savedTabs.Add(entity.NewTab("saved-tab", "saved-workspace", pane))
	state := entity.SnapshotFromWindowTabLists("historical", []entity.WindowTabListState{{Tabs: savedTabs}}, 0, time.Now())
	app := &App{tabs: entity.NewTabList(), browserWindows: map[string]*browserWindow{}, windowForTab: map[entity.TabID]*browserWindow{}}
	old := &browserWindow{id: "existing", tabs: entity.NewTabList()}
	app.registerBrowserWindow(old)
	restored := entity.WindowTabListsFromSnapshot(state, app.generateID)
	added := &browserWindow{id: "imported", tabs: entity.NewTabList()}
	app.assignRuntimeWindowTabLists([]*browserWindow{added}, restored, false)
	require.Same(t, old, app.browserWindows["existing"])
	require.Len(t, added.tabs.Tabs, 1)
	tab := added.tabs.Tabs[0]
	require.NotEqual(t, entity.TabID("saved-tab"), tab.ID)
	require.Same(t, added, app.windowForTab[tab.ID])
	roundTrip := entity.SnapshotFromWindowTabLists("live", []entity.WindowTabListState{{Tabs: added.tabs}}, 0, time.Now())
	require.Equal(t, "https://www.google.com/", roundTrip.Windows[0].Tabs[0].Workspace.Root.Pane.URI)
}

func TestRuntimeRestoreRollsBackWhenSecondWorkspaceFails(t *testing.T) {
	ctx := context.Background()
	ended := time.Now()
	sessions := repomocks.NewMockSessionRepository(t)
	sessions.EXPECT().FindByID(ctx, entity.SessionID("saved")).Return(&entity.Session{ID: "saved", EndedAt: &ended}, nil)
	saved := entity.NewTabList()
	for i := range 2 {
		saved.Add(entity.NewTab(entity.TabID(fmt.Sprintf("saved-%d", i)), entity.WorkspaceID(fmt.Sprintf("ws-%d", i)), entity.NewPane(entity.PaneID(fmt.Sprintf("pane-%d", i)))))
	}
	state := entity.SnapshotFromWindowTabLists("saved", []entity.WindowTabListState{{Tabs: saved}}, 0, ended)
	oldTabs := entity.NewTabList()
	oldTab := entity.NewTab("old-tab", "old-ws", entity.NewPane("old-pane"))
	oldTabs.Add(oldTab)
	old := &browserWindow{id: "old", tabs: oldTabs}
	oldView := &component.WorkspaceView{}
	app := &App{
		deps:          &Dependencies{CurrentSessionID: "live", SessionRepo: sessions, SessionStateRepo: mockSessionStateRepoWithSnapshot(t, "saved", state)},
		runtimeConfig: runtimeConfigStateForTest(&config.Config{}), tabs: entity.NewTabList(),
		browserWindows: map[string]*browserWindow{"old": old}, browserWindowOrder: []string{"old"}, lastFocusedWindowID: "old",
		windowForTab: map[entity.TabID]*browserWindow{oldTab.ID: old}, workspaceViews: map[entity.TabID]*component.WorkspaceView{oldTab.ID: oldView},
		snapshotService: portmocks.NewMockSnapshotService(t),
	}
	app.tabs.Add(oldTab)
	app.browserWindowFactory = func(context.Context, string) (*browserWindow, error) { return &browserWindow{id: "new"}, nil }
	attempts := 0
	app.workspaceViewCreateOverride = func(_ context.Context, tab *entity.Tab) bool {
		attempts++
		app.workspaceViews[tab.ID] = &component.WorkspaceView{}
		return attempts == 1
	}
	err := app.restoreSessionInNewWindows(ctx, "saved")
	require.ErrorContains(t, err, "create workspace")
	require.Equal(t, 2, attempts)
	require.Equal(t, []string{"old"}, app.browserWindowOrder)
	require.Len(t, app.browserWindows, 1)
	require.Same(t, old, app.browserWindows["old"])
	require.Len(t, app.windowForTab, 1)
	require.Same(t, old, app.windowForTab[oldTab.ID])
	require.Len(t, app.workspaceViews, 1)
	require.Same(t, oldView, app.workspaceViews[oldTab.ID])
	require.Equal(t, 1, app.tabs.Count())
	require.NotNil(t, app.tabs.Find(oldTab.ID))
	require.Equal(t, "old", app.lastFocusedWindowID)
	require.Equal(t, entity.SessionID("live"), app.deps.CurrentSessionID)
}

func TestStartupRestoreFailureLeavesEmptyInitialWindowForFallback(t *testing.T) {
	ctx := context.Background()
	saved := entity.NewTabList()
	saved.Add(entity.NewTab("saved-tab", "saved-ws", entity.NewPane("saved-pane")))
	state := entity.SnapshotFromWindowTabLists("saved", []entity.WindowTabListState{{Tabs: saved}, {Tabs: saved}}, 1, time.Now())
	first := &browserWindow{id: "initial", tabs: entity.NewTabList()}
	app := &App{
		deps:          &Dependencies{SessionStateRepo: mockSessionStateRepoWithSnapshot(t, "saved", state)},
		runtimeConfig: runtimeConfigStateForTest(&config.Config{}), tabs: entity.NewTabList(),
		browserWindows: map[string]*browserWindow{"initial": first}, browserWindowOrder: []string{"initial"}, lastFocusedWindowID: "initial",
		windowForTab: map[entity.TabID]*browserWindow{}, workspaceViews: map[entity.TabID]*component.WorkspaceView{},
	}
	app.browserWindowFactory = func(context.Context, string) (*browserWindow, error) { return &browserWindow{id: "extra"}, nil }
	attempts := 0
	app.workspaceViewCreateOverride = func(_ context.Context, tab *entity.Tab) bool {
		attempts++
		app.workspaceViews[tab.ID] = &component.WorkspaceView{}
		return attempts == 1
	}
	require.ErrorContains(t, app.restoreSession(ctx, "saved"), "create workspace")
	require.Equal(t, 2, attempts)
	require.Equal(t, []string{"initial"}, app.browserWindowOrder)
	require.Len(t, app.browserWindows, 1)
	require.Same(t, first, app.browserWindows["initial"])
	require.Zero(t, first.tabs.Count())
	require.Zero(t, app.tabs.Count())
	require.Empty(t, app.windowForTab)
	require.Empty(t, app.workspaceViews)
	require.Equal(t, "initial", app.lastFocusedWindowID)
	// The fallback targets the retained empty shell, not a half-imported window.
	require.Same(t, first, app.lastFocusedBrowserWindow())
}
