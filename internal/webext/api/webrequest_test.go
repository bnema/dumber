package api

import (
	"testing"
)

func TestNewWebRequestAPI(t *testing.T) {
	api := NewWebRequestAPI()
	if api == nil {
		t.Fatal("NewWebRequestAPI() returned nil")
	}
	if api.onBeforeRequestListeners == nil {
		t.Error("onBeforeRequestListeners not initialized")
	}
	if api.filters == nil {
		t.Error("filters not initialized")
	}
}

func TestOnBeforeRequest(t *testing.T) {
	tests := []struct {
		name        string
		extensionID string
		filter      *RequestFilter
		wantErr     bool
	}{
		{
			name:        "valid registration",
			extensionID: "ext1",
			filter: &RequestFilter{
				URLs: []string{"<all_urls>"},
			},
			wantErr: false,
		},
		{
			name:        "nil filter",
			extensionID: "ext2",
			filter:      nil,
			wantErr:     true,
		},
		{
			name:        "empty extension ID",
			extensionID: "",
			filter: &RequestFilter{
				URLs: []string{"*://*.example.com/*"},
			},
			wantErr: false, // Extension ID is just a key
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewWebRequestAPI()
			listener := func(details RequestDetails) *BlockingResponse {
				return nil
			}

			err := api.OnBeforeRequest(tt.extensionID, listener, tt.filter)
			if (err != nil) != tt.wantErr {
				t.Errorf("OnBeforeRequest() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(api.onBeforeRequestListeners[tt.extensionID]) != 1 {
					t.Errorf("Expected 1 listener, got %d", len(api.onBeforeRequestListeners[tt.extensionID]))
				}
			}
		})
	}
}

func TestHandleBeforeRequest_Cancel(t *testing.T) {
	tests := []struct {
		name       string
		extensions []struct {
			id       string
			response *BlockingResponse
			filter   *RequestFilter
		}
		details      RequestDetails
		wantCancel   bool
		wantRedirect string
	}{
		{
			name: "single extension blocks request",
			extensions: []struct {
				id       string
				response *BlockingResponse
				filter   *RequestFilter
			}{
				{
					id:       "blocker",
					response: &BlockingResponse{Cancel: true},
					filter:   &RequestFilter{URLs: []string{"<all_urls>"}},
				},
			},
			details: RequestDetails{
				URL:  "https://ads.example.com/banner.js",
				Type: ResourceTypeScript,
			},
			wantCancel: true,
		},
		{
			name: "extension redirects request",
			extensions: []struct {
				id       string
				response *BlockingResponse
				filter   *RequestFilter
			}{
				{
					id:       "redirector",
					response: &BlockingResponse{RedirectURL: "https://safe.example.com/blank.js"},
					filter:   &RequestFilter{URLs: []string{"<all_urls>"}},
				},
			},
			details: RequestDetails{
				URL:  "https://tracker.example.com/track.js",
				Type: ResourceTypeScript,
			},
			wantCancel:   false,
			wantRedirect: "https://safe.example.com/blank.js",
		},
		{
			name: "cancel takes priority over redirect",
			extensions: []struct {
				id       string
				response *BlockingResponse
				filter   *RequestFilter
			}{
				{
					id:       "redirector",
					response: &BlockingResponse{RedirectURL: "https://safe.example.com/blank.js"},
					filter:   &RequestFilter{URLs: []string{"<all_urls>"}},
				},
				{
					id:       "blocker",
					response: &BlockingResponse{Cancel: true},
					filter:   &RequestFilter{URLs: []string{"<all_urls>"}},
				},
			},
			details: RequestDetails{
				URL:  "https://bad.example.com/evil.js",
				Type: ResourceTypeScript,
			},
			wantCancel: true,
		},
		{
			name: "no blocking response",
			extensions: []struct {
				id       string
				response *BlockingResponse
				filter   *RequestFilter
			}{
				{
					id:       "observer",
					response: nil,
					filter:   &RequestFilter{URLs: []string{"<all_urls>"}},
				},
			},
			details: RequestDetails{
				URL:  "https://example.com/page.html",
				Type: ResourceTypeMain,
			},
			wantCancel: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewWebRequestAPI()

			// Register all extension listeners
			for _, ext := range tt.extensions {
				response := ext.response
				listener := func(details RequestDetails) *BlockingResponse {
					return response
				}
				err := api.OnBeforeRequest(ext.id, listener, ext.filter)
				if err != nil {
					t.Fatalf("Failed to register listener: %v", err)
				}
			}

			// Handle the request
			result := api.HandleBeforeRequest(tt.details)

			if tt.wantCancel {
				if result == nil || !result.Cancel {
					t.Errorf("Expected request to be cancelled, got result: %+v", result)
				}
			} else if tt.wantRedirect != "" {
				if result == nil {
					t.Errorf("Expected redirect, got nil result")
				} else if result.RedirectURL != tt.wantRedirect {
					t.Errorf("Expected redirect to %s, got %s", tt.wantRedirect, result.RedirectURL)
				}
			} else if result != nil && (result.Cancel || result.RedirectURL != "") {
				t.Errorf("Expected no blocking response, got: %+v", result)
			}
		})
	}
}

