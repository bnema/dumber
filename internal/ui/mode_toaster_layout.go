package ui

import (
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/input"
)

// Legend band geometry. The legend is a wide band anchored on the mode toaster,
// not a full-width bar: its width follows the window up to a cap, and its
// column count follows its width.
const (
	// legendMaxWidth caps the band width.
	legendMaxWidth = 1100
	// legendMinColumnWidth is the narrowest a column may get before the band
	// drops a column and grows taller instead.
	legendMinColumnWidth = 240
	// legendComfortColumnWidth is the column width the band shrinks to when the
	// mode has fewer groups than the width could hold columns.
	legendComfortColumnWidth = 360
	legendMaxColumns         = 4
	// legendColumnGap is the horizontal space between columns.
	legendColumnGap = 12
	// legendPanelPadding is the horizontal chrome (border and padding) around
	// the columns.
	legendPanelPadding = 28
	// legendMaxHeightPercent bounds the legend's scrolling body, as a share of
	// the content height.
	legendMaxHeightPercent = 45
	// legendChromeHeight is the vertical chrome (border, padding, hint line)
	// around the scrolling body, subtracted from the height budget.
	legendChromeHeight  = 40
	legendMinBodyHeight = 60
	// modeToasterMargin separates the toaster from the window edges, and the
	// legend band from the right edge.
	modeToasterMargin = 8
)

// legendColumnCount returns how many columns of at least legendMinColumnWidth
// fit a band of panelWidth, from 1 to legendMaxColumns.
func legendColumnCount(panelWidth int) int {
	columns := (panelWidth - legendPanelPadding + legendColumnGap) / (legendMinColumnWidth + legendColumnGap)
	return min(max(columns, 1), legendMaxColumns)
}

// legendBandWidth returns the band width and column count for a legend of
// groups groups when availableWidth pixels lie between the toaster's left edge
// and the right margin. A band for few groups does not stretch over columns it
// cannot fill.
func legendBandWidth(availableWidth, groups int) (width, columns int) {
	width = min(availableWidth, legendMaxWidth)
	if width <= 0 || groups <= 0 {
		return 0, 0
	}
	columns = legendColumnCount(width)
	if groups < columns {
		columns = groups
		width = min(width, legendPanelPadding+columns*legendComfortColumnWidth+(columns-1)*legendColumnGap)
	}
	return width, columns
}

// legendBodyMaxHeight returns the height past which the legend body scrolls.
func legendBodyMaxHeight(contentHeight int) int {
	return max(contentHeight*legendMaxHeightPercent/100-legendChromeHeight, legendMinBodyHeight)
}

// distributeLegendGroups splits groups, given by their heights in display
// order, into at most columns columns. Each column holds consecutive groups, so
// the order reads down the first column, then the next, and no group is split.
// Among all splits it picks the one with the shortest tallest column, then the
// most even one; remaining ties go to the earliest split. The result lists
// group indices per column and has no empty column.
func distributeLegendGroups(heights []int, columns int) [][]int {
	if len(heights) == 0 || columns < 1 {
		return nil
	}
	columns = min(columns, len(heights))
	best := legendSplit{tallest: -1}
	cuts := make([]int, 0, columns-1)
	var search func(start int)
	search = func(start int) {
		if len(cuts) == columns-1 {
			candidate := scoreLegendSplit(heights, cuts)
			if best.tallest < 0 || candidate.betterThan(best) {
				best = candidate
			}
			return
		}
		// Leave one group for each column still to come.
		remaining := columns - 1 - len(cuts)
		for cut := start; cut <= len(heights)-remaining; cut++ {
			cuts = append(cuts, cut)
			search(cut + 1)
			cuts = cuts[:len(cuts)-1]
		}
	}
	search(1)

	out := make([][]int, 0, columns)
	first := 0
	for _, cut := range append(best.cuts, len(heights)) {
		column := make([]int, 0, cut-first)
		for i := first; i < cut; i++ {
			column = append(column, i)
		}
		out = append(out, column)
		first = cut
	}
	return out
}

type legendSplit struct {
	cuts       []int
	tallest    int
	sumSquares int
}

func (s legendSplit) betterThan(other legendSplit) bool {
	if s.tallest != other.tallest {
		return s.tallest < other.tallest
	}
	return s.sumSquares < other.sumSquares
}

func scoreLegendSplit(heights, cuts []int) legendSplit {
	split := legendSplit{cuts: append([]int(nil), cuts...)}
	first, sum := 0, 0
	for _, cut := range append(append([]int(nil), cuts...), len(heights)) {
		sum = 0
		for _, h := range heights[first:cut] {
			sum += h
		}
		split.tallest = max(split.tallest, sum)
		split.sumSquares += sum * sum
		first = cut
	}
	return split
}

// modeToasterVisible reports whether the toaster is shown for mode. The
// indicator preference alone does not hide it while a legend can expand from
// it: the toaster is the legend's header.
func modeToasterVisible(mode input.Mode, cfg entity.WorkspaceStylingConfig) bool {
	return mode != input.ModeNormal && (cfg.ModeIndicatorToasterEnabled || modeLegendEnabled(cfg))
}

// modeLegendEnabled reports whether a legend can expand from the toaster.
// With the legend off the toaster is a plain indicator: no chevron, no toggle.
func modeLegendEnabled(cfg entity.WorkspaceStylingConfig) bool {
	return cfg.ModeLegend != "off"
}

// legendBodyShown reports whether the expanded legend is on screen. ready is
// set once the legend's show delay has elapsed. A suspended legend (an in-page
// interaction owns the keys) and a collapsed one stay hidden.
func legendBodyShown(ready, collapsed, suspended bool) bool {
	return ready && !collapsed && !suspended
}

// nextModeLegendCollapsed is the collapsed state after a click on the toaster.
func nextModeLegendCollapsed(collapsed bool) bool {
	return !collapsed
}
