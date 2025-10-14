package integration

import (
	"github.com/bnema/dumber/pkg/webkit"
	"testing"
)

// Integration: basic WebKit window and navigation works.
func Test_WebKit_BasicWindow_Integration(t *testing.T) {
	view, err := webkit.NewWebView(&webkit.Config{})
	if err != nil {
		t.Fatalf("NewWebView failed: %v", err)
	}
	if err := view.LoadURL("https://example.com"); err != nil {
		t.Fatalf("LoadURL should succeed, got: %v", err)
	}
}
