package dispatcher

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	domainurl "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyboardDispatcher_AltNumberCreatesTabsUntilRequestedIndexExists(t *testing.T) {
	ctx := context.Background()
	ids := []string{
		"tab-1", "ws-1", "pane-1",
		"tab-2", "ws-2", "pane-2",
		"tab-3", "ws-3", "pane-3",
	}
	idx := 0
	tabsUC := usecase.NewManageTabsUseCase(func() string {
		id := ids[idx]
		idx++
		return id
	})
	tabs := entity.NewTabList()
	tabCoord := coordinator.NewTabCoordinator(ctx, coordinator.TabCoordinatorConfig{
		TabsUC: tabsUC,
		Tabs:   tabs,
	})

	_, err := tabCoord.Create(ctx, "https://initial.example")
	require.NoError(t, err)

	d := NewKeyboardDispatcher(ctx, tabCoord, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, "https://new.example", func(context.Context) entity.PaneID { return "" })

	err = d.Dispatch(ctx, input.ActionSwitchTabIndex3)
	require.NoError(t, err)

	require.Len(t, tabs.Tabs, 3)
	require.NotNil(t, tabs.ActiveTab())
	assert.Equal(t, tabs.TabAt(2).ID, tabs.ActiveTabID)
	assert.Equal(t, tabs.TabAt(0).ID, tabs.PreviousActiveTabID)
	assert.Equal(t, 2, tabs.ActiveTab().Position)
	assert.Equal(t, domainurl.Normalize("https://new.example"), tabs.TabAt(1).Workspace.Root.Pane.URI)
	assert.Equal(t, domainurl.Normalize("https://new.example"), tabs.TabAt(2).Workspace.Root.Pane.URI)

	err = tabCoord.SwitchToLastActive(ctx)
	require.NoError(t, err)
	assert.Equal(t, tabs.TabAt(0).ID, tabs.ActiveTabID)
}

func TestKeyboardDispatcher_AltNumberSwitchesToExistingTabWithoutNewPaneURL(t *testing.T) {
	ctx := context.Background()
	ids := []string{
		"tab-1", "ws-1", "pane-1",
		"tab-2", "ws-2", "pane-2",
	}
	idx := 0
	tabsUC := usecase.NewManageTabsUseCase(func() string {
		id := ids[idx]
		idx++
		return id
	})
	tabs := entity.NewTabList()
	tabCoord := coordinator.NewTabCoordinator(ctx, coordinator.TabCoordinatorConfig{
		TabsUC: tabsUC,
		Tabs:   tabs,
	})

	_, err := tabCoord.Create(ctx, "https://first.example")
	require.NoError(t, err)
	_, err = tabCoord.Create(ctx, "https://second.example")
	require.NoError(t, err)

	d := NewKeyboardDispatcher(ctx, tabCoord, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, "", func(context.Context) entity.PaneID { return "" })

	err = d.Dispatch(ctx, input.ActionSwitchTabIndex2)
	require.NoError(t, err)

	require.Len(t, tabs.Tabs, 2)
	assert.Equal(t, tabs.TabAt(1).ID, tabs.ActiveTabID)
	assert.Equal(t, domainurl.Normalize("https://second.example"), tabs.TabAt(1).Workspace.Root.Pane.URI)
}

func TestKeyboardDispatcher_AltNumberErrorsWhenMissingTabRequiresCreationWithoutNewPaneURL(t *testing.T) {
	ctx := context.Background()
	ids := []string{
		"tab-1", "ws-1", "pane-1",
	}
	idx := 0
	tabsUC := usecase.NewManageTabsUseCase(func() string {
		id := ids[idx]
		idx++
		return id
	})
	tabs := entity.NewTabList()
	tabCoord := coordinator.NewTabCoordinator(ctx, coordinator.TabCoordinatorConfig{
		TabsUC: tabsUC,
		Tabs:   tabs,
	})

	_, err := tabCoord.Create(ctx, "https://first.example")
	require.NoError(t, err)

	d := NewKeyboardDispatcher(ctx, tabCoord, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, "", func(context.Context) entity.PaneID { return "" })

	err = d.Dispatch(ctx, input.ActionSwitchTabIndex2)
	require.Error(t, err)
	require.Len(t, tabs.Tabs, 1)
	assert.Equal(t, tabs.TabAt(0).ID, tabs.ActiveTabID)
}

func TestKeyboardDispatcher_PassesActivePaneIDToShellCallbacks(t *testing.T) {
	ctx := context.Background()
	activePaneID := entity.PaneID("pane-1")
	d := NewKeyboardDispatcher(ctx, &coordinator.TabCoordinator{}, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, "", func(context.Context) entity.PaneID {
		return activePaneID
	})

	tests := []struct {
		name   string
		set    func(func(context.Context, entity.PaneID) error)
		invoke func(context.Context) error
	}{
		{
			name:   "session open",
			set:    d.SetOnSessionOpen,
			invoke: d.handleSessionOpen,
		},
		{
			name:   "move pane to tab",
			set:    d.SetOnMovePaneToTab,
			invoke: d.handleMovePaneToTab,
		},
		{
			name:   "move pane to next tab",
			set:    d.SetOnMovePaneToNextTab,
			invoke: d.handleMovePaneToNextTab,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPaneID entity.PaneID
			tc.set(func(_ context.Context, paneID entity.PaneID) error {
				gotPaneID = paneID
				return nil
			})

			require.NoError(t, tc.invoke(ctx))
			assert.Equal(t, activePaneID, gotPaneID)
		})
	}
}
