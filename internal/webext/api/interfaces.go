package api

// WebRequestHandler defines the interface for handling web request events
type WebRequestHandler interface {
	// OnBeforeRequest registers a listener for before request events
	OnBeforeRequest(extensionID string, listener OnBeforeRequestListener, filter *RequestFilter) error

	// OnBeforeSendHeaders registers a listener for before send headers events
	OnBeforeSendHeaders(extensionID string, listener OnBeforeSendHeadersListener, filter *RequestFilter) error

	// OnHeadersReceived registers a listener for headers received events
	OnHeadersReceived(extensionID string, listener OnHeadersReceivedListener, filter *RequestFilter) error

	// OnCompleted registers a listener for request completed events
	OnCompleted(extensionID string, listener OnCompletedListener, filter *RequestFilter) error

	// OnErrorOccurred registers a listener for request error events
	OnErrorOccurred(extensionID string, listener OnErrorOccurredListener, filter *RequestFilter) error

	// HandleBeforeRequest processes a request through all registered onBeforeRequest listeners
	HandleBeforeRequest(details RequestDetails) *BlockingResponse

	// HandleBeforeSendHeaders processes request headers through all registered listeners
	HandleBeforeSendHeaders(details RequestDetails) *BlockingResponse

	// HandleHeadersReceived processes response headers through all registered listeners
	HandleHeadersReceived(details ResponseDetails) *BlockingResponse

	// HandleCompleted notifies all registered listeners that a request completed
	HandleCompleted(details ResponseDetails)

	// HandleErrorOccurred notifies all registered listeners that a request failed
	HandleErrorOccurred(details RequestDetails, errorMsg string)

	// RemoveListener removes all listeners for an extension
	RemoveListener(extensionID string)
}

// Ensure WebRequestAPI implements WebRequestHandler interface
var _ WebRequestHandler = (*WebRequestAPI)(nil)
