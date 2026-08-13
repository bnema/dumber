package coordinator

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	contentcoord "github.com/bnema/dumber/internal/ui/coordinator/content"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceCoordinator_SplitWithURLUsesExplicitURLAndActivatesPane(t *testing.T) {
	ctx := context.Background()
	ids := []string{"pane-2", "split-1"}
	index := 0
	ws := entity.NewWorkspace("ws-1", entity.NewPane("pane-1"))
	coord := NewWorkspaceCoordinator(ctx, WorkspaceCoordinatorConfig{
		PanesUC: usecase.NewManagePanesUseCase(func() string {
			id := ids[index]
			index++
			return id
		}, nil),
		NewPaneURL: "https://legacy.example/",
		GetActiveWS: func() (*entity.Workspace, *component.WorkspaceView) {
			return ws, nil
		},
	})

	const explicitURL = "https://explicit.example/path?token=secret"
	require.NoError(t, coord.SplitWithURL(ctx, usecase.SplitLeft, explicitURL))

	active := ws.ActivePane()
	require.NotNil(t, active)
	require.NotNil(t, active.Pane)
	assert.Equal(t, entity.PaneID("pane-2"), ws.ActivePaneID)
	assert.Equal(t, explicitURL, active.Pane.URI)
	assert.Equal(t, entity.SplitHorizontal, ws.Root.SplitDir)
	assert.Equal(t, entity.PaneID("pane-2"), ws.Root.Children[0].Pane.ID)
}

func TestWorkspaceCoordinator_SplitReturnsErrorWhenCreationTargetMissing(t *testing.T) {
	coord := NewWorkspaceCoordinator(context.Background(), WorkspaceCoordinatorConfig{})
	require.Error(t, coord.SplitWithURL(context.Background(), usecase.SplitRight, "https://example.com"))
}

func TestWorkspaceCoordinator_StackPaneUsesConfiguredURLThroughLegacyEntryPoint(t *testing.T) {
	const legacyURL = "https://legacy.example/"
	coord, ws := stackURLTestCoordinator(t, legacyURL)

	// The test coordinator intentionally has no WebView pool. Creation reaches
	// that detectable UI failure only after the requested URL enters the model.
	require.ErrorContains(t, coord.StackPane(context.Background()), "webview pool not configured")
	assert.Equal(t, legacyURL, ws.ActivePane().Pane.URI)
}

func TestWorkspaceCoordinator_StackPaneWithURLUsesExplicitURL(t *testing.T) {
	const explicitURL = "https://explicit.example/path?token=secret"
	coord, ws := stackURLTestCoordinator(t, "https://legacy.example/")

	require.ErrorContains(t, coord.StackPaneWithURL(context.Background(), explicitURL), "webview pool not configured")
	assert.Equal(t, explicitURL, ws.ActivePane().Pane.URI)
}

func stackURLTestCoordinator(t *testing.T, legacyURL string) (*WorkspaceCoordinator, *entity.Workspace) {
	t.Helper()
	if !gtk.InitCheck() || gdk.DisplayGetDefault() == nil {
		t.Skip("GTK native display prerequisite unavailable")
	}
	ctx := context.Background()
	factory := layout.NewGtkWidgetFactory()
	ws := entity.NewWorkspace("ws-1", entity.NewPane("pane-1"))
	wsView := component.NewWorkspaceView(ctx, factory)
	require.NoError(t, wsView.SetWorkspace(ctx, ws))
	ids := []string{"pane-2", "stack-1"}
	index := 0
	content := contentcoord.NewCoordinator(ctx, nil, nil, factory, nil, nil, nil, nil)
	coord := NewWorkspaceCoordinator(ctx, WorkspaceCoordinatorConfig{
		PanesUC: usecase.NewManagePanesUseCase(func() string {
			id := ids[index]
			index++
			return id
		}, nil),
		WidgetFactory:  factory,
		StackedPaneMgr: component.NewStackedPaneManager(factory),
		ContentCoord:   content,
		NewPaneURL:     legacyURL,
		GetActiveWS: func() (*entity.Workspace, *component.WorkspaceView) {
			return ws, wsView
		},
	})
	return coord, ws
}
