package api

import (
	"fmt"
	"log"
	"sync"
)

// TabsAPI implements chrome.tabs WebExtension API
type TabsAPI struct {
	mu        sync.RWMutex
	listeners struct {
		onUpdated []OnTabUpdatedListener
		onCreated []OnTabCreatedListener
		onRemoved []OnTabRemovedListener
		onActivated []OnTabActivatedListener
	}

	// Bridge to browser's TabManager
	// These will be set by the browser when initializing the extension
	queryFunc      func(QueryInfo) ([]Tab, error)
	getFunc        func(tabID int) (*Tab, error)
	updateFunc     func(tabID int, updateProperties UpdateProperties) (*Tab, error)
	reloadFunc     func(tabID int, reloadProperties *ReloadProperties) error
	sendMessageFunc func(tabID int, message interface{}, callback func(response interface{})) error
	createFunc     func(createProperties CreateProperties) (*Tab, error)
	removeFunc     func(tabIDs []int) error
}

// QueryInfo filters for querying tabs
type QueryInfo struct {
	Active        *bool   `json:"active,omitempty"`
	CurrentWindow *bool   `json:"currentWindow,omitempty"`
	URL           *string `json:"url,omitempty"`
	Title         *string `json:"title,omitempty"`
	Index         *int    `json:"index,omitempty"`
}

// UpdateProperties for updating a tab
type UpdateProperties struct {
	URL    *string `json:"url,omitempty"`
	Active *bool   `json:"active,omitempty"`
	Muted  *bool   `json:"muted,omitempty"`
}

// ReloadProperties for reloading a tab
type ReloadProperties struct {
	BypassCache bool `json:"bypassCache,omitempty"`
}

// CreateProperties for creating a new tab
type CreateProperties struct {
	URL    string `json:"url,omitempty"`
	Active bool   `json:"active,omitempty"`
	Index  int    `json:"index,omitempty"`
}

// OnTabUpdatedListener is called when a tab is updated
type OnTabUpdatedListener func(tabID int, changeInfo TabChangeInfo, tab Tab)

// OnTabCreatedListener is called when a tab is created
type OnTabCreatedListener func(tab Tab)

// OnTabRemovedListener is called when a tab is removed
type OnTabRemovedListener func(tabID int, removeInfo TabRemoveInfo)

// OnTabActivatedListener is called when a tab becomes active
type OnTabActivatedListener func(activeInfo TabActiveInfo)

// TabChangeInfo contains information about what changed in a tab
type TabChangeInfo struct {
	Status *string `json:"status,omitempty"` // "loading" or "complete"
	URL    *string `json:"url,omitempty"`
	Title  *string `json:"title,omitempty"`
}

// TabRemoveInfo contains information about a removed tab
type TabRemoveInfo struct {
	WindowID   int  `json:"windowId"`
	IsWindowClosing bool `json:"isWindowClosing"`
}

// TabActiveInfo contains information about an activated tab
type TabActiveInfo struct {
	TabID    int `json:"tabId"`
	WindowID int `json:"windowId"`
}

// NewTabsAPI creates a new tabs API instance
func NewTabsAPI() *TabsAPI {
	return &TabsAPI{
		listeners: struct {
			onUpdated   []OnTabUpdatedListener
			onCreated   []OnTabCreatedListener
			onRemoved   []OnTabRemovedListener
			onActivated []OnTabActivatedListener
		}{
			onUpdated:   make([]OnTabUpdatedListener, 0),
			onCreated:   make([]OnTabCreatedListener, 0),
			onRemoved:   make([]OnTabRemovedListener, 0),
			onActivated: make([]OnTabActivatedListener, 0),
		},
	}
}

// SetBridge sets the bridge functions to connect to the browser's TabManager
func (t *TabsAPI) SetBridge(
	queryFunc func(QueryInfo) ([]Tab, error),
	getFunc func(tabID int) (*Tab, error),
	updateFunc func(tabID int, updateProperties UpdateProperties) (*Tab, error),
	reloadFunc func(tabID int, reloadProperties *ReloadProperties) error,
	sendMessageFunc func(tabID int, message interface{}, callback func(response interface{})) error,
	createFunc func(createProperties CreateProperties) (*Tab, error),
	removeFunc func(tabIDs []int) error,
) {
	t.queryFunc = queryFunc
	t.getFunc = getFunc
	t.updateFunc = updateFunc
	t.reloadFunc = reloadFunc
	t.sendMessageFunc = sendMessageFunc
	t.createFunc = createFunc
	t.removeFunc = removeFunc
}

// Query gets all tabs that match the specified query info
func (t *TabsAPI) Query(queryInfo QueryInfo) ([]Tab, error) {
	if t.queryFunc == nil {
		return nil, fmt.Errorf("tabs API not bridged to TabManager")
	}

	tabs, err := t.queryFunc(queryInfo)
	if err != nil {
		return nil, err
	}

	log.Printf("[tabs] Query returned %d tabs", len(tabs))
	return tabs, nil
}