func TestHandleBeforeRequest_FilterMatching(t *testing.T) {
	tests := []struct {
		name         string
		filter       *RequestFilter
		details      RequestDetails
		shouldMatch  bool
		responseType string // "cancel", "redirect", or ""
	}{
		{
			name: "matches all URLs",
			filter: &RequestFilter{
				URLs: []string{"<all_urls>"},
			},
			details: RequestDetails{
				URL:  "https://example.com/page",
				Type: ResourceTypeMain,
			},
			shouldMatch:  true,
			responseType: "cancel",
		},
		{
			name: "matches resource type",
			filter: &RequestFilter{
				URLs:  []string{"<all_urls>"},
				Types: []ResourceType{ResourceTypeScript, ResourceTypeImage},
			},
			details: RequestDetails{
				URL:  "https://example.com/script.js",
				Type: ResourceTypeScript,
			},
			shouldMatch:  true,
			responseType: "cancel",
		},
		{
			name: "does not match resource type",
			filter: &RequestFilter{
				URLs:  []string{"<all_urls>"},
				Types: []ResourceType{ResourceTypeScript},
			},
			details: RequestDetails{
				URL:  "https://example.com/style.css",
				Type: ResourceTypeStylesheet,
			},
			shouldMatch:  false,
			responseType: "cancel",
		},
		{
			name: "matches tab ID",
			filter: &RequestFilter{
				URLs:  []string{"<all_urls>"},
				TabID: 123,
			},
			details: RequestDetails{
				URL:   "https://example.com/page",
				Type:  ResourceTypeMain,
				TabID: 123,
			},
			shouldMatch:  true,
			responseType: "cancel",
		},
		{
			name: "does not match tab ID",
			filter: &RequestFilter{
				URLs:  []string{"<all_urls>"},
				TabID: 123,
			},
			details: RequestDetails{
				URL:   "https://example.com/page",
				Type:  ResourceTypeMain,
				TabID: 456,
			},
			shouldMatch:  false,
			responseType: "cancel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewWebRequestAPI()

			var expectedResponse *BlockingResponse
			if tt.responseType == "cancel" {
				expectedResponse = &BlockingResponse{Cancel: true}
			} else if tt.responseType == "redirect" {
				expectedResponse = &BlockingResponse{RedirectURL: "https://blocked.example.com"}
			}

			listener := func(details RequestDetails) *BlockingResponse {
				return expectedResponse
			}

			err := api.OnBeforeRequest("test-ext", listener, tt.filter)
			if err != nil {
				t.Fatalf("Failed to register listener: %v", err)
			}

			result := api.HandleBeforeRequest(tt.details)

			if tt.shouldMatch {
				if result == nil {
					t.Errorf("Expected matching filter to return response, got nil")
				} else if tt.responseType == "cancel" && !result.Cancel {
					t.Errorf("Expected Cancel=true, got %+v", result)
				} else if tt.responseType == "redirect" && result.RedirectURL == "" {
					t.Errorf("Expected redirect URL, got %+v", result)
				}
			} else {
				if result != nil && (result.Cancel || result.RedirectURL != "") {
					t.Errorf("Expected no response for non-matching filter, got: %+v", result)
				}
			}
		})
	}
}

