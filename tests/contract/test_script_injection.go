package contract

import (
	"github.com/bnema/dumber/pkg/webkit"
	"testing"
)

// Contract: InjectScript executes JS without error once implemented.
func Test_InjectScript_Contract(t *testing.T) {
	view, err := webkit.NewWebView(&webkit.Config{})
	if err != nil {
		t.Fatalf("NewWebView failed: %v", err)
	}
	if err := view.InjectScript("console.log('hello')"); err != nil {
		t.Fatalf("InjectScript should succeed, got: %v", err)
	}
}
