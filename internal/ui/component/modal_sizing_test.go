package component

import (
	"testing"

	layoutmocks "github.com/bnema/dumber/internal/ui/layout/mocks"
	"github.com/stretchr/testify/assert"
)

func TestCalculateModalDimensions_NilParent(t *testing.T) {
	cfg := ModalSizeConfig{
		WidthPct:       0.6,
		MaxWidth:       600,
		TopMarginPct:   0.15,
		FallbackWidth:  800,
		FallbackHeight: 600,
	}

	width, marginTop := CalculateModalDimensions(nil, cfg)

	// Should use fallback values
	expectedWidth := int(float64(cfg.FallbackWidth) * cfg.WidthPct)
	expectedMargin := int(float64(cfg.FallbackHeight) * cfg.TopMarginPct)

	assert.Equal(t, expectedWidth, width)
	assert.Equal(t, expectedMargin, marginTop)
}

func TestCalculateModalDimensions_ZeroAllocatedSize(t *testing.T) {
	cfg := ModalSizeConfig{
		WidthPct:       0.5,
		MaxWidth:       500,
		TopMarginPct:   0.2,
		FallbackWidth:  1000,
		FallbackHeight: 800,
	}

	parent := layoutmocks.NewMockOverlayWidget(t)
	parent.EXPECT().GetAllocatedWidth().Return(0)
	parent.EXPECT().GetAllocatedHeight().Return(0)

	width, marginTop := CalculateModalDimensions(parent, cfg)

	// Should use fallback values when allocated size is 0
	expectedWidth := int(float64(cfg.FallbackWidth) * cfg.WidthPct)
	expectedMargin := int(float64(cfg.FallbackHeight) * cfg.TopMarginPct)

	assert.Equal(t, expectedWidth, width)
	assert.Equal(t, expectedMargin, marginTop)
}

func TestCalculateModalDimensions_WidthCapped(t *testing.T) {
	cfg := ModalSizeConfig{
		WidthPct:       0.8,
		MaxWidth:       600,
		TopMarginPct:   0.1,
		FallbackWidth:  800,
		FallbackHeight: 600,
	}

	parent := layoutmocks.NewMockOverlayWidget(t)
	parent.EXPECT().GetAllocatedWidth().Return(1920) // Large screen
	parent.EXPECT().GetAllocatedHeight().Return(1080)

	width, marginTop := CalculateModalDimensions(parent, cfg)

	// 0.8 * 1920 = 1536, but capped at MaxWidth 600
	assert.Equal(t, cfg.MaxWidth, width)
	assert.Equal(t, int(float64(1080)*cfg.TopMarginPct), marginTop)
}

func TestCalculateModalDimensions_WidthPercentage(t *testing.T) {
	cfg := ModalSizeConfig{
		WidthPct:       0.6,
		MaxWidth:       1000, // High cap, won't be hit
		TopMarginPct:   0.15,
		FallbackWidth:  800,
		FallbackHeight: 600,
	}

	parent := layoutmocks.NewMockOverlayWidget(t)
	parent.EXPECT().GetAllocatedWidth().Return(800)
	parent.EXPECT().GetAllocatedHeight().Return(600)

	width, marginTop := CalculateModalDimensions(parent, cfg)

	assert.Equal(t, 480, width)
	assert.Equal(t, 90, marginTop)
}

func TestScaleValue_DefaultScale(t *testing.T) {
	// When scale is 0 or negative, should default to 1.0
	assert.Equal(t, 50, ScaleValue(50, 0))
	assert.Equal(t, 50, ScaleValue(50, -1.0))
}

func TestScaleValue_NormalScale(t *testing.T) {
	assert.Equal(t, 50, ScaleValue(50, 1.0))
}

func TestScaleValue_Scaling(t *testing.T) {
	// 1.5x scale
	assert.Equal(t, 75, ScaleValue(50, 1.5))
	// 2x scale
	assert.Equal(t, 100, ScaleValue(50, 2.0))
	// 1.2x scale (common UI scale)
	assert.Equal(t, 60, ScaleValue(50, 1.2))
}

func TestMeasureWidgetHeight_NilWidget(t *testing.T) {
	height := MeasureWidgetHeight(nil, 100)
	assert.Equal(t, 0, height)
}

