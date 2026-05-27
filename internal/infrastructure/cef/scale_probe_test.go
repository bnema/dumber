package cef

import (
	"strings"
	"testing"
)

func TestCEFScaleProbeScriptCapturesViewportMetrics(t *testing.T) {
	for _, want := range []string{
		"window.devicePixelRatio",
		"window.innerWidth",
		"document.documentElement.clientWidth",
		"window.visualViewport",
		"width:100px;height:100px",
		"[SCALE-PROBE]",
	} {
		if !strings.Contains(cefScaleProbeScript, want) {
			t.Fatalf("cefScaleProbeScript missing %q", want)
		}
	}
}

func TestShouldRunCEFScaleProbeSkipsIntermediateHTTPStatusZero(t *testing.T) {
	if shouldRunCEFScaleProbe("https://example.com", 0) {
		t.Fatal("probe should skip intermediate HTTP status 0 for http URLs")
	}
	if !shouldRunCEFScaleProbe("https://example.com", 200) {
		t.Fatal("probe should run for successful HTTP load")
	}
	if shouldRunCEFScaleProbe("about:blank", 200) {
		t.Fatal("probe should skip about:blank")
	}
	if !shouldRunCEFScaleProbe("file:///tmp/probe.html", 0) {
		t.Fatal("probe should allow non-HTTP status 0 loads")
	}
}

func TestCEFScaleProbeSnapshotNormalizesMissingBridge(t *testing.T) {
	s := cefScaleProbeSnapshot(nil)
	if s.SurfaceWidth != 1 || s.SurfaceHeight != 1 {
		t.Fatalf("surface size = %dx%d, want 1x1", s.SurfaceWidth, s.SurfaceHeight)
	}
	if s.SurfaceScale != 1 || s.OSRBackingScale != 1 {
		t.Fatalf("scales = surface %v backing %v, want 1/1", s.SurfaceScale, s.OSRBackingScale)
	}
	if s.UserZoom != 1 || s.InternalCEFFactor != 1 {
		t.Fatalf("zoom = user %v internal %v, want 1/1", s.UserZoom, s.InternalCEFFactor)
	}
}
