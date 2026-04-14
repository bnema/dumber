package webkit

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/require"
)

func TestBuildButtons(t *testing.T) {
	items := []port.MenuItem{
		{Action: port.MenuActionBack, Label: "Back"},
		{},
		{Action: port.MenuActionReload, Label: "Reload"},
		{Action: port.MenuActionInspectElement, Label: "Inspect Element"},
	}

	renderItems := buildButtons(items)
	require.Len(t, renderItems, 4)

	require.Equal(t, "Back", renderItems[0].item.Label)
	require.False(t, renderItems[0].separator)

	require.True(t, renderItems[1].separator)

	require.Equal(t, "Reload", renderItems[2].item.Label)
	require.False(t, renderItems[2].separator)

	require.Equal(t, "Inspect Element", renderItems[3].item.Label)
	require.False(t, renderItems[3].separator)
}

func TestChoosePopoverParent(t *testing.T) {
	anchor := &gtk.Widget{}
	parent := &gtk.Widget{}

	require.Same(t, parent, choosePopoverParent(anchor, parent))
	require.Same(t, anchor, choosePopoverParent(anchor, nil))
}

func TestPopoverPointingRect(t *testing.T) {
	t.Run("uses translated parent coordinates when available", func(t *testing.T) {
		rect := popoverPointingRect(50, 80, func(srcX, srcY float64) (float64, float64, bool) {
			require.InDelta(t, 50.0, srcX, 0.0001)
			require.InDelta(t, 80.0, srcY, 0.0001)
			return 20, 30, true
		})

		require.Equal(t, 20, rect.X)
		require.Equal(t, 30, rect.Y)
		require.Equal(t, 1, rect.Width)
		require.Equal(t, 1, rect.Height)
	})

	t.Run("falls back to raw coordinates when translation fails", func(t *testing.T) {
		rect := popoverPointingRect(12, 34, func(srcX, srcY float64) (float64, float64, bool) {
			return srcX, srcY, false
		})

		require.Equal(t, 12, rect.X)
		require.Equal(t, 34, rect.Y)
	})
}