func TestHandleBeforeSendHeaders(t *testing.T) {
	tests := []struct {
		name        string
		response    *BlockingResponse
		details     RequestDetails
		wantCancel  bool
		wantHeaders bool
	}{
		{
			name: "modify request headers",
			response: &BlockingResponse{
				RequestHeaders: map[string]string{
					"X-Custom-Header": "value",
					"User-Agent":      "CustomBot/1.0",
				},
			},
			details: RequestDetails{
				URL:  "https://example.com/api",
				Type: ResourceTypeXMLHTTP,
			},
			wantCancel:  false,
			wantHeaders: true,
		},
		{
			name: "cancel request",
			response: &BlockingResponse{
				Cancel: true,
			},
			details: RequestDetails{
				URL:  "https://tracker.example.com/track",
				Type: ResourceTypeXMLHTTP,
			},
			wantCancel:  true,
			wantHeaders: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewWebRequestAPI()

			listener := func(details RequestDetails) *BlockingResponse {
				return tt.response
			}

			filter := &RequestFilter{URLs: []string{"<all_urls>"}}
			err := api.OnBeforeSendHeaders("test-ext", listener, filter)
			if err != nil {
				t.Fatalf("Failed to register listener: %v", err)
			}

			result := api.HandleBeforeSendHeaders(tt.details)

			if tt.wantCancel {
				if result == nil || !result.Cancel {
					t.Errorf("Expected Cancel=true, got %+v", result)
				}
			} else if tt.wantHeaders {
				if result == nil || result.RequestHeaders == nil {
					t.Errorf("Expected headers modification, got %+v", result)
				}
			}
		})
	}
}

func TestHandleHeadersReceived(t *testing.T) {
	tests := []struct {
		name        string
		response    *BlockingResponse
		details     ResponseDetails
		wantCancel  bool
		wantHeaders bool
	}{
		{
			name: "modify response headers",
			response: &BlockingResponse{
				ResponseHeaders: map[string]string{
					"X-Frame-Options":         "DENY",
					"Content-Security-Policy": "default-src 'self'",
				},
			},
			details: ResponseDetails{
				URL:        "https://example.com/page",
				Type:       ResourceTypeMain,
				StatusCode: 200,
			},
			wantCancel:  false,
			wantHeaders: true,
		},
		{
			name: "cancel on response",
			response: &BlockingResponse{
				Cancel: true,
			},
			details: ResponseDetails{
				URL:        "https://malicious.example.com/page",
				Type:       ResourceTypeMain,
				StatusCode: 200,
			},
			wantCancel:  true,
			wantHeaders: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewWebRequestAPI()

			listener := func(details ResponseDetails) *BlockingResponse {
				return tt.response
			}

			filter := &RequestFilter{URLs: []string{"<all_urls>"}}
			err := api.OnHeadersReceived("test-ext", listener, filter)
			if err != nil {
				t.Fatalf("Failed to register listener: %v", err)
			}

			result := api.HandleHeadersReceived(tt.details)

			if tt.wantCancel {
				if result == nil || !result.Cancel {
					t.Errorf("Expected Cancel=true, got %+v", result)
				}
			} else if tt.wantHeaders {
				if result == nil || result.ResponseHeaders == nil {
					t.Errorf("Expected headers modification, got %+v", result)
				}
			}
		})
	}
}

