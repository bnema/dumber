package cef

import (
	"testing"

	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/require"
)

func TestWebViewBridgeInputOptions_LeavesTargetSelectionToAdapter(t *testing.T) {
	nativeWidget := &gtk.Widget{}
	wv := &WebView{
		nativeWidget: nativeWidget,
		inputConfig: RuntimeInputConfig{
			ScrollWheelMultiplier:              1.25,
			ScrollPreciseMultiplier:            2.5,
			ScrollHorizontalMultiplier:         0.75,
			ScrollVerticalMultiplier:           1.5,
			ScrollMaxDelta:                     120,
			TouchpadNavigationEnabled:          true,
			TouchpadNavigationMinDelta:         80,
			TouchpadNavigationMaxVerticalRatio: 0.5,
		},
	}

	opts := wv.bridgeInputOptions()

	require.Zero(t, opts.Scale)
	require.InDelta(t, 1.25, opts.Scroll.WheelMultiplier, 0.001)
	require.InDelta(t, 2.5, opts.Scroll.PreciseMultiplier, 0.001)
	require.InDelta(t, 0.75, opts.Scroll.HorizontalMultiplier, 0.001)
	require.InDelta(t, 1.5, opts.Scroll.VerticalMultiplier, 0.001)
	require.Equal(t, int32(120), opts.Scroll.MaxDelta)
	require.NotNil(t, opts.OnMiddleClick)
	require.NotNil(t, opts.OnScroll)
	require.True(t, opts.NavigationSwipe.Enabled)
	require.InDelta(t, 80.0, opts.NavigationSwipe.MinDelta, 0.001)
	require.InDelta(t, 0.5, opts.NavigationSwipe.MaxVerticalRatio, 0.001)
	require.NotNil(t, opts.CanNavigateBack)
	require.NotNil(t, opts.CanNavigateForward)
	require.NotNil(t, opts.OnNavigateSwipe)
	require.NotNil(t, opts.SelectionText)
	require.NotNil(t, opts.OnClipboardShortcut)
	require.Same(t, nativeWidget, wv.nativeWidget)
}
