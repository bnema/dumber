package contract

import (
	"github.com/bnema/dumber/pkg/webkit"
	"testing"
)

// Contract: SetZoom/GetZoom operate without error and persist value.
func Test_ZoomControl_Contract(t *testing.T) {
	view, err := webkit.NewWebView(&webkit.Config{ZoomDefault: 1.0})
	if err != nil {
		t.Fatalf("NewWebView failed: %v", err)
	}
	if err := view.SetZoom(1.25); err != nil {
		t.Fatalf("SetZoom should succeed, got: %v", err)
	}
	z, err := view.GetZoom()
	if err != nil {
		t.Fatalf("GetZoom should succeed, got: %v", err)
	}
	if z != 1.25 {
		t.Fatalf("expected zoom 1.25, got %v", z)
	}
}