func TestHandleCompleted(t *testing.T) {
	tests := []struct {
		name         string
		details      ResponseDetails
		filter       *RequestFilter
		wantCalled   bool
		checkDetails func(*testing.T, ResponseDetails)
	}{
		{
			name: "listener_called_on_matching_request",
			details: ResponseDetails{
				URL:        "https://example.com/page",
				Type:       ResourceTypeMain,
				StatusCode: 200,
			},
			filter:     &RequestFilter{URLs: []string{"<all_urls>"}},
			wantCalled: true,
			checkDetails: func(t *testing.T, details ResponseDetails) {
				if details.URL != "https://example.com/page" {
					t.Errorf("Expected URL https://example.com/page, got %s", details.URL)
				}
				if details.StatusCode != 200 {
					t.Errorf("Expected status code 200, got %d", details.StatusCode)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewWebRequestAPI()
			called := false

			listener := func(details ResponseDetails) {
				called = true
				if tt.checkDetails != nil {
					tt.checkDetails(t, details)
				}
			}

			err := api.OnCompleted("test-ext", listener, tt.filter)
			if err != nil {
				t.Fatalf("Failed to register listener: %v", err)
			}

			api.HandleCompleted(tt.details)

			if called != tt.wantCalled {
				t.Errorf("listener called = %v, want %v", called, tt.wantCalled)
			}
		})
	}
}

func TestHandleErrorOccurred(t *testing.T) {
	tests := []struct {
		name         string
		details      RequestDetails
		errorMsg     string
		filter       *RequestFilter
		wantCalled   bool
		wantErrorMsg string
	}{
		{
			name: "listener_called_with_error_message",
			details: RequestDetails{
				URL:  "https://example.com/fail",
				Type: ResourceTypeMain,
			},
			errorMsg:     "net::ERR_CONNECTION_REFUSED",
			filter:       &RequestFilter{URLs: []string{"<all_urls>"}},
			wantCalled:   true,
			wantErrorMsg: "net::ERR_CONNECTION_REFUSED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewWebRequestAPI()
			called := false
			var receivedError string

			listener := func(details RequestDetails, errorMsg string) {
				called = true
				receivedError = errorMsg
				if details.URL != tt.details.URL {
					t.Errorf("Expected URL %s, got %s", tt.details.URL, details.URL)
				}
			}

			err := api.OnErrorOccurred("test-ext", listener, tt.filter)
			if err != nil {
				t.Fatalf("Failed to register listener: %v", err)
			}

			api.HandleErrorOccurred(tt.details, tt.errorMsg)

			if called != tt.wantCalled {
				t.Errorf("listener called = %v, want %v", called, tt.wantCalled)
			}
			if tt.wantCalled && receivedError != tt.wantErrorMsg {
				t.Errorf("Expected error message '%s', got '%s'", tt.wantErrorMsg, receivedError)
			}
		})
	}
}

func TestRemoveListener(t *testing.T) {
	tests := []struct {
		name              string
		extensionID       string
		registerListeners func(*WebRequestAPI, *RequestFilter)
		wantBeforeRequest int
		wantBeforeSend    int
		wantFilters       bool
	}{
		{
			name:        "removes_all_listeners_for_extension",
			extensionID: "ext1",
			registerListeners: func(api *WebRequestAPI, filter *RequestFilter) {
				listener := func(details RequestDetails) *BlockingResponse {
					return &BlockingResponse{Cancel: true}
				}
				api.OnBeforeRequest("ext1", listener, filter)
				api.OnBeforeSendHeaders("ext1", listener, filter)
			},
			wantBeforeRequest: 0,
			wantBeforeSend:    0,
			wantFilters:       false,
		},
		{
			name:        "removes_only_specified_extension",
			extensionID: "ext1",
			registerListeners: func(api *WebRequestAPI, filter *RequestFilter) {
				listener := func(details RequestDetails) *BlockingResponse {
					return &BlockingResponse{Cancel: true}
				}
				api.OnBeforeRequest("ext1", listener, filter)
				api.OnBeforeRequest("ext2", listener, filter)
			},
			wantBeforeRequest: 0, // ext1 should be removed
			wantBeforeSend:    0,
			wantFilters:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewWebRequestAPI()
			filter := &RequestFilter{URLs: []string{"<all_urls>"}}

			tt.registerListeners(api, filter)

			// Remove listeners for the extension
			api.RemoveListener(tt.extensionID)

			if len(api.onBeforeRequestListeners[tt.extensionID]) != tt.wantBeforeRequest {
				t.Errorf("onBeforeRequestListeners[%s] count = %d, want %d",
					tt.extensionID, len(api.onBeforeRequestListeners[tt.extensionID]), tt.wantBeforeRequest)
			}
			if len(api.onBeforeSendHeadersListeners[tt.extensionID]) != tt.wantBeforeSend {
				t.Errorf("onBeforeSendHeadersListeners[%s] count = %d, want %d",
					tt.extensionID, len(api.onBeforeSendHeadersListeners[tt.extensionID]), tt.wantBeforeSend)
			}

			filterExists := api.filters[tt.extensionID] != nil
			if filterExists != tt.wantFilters {
				t.Errorf("filter exists = %v, want %v", filterExists, tt.wantFilters)
			}
		})
	}
}

