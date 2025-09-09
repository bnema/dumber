package unit

import (
    "testing"
    "github.com/bnema/dumber/pkg/webkit"
)

// Memory test placeholder: baseline <100MB once implemented.
func Test_WebKit_Memory_Baseline(t *testing.T) {
    if _, err := webkit.NewWebView(&webkit.Config{}); err == nil {
        t.Fatalf("expected construction to fail prior to implementation (RED phase)")
    }
}

