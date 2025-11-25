package api

import (
	"context"
	"fmt"
	"sync"
)

// PageActionState holds the state for a page action on a specific tab
type PageActionState struct {
	Icon    string // Icon path or data URL
	Title   string
	Visible bool
}

// PageActionDispatcher handles pageAction.* API calls
type PageActionDispatcher struct {
	mu      sync.RWMutex
	actions map[string]map[int64]*PageActionState // key: extensionID -> tabID -> state
}

// NewPageActionDispatcher creates a new pageAction API dispatcher
func NewPageActionDispatcher() *PageActionDispatcher {
	return &PageActionDispatcher{
		actions: make(map[string]map[int64]*PageActionState),
	}
}

// SetIcon sets the icon for a page action on a specific tab
func (d *PageActionDispatcher) SetIcon(ctx context.Context, extID string, details map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Extract tabId (required)
	tabIDRaw, ok := details["tabId"]
	if !ok {
		return fmt.Errorf("pageAction.setIcon(): missing 'tabId' field")
	}

	tabID, ok := toInt64(tabIDRaw)
	if !ok || tabID <= 0 {
		return fmt.Errorf("pageAction.setIcon(): invalid 'tabId' value")
	}

	// Extract path (can be string or object)
	var iconPath string
	if pathRaw, hasPath := details["path"]; hasPath {
		switch v := pathRaw.(type) {
		case string:
			iconPath = v
		case map[string]interface{}:
			// Object format: {"16": "icon16.png", "32": "icon32.png"}
			// For now, just take the first available size
			for _, path := range v {
				if pathStr, ok := path.(string); ok {
					iconPath = pathStr
					break
				}
			}
		default:
			return fmt.Errorf("pageAction.setIcon(): 'path' must be a string or object")
		}
	}

	// Get or create state for this extension/tab
	state := d.getOrCreateState(extID, tabID)
	state.Icon = iconPath

	// TODO: Trigger UI update for extensions overlay
	// For now, just store the state

	return nil
}

// SetTitle sets the title (tooltip) for a page action on a specific tab
func (d *PageActionDispatcher) SetTitle(ctx context.Context, extID string, details map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Extract tabId (required)
	tabIDRaw, ok := details["tabId"]
	if !ok {
		return fmt.Errorf("pageAction.setTitle(): missing 'tabId' field")
	}

	tabID, ok := toInt64(tabIDRaw)
	if !ok || tabID <= 0 {
		return fmt.Errorf("pageAction.setTitle(): invalid 'tabId' value")
	}

	// Extract title (required)
	titleRaw, ok := details["title"]
	if !ok {
		return fmt.Errorf("pageAction.setTitle(): missing 'title' field")
	}

	title, ok := titleRaw.(string)
	if !ok {
		return fmt.Errorf("pageAction.setTitle(): 'title' must be a string")
	}

	// Get or create state for this extension/tab
	state := d.getOrCreateState(extID, tabID)
	state.Title = title

	// TODO: Trigger UI update for extensions overlay

	return nil
}

// GetTitle returns the title for a page action on a specific tab
func (d *PageActionDispatcher) GetTitle(ctx context.Context, extID string, details map[string]interface{}) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Extract tabId (required)
	tabIDRaw, ok := details["tabId"]
	if !ok {
		return "", fmt.Errorf("pageAction.getTitle(): missing 'tabId' field")
	}

	tabID, ok := toInt64(tabIDRaw)
	if !ok || tabID <= 0 {
		return "", fmt.Errorf("pageAction.getTitle(): invalid 'tabId' value")
	}

	// Get state for this extension/tab
	state := d.getState(extID, tabID)
	if state == nil {
		return "", nil // Return empty string if no state exists
	}

	return state.Title, nil
}

// Show makes a page action visible on a specific tab
func (d *PageActionDispatcher) Show(ctx context.Context, extID string, tabID int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if tabID <= 0 {
		return fmt.Errorf("pageAction.show(): invalid 'tabId' value")
	}

	// Get or create state for this extension/tab
	state := d.getOrCreateState(extID, tabID)
	state.Visible = true

	// TODO: Trigger UI update for extensions overlay

	return nil
}

// Hide makes a page action invisible on a specific tab
func (d *PageActionDispatcher) Hide(ctx context.Context, extID string, tabID int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if tabID <= 0 {
		return fmt.Errorf("pageAction.hide(): invalid 'tabId' value")
	}

	// Get or create state for this extension/tab
	state := d.getOrCreateState(extID, tabID)
	state.Visible = false

	// TODO: Trigger UI update for extensions overlay

	return nil
}

// getOrCreateState returns or creates the state for an extension/tab combination
func (d *PageActionDispatcher) getOrCreateState(extID string, tabID int64) *PageActionState {
	tabMap, exists := d.actions[extID]
	if !exists {
		tabMap = make(map[int64]*PageActionState)
		d.actions[extID] = tabMap
	}

	state, exists := tabMap[tabID]
	if !exists {
		state = &PageActionState{
			Visible: false, // Hidden by default
		}
		tabMap[tabID] = state
	}

	return state
}

// getState returns the state for an extension/tab combination, or nil if it doesn't exist
func (d *PageActionDispatcher) getState(extID string, tabID int64) *PageActionState {
	tabMap, exists := d.actions[extID]
	if !exists {
		return nil
	}

	return tabMap[tabID]
}

// GetPageActionState returns the current state for a page action (for UI rendering)
func (d *PageActionDispatcher) GetPageActionState(extID string, tabID int64) *PageActionState {
	d.mu.RLock()
	defer d.mu.RUnlock()

	state := d.getState(extID, tabID)
	if state == nil {
		return &PageActionState{Visible: false}
	}

	// Return a copy to prevent external mutations
	return &PageActionState{
		Icon:    state.Icon,
		Title:   state.Title,
		Visible: state.Visible,
	}
}

// toInt64 safely converts various numeric types to int64
func toInt64(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case int64:
		return val, true
	case int:
		return int64(val), true
	case float64:
		return int64(val), true
	case float32:
		return int64(val), true
	default:
		return 0, false
	}
}
