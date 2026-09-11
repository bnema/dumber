package entity

import (
	"math"
	"testing"
)

func TestZoomPercentageRoundsDisplayedValue(t *testing.T) {
	tests := []struct {
		name   string
		factor float64
		want   int
	}{
		{name: "default_zoom", factor: 1.0, want: 100},
		{name: "accumulated_default_error", factor: 0.9999999999999997, want: 100},
		{name: "accumulated_above_default", factor: 1.0000000000000002, want: 100},
		{name: "ordinary_percent", factor: 1.5, want: 150},
		{name: "minimum_zoom", factor: ZoomMin, want: 25},
		{name: "maximum_zoom", factor: ZoomMax, want: 500},
		{name: "rounds_up", factor: 1.006, want: 101},
		{name: "rounds_down", factor: 1.004, want: 100},
		{name: "three_digit_percent_below_half", factor: 2.344, want: 234},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zoom := NewZoomLevel("example.com", tt.factor)
			if got := zoom.Percentage(); got != tt.want {
				t.Fatalf("Percentage()=%d, want %d for factor %.16f", got, tt.want, tt.factor)
			}
		})
	}
}

func TestZoomPercentageSurvivesRepeatedSteps(t *testing.T) {
	zoom := NewZoomLevel("example.com", ZoomDefault)

	for range 10 {
		zoom.ZoomIn()
	}
	if got := zoom.Percentage(); got != 200 {
		t.Fatalf("after 10 zoom-in steps Percentage()=%d, want 200", got)
	}

	for range 10 {
		zoom.ZoomOut()
	}
	if got := zoom.Percentage(); got != 100 {
		t.Fatalf("after 10 zoom-out steps Percentage()=%d, want 100", got)
	}
	if math.Abs(zoom.ZoomFactor-ZoomDefault) > 1e-9 {
		t.Fatalf("persisted factor drifted to %.16f, want default", zoom.ZoomFactor)
	}
	if !zoom.IsDefault() {
		t.Fatal("factor behind a 100%% display must still compare equal to the default zoom")
	}
}
