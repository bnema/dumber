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

func TestZoomConversionsCompensateDeviceSizedOSRBackingAndSurfaceScale(t *testing.T) {
	tests := []struct {
		name         string
		surfaceScale float64
		backingScale float64
		pageZoom     float64
		wantInternal float64
	}{
		{
			name:         "fractional_hidpi_keeps_100_percent_internal_zoom",
			surfaceScale: 1.2,
			backingScale: 1.2,
			pageZoom:     1.0,
			wantInternal: 1.0,
		},
		{
			name:         "integer_hidpi_keeps_requested_user_zoom",
			surfaceScale: 2.0,
			backingScale: 2.0,
			pageZoom:     1.2,
			wantInternal: 1.2,
		},
		{
			name:         "normal_osr_hidpi_keeps_requested_user_zoom",
			surfaceScale: 1.2,
			backingScale: 1.0,
			pageZoom:     1.0,
			wantInternal: 1.0,
		},
		{
			name:         "device_sized_backing_remains_compensated",
			surfaceScale: 1.25,
			backingScale: 2.0,
			pageZoom:     1.2,
			wantInternal: 0.75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := cefZoomFromPageAndScaleFactors(tt.pageZoom, tt.surfaceScale, tt.backingScale)
			assert.InDelta(t, tt.wantInternal, factorFromCEFZoom(level), 1e-9)
			assert.InDelta(t, tt.pageZoom, pageZoomFromCEFAndScaleLevel(level, tt.surfaceScale, tt.backingScale), 1e-9)
		})
	}
}

func TestZoomReapplyGuardTracksEffectiveScaleRatio(t *testing.T) {
	wv := &WebView{}

	wv.recordAppliedZoomScaleRatio(1.2, 1.2)

	assert.False(t, wv.shouldReapplyZoomForScaleRatio(2.0, 2.0), "same effective ratio should not reapply")
	assert.True(t, wv.shouldReapplyZoomForScaleRatio(1.2, 2.0), "changed active-backing ratio should reapply")
}
