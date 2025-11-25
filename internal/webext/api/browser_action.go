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
		return &BadgeState{}
	}

	// Return a copy to prevent external mutations
	return &BadgeState{
		Text:            badge.Text,
		BackgroundColor: badge.BackgroundColor,
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
