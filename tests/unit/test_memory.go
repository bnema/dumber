package unit

import (
	"github.com/bnema/dumber/pkg/webkit"
	"testing"
)

// Memory test placeholder: baseline <100MB once implemented.
func Test_WebKit_Memory_Baseline(t *testing.T) {
	if _, err := webkit.NewWebView(&webkit.Config{}); err == nil {
		t.Fatalf("expected construction to fail prior to implementation (RED phase)")
	}
}
