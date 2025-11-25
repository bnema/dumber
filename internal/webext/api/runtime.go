package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

type runtimeManager interface {
	DispatchRuntimeMessage(extID string, sender MessageSender, message interface{}) (interface{}, error)
	ConnectBackgroundPort(extID string, desc PortDescriptor) error
	DeliverPortMessage(extID, portID string, message interface{}) error
	DisconnectPort(extID, portID string)
}

// RuntimeAPI implements chrome.runtime WebExtension API
type RuntimeAPI struct {
	extensionID string
	mu          sync.RWMutex
	listeners   []OnMessageListener
}

// OnMessageListener is a callback for chrome.runtime.onMessage
// Returns true if the message was handled asynchronously (sendResponse will be called later)
type OnMessageListener func(message interface{}, sender MessageSender, sendResponse func(interface{})) bool

// MessageSender represents the sender of a message
type MessageSender struct {
	Tab          *Tab   `json:"tab,omitempty"`
	FrameID      int    `json:"frameId,omitempty"`
	ID           string `json:"id,omitempty"`  // Extension ID
	URL          string `json:"url,omitempty"` // URL of the frame
	TLSChannelID string `json:"tlsChannelId,omitempty"`
}

// Tab represents a browser tab (used by both runtime and tabs APIs)
type Tab struct {
	ID       int    `json:"id"`
	Index    int    `json:"index"`
	WindowID int    `json:"windowId,omitempty"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Active   bool   `json:"active"`
	Favicon  string `json:"favIconUrl,omitempty"`
}

// PopupInfo contains information about an extension popup for sender context
type PopupInfo struct {
	ExtensionID string
	URL         string
}

// NewRuntimeAPI creates a new RuntimeAPI instance for an extension
func NewRuntimeAPI(extensionID string) *RuntimeAPI {
	return &RuntimeAPI{
		extensionID: extensionID,
		listeners:   make([]OnMessageListener, 0),
	}
}

// SendMessage sends a message to other parts of the extension (background, content scripts)
// This is called from content scripts or background scripts
func (r *RuntimeAPI) SendMessage(message interface{}, callback func(response interface{})) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	log.Printf("[runtime] SendMessage from extension %s: %+v", r.extensionID, message)

	// Dispatch to all registered listeners
	handled := false
	for _, listener := range r.listeners {
		// Create a sender context
		sender := MessageSender{
			ID: r.extensionID,
		}

		// Call the listener
		async := listener(message, sender, callback)
		if async {
			handled = true
			break // First async handler wins
		}
	}

	if !handled && callback != nil {
		// No listeners handled it, return undefined
		callback(nil)
	}

	return nil
}

// OnMessage registers a listener for chrome.runtime.onMessage events
func (r *RuntimeAPI) OnMessage(listener OnMessageListener) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.listeners = append(r.listeners, listener)
	log.Printf("[runtime] Registered onMessage listener for extension %s (total: %d)", r.extensionID, len(r.listeners))
}

// GetManifest returns the extension manifest
// This will be implemented to return the parsed manifest for the extension
func (r *RuntimeAPI) GetManifest() (map[string]interface{}, error) {
	// TODO: Look up manifest from extension manager
	return nil, fmt.Errorf("not implemented yet")
}

// GetURL converts a relative path to a fully-qualified extension URL
// For example: chrome.runtime.getURL("icon.png") -> "chrome-extension://<id>/icon.png"
func (r *RuntimeAPI) GetURL(path string) string {
	// WebExtensions use chrome-extension:// scheme
	// We'll use dumb-extension:// for Dumber
	return fmt.Sprintf("dumb-extension://%s/%s", r.extensionID, path)
}

// GetBackgroundPage returns the JavaScript window object for the background page.
// Backgrounds now run inside WebViews, so there is no embeddable VM to return here.
func (r *RuntimeAPI) GetBackgroundPage(callback func(window interface{})) {
	if callback != nil {
		callback(nil)
	}
}

// RuntimeMessage represents a message sent via chrome.runtime
type RuntimeMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Connect creates a long-lived connection for message passing
// This is more complex than SendMessage and typically used for content script <-> background communication
func (r *RuntimeAPI) Connect(connectInfo *ConnectInfo) *Port {
	// TODO: Implement Port-based messaging for long-lived connections
	log.Printf("[runtime] Connect called (not yet implemented)")
	return nil
}

// ConnectInfo contains information about a connection
type ConnectInfo struct {
	Name                string `json:"name,omitempty"`
	IncludeTLSChannelID bool   `json:"includeTlsChannelId,omitempty"`
}

// Port represents a long-lived connection for message passing
type Port struct {
	Name         string
	OnMessage    func(message interface{})
	OnDisconnect func()
	PostMessage  func(message interface{})
	Disconnect   func()
}

// PortConnection tracks a port connection between two extension contexts
type PortConnection struct {
	PortID      string        // Unique port identifier
	Name        string        // Port name from connectInfo
	ExtensionID string        // Extension that owns this port
	SourceView  uint64        // WebView ID that created the port (popup/content script)
	TargetView  uint64        // WebView ID that should receive messages (usually background)
	Sender      MessageSender // Information about the source
	Created     time.Time     // When the port was created
}

// --- Dispatcher-compatible API (works across all extensions) ---

// RuntimeAPIDispatcher provides runtime API methods for the dispatcher
// This works with any extension ID passed as a parameter
type RuntimeAPIDispatcher struct {
	manager interface{} // Extension manager (to get extension metadata)
	mu      sync.RWMutex
	ports   map[string]*PortConnection // portID -> connection info

	emitPortEvent func(viewID uint64, event map[string]interface{}) error
}

// NewRuntimeAPIDispatcher creates a runtime API for the dispatcher
func NewRuntimeAPIDispatcher(manager interface{}) *RuntimeAPIDispatcher {
	return &RuntimeAPIDispatcher{
		manager: manager,
		ports:   make(map[string]*PortConnection),
	}
}

// SetPortEventEmitter sets the callback used to deliver port events back to a WebView.
func (r *RuntimeAPIDispatcher) SetPortEventEmitter(fn func(viewID uint64, event map[string]interface{}) error) {
	r.emitPortEvent = fn
}

// SendMessage sends a message to other parts of the extension
// For now, this is a stub that will be enhanced when we implement message routing
func (r *RuntimeAPIDispatcher) SendMessage(ctx context.Context, extID string, message interface{}) (interface{}, error) {
	log.Printf("[runtime] SendMessage from extension %s: %+v", extID, message)

	mgr, ok := r.manager.(runtimeManager)
	if !ok {
		return nil, fmt.Errorf("runtime manager does not support messaging")
	}

	sender := MessageSender{ID: extID}
	if srcURL, ok := ctx.Value("sourceURL").(string); ok {
		sender.URL = srcURL
	}

	return mgr.DispatchRuntimeMessage(extID, sender, message)
}

// Connect sets up a port connection between extension contexts
func (r *RuntimeAPIDispatcher) Connect(ctx context.Context, extID string, connectInfo *ConnectInfo) (interface{}, error) {
	mgr, ok := r.manager.(runtimeManager)
	if !ok {
		return nil, fmt.Errorf("runtime manager does not support ports")
	}

	portID := fmt.Sprintf("port-%d", time.Now().UnixNano())

	name := ""
	if connectInfo != nil {
		name = connectInfo.Name
	}

	sourceViewID, _ := ctx.Value("sourceViewID").(uint64)

	sender := MessageSender{
		ID:      extID,
		FrameID: 0, // Main frame by default
	}

	if paneInfo, ok := ctx.Value("sourcePaneInfo").(*PaneInfo); ok && paneInfo != nil {
		sender.URL = paneInfo.URL
		sender.Tab = &Tab{
			ID:       int(paneInfo.ID),
			Index:    paneInfo.Index,
			WindowID: int(paneInfo.WindowID),
			URL:      paneInfo.URL,
			Title:    paneInfo.Title,
			Active:   paneInfo.Active,
		}
	} else if popupInfo, ok := ctx.Value("sourcePopupInfo").(*PopupInfo); ok && popupInfo != nil {
		// Fallback for extension popups - use popup URL as sender.url
		sender.URL = popupInfo.URL
	}

	cb := PortCallbacks{
		OnMessage: func(msg interface{}) {
			if r.emitPortEvent == nil {
				return
			}
			event := map[string]interface{}{
				"type":    "port-message",
				"portId":  portID,
				"message": msg,
			}
			if err := r.emitPortEvent(sourceViewID, event); err != nil {
				log.Printf("[runtime] emitPortEvent failed: %v", err)
			}
		},
		OnDisconnect: func() {
			if r.emitPortEvent == nil {
				return
			}
			event := map[string]interface{}{
				"type":   "port-disconnect",
				"portId": portID,
			}
			if err := r.emitPortEvent(sourceViewID, event); err != nil {
				log.Printf("[runtime] emitPortEvent(disconnect) failed: %v", err)
			}
		},
	}

	desc := PortDescriptor{
		ID:        portID,
		Name:      name,
		Sender:    sender,
		Callbacks: cb,
	}

	r.mu.Lock()
	if r.ports == nil {
		r.ports = make(map[string]*PortConnection)
	}
	r.ports[portID] = &PortConnection{
		PortID:      portID,
		Name:        name,
		ExtensionID: extID,
		SourceView:  sourceViewID,
		Sender:      sender,
		Created:     time.Now(),
	}
	r.mu.Unlock()

	if err := mgr.ConnectBackgroundPort(extID, desc); err != nil {
		return nil, fmt.Errorf("connect failed: %w", err)
	}

	return map[string]string{"portId": portID}, nil
}

// PortPostMessage forwards a message from one port endpoint to another
func (r *RuntimeAPIDispatcher) PortPostMessage(ctx context.Context, extID, portID string, message interface{}) (interface{}, error) {
	r.mu.RLock()
	conn, exists := r.ports[portID]
	r.mu.RUnlock()

	if !exists || conn.ExtensionID != extID {
		return nil, fmt.Errorf("port not found")
	}

	mgr, ok := r.manager.(runtimeManager)
	if !ok {
		return nil, fmt.Errorf("runtime manager does not support ports")
	}

	if err := mgr.DeliverPortMessage(extID, portID, message); err != nil {
		return nil, err
	}
	return nil, nil
}

// PortDisconnect removes the port connection and notifies both endpoints
func (r *RuntimeAPIDispatcher) PortDisconnect(ctx context.Context, extID, portID string) (interface{}, error) {
	r.mu.Lock()
	conn, exists := r.ports[portID]
	delete(r.ports, portID)
	r.mu.Unlock()

	if !exists || conn.ExtensionID != extID {
		return nil, fmt.Errorf("port not found")
	}

	if mgr, ok := r.manager.(runtimeManager); ok {
		mgr.DisconnectPort(extID, portID)
	}

	return nil, nil
}

// GetPortConnection returns the connection info for a port.
func (r *RuntimeAPIDispatcher) GetPortConnection(portID string) (*PortConnection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conn, exists := r.ports[portID]
	return conn, exists
}

// GetPortsByView returns all ports associated with a WebView (for cleanup on destruction)
func (r *RuntimeAPIDispatcher) GetPortsByView(viewID uint64) []*PortConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var ports []*PortConnection
	for _, conn := range r.ports {
		if conn.SourceView == viewID || conn.TargetView == viewID {
			ports = append(ports, conn)
		}
	}
	return ports
}

// RemovePort removes a port from the registry (for cleanup)
func (r *RuntimeAPIDispatcher) RemovePort(portID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.ports, portID)
}

// PlatformInfo represents platform information
type PlatformInfo struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	NaclArch string `json:"nacl_arch"`
}

// GetPlatformInfo returns information about the current platform
// API: browser.runtime.getPlatformInfo()
func (r *RuntimeAPIDispatcher) GetPlatformInfo(ctx context.Context) (*PlatformInfo, error) {
	return &PlatformInfo{
		OS:       getPlatformOS(),
		Arch:     getPlatformArch(),
		NaclArch: getPlatformArch(), // Same as arch for compatibility
	}, nil
}

// getPlatformOS returns the operating system name
func getPlatformOS() string {
	// For now, we only support Linux (like Epiphany)
	// Future: could detect via runtime.GOOS
	return "linux"
}

// getPlatformArch returns the CPU architecture
func getPlatformArch() string {
	// Detect architecture at compile time
	// https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/API/runtime/PlatformArch
	return getPlatformArchConst()
}

// OpenOptionsPage opens the extension's options page
// API: browser.runtime.openOptionsPage()
func (r *RuntimeAPIDispatcher) OpenOptionsPage(ctx context.Context, extID string) error {
	// Get extension to check for options page
	type extensionGetter interface {
		GetExtension(id string) (*extensionWithManifest, bool)
	}

	getter, ok := r.manager.(extensionGetter)
	if !ok {
		return fmt.Errorf("manager does not support GetExtension")
	}

	ext, exists := getter.GetExtension(extID)
	if !exists {
		return fmt.Errorf("extension not found: %s", extID)
	}

	// Check if extension has an options page
	if ext.Manifest.Options == nil || ext.Manifest.Options.Page == "" {
		return fmt.Errorf("extension does not have an options page")
	}

	// TODO: Open options page in a new window/tab
	// For now, log and return success
	log.Printf("[runtime] openOptionsPage: would open %s for extension %s", ext.Manifest.Options.Page, extID)

	// This will need integration with the browser to actually open the page
	// The page should be loaded as: dumb-extension://<extID>/<optionsPage>
	return nil
}

// extensionWithManifest is an interface to avoid circular imports
type extensionWithManifest struct {
	ID       string
	Manifest *manifestWithOptions
}

type manifestWithOptions struct {
	Name    string
	Options *optionsPage
}

type optionsPage struct {
	Page      string
	OpenInTab bool
}
