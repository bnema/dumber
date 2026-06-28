package dispatcher

import (
	"context"
	"fmt"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyboardDispatcher_TabActionsUseInjectedKeyboardActions(t *testing.T) {
	ctx := context.Background()
	called := make([]string, 0)
	var switchedIndex int
	d := NewKeyboardDispatcher(
		ctx,
		&coordinator.WorkspaceCoordinator{},
		&coordinator.NavigationCoordinator{},
		nil,
		nil,
		KeyboardActions{
			NewTab: func(context.Context) error {
				called = append(called, "new")
				return nil
			},
			CloseTab: func(context.Context) error {
				called = append(called, "close")
				return nil
			},
			NextTab: func(context.Context) error {
				called = append(called, "next")
				return nil
			},
			PreviousTab: func(context.Context) error {
				called = append(called, "previous")
				return nil
			},
			SwitchLastTab: func(context.Context) error {
				called = append(called, "last")
				return nil
			},
			SwitchTabIndex: func(_ context.Context, index int) error {
				called = append(called, "index")
				switchedIndex = index
				return nil
			},
		},
		func(context.Context) entity.PaneID { return "" },
	)

	require.NoError(t, d.Dispatch(ctx, input.ActionNewTab))
	require.NoError(t, d.Dispatch(ctx, input.ActionCloseTab))
	require.NoError(t, d.Dispatch(ctx, input.ActionNextTab))
	require.NoError(t, d.Dispatch(ctx, input.ActionPreviousTab))
	require.NoError(t, d.Dispatch(ctx, input.ActionSwitchLastTab))
	require.NoError(t, d.Dispatch(ctx, input.ActionSwitchTabIndex5))

	assert.Equal(t, []string{"new", "close", "next", "previous", "last", "index"}, called)
	assert.Equal(t, 4, switchedIndex)
}

func TestKeyboardDispatcher_ToggleHistorySidebarCallsCallback(t *testing.T) {
	ctx := context.Background()
	d := NewKeyboardDispatcher(ctx, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, KeyboardActions{}, func(context.Context) entity.PaneID { return "" })

	var called bool
	d.SetOnToggleHistorySidebar(func(context.Context) error {
		called = true
		return nil
	})

	err := d.Dispatch(ctx, input.ActionToggleHistorySystemView)
	require.NoError(t, err)
	assert.True(t, called, "onToggleHistorySidebar should have been called")
}

func TestKeyboardDispatcher_ToggleHistorySystemViewReturnsErrorWhenHandlerMissing(t *testing.T) {
	ctx := context.Background()
	d := NewKeyboardDispatcher(ctx, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, KeyboardActions{}, func(context.Context) entity.PaneID { return "" })

	err := d.Dispatch(ctx, input.ActionToggleHistorySystemView)
	require.Error(t, err)
	assert.ErrorContains(t, err, "history sidebar unavailable")
}

func TestKeyboardDispatcher_ToggleHistorySidebarErrorPropagation(t *testing.T) {
	ctx := context.Background()
	d := NewKeyboardDispatcher(ctx, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, KeyboardActions{}, func(context.Context) entity.PaneID { return "" })

	wantErr := fmt.Errorf("sidebar toggle failed")
	d.SetOnToggleHistorySidebar(func(context.Context) error {
		return wantErr
	})

	err := d.Dispatch(ctx, input.ActionToggleHistorySystemView)
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr, "onToggleHistorySidebar error should propagate")
}

func TestKeyboardDispatcher_ToggleHistorySidebarSetThenUnsetReturnsError(t *testing.T) {
	ctx := context.Background()
	d := NewKeyboardDispatcher(ctx, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, KeyboardActions{}, func(context.Context) entity.PaneID { return "" })

	d.SetOnToggleHistorySidebar(func(context.Context) error {
		return nil
	})
	d.SetOnToggleHistorySidebar(nil)

	err := d.Dispatch(ctx, input.ActionToggleHistorySystemView)
	require.Error(t, err)
	assert.ErrorContains(t, err, "history sidebar unavailable")
}

func TestKeyboardDispatcher_ToggleFavoritesSidebarCallsCallbackAndPropagatesErrors(t *testing.T) {
	ctx := context.Background()
	d := NewKeyboardDispatcher(ctx, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, KeyboardActions{}, func(context.Context) entity.PaneID { return "" })

	missingErr := d.Dispatch(ctx, input.ActionToggleFavoritesSystemView)
	require.Error(t, missingErr)
	assert.ErrorContains(t, missingErr, "favorites sidebar unavailable")

	wantErr := fmt.Errorf("favorites failed")
	d.SetOnToggleFavoritesSidebar(func(context.Context) error { return wantErr })
	err := d.Dispatch(ctx, input.ActionToggleFavoritesSystemView)
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)

	var called bool
	d.SetOnToggleFavoritesSidebar(func(context.Context) error { called = true; return nil })
	require.NoError(t, d.Dispatch(ctx, input.ActionToggleFavoritesSystemView))
	assert.True(t, called)
}

func TestKeyboardDispatcher_ToggleCurrentPageFavoriteCallsCallbackAndPropagatesErrors(t *testing.T) {
	ctx := context.Background()
	d := NewKeyboardDispatcher(ctx, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, KeyboardActions{}, func(context.Context) entity.PaneID { return "" })

	missingErr := d.Dispatch(ctx, input.ActionToggleCurrentPageFavorite)
	require.Error(t, missingErr)
	assert.ErrorContains(t, missingErr, "toggle current page handler not wired")

	wantErr := fmt.Errorf("toggle failed")
	d.SetOnToggleCurrentPageFavorite(func(context.Context) error { return wantErr })
	err := d.Dispatch(ctx, input.ActionToggleCurrentPageFavorite)
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)

	var called bool
	d.SetOnToggleCurrentPageFavorite(func(context.Context) error { called = true; return nil })
	require.NoError(t, d.Dispatch(ctx, input.ActionToggleCurrentPageFavorite))
	assert.True(t, called)
}

func TestKeyboardDispatcher_PassesActivePaneIDToShellCallbacks(t *testing.T) {
	ctx := context.Background()
	activePaneID := entity.PaneID("pane-1")
	d := NewKeyboardDispatcher(ctx, &coordinator.WorkspaceCoordinator{}, &coordinator.NavigationCoordinator{}, nil, nil, KeyboardActions{}, func(context.Context) entity.PaneID {
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
		{
			name:   "eject pane to window",
			set:    d.SetOnEjectPaneToWindow,
			invoke: d.handleEjectPaneToWindow,
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
