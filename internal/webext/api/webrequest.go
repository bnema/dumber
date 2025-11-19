package api

import (
	"fmt"
	"log"
	"sync"
)

// ResourceType represents the type of resource being requested
type ResourceType string

const (
	ResourceTypeMain        ResourceType = "main_frame"
	ResourceTypeSub         ResourceType = "sub_frame"
	ResourceTypeStylesheet  ResourceType = "stylesheet"
	ResourceTypeScript      ResourceType = "script"
	ResourceTypeImage       ResourceType = "image"
	ResourceTypeFont        ResourceType = "font"
	ResourceTypeObject      ResourceType = "object"
	ResourceTypeXMLHTTP     ResourceType = "xmlhttprequest"
	ResourceTypePing        ResourceType = "ping"
	ResourceTypeCSP         ResourceType = "csp_report"
	ResourceTypeMedia       ResourceType = "media"
	ResourceTypeWebSocket   ResourceType = "websocket"
	ResourceTypeWebTransport ResourceType = "webtransport"
	ResourceTypeOther       ResourceType = "other"
)

// RequestDetails contains information about a web request
type RequestDetails struct {
	RequestID     string                 `json:"requestId"`
	URL           string                 `json:"url"`
	Method        string                 `json:"method"`
	FrameID       int64                  `json:"frameId"`
	ParentFrameID int64                  `json:"parentFrameId"`
	TabID         int64                  `json:"tabId"`
	Type          ResourceType           `json:"type"`
	TimeStamp     float64                `json:"timeStamp"`
	Initiator     string                 `json:"initiator,omitempty"`
	RequestHeaders map[string]string     `json:"requestHeaders,omitempty"`
}

// ResponseDetails contains information about a web response
type ResponseDetails struct {
	RequestID       string            `json:"requestId"`
	URL             string            `json:"url"`
	Method          string            `json:"method"`
	FrameID         int64             `json:"frameId"`
	ParentFrameID   int64             `json:"parentFrameId"`
	TabID           int64             `json:"tabId"`
	Type            ResourceType      `json:"type"`
	TimeStamp       float64           `json:"timeStamp"`
	StatusCode      int               `json:"statusCode"`
	StatusLine      string            `json:"statusLine"`
	ResponseHeaders map[string]string `json:"responseHeaders,omitempty"`
}

// BlockingResponse represents an extension's decision about a request
type BlockingResponse struct {
	Cancel           bool              `json:"cancel"`
	RedirectURL      string            `json:"redirectUrl,omitempty"`
	RequestHeaders   map[string]string `json:"requestHeaders,omitempty"`
	ResponseHeaders  map[string]string `json:"responseHeaders,omitempty"`
}

// OnBeforeRequestListener is called before a request is made
type OnBeforeRequestListener func(details RequestDetails) *BlockingResponse

// OnBeforeSendHeadersListener is called before request headers are sent
type OnBeforeSendHeadersListener func(details RequestDetails) *BlockingResponse

// OnHeadersReceivedListener is called when response headers are received
type OnHeadersReceivedListener func(details ResponseDetails) *BlockingResponse

// OnCompletedListener is called when a request completes successfully
type OnCompletedListener func(details ResponseDetails)

// OnErrorOccurredListener is called when a request fails
type OnErrorOccurredListener func(details RequestDetails, error string)

// WebRequestAPI manages webRequest event listeners
type WebRequestAPI struct {
	mu sync.RWMutex

	// Listener registries per extension
	onBeforeRequestListeners      map[string][]OnBeforeRequestListener
	onBeforeSendHeadersListeners  map[string][]OnBeforeSendHeadersListener
	onHeadersReceivedListeners    map[string][]OnHeadersReceivedListener
	onCompletedListeners          map[string][]OnCompletedListener
	onErrorOccurredListeners      map[string][]OnErrorOccurredListener

	// Filter configurations per extension
	filters map[string]*RequestFilter
}

// RequestFilter specifies which requests to monitor
type RequestFilter struct {
	URLs      []string       `json:"urls"`
	Types     []ResourceType `json:"types,omitempty"`
	TabID     int64          `json:"tabId,omitempty"`
	WindowID  int64          `json:"windowId,omitempty"`
}

