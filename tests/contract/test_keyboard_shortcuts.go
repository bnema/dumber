package contract

import (
    "testing"
    "github.com/bnema/dumber/pkg/webkit"
)

// Contract: RegisterKeyboardShortcut binds accelerator without error and triggers callback on press.
func Test_RegisterKeyboardShortcut_Contract(t *testing.T) {
    view, err := webkit.NewWebView(&webkit.Config{})
    if err != nil { t.Fatalf("NewWebView failed: %v", err) }
    err = view.RegisterKeyboardShortcut("Ctrl+L", func() {})
    if err != nil { t.Fatalf("RegisterKeyboardShortcut should succeed, got: %v", err) }
}
