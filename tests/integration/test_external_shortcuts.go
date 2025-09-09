package integration

import (
	"github.com/bnema/dumber/pkg/webkit"
	"testing"
)

// Integration: keyboard shortcuts work on external sites once implemented.
func Test_WebKit_ExternalShortcuts_Integration(t *testing.T) {
	view, err := webkit.NewWebView(&webkit.Config{})
	if err != nil {
		t.Fatalf("NewWebView failed: %v", err)
	}
	if err := view.RegisterKeyboardShortcut("Ctrl+K", func() {}); err != nil {
		t.Fatalf("RegisterKeyboardShortcut should succeed, got: %v", err)
	}
}
