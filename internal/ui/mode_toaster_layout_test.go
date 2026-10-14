package ui

import (
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/stretchr/testify/require"
)

func TestLegendColumnCount(t *testing.T) {
	for _, tc := range []struct {
		width, want int
	}{
		{0, 1},
		{200, 1},
		{legendPanelPadding + legendMinColumnWidth, 1},
		{legendPanelPadding + legendMinColumnWidth + legendColumnGap + legendMinColumnWidth - 1, 1},
		{legendPanelPadding + 2*legendMinColumnWidth + legendColumnGap, 2},
		{legendPanelPadding + 3*legendMinColumnWidth + 2*legendColumnGap, 3},
		{legendPanelPadding + 4*legendMinColumnWidth + 3*legendColumnGap, 4},
		{legendMaxWidth, 4},
		{5000, legendMaxColumns},
	} {
		require.Equal(t, tc.want, legendColumnCount(tc.width), "width %d", tc.width)
	}
}

func TestLegendColumnCountIsMonotonic(t *testing.T) {
	previous := 0
	for width := 0; width <= 2000; width++ {
		got := legendColumnCount(width)
		require.GreaterOrEqual(t, got, previous, "width %d", width)
		require.GreaterOrEqual(t, got, 1)
		require.LessOrEqual(t, got, legendMaxColumns)
		previous = got
	}
}

func TestLegendBandWidthIsCappedAndNeverFullWidth(t *testing.T) {
	width, columns := legendBandWidth(2400, 10)
	require.Equal(t, legendMaxWidth, width)
	require.Equal(t, 4, columns)

	width, columns = legendBandWidth(800, 10)
	require.Equal(t, 800, width)
	require.Equal(t, legendColumnCount(800), columns)

	width, columns = legendBandWidth(300, 10)
	require.Equal(t, 300, width)
	require.Equal(t, 1, columns)
}

func TestLegendBandWidthShrinksForFewGroups(t *testing.T) {
	width, columns := legendBandWidth(2400, 2)
	require.Equal(t, 2, columns)
	require.Less(t, width, legendMaxWidth)
	require.GreaterOrEqual(t, legendColumnCount(width), columns, "the shrunk band still fits its columns")

	width, columns = legendBandWidth(2400, 1)
	require.Equal(t, 1, columns)
	require.GreaterOrEqual(t, width, legendMinColumnWidth+legendPanelPadding)
}

func TestLegendBandWidthWithoutRoom(t *testing.T) {
	width, columns := legendBandWidth(0, 5)
	require.Zero(t, width)
	require.Zero(t, columns)
	width, columns = legendBandWidth(500, 0)
	require.Zero(t, width)
	require.Zero(t, columns)
}

func TestLegendBodyMaxHeight(t *testing.T) {
	require.Equal(t, 1000*45/100-legendChromeHeight, legendBodyMaxHeight(1000))
	require.Equal(t, legendMinBodyHeight, legendBodyMaxHeight(100), "a tiny window still shows a few rows")
}

// columnsOf flattens a distribution back into group order.
func flattenDistribution(columns [][]int) []int {
	var out []int
	for _, column := range columns {
		out = append(out, column...)
	}
	return out
}

func tallest(heights []int, columns [][]int) int {
	best := 0
	for _, column := range columns {
		sum := 0
		for _, i := range column {
			sum += heights[i]
		}
		best = max(best, sum)
	}
	return best
}

func TestDistributeLegendGroupsKeepsOrderAndNeverSplitsGroups(t *testing.T) {
	heights := []int{4, 1, 7, 3, 2, 9, 2, 5, 1, 3}
	want := make([]int, len(heights))
	for i := range want {
		want[i] = i
	}
	for columns := 1; columns <= legendMaxColumns; columns++ {
		got := distributeLegendGroups(heights, columns)
		require.Len(t, got, columns)
		require.Equal(t, want, flattenDistribution(got), "columns=%d keeps the fixed group order, each group once", columns)
		for _, column := range got {
			require.NotEmpty(t, column, "no empty column")
		}
	}
}

func TestDistributeLegendGroupsBalancesColumnHeights(t *testing.T) {
	// One 8-row group next to four 1-row groups, in two columns: the tall
	// group alone is the tallest column, the rest share the other.
	heights := []int{8, 2, 2, 2, 2}
	got := distributeLegendGroups(heights, 2)
	require.Equal(t, [][]int{{0}, {1, 2, 3, 4}}, got)
	require.Equal(t, 8, tallest(heights, got))

	// Even groups split evenly.
	got = distributeLegendGroups([]int{3, 3, 3, 3}, 2)
	require.Equal(t, [][]int{{0, 1}, {2, 3}}, got)
}

