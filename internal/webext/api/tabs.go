package api

import (
	"context"
	"fmt"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
)

// PaneInfo represents information about a browser pane
// Note: This is duplicated from the parent package to avoid import cycles
type PaneInfo struct {
	ID       uint64
	Index    int
	Active   bool
	URL      string
	Title    string
	WindowID uint64
}

// PaneInfoProvider provides pane information (from the manager)
type PaneInfoProvider interface {
	GetAllPanes() []PaneInfo
	GetActivePane() *PaneInfo
}

// TabOperations provides browser operations for tab management
type TabOperations interface {
	CreateTab(url string, active bool) (*Tab, error)
	RemoveTab(tabID int64) error
	UpdateTab(tabID int64, url string) (*Tab, error)
	ReloadTab(tabID int64) error
}

// WebViewLookup provides access to WebViews by ID
type WebViewLookup interface {
	GetViewByID(viewID uint64) WebViewOperations
}

// WebViewOperations defines operations that can be performed on a WebView
type WebViewOperations interface {
	Reload() error
	GetZoom() float64
	SetZoom(zoom float64) error
	GetCurrentURL() string
	InjectScript(script string) error
	GetUserContentManager() *webkit.UserContentManager
}

// CSSManager extends HostPermissionChecker with CSS tracking capabilities
type CSSManager interface {
	HasHostPermission(url string) bool
	HasPermission(permission string) bool
	AddCustomCSS(code string) *webkit.UserStyleSheet
	GetCustomCSS(code string) *webkit.UserStyleSheet
}

// TabsAPIDispatcher handles tabs.* API calls
type TabsAPIDispatcher struct {
	provider   PaneInfoProvider
	operations TabOperations
	viewLookup WebViewLookup
}

// NewTabsAPIDispatcher creates a new tabs API dispatcher
func NewTabsAPIDispatcher(provider PaneInfoProvider, operations TabOperations, viewLookup WebViewLookup) *TabsAPIDispatcher {
	return &TabsAPIDispatcher{
		provider:   provider,
		operations: operations,
		viewLookup: viewLookup,
	}
}

// Query returns tabs matching the given criteria
func (d *TabsAPIDispatcher) Query(ctx context.Context, queryInfo map[string]interface{}) ([]Tab, error) {
	if d.provider == nil {
		return []Tab{}, nil
	}

	// Check query filters
	active, hasActive := queryInfo["active"]
	currentWindow, hasCurrentWindow := queryInfo["currentWindow"]

	// If filtering for active tab in current window, return just the active pane
	if hasActive && active == true && (!hasCurrentWindow || currentWindow == true) {
		if activePane := d.provider.GetActivePane(); activePane != nil {
			return []Tab{
				{
					ID:       int(activePane.ID),
					Index:    activePane.Index,
					WindowID: int(activePane.WindowID),
					Active:   activePane.Active,
					URL:      activePane.URL,
					Title:    activePane.Title,
				},
			}, nil
		}
		return []Tab{}, nil
	}

	// Return all panes
	panes := d.provider.GetAllPanes()
	tabs := make([]Tab, 0, len(panes))
	for _, pane := range panes {
		// Apply filters
		if hasActive && active == true && !pane.Active {
			continue
		}
		if hasActive && active == false && pane.Active {
			continue
		}

		tabs = append(tabs, Tab{
			ID:       int(pane.ID),
			Index:    pane.Index,
			WindowID: int(pane.WindowID),
			Active:   pane.Active,
			URL:      pane.URL,
			Title:    pane.Title,
		})
	}

	return tabs, nil
}

// Get returns a specific tab by ID
func (d *TabsAPIDispatcher) Get(ctx context.Context, tabID int64) (*Tab, error) {
	if d.provider == nil {
		return nil, fmt.Errorf("tab not found: %d", tabID)
	}

	// Search through all panes for matching ID
	panes := d.provider.GetAllPanes()
	for _, pane := range panes {
		if int(pane.ID) == int(tabID) {
			return &Tab{
				ID:       int(pane.ID),
				Index:    pane.Index,
				WindowID: int(pane.WindowID),
				Active:   pane.Active,
				URL:      pane.URL,
				Title:    pane.Title,
			}, nil
		}
	}

	return nil, fmt.Errorf("tab not found: %d", tabID)
}

