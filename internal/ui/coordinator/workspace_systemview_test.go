package coordinator

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceCoordinator_ToggleSystemViewRightSplitsWithTargetURL(t *testing.T) {
	ctx := context.Background()
	ids := []string{"pane-2", "split-1"}
	idx := 0
	uc := usecase.NewManagePanesUseCase(func() string {
		id := ids[idx]
		idx++
		return id
	})

	initialPane := entity.NewPane("pane-1")
	initialPane.URI = "https://example.com"
	ws := entity.NewWorkspace("ws-1", initialPane)

	coord := NewWorkspaceCoordinator(ctx, WorkspaceCoordinatorConfig{
		PanesUC: uc,
		GetActiveWS: func() (*entity.Workspace, *component.WorkspaceView) {
			return ws, nil
		},
	})

	err := coord.ToggleSystemViewRight(ctx, "dumb://history")
	require.NoError(t, err)

	require.Equal(t, 2, ws.PaneCount())
	active := ws.ActivePane()
	require.NotNil(t, active)
	require.NotNil(t, active.Pane)
	assert.Equal(t, entity.PaneID("pane-2"), active.Pane.ID)
	assert.Equal(t, "dumb://history", active.Pane.URI)
}

func TestWorkspaceCoordinator_ToggleSystemViewRightFocusesExistingPane(t *testing.T) {
	ctx := context.Background()
	left := testLeafNode("pane-1")
	left.Pane.URI = "https://example.com"
	right := testLeafNode("pane-2")
	right.Pane.URI = "dumb://history"
	root := testSplitNode("split-1", left, right)
	ws := &entity.Workspace{ID: "ws-1", Root: root, ActivePaneID: left.Pane.ID}

	coord := NewWorkspaceCoordinator(ctx, WorkspaceCoordinatorConfig{
		GetActiveWS: func() (*entity.Workspace, *component.WorkspaceView) {
			return ws, nil
		},
	})

	err := coord.ToggleSystemViewRight(ctx, "dumb://history")
	require.NoError(t, err)

	assert.Equal(t, entity.PaneID("pane-2"), ws.ActivePaneID)
	assert.Equal(t, 2, ws.PaneCount())
}

func TestWorkspaceCoordinator_ToggleSystemViewRightClosesActivePane(t *testing.T) {
	ctx := context.Background()
	uc := usecase.NewManagePanesUseCase(func() string { return "unused" })
	left := testLeafNode("pane-1")
	left.Pane.URI = "https://example.com"
	right := testLeafNode("pane-2")
	right.Pane.URI = "dumb://history"
	root := testSplitNode("split-1", left, right)
	ws := &entity.Workspace{ID: "ws-1", Root: root, ActivePaneID: right.Pane.ID}

	coord := NewWorkspaceCoordinator(ctx, WorkspaceCoordinatorConfig{
		PanesUC: uc,
		GetActiveWS: func() (*entity.Workspace, *component.WorkspaceView) {
			return ws, nil
		},
	})

	err := coord.ToggleSystemViewRight(ctx, "dumb://history")
	require.NoError(t, err)

	assert.Equal(t, 1, ws.PaneCount())
	assert.Equal(t, entity.PaneID("pane-1"), ws.ActivePaneID)
	active := ws.ActivePane()
	require.NotNil(t, active)
	assert.Equal(t, "https://example.com", active.Pane.URI)
}
