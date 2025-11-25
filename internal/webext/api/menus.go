package api

import (
	"context"
	"fmt"
	"sync"
)

// MenuItemType represents the type of menu item
type MenuItemType string

const (
	MenuTypeNormal    MenuItemType = "normal"
	MenuTypeCheckbox  MenuItemType = "checkbox"
	MenuTypeRadio     MenuItemType = "radio"
	MenuTypeSeparator MenuItemType = "separator"
)

// MenuContext represents where the menu item appears
type MenuContext string

const (
	MenuContextAll           MenuContext = "all"
	MenuContextAudio         MenuContext = "audio"
	MenuContextBrowserAction MenuContext = "browser_action"
	MenuContextEditable      MenuContext = "editable"
	MenuContextFrame         MenuContext = "frame"
	MenuContextImage         MenuContext = "image"
	MenuContextLink          MenuContext = "link"
	MenuContextPage          MenuContext = "page"
	MenuContextPassword      MenuContext = "password"
	MenuContextSelection     MenuContext = "selection"
	MenuContextTab           MenuContext = "tab"
	MenuContextVideo         MenuContext = "video"
	MenuContextPageAction    MenuContext = "page_action"
)

// MenuItem represents a context menu item
type MenuItem struct {
	ID                  string               `json:"id"`
	ParentID            *string              `json:"parentId,omitempty"`
	Title               *string              `json:"title,omitempty"`
	Type                MenuItemType         `json:"type"`
	Contexts            []MenuContext        `json:"contexts"`
	Checked             bool                 `json:"checked"`
	Enabled             bool                 `json:"enabled"`
	Visible             bool                 `json:"visible"`
	DocumentURLPatterns []string             `json:"documentUrlPatterns,omitempty"`
	TargetURLPatterns   []string             `json:"targetUrlPatterns,omitempty"`
	Children            map[string]*MenuItem `json:"-"` // Not exposed in API
}

// MenusAPIDispatcher handles menus.* API calls
type MenusAPIDispatcher struct {
	mu    sync.RWMutex
	menus map[string]map[string]*MenuItem // extensionID -> menuID -> MenuItem
}

// NewMenusAPIDispatcher creates a new menus API dispatcher
func NewMenusAPIDispatcher() *MenusAPIDispatcher {
	return &MenusAPIDispatcher{
		menus: make(map[string]map[string]*MenuItem),
	}
}

// Create creates a new context menu item
func (d *MenusAPIDispatcher) Create(ctx context.Context, extensionID string, createProperties map[string]interface{}) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Ensure menus map exists for this extension
	if d.menus[extensionID] == nil {
		d.menus[extensionID] = make(map[string]*MenuItem)
	}

	// Extract ID (required)
	id, ok := createProperties["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("menus.create(): missing or invalid 'id' field")
	}

	// Create menu item
	item := &MenuItem{
		ID:       id,
		Type:     MenuTypeNormal,
		Contexts: []MenuContext{MenuContextPage}, // Default context
		Enabled:  true,
		Visible:  true,
		Children: make(map[string]*MenuItem),
	}

	// Extract optional fields
	if parentID, ok := createProperties["parentId"].(string); ok && parentID != "" {
		item.ParentID = &parentID
	}

	if title, ok := createProperties["title"].(string); ok && title != "" {
		item.Title = &title
	}

	if typeStr, ok := createProperties["type"].(string); ok {
		switch MenuItemType(typeStr) {
		case MenuTypeNormal, MenuTypeCheckbox, MenuTypeRadio, MenuTypeSeparator:
			item.Type = MenuItemType(typeStr)
		default:
			item.Type = MenuTypeNormal
		}
	}

	if checked, ok := createProperties["checked"].(bool); ok {
		item.Checked = checked
	}

	if enabled, ok := createProperties["enabled"].(bool); ok {
		item.Enabled = enabled
	}

	if visible, ok := createProperties["visible"].(bool); ok {
		item.Visible = visible
	}

	// Extract contexts array
	if contextsRaw, ok := createProperties["contexts"].([]interface{}); ok {
		contexts := make([]MenuContext, 0, len(contextsRaw))
		for _, ctx := range contextsRaw {
			if ctxStr, ok := ctx.(string); ok {
				contexts = append(contexts, MenuContext(ctxStr))
			}
		}
		if len(contexts) > 0 {
			item.Contexts = contexts
		}
	}

	// Extract URL patterns
	if docPatterns, ok := createProperties["documentUrlPatterns"].([]interface{}); ok {
		patterns := make([]string, 0, len(docPatterns))
		for _, p := range docPatterns {
			if pStr, ok := p.(string); ok {
				patterns = append(patterns, pStr)
			}
		}
		item.DocumentURLPatterns = patterns
	}

	if targetPatterns, ok := createProperties["targetUrlPatterns"].([]interface{}); ok {
		patterns := make([]string, 0, len(targetPatterns))
		for _, p := range targetPatterns {
			if pStr, ok := p.(string); ok {
				patterns = append(patterns, pStr)
			}
		}
		item.TargetURLPatterns = patterns
	}

	// Validate: separators don't need titles, others do
	if item.Type != MenuTypeSeparator && item.Title == nil {
		return "", fmt.Errorf("menus.create(): missing 'title' field for non-separator item")
	}

	// Insert menu item into hierarchy
	if !d.insertMenuItem(extensionID, item) {
		return "", fmt.Errorf("menus.create(): parentId '%s' not found", *item.ParentID)
	}

	return id, nil
}