// Get retrieves details about a specific tab
func (t *TabsAPI) Get(tabID int) (*Tab, error) {
	if t.getFunc == nil {
		return nil, fmt.Errorf("tabs API not bridged to TabManager")
	}

	tab, err := t.getFunc(tabID)
	if err != nil {
		return nil, err
	}

	log.Printf("[tabs] Get tab %d: %s", tabID, tab.Title)
	return tab, nil
}

// Update modifies a tab's properties
func (t *TabsAPI) Update(tabID int, updateProperties UpdateProperties) (*Tab, error) {
	if t.updateFunc == nil {
		return nil, fmt.Errorf("tabs API not bridged to TabManager")
	}

	tab, err := t.updateFunc(tabID, updateProperties)
	if err != nil {
		return nil, err
	}

	log.Printf("[tabs] Updated tab %d", tabID)
	return tab, nil
}

// Reload reloads a tab
func (t *TabsAPI) Reload(tabID int, reloadProperties *ReloadProperties) error {
	if t.reloadFunc == nil {
		return fmt.Errorf("tabs API not bridged to TabManager")
	}

	err := t.reloadFunc(tabID, reloadProperties)
	if err != nil {
		return err
	}

	log.Printf("[tabs] Reloaded tab %d", tabID)
	return nil
}

// SendMessage sends a message to content scripts in a tab
func (t *TabsAPI) SendMessage(tabID int, message interface{}, callback func(response interface{})) error {
	if t.sendMessageFunc == nil {
		return fmt.Errorf("tabs API not bridged to TabManager")
	}

	err := t.sendMessageFunc(tabID, message, callback)
	if err != nil {
		return err
	}

	log.Printf("[tabs] Sent message to tab %d", tabID)
	return nil
}

// Create creates a new tab
func (t *TabsAPI) Create(createProperties CreateProperties) (*Tab, error) {
	if t.createFunc == nil {
		return nil, fmt.Errorf("tabs API not bridged to TabManager")
	}

	tab, err := t.createFunc(createProperties)
	if err != nil {
		return nil, err
	}

	log.Printf("[tabs] Created new tab %d: %s", tab.ID, tab.URL)

	// Notify listeners
	t.notifyTabCreated(*tab)

	return tab, nil
}

// Remove closes one or more tabs
func (t *TabsAPI) Remove(tabIDs []int) error {
	if t.removeFunc == nil {
		return fmt.Errorf("tabs API not bridged to TabManager")
	}

	err := t.removeFunc(tabIDs)
	if err != nil {
		return err
	}

	log.Printf("[tabs] Removed %d tab(s)", len(tabIDs))

	// Notify listeners
	for _, tabID := range tabIDs {
		t.notifyTabRemoved(tabID, TabRemoveInfo{
			WindowID:        1, // TODO: Get actual window ID
			IsWindowClosing: false,
		})
	}

	return nil
}

// OnUpdated registers a listener for tab update events
func (t *TabsAPI) OnUpdated(listener OnTabUpdatedListener) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.listeners.onUpdated = append(t.listeners.onUpdated, listener)
	log.Printf("[tabs] Registered onUpdated listener (total: %d)", len(t.listeners.onUpdated))
}

// OnCreated registers a listener for tab creation events
func (t *TabsAPI) OnCreated(listener OnTabCreatedListener) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.listeners.onCreated = append(t.listeners.onCreated, listener)
	log.Printf("[tabs] Registered onCreated listener (total: %d)", len(t.listeners.onCreated))
}

// OnRemoved registers a listener for tab removal events
func (t *TabsAPI) OnRemoved(listener OnTabRemovedListener) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.listeners.onRemoved = append(t.listeners.onRemoved, listener)
	log.Printf("[tabs] Registered onRemoved listener (total: %d)", len(t.listeners.onRemoved))
}

// OnActivated registers a listener for tab activation events
func (t *TabsAPI) OnActivated(listener OnTabActivatedListener) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.listeners.onActivated = append(t.listeners.onActivated, listener)
	log.Printf("[tabs] Registered onActivated listener (total: %d)", len(t.listeners.onActivated))
}

// NotifyTabUpdated is called by the browser when a tab is updated
func (t *TabsAPI) NotifyTabUpdated(tabID int, changeInfo TabChangeInfo, tab Tab) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, listener := range t.listeners.onUpdated {
		go listener(tabID, changeInfo, tab)
	}
}

// notifyTabCreated is called when a tab is created
func (t *TabsAPI) notifyTabCreated(tab Tab) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, listener := range t.listeners.onCreated {
		go listener(tab)
	}
}

// notifyTabRemoved is called when a tab is removed
func (t *TabsAPI) notifyTabRemoved(tabID int, removeInfo TabRemoveInfo) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, listener := range t.listeners.onRemoved {
		go listener(tabID, removeInfo)
	}
}

// NotifyTabActivated is called by the browser when a tab is activated
func (t *TabsAPI) NotifyTabActivated(tabID int, windowID int) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	activeInfo := TabActiveInfo{
		TabID:    tabID,
		WindowID: windowID,
	}

	for _, listener := range t.listeners.onActivated {
		go listener(activeInfo)
	}
}