// NewWebRequestAPI creates a new WebRequest API instance
func NewWebRequestAPI() *WebRequestAPI {
	return &WebRequestAPI{
		onBeforeRequestListeners:      make(map[string][]OnBeforeRequestListener),
		onBeforeSendHeadersListeners:  make(map[string][]OnBeforeSendHeadersListener),
		onHeadersReceivedListeners:    make(map[string][]OnHeadersReceivedListener),
		onCompletedListeners:          make(map[string][]OnCompletedListener),
		onErrorOccurredListeners:      make(map[string][]OnErrorOccurredListener),
		filters:                       make(map[string]*RequestFilter),
	}
}

// OnBeforeRequest registers a listener for before request events
func (w *WebRequestAPI) OnBeforeRequest(extensionID string, listener OnBeforeRequestListener, filter *RequestFilter) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if filter == nil {
		return fmt.Errorf("filter is required")
	}

	w.onBeforeRequestListeners[extensionID] = append(w.onBeforeRequestListeners[extensionID], listener)
	w.filters[extensionID] = filter

	log.Printf("[webRequest] Extension %s registered onBeforeRequest listener", extensionID)
	return nil
}

// OnBeforeSendHeaders registers a listener for before send headers events
func (w *WebRequestAPI) OnBeforeSendHeaders(extensionID string, listener OnBeforeSendHeadersListener, filter *RequestFilter) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if filter == nil {
		return fmt.Errorf("filter is required")
	}

	w.onBeforeSendHeadersListeners[extensionID] = append(w.onBeforeSendHeadersListeners[extensionID], listener)
	w.filters[extensionID] = filter

	log.Printf("[webRequest] Extension %s registered onBeforeSendHeaders listener", extensionID)
	return nil
}

// OnHeadersReceived registers a listener for headers received events
func (w *WebRequestAPI) OnHeadersReceived(extensionID string, listener OnHeadersReceivedListener, filter *RequestFilter) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if filter == nil {
		return fmt.Errorf("filter is required")
	}

	w.onHeadersReceivedListeners[extensionID] = append(w.onHeadersReceivedListeners[extensionID], listener)
	w.filters[extensionID] = filter

	log.Printf("[webRequest] Extension %s registered onHeadersReceived listener", extensionID)
	return nil
}

// OnCompleted registers a listener for request completed events
func (w *WebRequestAPI) OnCompleted(extensionID string, listener OnCompletedListener, filter *RequestFilter) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if filter == nil {
		return fmt.Errorf("filter is required")
	}

	w.onCompletedListeners[extensionID] = append(w.onCompletedListeners[extensionID], listener)
	w.filters[extensionID] = filter

	log.Printf("[webRequest] Extension %s registered onCompleted listener", extensionID)
	return nil
}

// OnErrorOccurred registers a listener for request error events
func (w *WebRequestAPI) OnErrorOccurred(extensionID string, listener OnErrorOccurredListener, filter *RequestFilter) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if filter == nil {
		return fmt.Errorf("filter is required")
	}

	w.onErrorOccurredListeners[extensionID] = append(w.onErrorOccurredListeners[extensionID], listener)
	w.filters[extensionID] = filter

	log.Printf("[webRequest] Extension %s registered onErrorOccurred listener", extensionID)
	return nil
}

// HandleBeforeRequest processes a request through all registered onBeforeRequest listeners
// Returns the aggregated blocking response (cancel takes priority, then redirect)
func (w *WebRequestAPI) HandleBeforeRequest(details RequestDetails) *BlockingResponse {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var finalResponse *BlockingResponse

	for extensionID, listeners := range w.onBeforeRequestListeners {
		filter := w.filters[extensionID]
		if !w.matchesFilter(details.URL, details.Type, details.TabID, filter) {
			continue
		}

		for _, listener := range listeners {
			response := listener(details)
			if response != nil {
				// Cancel takes highest priority
				if response.Cancel {
					log.Printf("[webRequest] Extension %s blocked request to %s", extensionID, details.URL)
					return &BlockingResponse{Cancel: true}
				}

				// Redirect is second priority
				if response.RedirectURL != "" {
					log.Printf("[webRequest] Extension %s redirecting %s to %s", extensionID, details.URL, response.RedirectURL)
					finalResponse = response
				}

				// Modify headers if no higher priority action
				if finalResponse == nil && response.RequestHeaders != nil {
					log.Printf("[webRequest] Extension %s modifying request headers for %s", extensionID, details.URL)
					finalResponse = response
				}
			}
		}
	}

	return finalResponse
}

