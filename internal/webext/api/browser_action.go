package api

import (
	"context"
	"fmt"
	"sync"
)

// BadgeState holds the badge configuration for an extension
type BadgeState struct {
	Text            string
	BackgroundColor string // CSS color string (hex, rgb, etc.)
	TextColor       string // CSS color string for badge text
	Title           string // Tooltip title
	Enabled         bool   // Whether the action is enabled (default: true)
}

// PopupManager interface for opening popups (to avoid circular dependency)
type PopupManager interface {
	OpenPopup(extID string, url string) error
}

// BrowserActionDispatcher handles browserAction.* API calls
type BrowserActionDispatcher struct {
	mu           sync.RWMutex
	badges       map[string]*BadgeState // key: extensionID
	popupURLs    map[string]string      // key: extensionID, value: popup URL
	popupManager PopupManager           // Manager for opening popups
}

// NewBrowserActionDispatcher creates a new browserAction API dispatcher
func NewBrowserActionDispatcher() *BrowserActionDispatcher {
	return &BrowserActionDispatcher{
		badges:    make(map[string]*BadgeState),
		popupURLs: make(map[string]string),
	}
}

// SetPopupManager sets the popup manager for opening popups
func (d *BrowserActionDispatcher) SetPopupManager(pm PopupManager) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.popupManager = pm
}

// SetBadgeText sets the badge text for an extension
func (d *BrowserActionDispatcher) SetBadgeText(ctx context.Context, extID string, details map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Extract text from details
	text, ok := details["text"]
	if !ok {
		return fmt.Errorf("browserAction.setBadgeText(): missing 'text' field")
	}

	textStr, ok := text.(string)
	if !ok {
		return fmt.Errorf("browserAction.setBadgeText(): 'text' must be a string")
	}

	// Check for tab-specific badges (not supported yet)
	if tabID, hasTabID := details["tabId"]; hasTabID && tabID != nil {
		return fmt.Errorf("browserAction.setBadgeText(): tabId is not supported yet")
	}
	if windowID, hasWindowID := details["windowId"]; hasWindowID && windowID != nil {
		return fmt.Errorf("browserAction.setBadgeText(): windowId is not supported yet")
	}

	// Get or create badge state for this extension
	badge, exists := d.badges[extID]
	if !exists {
		badge = &BadgeState{}
		d.badges[extID] = badge
	}

	badge.Text = textStr

	// TODO: Trigger UI update for extensions overlay
	// For now, just store the state

	return nil
}

// SetBadgeBackgroundColor sets the badge background color for an extension
func (d *BrowserActionDispatcher) SetBadgeBackgroundColor(ctx context.Context, extID string, details map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Extract color from details
	// Color can be either a string or an array of RGBA values
	colorRaw, ok := details["color"]
	if !ok {
		return fmt.Errorf("browserAction.setBadgeBackgroundColor(): missing 'color' field")
	}

	var colorStr string

	switch v := colorRaw.(type) {
	case string:
		colorStr = v
	case []interface{}:
		// Array format: [r, g, b, a] where values are 0-255
		if len(v) < 3 || len(v) > 4 {
			return fmt.Errorf("browserAction.setBadgeBackgroundColor(): color array must have 3 or 4 elements")
		}

		// Convert to rgba() CSS format
		r, rOk := v[0].(float64)
		g, gOk := v[1].(float64)
		b, bOk := v[2].(float64)

		if !rOk || !gOk || !bOk {
			return fmt.Errorf("browserAction.setBadgeBackgroundColor(): invalid color array values")
		}

		if len(v) == 4 {
			a, aOk := v[3].(float64)
			if !aOk {
				return fmt.Errorf("browserAction.setBadgeBackgroundColor(): invalid alpha value")
			}
			colorStr = fmt.Sprintf("rgba(%d, %d, %d, %.2f)", int(r), int(g), int(b), a/255.0)
		} else {
			colorStr = fmt.Sprintf("rgb(%d, %d, %d)", int(r), int(g), int(b))
		}
	default:
		return fmt.Errorf("browserAction.setBadgeBackgroundColor(): 'color' must be a string or array")
	}

	// Check for tab-specific badges (not supported yet)
	if tabID, hasTabID := details["tabId"]; hasTabID && tabID != nil {
		return fmt.Errorf("browserAction.setBadgeBackgroundColor(): tabId is not supported yet")
	}
	if windowID, hasWindowID := details["windowId"]; hasWindowID && windowID != nil {
		return fmt.Errorf("browserAction.setBadgeBackgroundColor(): windowId is not supported yet")
	}

	// Get or create badge state for this extension
	badge, exists := d.badges[extID]
	if !exists {
		badge = &BadgeState{}
		d.badges[extID] = badge
	}

	badge.BackgroundColor = colorStr

	// TODO: Trigger UI update for extensions overlay
	// For now, just store the state

	return nil
}

