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

	// HandleSendMessageResponse handles a response from tabs.sendMessage
	HandleSendMessageResponse(requestID string, response interface{})
}

// PopupInfo contains information about an extension popup for runtime.connect sender info
type PopupInfo struct {
	ExtensionID string
	URL         string
}

// PopupInfoProvider provides popup information by WebView ID.
// This interface is implemented by ExtensionPopupManager.
type PopupInfoProvider interface {
	// GetPopupInfoByViewID returns popup info if the view ID belongs to an extension popup
	GetPopupInfoByViewID(viewID uint64) *PopupInfo
}
