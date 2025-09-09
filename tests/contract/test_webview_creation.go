package contract

import (
    "testing"

    "github.com/bnema/dumber/pkg/webkit"
)

// Contract: NewWebView creates a WebView without error and allows initial configuration.
func Test_NewWebView_Contract(t *testing.T) {
    cfg := &webkit.Config{InitialURL: "https://example.com", ZoomDefault: 1.0}
    view, err := webkit.NewWebView(cfg)
    if err != nil {
        t.Fatalf("expected NewWebView to succeed, got error: %v", err)
    }
    if view == nil {
        t.Fatalf("expected non-nil WebView")
    }
}