// GetBadgeState returns the current badge state for an extension
func (d *BrowserActionDispatcher) GetBadgeState(extID string) *BadgeState {
	d.mu.RLock()
	defer d.mu.RUnlock()

	badge, exists := d.badges[extID]
	if !exists {
		return &BadgeState{Enabled: true} // Default enabled
	}

	// Return a copy to prevent external mutations
	return &BadgeState{
		Text:            badge.Text,
		BackgroundColor: badge.BackgroundColor,
		TextColor:       badge.TextColor,
		Title:           badge.Title,
		Enabled:         badge.Enabled,
	}
}

// SetPopup sets the popup URL for a browser action
func (d *BrowserActionDispatcher) SetPopup(ctx context.Context, extID string, details map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Extract popup URL from details
	popupRaw, ok := details["popup"]
	if !ok {
		return fmt.Errorf("browserAction.setPopup(): missing 'popup' field")
	}

	popupURL, ok := popupRaw.(string)
	if !ok {
		return fmt.Errorf("browserAction.setPopup(): 'popup' must be a string")
	}

	// Check for tab-specific popups (not supported yet)
	if tabID, hasTabID := details["tabId"]; hasTabID && tabID != nil {
		return fmt.Errorf("browserAction.setPopup(): tabId is not supported yet")
	}

	// Store popup URL for this extension
	if popupURL == "" {
		delete(d.popupURLs, extID)
	} else {
		d.popupURLs[extID] = popupURL
	}

	return nil
}

// GetPopup returns the popup URL for a browser action
func (d *BrowserActionDispatcher) GetPopup(ctx context.Context, extID string, details map[string]interface{}) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check for tab-specific popups (not supported yet)
	if tabID, hasTabID := details["tabId"]; hasTabID && tabID != nil {
		return "", fmt.Errorf("browserAction.getPopup(): tabId is not supported yet")
	}

	// Return stored popup URL or empty string
	popupURL := d.popupURLs[extID]
	return popupURL, nil
}

// OpenPopup opens the extension popup programmatically
func (d *BrowserActionDispatcher) OpenPopup(ctx context.Context, extID string) error {
	d.mu.RLock()
	popupURL := d.popupURLs[extID]
	popupManager := d.popupManager
	d.mu.RUnlock()

	if popupURL == "" {
		return fmt.Errorf("browserAction.openPopup(): no popup URL configured for extension %s", extID)
	}

	if popupManager == nil {
		return fmt.Errorf("browserAction.openPopup(): popup manager not available")
	}

	return popupManager.OpenPopup(extID, popupURL)
}

// SetTitle sets the browser action's title (tooltip)
func (d *BrowserActionDispatcher) SetTitle(ctx context.Context, extID string, details map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	title, ok := details["title"]
	if !ok {
		return fmt.Errorf("browserAction.setTitle(): missing 'title' field")
	}

	titleStr, ok := title.(string)
	if !ok {
		return fmt.Errorf("browserAction.setTitle(): 'title' must be a string")
	}

	// Check for tab-specific titles (not supported yet)
	if tabID, hasTabID := details["tabId"]; hasTabID && tabID != nil {
		return fmt.Errorf("browserAction.setTitle(): tabId is not supported yet")
	}

	badge, exists := d.badges[extID]
	if !exists {
		badge = &BadgeState{Enabled: true}
		d.badges[extID] = badge
	}

	badge.Title = titleStr
	return nil
}

// GetTitle gets the browser action's title (tooltip)
func (d *BrowserActionDispatcher) GetTitle(ctx context.Context, extID string, details map[string]interface{}) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check for tab-specific titles (not supported yet)
	if tabID, hasTabID := details["tabId"]; hasTabID && tabID != nil {
		return "", fmt.Errorf("browserAction.getTitle(): tabId is not supported yet")
	}

	badge, exists := d.badges[extID]
	if !exists {
		return "", nil
	}

	return badge.Title, nil
}

// GetBadgeText gets the browser action's badge text
func (d *BrowserActionDispatcher) GetBadgeText(ctx context.Context, extID string, details map[string]interface{}) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check for tab-specific badges (not supported yet)
	if tabID, hasTabID := details["tabId"]; hasTabID && tabID != nil {
		return "", fmt.Errorf("browserAction.getBadgeText(): tabId is not supported yet")
	}

	badge, exists := d.badges[extID]
	if !exists {
		return "", nil
	}

	return badge.Text, nil
}

// GetBadgeBackgroundColor gets the badge's background color
func (d *BrowserActionDispatcher) GetBadgeBackgroundColor(ctx context.Context, extID string, details map[string]interface{}) (interface{}, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check for tab-specific badges (not supported yet)
	if tabID, hasTabID := details["tabId"]; hasTabID && tabID != nil {
		return nil, fmt.Errorf("browserAction.getBadgeBackgroundColor(): tabId is not supported yet")
	}

	badge, exists := d.badges[extID]
	if !exists || badge.BackgroundColor == "" {
		// Return default color as RGBA array [0, 0, 0, 0]
		return []int{0, 0, 0, 0}, nil
	}

	// Return as CSS color string (could also parse and return RGBA array)
	return badge.BackgroundColor, nil
}