func TestDistributeLegendGroupsMinimizesTallestColumn(t *testing.T) {
	heights := []int{5, 1, 1, 1, 6, 2, 2, 4}
	for columns := 1; columns <= 4; columns++ {
		got := distributeLegendGroups(heights, columns)
		require.Equal(t, bruteForceTallest(heights, columns), tallest(heights, got), "columns=%d", columns)
	}
}

// bruteForceTallest tries every way to cut heights into columns runs.
func bruteForceTallest(heights []int, columns int) int {
	if columns == 1 {
		sum := 0
		for _, h := range heights {
			sum += h
		}
		return sum
	}
	best := -1
	sum := 0
	for cut := 1; cut <= len(heights)-columns+1; cut++ {
		sum += heights[cut-1]
		rest := bruteForceTallest(heights[cut:], columns-1)
		candidate := max(sum, rest)
		if best < 0 || candidate < best {
			best = candidate
		}
	}
	return best
}

func TestDistributeLegendGroupsIsDeterministic(t *testing.T) {
	heights := []int{2, 2, 2, 2, 2, 2}
	first := distributeLegendGroups(heights, 3)
	for range 20 {
		require.Equal(t, first, distributeLegendGroups(heights, 3))
	}
	require.Equal(t, [][]int{{0, 1}, {2, 3}, {4, 5}}, first)
}

func TestDistributeLegendGroupsEdgeCases(t *testing.T) {
	require.Nil(t, distributeLegendGroups(nil, 3))
	require.Nil(t, distributeLegendGroups([]int{2, 3}, 0))
	// More columns than groups: one group per column.
	require.Equal(t, [][]int{{0}, {1}}, distributeLegendGroups([]int{2, 3}, 4))
	require.Equal(t, [][]int{{0}}, distributeLegendGroups([]int{7}, 3))
	require.Equal(t, [][]int{{0, 1, 2}}, distributeLegendGroups([]int{1, 1, 1}, 1))
}

func TestBuildLegendGroupsOrdersGroupsAndRows(t *testing.T) {
	actions := map[string]entity.ActionBinding{
		"enter-tab-mode":  {Keys: []string{"t"}, Desc: "Tab mode"},
		"split-right":     {Keys: []string{"r"}},
		"split-left":      {Keys: []string{"l"}},
		"focus-up":        {Keys: []string{"k"}},
		"confirm":         {Keys: []string{"enter"}},
		"close-pane":      {Keys: []string{"x"}},
		"unbound":         {},
		"resize-increase": {Keys: []string{"+"}},
	}
	groups := buildLegendGroups(input.ModePane, actions)
	var names []string
	for _, group := range groups {
		names = append(names, group.Name)
	}
	require.Equal(t, []string{"SPLIT", "FOCUS", "RESIZE", "MANAGE", "MODES", "EXIT"}, names)
	require.Equal(t, "split left", groups[0].Rows[0].Desc, "rows are sorted and fall back to the action name")
	require.Equal(t, "split-left", groups[0].Rows[0].Action)
	require.Equal(t, "Tab mode", groups[4].Rows[0].Desc)
	require.Equal(t, []int{3, 2, 2, 2, 2, 2}, legendGroupHeights(groups))
}

func TestBuildLegendGroupsEmpty(t *testing.T) {
	require.Empty(t, buildLegendGroups(input.ModeVim, nil))
	require.Empty(t, buildLegendGroups(input.ModeVim, map[string]entity.ActionBinding{"visual": {}}))
}

func TestModeToasterVisibility(t *testing.T) {
	on := entity.WorkspaceStylingConfig{ModeIndicatorToasterEnabled: true, ModeLegend: "delay"}
	require.True(t, modeToasterVisible(input.ModeVim, on))
	require.False(t, modeToasterVisible(input.ModeNormal, on))

	// The indicator preference off, legend on: the pill is the legend's header.
	headerOnly := entity.WorkspaceStylingConfig{ModeLegend: "always"}
	require.True(t, modeToasterVisible(input.ModePane, headerOnly))

	// Both off: nothing.
	require.False(t, modeToasterVisible(input.ModePane, entity.WorkspaceStylingConfig{ModeLegend: "off"}))
	// Legend off, indicator on: plain pill.
	require.True(t, modeToasterVisible(input.ModePane, entity.WorkspaceStylingConfig{ModeIndicatorToasterEnabled: true, ModeLegend: "off"}))
}

func TestLegendBodyShown(t *testing.T) {
	require.True(t, legendBodyShown(true, false, false))
	require.False(t, legendBodyShown(false, false, false), "show delay not elapsed")
	require.False(t, legendBodyShown(true, true, false), "collapsed")
	require.False(t, legendBodyShown(true, false, true), "suspended by a page interaction")
}

func TestNextModeLegendCollapsed(t *testing.T) {
	require.True(t, nextModeLegendCollapsed(false))
	require.False(t, nextModeLegendCollapsed(true))
}
