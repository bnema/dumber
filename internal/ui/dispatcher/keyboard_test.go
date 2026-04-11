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

	d := NewKeyboardDispatcher(ctx, tabCoord, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, "https://new.example")

	err = d.Dispatch(ctx, input.ActionSwitchTabIndex3)
	require.NoError(t, err)

	require.Len(t, tabs.Tabs, 3)
	require.NotNil(t, tabs.ActiveTab())
	assert.Equal(t, tabs.TabAt(2).ID, tabs.ActiveTabID)
	assert.Equal(t, 2, tabs.ActiveTab().Position)
	assert.Equal(t, domainurl.Normalize("https://new.example"), tabs.TabAt(1).Workspace.Root.Pane.URI)
	assert.Equal(t, domainurl.Normalize("https://new.example"), tabs.TabAt(2).Workspace.Root.Pane.URI)
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

	d := NewKeyboardDispatcher(ctx, tabCoord, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, "")

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

	d := NewKeyboardDispatcher(ctx, tabCoord, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, "")

	err = d.Dispatch(ctx, input.ActionSwitchTabIndex2)
	require.Error(t, err)
	require.Len(t, tabs.Tabs, 1)
	assert.Equal(t, tabs.TabAt(0).ID, tabs.ActiveTabID)
}