// HandleBeforeSendHeaders processes request headers through all registered listeners
func (w *WebRequestAPI) HandleBeforeSendHeaders(details RequestDetails) *BlockingResponse {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var finalResponse *BlockingResponse

	for extensionID, listeners := range w.onBeforeSendHeadersListeners {
		filter := w.filters[extensionID]
		if !w.matchesFilter(details.URL, details.Type, details.TabID, filter) {
			continue
		}

		for _, listener := range listeners {
			response := listener(details)
			if response != nil {
				if response.Cancel {
					return &BlockingResponse{Cancel: true}
				}
				if response.RequestHeaders != nil {
					finalResponse = response
				}
			}
		}
	}

	return finalResponse
}

// HandleHeadersReceived processes response headers through all registered listeners
func (w *WebRequestAPI) HandleHeadersReceived(details ResponseDetails) *BlockingResponse {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var finalResponse *BlockingResponse

	for extensionID, listeners := range w.onHeadersReceivedListeners {
		filter := w.filters[extensionID]
		if !w.matchesFilter(details.URL, details.Type, details.TabID, filter) {
			continue
		}

		for _, listener := range listeners {
			response := listener(details)
			if response != nil {
				if response.Cancel {
					return &BlockingResponse{Cancel: true}
				}
				if response.RedirectURL != "" || response.ResponseHeaders != nil {
					finalResponse = response
				}
			}
		}
	}

	return finalResponse
}

// HandleCompleted notifies all registered listeners that a request completed
func (w *WebRequestAPI) HandleCompleted(details ResponseDetails) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for extensionID, listeners := range w.onCompletedListeners {
		filter := w.filters[extensionID]
		if !w.matchesFilter(details.URL, details.Type, details.TabID, filter) {
			continue
		}

		for _, listener := range listeners {
			listener(details)
		}
	}
}

// HandleErrorOccurred notifies all registered listeners that a request failed
func (w *WebRequestAPI) HandleErrorOccurred(details RequestDetails, errorMsg string) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for extensionID, listeners := range w.onErrorOccurredListeners {
		filter := w.filters[extensionID]
		if !w.matchesFilter(details.URL, details.Type, details.TabID, filter) {
			continue
		}

		for _, listener := range listeners {
			listener(details, errorMsg)
		}
	}
}

// matchesFilter checks if a request matches the extension's filter
func (w *WebRequestAPI) matchesFilter(url string, resourceType ResourceType, tabID int64, filter *RequestFilter) bool {
	if filter == nil {
		return false
	}

	// Check tab ID filter
	if filter.TabID != 0 && filter.TabID != tabID {
		return false
	}

	// Check resource type filter
	if len(filter.Types) > 0 {
		matchedType := false
		for _, t := range filter.Types {
			if t == resourceType {
				matchedType = true
				break
			}
		}
		if !matchedType {
			return false
		}
	}

	// Check URL patterns
	if len(filter.URLs) > 0 {
		matchedURL := false
		for _, pattern := range filter.URLs {
			if matchesURLPattern(url, pattern) {
				matchedURL = true
				break
			}
		}
		if !matchedURL {
			return false
		}
	}

	return true
}

// matchesURLPattern checks if a URL matches a pattern
// Simplified version - for production should use internal/webext/matcher.go
func matchesURLPattern(url, pattern string) bool {
	// Special case: <all_urls> matches everything
	if pattern == "<all_urls>" {
		return true
	}

	// TODO: Implement proper WebExtension URL pattern matching
	// For now, use simple contains check (very permissive)
	// In production, should use the MatchPattern logic from internal/webext/matcher.go
	return true
}

// RemoveListener removes all listeners for an extension
func (w *WebRequestAPI) RemoveListener(extensionID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.onBeforeRequestListeners, extensionID)
	delete(w.onBeforeSendHeadersListeners, extensionID)
	delete(w.onHeadersReceivedListeners, extensionID)
	delete(w.onCompletedListeners, extensionID)
	delete(w.onErrorOccurredListeners, extensionID)
	delete(w.filters, extensionID)

	log.Printf("[webRequest] Removed all listeners for extension %s", extensionID)
}
