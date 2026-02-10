package coordinator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/ui/component"
)

func TestContentCoordinator_ActiveWebView_UsesOverride(t *testing.T) {
	t.Parallel()

	mainWV := &webkit.WebView{}
	floatingWV := &webkit.WebView{}

	c := &ContentCoordinator{
		webViews: map[entity.PaneID]*webkit.WebView{
			"main-pane":     mainWV,
			"floating-pane": floatingWV,
		},
	}

	c.SetActivePaneOverride("floating-pane")

	assert.Equal(t, floatingWV, c.ActiveWebView(context.Background()))
	assert.Equal(t, entity.PaneID("floating-pane"), c.ActivePaneID(context.Background()))
}

func TestContentCoordinator_ActiveWebView_ClearOverrideFallsBack(t *testing.T) {
	t.Parallel()

	mainWV := &webkit.WebView{}

	c := &ContentCoordinator{
		webViews: map[entity.PaneID]*webkit.WebView{
			"main-pane": mainWV,
		},
	}

	c.SetActivePaneOverride("main-pane")
	c.ClearActivePaneOverride()

	assert.Nil(t, c.ActiveWebView(context.Background()))
	assert.Equal(t, entity.PaneID(""), c.ActivePaneID(context.Background()))
}

func TestContentCoordinator_ActiveWebView_ClearOverrideFallsBackToWorkspace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mainWV := &webkit.WebView{}
	mainPane := entity.NewPane(entity.PaneID("main-pane"))
	ws := entity.NewWorkspace(entity.WorkspaceID("ws-1"), mainPane)

	c := &ContentCoordinator{
		webViews: map[entity.PaneID]*webkit.WebView{
			mainPane.ID: mainWV,
		},
		getActiveWS: func() (*entity.Workspace, *component.WorkspaceView) {
			return ws, nil
		},
	}

	c.SetActivePaneOverride("floating-pane")
	c.ClearActivePaneOverride()

	assert.Equal(t, mainWV, c.ActiveWebView(ctx))
	assert.Equal(t, ws.ActivePaneID, c.ActivePaneID(ctx))
}