func TestMultipleExtensions(t *testing.T) {
	tests := []struct {
		name       string
		extensions []struct {
			id       string
			response *BlockingResponse
		}
		details    RequestDetails
		wantCancel bool
	}{
		{
			name: "cancel_takes_priority_among_multiple_extensions",
			extensions: []struct {
				id       string
				response *BlockingResponse
			}{
				{id: "ext1", response: nil},                             // Allows request
				{id: "ext2", response: &BlockingResponse{Cancel: true}}, // Blocks request
			},
			details: RequestDetails{
				URL:  "https://ads.example.com/banner.js",
				Type: ResourceTypeScript,
			},
			wantCancel: true,
		},
		{
			name: "all_extensions_allow_request",
			extensions: []struct {
				id       string
				response *BlockingResponse
			}{
				{id: "ext1", response: nil},
				{id: "ext2", response: nil},
				{id: "ext3", response: nil},
			},
			details: RequestDetails{
				URL:  "https://example.com/page.html",
				Type: ResourceTypeMain,
			},
			wantCancel: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewWebRequestAPI()
			filter := &RequestFilter{URLs: []string{"<all_urls>"}}

			// Register all extension listeners
			for _, ext := range tt.extensions {
				response := ext.response
				listener := func(details RequestDetails) *BlockingResponse {
					return response
				}
				api.OnBeforeRequest(ext.id, listener, filter)
			}

			result := api.HandleBeforeRequest(tt.details)

			if tt.wantCancel {
				if result == nil || !result.Cancel {
					t.Errorf("Expected request to be cancelled, got: %+v", result)
				}
			} else {
				if result != nil && result.Cancel {
					t.Errorf("Expected request to be allowed, got: %+v", result)
				}
			}
		})
	}
}

func TestMatchesFilter(t *testing.T) {
	api := NewWebRequestAPI()

	tests := []struct {
		name         string
		url          string
		resourceType ResourceType
		tabID        int64
		filter       *RequestFilter
		want         bool
	}{
		{
			name:         "nil filter",
			url:          "https://example.com",
			resourceType: ResourceTypeMain,
			tabID:        1,
			filter:       nil,
			want:         false,
		},
		{
			name:         "all URLs match",
			url:          "https://example.com",
			resourceType: ResourceTypeMain,
			tabID:        1,
			filter:       &RequestFilter{URLs: []string{"<all_urls>"}},
			want:         true,
		},
		{
			name:         "resource type matches",
			url:          "https://example.com/script.js",
			resourceType: ResourceTypeScript,
			tabID:        1,
			filter: &RequestFilter{
				URLs:  []string{"<all_urls>"},
				Types: []ResourceType{ResourceTypeScript},
			},
			want: true,
		},
		{
			name:         "resource type does not match",
			url:          "https://example.com/style.css",
			resourceType: ResourceTypeStylesheet,
			tabID:        1,
			filter: &RequestFilter{
				URLs:  []string{"<all_urls>"},
				Types: []ResourceType{ResourceTypeScript},
			},
			want: false,
		},
		{
			name:         "tab ID matches",
			url:          "https://example.com",
			resourceType: ResourceTypeMain,
			tabID:        123,
			filter: &RequestFilter{
				URLs:  []string{"<all_urls>"},
				TabID: 123,
			},
			want: true,
		},
		{
			name:         "tab ID does not match",
			url:          "https://example.com",
			resourceType: ResourceTypeMain,
			tabID:        456,
			filter: &RequestFilter{
				URLs:  []string{"<all_urls>"},
				TabID: 123,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := api.matchesFilter(tt.url, tt.resourceType, tt.tabID, tt.filter)
			if got != tt.want {
				t.Errorf("matchesFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesURLPattern(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		pattern string
		want    bool
	}{
		{
			name:    "all_urls matches any URL",
			url:     "https://example.com/page",
			pattern: "<all_urls>",
			want:    true,
		},
		{
			name:    "all_urls matches http",
			url:     "http://example.com/page",
			pattern: "<all_urls>",
			want:    true,
		},
		// TODO: Add more specific pattern tests when proper matching is implemented
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesURLPattern(tt.url, tt.pattern)
			if got != tt.want {
				t.Errorf("matchesURLPattern() = %v, want %v", got, tt.want)
			}
		})
	}
}
