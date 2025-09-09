package integration

import (
	"github.com/bnema/dumber/pkg/webkit"
	"testing"
)

// Integration: navigate to google.fr without error once implemented.
func Test_WebKit_GoogleNavigation_Integration(t *testing.T) {
	view, err := webkit.NewWebView(&webkit.Config{})
	if err != nil {
		t.Fatalf("NewWebView failed: %v", err)
	}
	if err := view.LoadURL("https://www.google.fr"); err != nil {
		t.Fatalf("LoadURL should succeed, got: %v", err)
	}
}
