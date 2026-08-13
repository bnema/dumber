package ui

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/shared/syncdispatch"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalLinkSplitDirectionPlacements(t *testing.T) {
	tests := map[entity.ExternalLinkPlacement]usecase.SplitDirection{
		"":                                 usecase.SplitRight,
		entity.ExternalLinkPlacementLeft:   usecase.SplitLeft,
		entity.ExternalLinkPlacementRight:  usecase.SplitRight,
		entity.ExternalLinkPlacementTop:    usecase.SplitUp,
		entity.ExternalLinkPlacementBottom: usecase.SplitDown,
	}
	for placement, want := range tests {
		t.Run(string(placement), func(t *testing.T) {
			assert.Equal(t, want, externalLinkSplitDirection(placement))
		})
	}
}

func TestApp_OpenExternalURLTabbedTargetsLastFocusedWindowAndPreservesURL(t *testing.T) {
	ctx := context.Background()
	const originalURL = "https://example.com/path?token=secret#fragment"
	active := entity.NewTab("tab-1", "ws-1", entity.NewPane("pane-1"))
	tabs := entity.NewTabList()
	tabs.Add(active)
	focused := &browserWindow{id: "focused", tabs: tabs}

	ids := []string{"tab-2", "ws-2", "pane-2"}
	index := 0
	app := &App{
		runtimeConfig: runtimeConfigStateForTest(&config.Config{Workspace: entity.WorkspaceConfig{ExternalLinks: entity.ExternalLinksConfig{Behavior: entity.ExternalLinkBehaviorTabbed}}}),
		tabsUC:        usecase.NewManageTabsUseCase(func() string { id := ids[index]; index++; return id }, nil),
		browserWindows: map[string]*browserWindow{
			focused.id: focused,
		},
		browserWindowOrder:  []string{focused.id},
		lastFocusedWindowID: focused.id,
		workspaceViews:      map[entity.TabID]*component.WorkspaceView{},
		windowForTab:        map[entity.TabID]*browserWindow{},
	}
	app.tabCoord = coordinator.NewTabCoordinator(ctx, coordinator.TabCoordinatorConfig{TabsUC: app.tabsUC})
	app.tabCoord.SetOnTabCreated(func(_ context.Context, _ coordinator.TabTarget, tab *entity.Tab) {
		app.workspaceViews[tab.ID] = &component.WorkspaceView{}
		app.setBrowserWindowForTab(tab.ID, focused)
	})

	require.NoError(t, app.OpenExternalURL(ctx, originalURL))
	require.Len(t, tabs.Tabs, 2)
	created := tabs.ActiveTab()
	require.NotNil(t, created)
	require.NotNil(t, created.Workspace)
	assert.Equal(t, originalURL, created.Workspace.ActivePane().Pane.URI)
	assert.Same(t, focused, app.browserWindowForTab(created.ID))
}

func TestApp_OpenExternalURLSnapshotsRuntimeConfigOnce(t *testing.T) {
	ctx := context.Background()
	provider := mocks.NewMockRuntimeConfigProvider(t)
	provider.EXPECT().Current().Return(entity.RuntimeConfigSnapshot{UI: entity.RuntimeUIConfig{Workspace: entity.WorkspaceConfig{ExternalLinks: entity.ExternalLinksConfig{Behavior: entity.ExternalLinkBehaviorWindowed}}}}).Once()
	var factoryCalls int
	var dispatchCalls int
	app := &App{
		runtimeConfig: newRuntimeConfigState(provider),
		dispatchOnMainThread: func(label string, fn func()) syncdispatch.SyncDispatchResult {
			dispatchCalls++
			fn()
			return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchInline}
		},
		browserWindowFactory: func(_ context.Context, _ string) (*browserWindow, error) {
			factoryCalls++
			return &browserWindow{id: "windowed", tabs: entity.NewTabList()}, nil
		},
		browserWindows: map[string]*browserWindow{},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{},
		windowForTab:   map[entity.TabID]*browserWindow{},
	}

	require.NoError(t, app.OpenExternalURL(ctx, "https://example.com"))
	assert.Equal(t, 1, dispatchCalls)
	assert.Equal(t, 1, factoryCalls)
}

func TestApp_OpenExternalURLFallsBackWhenLastFocusedWindowIsStale(t *testing.T) {
	ctx := context.Background()
	const originalURL = "https://example.com/stale?token=secret"
	staleTab := entity.NewTab("stale-tab", "stale-workspace", entity.NewPane("stale-pane"))
	staleTabs := entity.NewTabList()
	staleTabs.Add(staleTab)
	stale := &browserWindow{id: "stale", tabs: staleTabs}
	var factoryURL string
	app := &App{
		runtimeConfig:       runtimeConfigStateForTest(&config.Config{Workspace: entity.WorkspaceConfig{ExternalLinks: entity.ExternalLinksConfig{Behavior: entity.ExternalLinkBehaviorTabbed}}}),
		browserWindows:      map[string]*browserWindow{},
		lastFocusedWindowID: stale.id,
		windowForTab:        map[entity.TabID]*browserWindow{staleTab.ID: stale},
		workspaceViews:      map[entity.TabID]*component.WorkspaceView{},
		browserWindowFactory: func(_ context.Context, url string) (*browserWindow, error) {
			factoryURL = url
			return &browserWindow{id: "fallback", tabs: entity.NewTabList()}, nil
		},
	}

	require.NoError(t, app.OpenExternalURL(ctx, originalURL))
	assert.Equal(t, originalURL, factoryURL)
}

func TestApp_OpenExternalURLFallbackPreservesOriginalURL(t *testing.T) {
	ctx := context.Background()
	const originalURL = "https://example.com/path?token=secret#fragment"
	var factoryURL string
	app := &App{
		runtimeConfig: runtimeConfigStateForTest(&config.Config{Workspace: entity.WorkspaceConfig{ExternalLinks: entity.ExternalLinksConfig{Behavior: entity.ExternalLinkBehaviorSplit, Placement: entity.ExternalLinkPlacementLeft}}}),
		browserWindowFactory: func(_ context.Context, url string) (*browserWindow, error) {
			factoryURL = url
			return &browserWindow{id: "fallback", tabs: entity.NewTabList()}, nil
		},
		browserWindows: map[string]*browserWindow{},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{},
		windowForTab:   map[entity.TabID]*browserWindow{},
	}

	require.NoError(t, app.OpenExternalURL(ctx, originalURL))
	assert.Equal(t, originalURL, factoryURL)
}
