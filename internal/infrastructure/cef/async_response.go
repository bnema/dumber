package cef

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	purecef "bnema.dev/purego-cef/cef"
)

// asyncResponse is a complete response computed off the CEF IO thread.
type asyncResponse struct {
	status      int
	contentType string
	data        []byte
	headers     map[string]string
}

// newAsyncResponseResourceHandler runs produce on a goroutine and answers
// with its result once ready (Open returns immediately and resumes the
// request through callback.Cont), so a slow producer never blocks the IO
// thread that serves every other request.
func newAsyncResponseResourceHandler(ctx context.Context, produce func(context.Context) asyncResponse) purecef.ResourceHandler {
	reqCtx, cancel := context.WithCancel(ctx) //nolint:gosec // canceled by load completion or CEF Cancel callback
	return cefNewResourceHandler(&faviconResourceHandler{
		ctx:     reqCtx,
		cancel:  cancel,
		produce: produce,
		done:    make(chan struct{}),
	})
}

func safeProduce(ctx context.Context, produce func(context.Context) asyncResponse) (r asyncResponse) {
	defer func() {
		if p := recover(); p != nil {
			data, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("handler panicked: %v", p)})
			r = apiAsyncResponse(http.StatusInternalServerError, data)
		}
	}()
	return produce(ctx)
}

// apiAsyncResponse is a JSON API response with the public API headers.
func apiAsyncResponse(status int, data []byte) asyncResponse {
	return asyncResponse{status: status, contentType: "application/json", data: data, headers: apiResponseHeaders()}
}

// bridgeMessageType returns the type field of a bridge message body.
func bridgeMessageType(body []byte) string {
	var msg struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(body, &msg) != nil {
		return ""
	}
	return msg.Type
}
