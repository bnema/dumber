package contract

import (
    "testing"
    "github.com/bnema/dumber/pkg/webkit"
)

// Contract: LoadURL navigates to a valid URL without error.
func Test_LoadURL_Navigation_Contract(t *testing.T) {
    view, err := webkit.NewWebView(&webkit.Config{})
    if err != nil {
        t.Fatalf("NewWebView failed: %v", err)
    }
    if err := view.LoadURL("https://example.com"); err != nil {
        t.Fatalf("LoadURL should succeed, got: %v", err)
    }
}
