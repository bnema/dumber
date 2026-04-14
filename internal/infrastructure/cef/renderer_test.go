package cef

import (
	"testing"

	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/require"
)

func TestChoosePopoverParent(t *testing.T) {
	anchor := &gtk.Widget{}
	parent := &gtk.Widget{}

	require.Same(t, parent, choosePopoverParent(anchor, parent))
	require.Same(t, anchor, choosePopoverParent(anchor, nil))
}

func TestPopoverPointingRect(t *testing.T) {
	t.Run("uses translated parent coordinates when available", func(t *testing.T) {
		rect := popoverPointingRect(64, 96, func(srcX, srcY float64) (float64, float64, bool) {
			require.InDelta(t, 64.0, srcX, 0.0001)
			require.InDelta(t, 96.0, srcY, 0.0001)
			return 24, 48, true
		})

		require.Equal(t, 24, rect.X)
		require.Equal(t, 48, rect.Y)
		require.Equal(t, 1, rect.Width)
		require.Equal(t, 1, rect.Height)
	})

	t.Run("falls back to raw coordinates when translation fails", func(t *testing.T) {
		rect := popoverPointingRect(10, 20, func(srcX, srcY float64) (float64, float64, bool) {
			return srcX, srcY, false
		})

		require.Equal(t, 10, rect.X)
		require.Equal(t, 20, rect.Y)
	})
}
