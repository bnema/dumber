package api

// ResourceType represents the type of resource being requested
type ResourceType string

const (
	ResourceTypeMain         ResourceType = "main_frame"
	ResourceTypeSub          ResourceType = "sub_frame"
	ResourceTypeStylesheet   ResourceType = "stylesheet"
	ResourceTypeScript       ResourceType = "script"
	ResourceTypeImage        ResourceType = "image"
	ResourceTypeFont         ResourceType = "font"
	ResourceTypeObject       ResourceType = "object"
	ResourceTypeXMLHTTP      ResourceType = "xmlhttprequest"
	ResourceTypePing         ResourceType = "ping"
	ResourceTypeCSP          ResourceType = "csp_report"
	ResourceTypeMedia        ResourceType = "media"
	ResourceTypeWebSocket    ResourceType = "websocket"
	ResourceTypeWebTransport ResourceType = "webtransport"
	ResourceTypeOther        ResourceType = "other"
)

// RequestDetails contains information about a web request
type RequestDetails struct {
	RequestID      string            `json:"requestId"`
	URL            string            `json:"url"`
	Method         string            `json:"method"`
	FrameID        int64             `json:"frameId"`
	ParentFrameID  int64             `json:"parentFrameId"`
	TabID          int64             `json:"tabId"`
	Type           ResourceType      `json:"type"`
	TimeStamp      float64           `json:"timeStamp"`
	Initiator      string            `json:"initiator,omitempty"`
	RequestHeaders map[string]string `json:"requestHeaders,omitempty"`
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
	Cancel          bool              `json:"cancel"`
	RedirectURL     string            `json:"redirectUrl,omitempty"`
	RequestHeaders  map[string]string `json:"requestHeaders,omitempty"`
	ResponseHeaders map[string]string `json:"responseHeaders,omitempty"`
}

// RequestFilter specifies which requests to monitor
type RequestFilter struct {
	URLs     []string       `json:"urls"`
	Types    []ResourceType `json:"types,omitempty"`
	TabID    int64          `json:"tabId,omitempty"`
	WindowID int64          `json:"windowId,omitempty"`
}