// insertMenuItem inserts a menu item into the menu hierarchy
func (d *MenusAPIDispatcher) insertMenuItem(extensionID string, item *MenuItem) bool {
	menus := d.menus[extensionID]

	// If no parent, insert at root level
	if item.ParentID == nil {
		menus[item.ID] = item
		return true
	}

	// Try to find parent at root level
	if parent, exists := menus[*item.ParentID]; exists {
		parent.Children[item.ID] = item
		return true
	}

	// Recursively search for parent in all menu items
	for _, rootItem := range menus {
		if d.findAndInsert(rootItem.Children, item) {
			return true
		}
	}

	return false
}

// findAndInsert recursively searches for a parent and inserts the item
func (d *MenusAPIDispatcher) findAndInsert(children map[string]*MenuItem, item *MenuItem) bool {
	if item.ParentID == nil {
		return false
	}

	// Check if parent is in this level
	if parent, exists := children[*item.ParentID]; exists {
		parent.Children[item.ID] = item
		return true
	}

	// Recursively check children
	for _, child := range children {
		if d.findAndInsert(child.Children, item) {
			return true
		}
	}

	return false
}

// Remove removes a menu item by ID
func (d *MenusAPIDispatcher) Remove(ctx context.Context, extensionID string, menuID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	menus, exists := d.menus[extensionID]
	if !exists {
		return fmt.Errorf("menus.remove(): menu item '%s' not found", menuID)
	}

	// Try to remove from root level
	if _, exists := menus[menuID]; exists {
		delete(menus, menuID)
		return nil
	}

	// Recursively search and remove
	for _, rootItem := range menus {
		if d.removeFromChildren(rootItem.Children, menuID) {
			return nil
		}
	}

	return fmt.Errorf("menus.remove(): menu item '%s' not found", menuID)
}

// removeFromChildren recursively removes a menu item from children
func (d *MenusAPIDispatcher) removeFromChildren(children map[string]*MenuItem, menuID string) bool {
	// Check if item is in this level
	if _, exists := children[menuID]; exists {
		delete(children, menuID)
		return true
	}

	// Recursively check children
	for _, child := range children {
		if d.removeFromChildren(child.Children, menuID) {
			return true
		}
	}

	return false
}

// RemoveAll removes all menu items for an extension
func (d *MenusAPIDispatcher) RemoveAll(ctx context.Context, extensionID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.menus, extensionID)
	return nil
}

// GetMenus returns all root-level menus for an extension (used internally for context menu building)
func (d *MenusAPIDispatcher) GetMenus(extensionID string) map[string]*MenuItem {
	d.mu.RLock()
	defer d.mu.RUnlock()

	menus, exists := d.menus[extensionID]
	if !exists {
		return nil
	}

	return menus
}

// CleanupExtension removes all menus for an extension (called when extension is unloaded)
func (d *MenusAPIDispatcher) CleanupExtension(extensionID string) {
	d.RemoveAll(context.Background(), extensionID)
}
