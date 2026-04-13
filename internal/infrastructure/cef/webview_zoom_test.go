package cef

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestZoomConversionsRoundTrip(t *testing.T) {
	factors := []float64{0.5, 0.8, 1.0, 1.1, 1.2, 1.4, 2.0}
	for _, factor := range factors {
		t.Run(fmt.Sprintf("factor_%.1f", factor), func(t *testing.T) {
			level := cefZoomFromFactor(factor)
			got := factorFromCEFZoom(level)
			assert.InDelta(t, factor, got, 1e-9, "round-trip mismatch for factor %.6f: level %.6f -> factor %.12f", factor, level, got)
		})
	}
}
