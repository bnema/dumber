package component

import (
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// ModalSizeConfig holds configuration for modal sizing calculations.
type ModalSizeConfig struct {
	WidthPct       float64 // Percentage of parent width (e.g., 0.6)
	MaxWidth       int     // Maximum width in pixels
	TopMarginPct   float64 // Top margin as percentage of parent height
	FallbackWidth  int     // Fallback when parent not allocated
	FallbackHeight int     // Fallback height
}

// RowHeightDefaults holds default (unscaled) row heights for list-based modals.
// These are base pixel values that should be multiplied by UI scale.
type RowHeightDefaults struct {
	Standard int // Standard list row (e.g., session row, history row)
	Compact  int // Compact row (e.g., tree row, sub-item)
	Divider  int // Divider/separator row
}

// ListDisplayDefaults holds display limits for list-based modals.
type ListDisplayDefaults struct {
	MaxVisibleRows      int // Maximum rows visible before scrolling
	MaxResults          int // Maximum results to fetch/display
	SmallMaxVisibleRows int // Reduced row count when rows don't fit (0 = no adaptation)
}

// Package-level defaults for row heights.
var DefaultRowHeights = RowHeightDefaults{
	Standard: 50,
	Compact:  28,
	Divider:  30,
}

// OmniboxSizeDefaults provides default sizing for omnibox modal.
var OmniboxSizeDefaults = ModalSizeConfig{
	WidthPct:       0.8,
	MaxWidth:       800,
	TopMarginPct:   0.2,
	FallbackWidth:  800,
	FallbackHeight: 600,
}

// OmniboxListDefaults provides display limits for omnibox modal.
var OmniboxListDefaults = ListDisplayDefaults{
	MaxVisibleRows:      10,
	MaxResults:          10,
	SmallMaxVisibleRows: 5,
}

// SessionManagerSizeDefaults provides default sizing for session manager modal.
var SessionManagerSizeDefaults = ModalSizeConfig{
	WidthPct:       0.6,
	MaxWidth:       600,
	TopMarginPct:   0.15,
	FallbackWidth:  600,
	FallbackHeight: 600,
}

// SessionManagerListDefaults provides display limits for session manager modal.
var SessionManagerListDefaults = ListDisplayDefaults{
	MaxVisibleRows: 6,
	MaxResults:     50,
}

// TabPickerSizeDefaults provides default sizing for tab picker modal.
var TabPickerSizeDefaults = ModalSizeConfig{
	WidthPct:       0.6,
	MaxWidth:       600,
	TopMarginPct:   0.15,
	FallbackWidth:  600,
	FallbackHeight: 600,
}

// TabPickerListDefaults provides display limits for tab picker modal.
var TabPickerListDefaults = ListDisplayDefaults{
	MaxVisibleRows: 8,
	MaxResults:     20,
}

// PermissionPopupSizeDefaults provides default sizing for permission popup modal.
var PermissionPopupSizeDefaults = ModalSizeConfig{
	WidthPct:       0.4,
	MaxWidth:       400,
	TopMarginPct:   0.3,
	FallbackWidth:  400,
	FallbackHeight: 300,
}

// EffectiveMaxRows returns the maximum number of visible rows that fit
// in the available space. It computes available height by subtracting the
// top margin (TopMarginPct of parentHeight) and chrome height (header tabs +
// search entry, estimated as 2× rowHeight to scale with the UI). If
// MaxVisibleRows worth of rows don't fit, it falls back to SmallMaxVisibleRows.
func EffectiveMaxRows(parentHeight, rowHeight int, sizeCfg ModalSizeConfig, defaults ListDisplayDefaults) int {
	if parentHeight <= 0 || rowHeight <= 0 || defaults.SmallMaxVisibleRows <= 0 {
		return defaults.MaxVisibleRows
	}
	topMargin := int(float64(parentHeight) * sizeCfg.TopMarginPct)
	chromeHeight := 2 * rowHeight // header tabs + search entry, scales with UI
	available := parentHeight - topMargin - chromeHeight
	needed := defaults.MaxVisibleRows * rowHeight
	if available < needed {
		return defaults.SmallMaxVisibleRows
	}
	return defaults.MaxVisibleRows
}

// CalculateModalDimensions computes width and top margin based on parent overlay.
// Returns calculated width and top margin in pixels.
func CalculateModalDimensions(parent layout.OverlayWidget, cfg ModalSizeConfig) (width, marginTop int) {
	var parentWidth, parentHeight int

	if parent != nil {
		parentWidth = parent.GetAllocatedWidth()
		parentHeight = parent.GetAllocatedHeight()
	}

	// Use fallback if parent not allocated or too small to be useful
	// A width < 100 is too small for any meaningful modal
	if parentWidth < 100 {
		parentWidth = cfg.FallbackWidth
	}
	if parentHeight < 100 {
		parentHeight = cfg.FallbackHeight
	}

	width = int(float64(parentWidth) * cfg.WidthPct)
	if width > cfg.MaxWidth {
		width = cfg.MaxWidth
	}

	marginTop = int(float64(parentHeight) * cfg.TopMarginPct)
	return width, marginTop
}

// CalculateOverlayDimensions computes width and height from overlay allocation percentages.
func CalculateOverlayDimensions(
	parent layout.OverlayWidget,
	widthPct, heightPct float64,
	fallbackWidth, fallbackHeight int,
) (width, height int) {
	parentWidth := fallbackWidth
	parentHeight := fallbackHeight

	if parent != nil {
		if allocatedWidth := parent.GetAllocatedWidth(); allocatedWidth >= 100 {
			parentWidth = allocatedWidth
		}
		if allocatedHeight := parent.GetAllocatedHeight(); allocatedHeight >= 100 {
			parentHeight = allocatedHeight
		}
	}

	if widthPct <= 0 || widthPct > 1 {
		widthPct = 1.0
	}
	if heightPct <= 0 || heightPct > 1 {
		heightPct = 1.0
	}

	width = int(float64(parentWidth) * widthPct)
	height = int(float64(parentHeight) * heightPct)
	return width, height
}

// SetScrolledWindowHeight safely sets min/max content height on a ScrolledWindow.
// Resets min to -1 first to avoid GTK assertion (min <= max) when shrinking.
func SetScrolledWindowHeight(sw *gtk.ScrolledWindow, height int) {
	if sw == nil {
		return
	}
	sw.SetMinContentHeight(-1)
	sw.SetMaxContentHeight(height)
	sw.SetMinContentHeight(height)
}

// MeasureWidgetHeight returns the natural height of a widget for a given width.
// Returns 0 if widget is nil or measurement fails.
func MeasureWidgetHeight(widget *gtk.Widget, forWidth int) int {
	if widget == nil {
		return 0
	}
	var minH, natH int
	widget.Measure(gtk.OrientationVerticalValue, forWidth, &minH, &natH, nil, nil)
	return natH
}

// ScaleValue scales a base pixel value by UI scale factor.
// Returns the base value if scale is <= 0.
func ScaleValue(base int, uiScale float64) int {
	if uiScale <= 0 {
		uiScale = 1.0
	}
	return int(float64(base) * uiScale)
}
