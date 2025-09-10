package contract

import (
	"github.com/bnema/dumber/pkg/webkit"
	"testing"
)

// Contract: LoadURL navigates to a valid URL without error.
func TestLoadURLNavigationContract(t *testing.T) {
	view, err := webkit.NewWebView(&webkit.Config{})
	if err != nil {
		t.Fatalf("NewWebView failed: %v", err)
	}
	if err := view.LoadURL("https://example.com"); err != nil {
		t.Fatalf("LoadURL should succeed, got: %v", err)
	}
}
