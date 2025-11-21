package api

import (
	"context"
	"fmt"
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

// TabsAPIDispatcher handles tabs.* API calls
type TabsAPIDispatcher struct {
	provider PaneInfoProvider
}

// NewTabsAPIDispatcher creates a new tabs API dispatcher
func NewTabsAPIDispatcher(provider PaneInfoProvider) *TabsAPIDispatcher {
	return &TabsAPIDispatcher{
		provider: provider,
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
