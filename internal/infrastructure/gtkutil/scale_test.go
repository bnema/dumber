package gtkutil

import (
	"math"
	"testing"
)

func TestNormalizeScale(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  float64
	}{
		{name: "zero", input: 0, want: 1},
		{name: "fractional", input: 1.2, want: 1.2},
		{name: "nan", input: math.NaN(), want: 1},
		{name: "positive_infinity", input: math.Inf(1), want: 1},
		{name: "negative_infinity", input: math.Inf(-1), want: 1},
		{name: "negative", input: -1, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeScale(tt.input); got != tt.want {
				t.Fatalf("NormalizeScale(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestDeviceLogicalConversions(t *testing.T) {
	if got := DeviceToLogical(320, 1.25); got != 256 {
		t.Fatalf("DeviceToLogical(320, 1.25) = %d, want 256", got)
	}
}
