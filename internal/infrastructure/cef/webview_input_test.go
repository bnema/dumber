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
			ScrollTouchpadMultiplier:           0.35,
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
	require.InDelta(t, 0.35, opts.Scroll.TouchpadMultiplier, 0.001)
	require.InDelta(t, 0.75, opts.Scroll.HorizontalMultiplier, 0.001)
	require.InDelta(t, 1.5, opts.Scroll.VerticalMultiplier, 0.001)
	require.Equal(t, int32(120), opts.Scroll.MaxDelta)
	require.NotNil(t, opts.OnMiddleClick)
	require.NotNil(t, opts.OnTouchpadSwipe)
	require.NotNil(t, opts.SelectionText)
	require.NotNil(t, opts.OnClipboardShortcut)
	require.Same(t, nativeWidget, wv.nativeWidget)
}

func TestChooseTouchpadNavigationAction(t *testing.T) {
	cfg := RuntimeInputConfig{
		TouchpadNavigationEnabled:          true,
		TouchpadNavigationMinDelta:         80,
		TouchpadNavigationMaxVerticalRatio: 0.5,
	}

	tests := []struct {
		name         string
		cfg          *RuntimeInputConfig
		dx, dy       float64
		canGoBack    bool
		canGoForward bool
		want         touchpadNavigationAction
	}{
		{name: "right swipe goes back", dx: 100, dy: 10, canGoBack: true, want: touchpadNavigationBack},
		{name: "left swipe goes forward", dx: -100, dy: 10, canGoForward: true, want: touchpadNavigationForward},
		{name: "below threshold ignored", dx: 40, dy: 0, canGoBack: true, want: touchpadNavigationNone},
		{name: "disabled ignored", cfg: &RuntimeInputConfig{TouchpadNavigationEnabled: false, TouchpadNavigationMinDelta: 80, TouchpadNavigationMaxVerticalRatio: 0.5}, dx: 100, dy: 0, canGoBack: true, want: touchpadNavigationNone},
		{name: "exact threshold goes back", dx: 80, dy: 0, canGoBack: true, want: touchpadNavigationBack},
		{name: "vertical diagonal ignored", dx: 100, dy: 80, canGoBack: true, want: touchpadNavigationNone},
		{name: "unavailable back ignored", dx: 100, dy: 0, canGoBack: false, want: touchpadNavigationNone},
		{name: "unavailable forward ignored", dx: -100, dy: 0, canGoForward: false, want: touchpadNavigationNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCfg := cfg
			if tt.cfg != nil {
				testCfg = *tt.cfg
			}
			got := chooseTouchpadNavigationAction(testCfg, tt.dx, tt.dy, tt.canGoBack, tt.canGoForward)
			require.Equal(t, tt.want, got)
		})
	}
}
