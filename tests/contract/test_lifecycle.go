package contract

import (
    "testing"
    "github.com/bnema/dumber/pkg/webkit"
)

// Contract: Show/Hide/Destroy lifecycle operations succeed without error.
func Test_Lifecycle_Contract(t *testing.T) {
    view, err := webkit.NewWebView(&webkit.Config{})
    if err != nil { t.Fatalf("NewWebView failed: %v", err) }
    if err := view.Show(); err != nil { t.Fatalf("Show should succeed: %v", err) }
    if err := view.Hide(); err != nil { t.Fatalf("Hide should succeed: %v", err) }
    if err := view.Destroy(); err != nil { t.Fatalf("Destroy should succeed: %v", err) }
}
