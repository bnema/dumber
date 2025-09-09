package integration

import (
	"github.com/bnema/dumber/pkg/webkit"
	"testing"
)

// Integration: zoom persists across operations (with DB) once implemented.
func Test_WebKit_ZoomPersistence_Integration(t *testing.T) {
	view, err := webkit.NewWebView(&webkit.Config{ZoomDefault: 1.0})
	if err != nil {
		t.Fatalf("NewWebView failed: %v", err)
	}
	if err := view.SetZoom(1.1); err != nil {
		t.Fatalf("SetZoom should succeed, got: %v", err)
	}
	z, err := view.GetZoom()
	if err != nil {
		t.Fatalf("GetZoom should succeed, got: %v", err)
	}
	if z != 1.1 {
		t.Fatalf("expected zoom 1.1, got %v", z)
	}
}
