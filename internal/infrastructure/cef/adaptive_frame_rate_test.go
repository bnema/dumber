package cef

import "testing"

func TestAdaptiveFrameRateForRefresh(t *testing.T) {
	tests := []struct {
		name           string
		refreshMilliHz int
		fallbackFPS    int32
		maxFPS         int32
		want           int32
	}{
		{name: "fallback unknown", refreshMilliHz: 0, fallbackFPS: 60, maxFPS: 240, want: 60},
		{name: "uses monitor refresh", refreshMilliHz: 144000, fallbackFPS: 60, maxFPS: 240, want: 144},
		{name: "hard caps extreme monitor", refreshMilliHz: 500000, fallbackFPS: 60, maxFPS: 240, want: 240},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adaptiveFrameRateForRefresh(tt.refreshMilliHz, tt.fallbackFPS, tt.maxFPS)
			if got != tt.want {
				t.Fatalf("adaptiveFrameRateForRefresh() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsWaylandDisplayName(t *testing.T) {
	if !isWaylandDisplayName("Wayland") || !isWaylandDisplayName("wayland-0") {
		t.Fatal("expected wayland display names to match")
	}
	if isWaylandDisplayName("x11") || isWaylandDisplayName("") {
		t.Fatal("unexpected non-wayland display match")
	}
}
