package api

import (
	"context"
	"fmt"
)

// Window represents a browser window (workspace) state
type Window struct {
	ID          int    `json:"id"`
	Focused     bool   `json:"focused"`
	AlwaysOnTop bool   `json:"alwaysOnTop"`
	Type        string `json:"type"`  // "normal", "popup", "panel", "app", "devtools"
	State       string `json:"state"` // "normal", "minimized", "maximized", "fullscreen"
	Incognito   bool   `json:"incognito"`
	Tabs        []Tab  `json:"tabs,omitempty"` // Only populated if populate=true
}

// WindowsAPIDispatcher handles windows.* API calls
type WindowsAPIDispatcher struct {
	provider PaneInfoProvider // Provides access to workspace/pane information
}

// NewWindowsAPIDispatcher creates a new windows API dispatcher
func NewWindowsAPIDispatcher(provider PaneInfoProvider) *WindowsAPIDispatcher {
	return &WindowsAPIDispatcher{
		provider: provider,
	}
}

// Get retrieves details about a specific window
func (d *WindowsAPIDispatcher) Get(ctx context.Context, windowID int64, getInfo map[string]interface{}) (*Window, error) {
	if d.provider == nil {
		return nil, fmt.Errorf("window not found: %d", windowID)
	}

	// For now, dumber has a single workspace (window ID = 1)
	// In the future, this could be extended to support multiple workspaces
	if windowID != 1 {
		return nil, fmt.Errorf("window not found: %d", windowID)
	}

	// Check if we should populate tabs
	populate := false
	if getInfo != nil {
		if populateVal, ok := getInfo["populate"]; ok {
			if populateBool, ok := populateVal.(bool); ok {
				populate = populateBool
			}
		}
	}

	return d.getCurrentWindow(populate), nil
}

// GetCurrent retrieves the current (focused) window
func (d *WindowsAPIDispatcher) GetCurrent(ctx context.Context, getInfo map[string]interface{}) (*Window, error) {
	// Check if we should populate tabs
	populate := false
	if getInfo != nil {
		if populateVal, ok := getInfo["populate"]; ok {
			if populateBool, ok := populateVal.(bool); ok {
				populate = populateBool
			}
		}
	}

	return d.getCurrentWindow(populate), nil
}

// GetLastFocused retrieves the last focused window
func (d *WindowsAPIDispatcher) GetLastFocused(ctx context.Context, getInfo map[string]interface{}) (*Window, error) {
	// Same as GetCurrent since we only have one window
	return d.GetCurrent(ctx, getInfo)
}

// GetAll retrieves all windows
func (d *WindowsAPIDispatcher) GetAll(ctx context.Context, getInfo map[string]interface{}) ([]Window, error) {
	// Check if we should populate tabs
	populate := false
	if getInfo != nil {
		if populateVal, ok := getInfo["populate"]; ok {
			if populateBool, ok := populateVal.(bool); ok {
				populate = populateBool
			}
		}
	}

	// For now, return a single window (the current workspace)
	window := d.getCurrentWindow(populate)
	return []Window{*window}, nil
}

// Create creates a new window
func (d *WindowsAPIDispatcher) Create(ctx context.Context, createData map[string]interface{}) (*Window, error) {
	// Creating new windows (workspaces) is not supported yet
	// This would require integration with workspace creation in the browser
	return nil, fmt.Errorf("windows.create(): creating new windows is not supported yet")
}

// Remove removes (closes) a window
func (d *WindowsAPIDispatcher) Remove(ctx context.Context, windowID int64) error {
	// Closing the main window would close the entire browser
	// This is not supported for safety reasons
	return fmt.Errorf("windows.remove(): closing windows is not supported")
}

// getCurrentWindow builds a Window object for the current workspace
func (d *WindowsAPIDispatcher) getCurrentWindow(populate bool) *Window {
	window := &Window{
		ID:          1, // Single workspace for now
		Focused:     true,
		AlwaysOnTop: false,
		Type:        "normal",
		State:       "normal", // TODO: Could detect maximized/fullscreen state
		Incognito:   false,
	}

	// Populate tabs if requested
	if populate && d.provider != nil {
		panes := d.provider.GetAllPanes()
		tabs := make([]Tab, 0, len(panes))
		for _, pane := range panes {
			tabs = append(tabs, Tab{
				ID:       int(pane.ID),
				Index:    pane.Index,
				WindowID: int(pane.WindowID),
				Active:   pane.Active,
				URL:      pane.URL,
				Title:    pane.Title,
			})
		}
		window.Tabs = tabs
	}

	return window
}