// Create creates a new tab
func (d *TabsAPIDispatcher) Create(ctx context.Context, checker HostPermissionChecker, createProperties map[string]interface{}) (*Tab, error) {
	// Extract URL from properties
	urlStr, _ := createProperties["url"].(string)

	// Validate URL if provided (prevent privileged schemes)
	if urlStr != "" && !isUnprivilegedURL(urlStr) {
		return nil, fmt.Errorf("tabs.create(): URL '%s' is not allowed", urlStr)
	}

	// Extract active flag (defaults to true)
	active := true
	if activeVal, ok := createProperties["active"].(bool); ok {
		active = activeVal
	}

	// Create the tab via browser operations
	if d.operations == nil {
		return nil, fmt.Errorf("tabs.create(): tab operations not available")
	}

	return d.operations.CreateTab(urlStr, active)
}

// Remove closes one or more tabs
func (d *TabsAPIDispatcher) Remove(ctx context.Context, tabIDs interface{}) error {
	if d.operations == nil {
		return fmt.Errorf("tabs.remove(): tab operations not available")
	}

	// Handle single tab ID
	switch v := tabIDs.(type) {
	case float64:
		return d.operations.RemoveTab(int64(v))
	case int64:
		return d.operations.RemoveTab(v)
	case []interface{}:
		// Handle array of tab IDs
		for _, id := range v {
			switch tid := id.(type) {
			case float64:
				if err := d.operations.RemoveTab(int64(tid)); err != nil {
					return err
				}
			case int64:
				if err := d.operations.RemoveTab(tid); err != nil {
					return err
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("tabs.remove(): invalid tab ID type")
	}
}

// Update modifies tab properties
func (d *TabsAPIDispatcher) Update(ctx context.Context, tabID int64, updateProperties map[string]interface{}) (*Tab, error) {
	if d.operations == nil {
		return nil, fmt.Errorf("tabs.update(): tab operations not available")
	}

	// Extract URL if provided
	urlStr, hasURL := updateProperties["url"].(string)
	if hasURL {
		// Validate URL (prevent privileged schemes)
		if !isUnprivilegedURL(urlStr) {
			return nil, fmt.Errorf("tabs.update(): URL '%s' is not allowed", urlStr)
		}

		return d.operations.UpdateTab(tabID, urlStr)
	}

	// If no URL update, just return current tab info
	return d.Get(ctx, tabID)
}

// Reload reloads a tab's page
func (d *TabsAPIDispatcher) Reload(ctx context.Context, tabID int64, reloadProperties map[string]interface{}) error {
	if d.operations == nil {
		return fmt.Errorf("tabs.reload(): tab operations not available")
	}

	// Note: bypassCache property is ignored for now (WebKit's Reload() doesn't differentiate)
	return d.operations.ReloadTab(tabID)
}

// ExecuteScript executes JavaScript in a tab
func (d *TabsAPIDispatcher) ExecuteScript(ctx context.Context, checker HostPermissionChecker, tabID int64, details map[string]interface{}) (interface{}, error) {
	if d.viewLookup == nil {
		return nil, fmt.Errorf("tabs.executeScript(): view lookup not available")
	}

	// Get the webview
	view := d.viewLookup.GetViewByID(uint64(tabID))
	if view == nil {
		return nil, fmt.Errorf("tabs.executeScript(): tab not found: %d", tabID)
	}

	// Check host permission for the tab's current URL
	currentURL := view.GetCurrentURL()
	if !checker.HasHostPermission(currentURL) && !checker.HasPermission("activeTab") {
		return nil, fmt.Errorf("tabs.executeScript(): permission denied for URL '%s'", currentURL)
	}

	// Extract script code
	code, ok := details["code"].(string)
	if !ok || code == "" {
		return nil, fmt.Errorf("tabs.executeScript(): missing 'code' field")
	}

	// Execute the script
	if err := view.InjectScript(code); err != nil {
		return nil, fmt.Errorf("tabs.executeScript(): script execution failed: %w", err)
	}

	// Note: return values not yet supported
	return nil, nil
}

// InsertCSS injects CSS into a tab
func (d *TabsAPIDispatcher) InsertCSS(ctx context.Context, checker CSSManager, tabID int64, details map[string]interface{}) error {
	if d.viewLookup == nil {
		return fmt.Errorf("tabs.insertCSS(): view lookup not available")
	}

	// Get the webview
	view := d.viewLookup.GetViewByID(uint64(tabID))
	if view == nil {
		return fmt.Errorf("tabs.insertCSS(): tab not found: %d", tabID)
	}

	// Check host permission for the tab's current URL
	currentURL := view.GetCurrentURL()
	if !checker.HasHostPermission(currentURL) && !checker.HasPermission("activeTab") {
		return fmt.Errorf("tabs.insertCSS(): permission denied for URL '%s'", currentURL)
	}

	// Extract CSS code
	code, ok := details["code"].(string)
	if !ok || code == "" {
		return fmt.Errorf("tabs.insertCSS(): missing 'code' field")
	}

	// Get UserContentManager from view
	ucm := view.GetUserContentManager()
	if ucm == nil {
		return fmt.Errorf("tabs.insertCSS(): UserContentManager not available")
	}

	// Create/retrieve stylesheet via extension's CSS manager
	sheet := checker.AddCustomCSS(code)
	if sheet == nil {
		return fmt.Errorf("tabs.insertCSS(): failed to create stylesheet")
	}

	// Add to UserContentManager
	ucm.AddStyleSheet(sheet)
	return nil
}

// RemoveCSS removes previously injected CSS from a tab
func (d *TabsAPIDispatcher) RemoveCSS(ctx context.Context, checker CSSManager, tabID int64, details map[string]interface{}) error {
	if d.viewLookup == nil {
		return fmt.Errorf("tabs.removeCSS(): view lookup not available")
	}

	// Get the webview
	view := d.viewLookup.GetViewByID(uint64(tabID))
	if view == nil {
		return fmt.Errorf("tabs.removeCSS(): tab not found: %d", tabID)
	}

	// Check host permission for the tab's current URL
	currentURL := view.GetCurrentURL()
	if !checker.HasHostPermission(currentURL) && !checker.HasPermission("activeTab") {
		return fmt.Errorf("tabs.removeCSS(): permission denied for URL '%s'", currentURL)
	}

	// Extract CSS code
	code, ok := details["code"].(string)
	if !ok || code == "" {
		return fmt.Errorf("tabs.removeCSS(): missing 'code' field")
	}

	// Get UserContentManager from view
	ucm := view.GetUserContentManager()
	if ucm == nil {
		return fmt.Errorf("tabs.removeCSS(): UserContentManager not available")
	}

	// Retrieve previously injected stylesheet
	sheet := checker.GetCustomCSS(code)
	if sheet == nil {
		// CSS not found - this is not an error per WebExtension spec
		return nil
	}

	// Remove from UserContentManager
	ucm.RemoveStyleSheet(sheet)
	return nil
}

// GetZoom gets the current zoom factor for a tab
func (d *TabsAPIDispatcher) GetZoom(ctx context.Context, tabID int64) (float64, error) {
	if d.viewLookup == nil {
		return 0, fmt.Errorf("tabs.getZoom(): view lookup not available")
	}

	// Get the webview
	view := d.viewLookup.GetViewByID(uint64(tabID))
	if view == nil {
		return 0, fmt.Errorf("tabs.getZoom(): tab not found: %d", tabID)
	}

	return view.GetZoom(), nil
}

// SetZoom sets the zoom factor for a tab
func (d *TabsAPIDispatcher) SetZoom(ctx context.Context, tabID int64, zoomFactor float64) error {
	if d.viewLookup == nil {
		return fmt.Errorf("tabs.setZoom(): view lookup not available")
	}

	// Validate zoom range (matching Epiphany's validation)
	if zoomFactor < 0.3 || zoomFactor > 5.0 {
		return fmt.Errorf("tabs.setZoom(): zoomFactor must be between 0.3 and 5.0")
	}

	// Get the webview
	view := d.viewLookup.GetViewByID(uint64(tabID))
	if view == nil {
		return fmt.Errorf("tabs.setZoom(): tab not found: %d", tabID)
	}

	return view.SetZoom(zoomFactor)
}

// isUnprivilegedURL checks if a URL is allowed for tab creation/navigation
// This prevents extensions from navigating to privileged schemes
func isUnprivilegedURL(urlStr string) bool {
	if urlStr == "" {
		return true // Empty URL is allowed (new tab page)
	}

	// Forbidden schemes per WebExtension spec
	forbidden := []string{"data:", "javascript:", "chrome:", "file:", "about:"}
	for _, scheme := range forbidden {
		if len(urlStr) >= len(scheme) && urlStr[:len(scheme)] == scheme {
			return false
		}
	}

	return true
}
