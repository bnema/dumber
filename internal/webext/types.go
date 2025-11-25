package webext

import (
	"github.com/bnema/dumber/internal/webext/api"
	"github.com/bnema/dumber/pkg/webkit"
)

// ViewLookup provides access to WebViews and pane info by ID.
// This interface is implemented by BrowserApp.
type ViewLookup interface {
	// GetViewByID finds a WebView by its ID across all contexts (popups, tabs)
	GetViewByID(viewID uint64) *webkit.WebView

	// GetPaneInfoByViewID finds pane information for a WebView by its ID
	GetPaneInfoByViewID(viewID uint64) *api.PaneInfo
}
