package cef

import (
	"math"
	"testing"
)

func TestNormalizedApplicationScale(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{name: "default zero", in: 0, want: 1},
		{name: "negative", in: -1, want: 1},
		{name: "nan", in: math.NaN(), want: 1},
		{name: "inf", in: math.Inf(1), want: 1},
		{name: "valid", in: 1.2, want: 1.2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizedApplicationScale(tt.in); got != tt.want {
				t.Fatalf("normalizedApplicationScale(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
