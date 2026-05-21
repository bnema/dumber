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
		{name: "uses lower refresh monitor", refreshMilliHz: 50000, fallbackFPS: 60, maxFPS: 240, want: 50},
		{name: "floors very low refresh monitor", refreshMilliHz: 24000, fallbackFPS: 60, maxFPS: 240, want: 30},
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
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "exact lowercase", input: "wayland", want: true},
		{name: "exact uppercase", input: "WAYLAND", want: true},
		{name: "mixed case", input: "WayLand", want: true},
		{name: "numeric suffix", input: "wayland-1", want: true},
		{name: "punctuation suffix", input: "wayland:0", want: true},
		{name: "leading whitespace", input: " wayland ", want: true},
		{name: "substring", input: "mywayland-display", want: true},
		{name: "x11", input: "x11", want: false},
		{name: "empty", input: "", want: false},
		{name: "whitespace", input: "   ", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWaylandDisplayName(tt.input); got != tt.want {
				t.Fatalf("isWaylandDisplayName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
