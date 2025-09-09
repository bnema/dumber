package unit

import (
    "errors"
    "testing"
    "github.com/bnema/dumber/pkg/webkit"
)

// Unit: WebKit error handling should propagate errors with context once implemented.
func Test_WebKit_ErrorHandling(t *testing.T) {
    _, err := webkit.NewWebView(nil)
    if err == nil {
        t.Fatalf("expected error prior to implementation (RED phase)")
    }
    if !errors.Is(err, webkit.ErrNotImplemented) {
        t.Fatalf("expected ErrNotImplemented, got %v", err)
    }
}