func TestDefaultRowHeights_Values(t *testing.T) {
	// Ensure defaults are sensible
	assert.Positive(t, DefaultRowHeights.Standard)
	assert.Positive(t, DefaultRowHeights.Compact)
	assert.Positive(t, DefaultRowHeights.Divider)
	// Standard should be larger than compact
	assert.Greater(t, DefaultRowHeights.Standard, DefaultRowHeights.Compact)
}

func TestOmniboxSizeDefaults_Values(t *testing.T) {
	assert.Greater(t, OmniboxSizeDefaults.WidthPct, 0.0)
	assert.LessOrEqual(t, OmniboxSizeDefaults.WidthPct, 1.0)
	assert.Positive(t, OmniboxSizeDefaults.MaxWidth)
	assert.Greater(t, OmniboxSizeDefaults.TopMarginPct, 0.0)
	assert.LessOrEqual(t, OmniboxSizeDefaults.TopMarginPct, 1.0)
}

func TestSessionManagerSizeDefaults_Values(t *testing.T) {
	assert.Greater(t, SessionManagerSizeDefaults.WidthPct, 0.0)
	assert.LessOrEqual(t, SessionManagerSizeDefaults.WidthPct, 1.0)
	assert.Positive(t, SessionManagerSizeDefaults.MaxWidth)
	assert.Greater(t, SessionManagerSizeDefaults.TopMarginPct, 0.0)
	assert.LessOrEqual(t, SessionManagerSizeDefaults.TopMarginPct, 1.0)
}

func TestListDisplayDefaults_Values(t *testing.T) {
	assert.Positive(t, OmniboxListDefaults.MaxVisibleRows)
	assert.Positive(t, OmniboxListDefaults.MaxResults)
	assert.Positive(t, SessionManagerListDefaults.MaxVisibleRows)
	assert.Positive(t, SessionManagerListDefaults.MaxResults)
}

func TestEffectiveMaxRows(t *testing.T) {
	sizeCfg := ModalSizeConfig{TopMarginPct: 0.2}
	defaults := ListDisplayDefaults{
		MaxVisibleRows:      10,
		MaxResults:          10,
		SmallMaxVisibleRows: 5,
	}

	tests := []struct {
		name         string
		parentHeight int
		rowHeight    int
		want         int
	}{
		{
			name:         "tall pane returns MaxVisibleRows",
			parentHeight: 1200,
			rowHeight:    72,
			want:         10,
		},
		{
			name:         "short pane returns SmallMaxVisibleRows",
			parentHeight: 800,
			rowHeight:    72,
			want:         5,
		},
		{
			name:         "zero parentHeight returns MaxVisibleRows",
			parentHeight: 0,
			rowHeight:    72,
			want:         10,
		},
		{
			name:         "zero rowHeight returns MaxVisibleRows",
			parentHeight: 800,
			rowHeight:    0,
			want:         10,
		},
		{
			name:         "negative parentHeight returns MaxVisibleRows",
			parentHeight: -100,
			rowHeight:    72,
			want:         10,
		},
		{
			name:         "exact fit returns MaxVisibleRows",
			parentHeight: 1150, // available = 1150 - 230 - 144 = 776, needed = 720
			rowHeight:    72,
			want:         10,
		},
		{
			name:         "boundary: available equals needed still fits",
			parentHeight: 1080, // available = 1080 - 216 - 144 = 720, needed = 720 → fits
			rowHeight:    72,
			want:         10,
		},
		{
			name:         "boundary: one pixel under triggers small",
			parentHeight: 1079, // available = 1079 - 215 - 144 = 720, needed = 720 → check
			rowHeight:    72,
			want:         10, // int(1079*0.2) = 215, 1079-215-144 = 720 → still fits
		},
		{
			name:         "just below boundary triggers small",
			parentHeight: 1078, // available = 1078 - 215 - 144 = 719, needed = 720 → doesn't fit
			rowHeight:    72,
			want:         5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveMaxRows(tt.parentHeight, tt.rowHeight, sizeCfg, defaults)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEffectiveMaxRows_NoAdaptation(t *testing.T) {
	sizeCfg := ModalSizeConfig{TopMarginPct: 0.2}
	defaults := ListDisplayDefaults{
		MaxVisibleRows:      10,
		MaxResults:          10,
		SmallMaxVisibleRows: 0, // adaptation disabled
	}

	// Even with a short pane, should return MaxVisibleRows when SmallMaxVisibleRows is 0
	got := EffectiveMaxRows(500, 72, sizeCfg, defaults)
	assert.Equal(t, 10, got)
}
