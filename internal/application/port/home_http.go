package port

import (
	"context"
	"time"
)

// HomeHTTPRequest is an HTTP request issued for a home page widget or action.
type HomeHTTPRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
	// Timeout bounds the whole request. Zero selects the client default.
	Timeout time.Duration
}

// HomeHTTPResponse is the status and body of a home widget HTTP response.
type HomeHTTPResponse struct {
	StatusCode int
	Body       []byte
}

// HomeHTTPClient performs HTTP requests for home widgets and actions.
// A non-2xx status is not an error. Errors never include header values.
type HomeHTTPClient interface {
	Do(ctx context.Context, req HomeHTTPRequest) (HomeHTTPResponse, error)
}
