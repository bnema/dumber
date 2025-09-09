package unit

import (
    "testing"
    "github.com/bnema/dumber/pkg/webkit"
)

// Performance test placeholder: startup time <500ms (to be measured once implemented).
func Test_WebKit_Performance_StartupTime(t *testing.T) {
    if _, err := webkit.NewWebView(&webkit.Config{}); err == nil {
        t.Fatalf("expected construction to fail prior to implementation (RED phase)")
    }
}

