package dispatcher

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
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

func TestKeyboardDispatcher_ToggleHistorySystemViewOpensRightSplit(t *testing.T) {
	ctx := context.Background()
	ids := []string{"pane-2", "split-1"}
	idx := 0
	panesUC := usecase.NewManagePanesUseCase(func() string {
		id := ids[idx]
		idx++
		return id
	})

	initialPane := entity.NewPane("pane-1")
	initialPane.URI = "https://example.com"
	ws := entity.NewWorkspace("ws-1", initialPane)
	wsCoord := coordinator.NewWorkspaceCoordinator(ctx, coordinator.WorkspaceCoordinatorConfig{
		PanesUC: panesUC,
		GetActiveWS: func() (*entity.Workspace, *component.WorkspaceView) {
			return ws, nil
		},
	})

	d := NewKeyboardDispatcher(ctx, wsCoord, &coordinator.NavigationCoordinator{}, nil, nil, KeyboardActions{}, func(context.Context) entity.PaneID { return "" })

	err := d.Dispatch(ctx, input.ActionToggleHistorySystemView)
	require.NoError(t, err)

	require.Equal(t, 2, ws.PaneCount())
	active := ws.ActivePane()
	require.NotNil(t, active)
	require.NotNil(t, active.Pane)
	assert.Equal(t, entity.PaneID("pane-2"), active.Pane.ID)
	assert.Equal(t, "dumb://history", active.Pane.URI)
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
