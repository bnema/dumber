package coordinator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
)

func TestContentCoordinator_ActiveWebView_UsesOverride(t *testing.T) {
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
