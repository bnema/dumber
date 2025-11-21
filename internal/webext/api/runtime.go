package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
)

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

// --- Dispatcher-compatible API (works across all extensions) ---

// RuntimeAPIDispatcher provides runtime API methods for the dispatcher
// This works with any extension ID passed as a parameter
type RuntimeAPIDispatcher struct {
	manager interface{} // Extension manager (to get extension metadata)
	mu      sync.RWMutex
	ports   map[string]string
}

// NewRuntimeAPIDispatcher creates a runtime API for the dispatcher
func NewRuntimeAPIDispatcher(manager interface{}) *RuntimeAPIDispatcher {
	return &RuntimeAPIDispatcher{
		manager: manager,
		ports:   make(map[string]string),
	}
}

// SendMessage sends a message to other parts of the extension
// For now, this is a stub that will be enhanced when we implement message routing
func (r *RuntimeAPIDispatcher) SendMessage(ctx context.Context, extID string, message interface{}) (interface{}, error) {
	log.Printf("[runtime] SendMessage from extension %s: %+v", extID, message)

	// TODO: Implement message routing to background page / content scripts
	// For now, return undefined to indicate no response
	return nil, nil
}

// Connect sets up a lightweight placeholder port. Full port routing is handled in browser.go.
func (r *RuntimeAPIDispatcher) Connect(ctx context.Context, extID string, connectInfo *ConnectInfo) (interface{}, error) {
	portID := fmt.Sprintf("port-%d", time.Now().UnixNano())

	r.mu.Lock()
	if r.ports == nil {
		r.ports = make(map[string]string)
	}
	r.ports[portID] = extID
	r.mu.Unlock()

	name := ""
	if connectInfo != nil {
		name = connectInfo.Name
	}
	log.Printf("[runtime] runtime.connect placeholder ext=%s port=%s name=%s", extID, portID, name)

	// UI process will handle actual port wiring when it receives the follow-up port events.
	return map[string]string{"portId": portID}, nil
}

// PortPostMessage is currently a no-op placeholder to keep promise chains alive.
func (r *RuntimeAPIDispatcher) PortPostMessage(ctx context.Context, extID, portID string, message interface{}) (interface{}, error) {
	log.Printf("[runtime] port.postMessage placeholder ext=%s port=%s msg=%v", extID, portID, message)
	return nil, nil
}

// PortDisconnect removes the placeholder port entry.
func (r *RuntimeAPIDispatcher) PortDisconnect(ctx context.Context, extID, portID string) (interface{}, error) {
	r.mu.Lock()
	delete(r.ports, portID)
	r.mu.Unlock()

	log.Printf("[runtime] port.disconnect placeholder ext=%s port=%s", extID, portID)
	return nil, nil
}