// SetBadgeTextColor sets the badge's text color
func (d *BrowserActionDispatcher) SetBadgeTextColor(ctx context.Context, extID string, details map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	colorRaw, ok := details["color"]
	if !ok {
		return fmt.Errorf("browserAction.setBadgeTextColor(): missing 'color' field")
	}

	var colorStr string

	switch v := colorRaw.(type) {
	case string:
		colorStr = v
	case []interface{}:
		// Array format: [r, g, b, a] where values are 0-255
		if len(v) < 3 || len(v) > 4 {
			return fmt.Errorf("browserAction.setBadgeTextColor(): color array must have 3 or 4 elements")
		}

		r, rOk := v[0].(float64)
		g, gOk := v[1].(float64)
		b, bOk := v[2].(float64)

		if !rOk || !gOk || !bOk {
			return fmt.Errorf("browserAction.setBadgeTextColor(): invalid color array values")
		}

		if len(v) == 4 {
			a, aOk := v[3].(float64)
			if !aOk {
				return fmt.Errorf("browserAction.setBadgeTextColor(): invalid alpha value")
			}
			colorStr = fmt.Sprintf("rgba(%d, %d, %d, %.2f)", int(r), int(g), int(b), a/255.0)
		} else {
			colorStr = fmt.Sprintf("rgb(%d, %d, %d)", int(r), int(g), int(b))
		}
	default:
		return fmt.Errorf("browserAction.setBadgeTextColor(): 'color' must be a string or array")
	}

	// Check for tab-specific badges (not supported yet)
	if tabID, hasTabID := details["tabId"]; hasTabID && tabID != nil {
		return fmt.Errorf("browserAction.setBadgeTextColor(): tabId is not supported yet")
	}

	badge, exists := d.badges[extID]
	if !exists {
		badge = &BadgeState{Enabled: true}
		d.badges[extID] = badge
	}

	badge.TextColor = colorStr
	return nil
}

// GetBadgeTextColor gets the badge's text color
func (d *BrowserActionDispatcher) GetBadgeTextColor(ctx context.Context, extID string, details map[string]interface{}) (interface{}, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check for tab-specific badges (not supported yet)
	if tabID, hasTabID := details["tabId"]; hasTabID && tabID != nil {
		return nil, fmt.Errorf("browserAction.getBadgeTextColor(): tabId is not supported yet")
	}

	badge, exists := d.badges[extID]
	if !exists || badge.TextColor == "" {
		// Return default color as RGBA array [255, 255, 255, 255] (white)
		return []int{255, 255, 255, 255}, nil
	}

	return badge.TextColor, nil
}

// Enable enables the browser action for a tab (or globally if no tabId)
func (d *BrowserActionDispatcher) Enable(ctx context.Context, extID string, tabID *int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Tab-specific enable not supported yet
	if tabID != nil {
		return fmt.Errorf("browserAction.enable(): tabId is not supported yet")
	}

	badge, exists := d.badges[extID]
	if !exists {
		badge = &BadgeState{Enabled: true}
		d.badges[extID] = badge
	}

	badge.Enabled = true
	return nil
}

// Disable disables the browser action for a tab (or globally if no tabId)
func (d *BrowserActionDispatcher) Disable(ctx context.Context, extID string, tabID *int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Tab-specific disable not supported yet
	if tabID != nil {
		return fmt.Errorf("browserAction.disable(): tabId is not supported yet")
	}

	badge, exists := d.badges[extID]
	if !exists {
		badge = &BadgeState{Enabled: true}
		d.badges[extID] = badge
	}

	badge.Enabled = false
	return nil
}

// IsEnabled checks whether the browser action is enabled
func (d *BrowserActionDispatcher) IsEnabled(ctx context.Context, extID string, details map[string]interface{}) (bool, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check for tab-specific enabled state (not supported yet)
	if tabID, hasTabID := details["tabId"]; hasTabID && tabID != nil {
		return false, fmt.Errorf("browserAction.isEnabled(): tabId is not supported yet")
	}

	badge, exists := d.badges[extID]
	if !exists {
		return true, nil // Default enabled
	}

	return badge.Enabled, nil
}

// GetUserSettings gets the user-specified settings for the browser action
func (d *BrowserActionDispatcher) GetUserSettings(ctx context.Context, extID string) (map[string]interface{}, error) {
	// Return current user settings for the extension's browser action
	// Currently returns default values - could be extended to persist user preferences
	return map[string]interface{}{
		"isOnToolbar": true, // Extension is pinned to toolbar
	}, nil
}
