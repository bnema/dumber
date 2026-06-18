package cef

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/bnema/purego-cef2gtk"
	"github.com/bnema/puregotk/v4/gdk"
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
	require.False(t, opts.NavigationSwipe.Enabled)
	require.Zero(t, opts.NavigationSwipe.MinDelta)
	require.Zero(t, opts.NavigationSwipe.MaxVerticalRatio)
	require.Nil(t, opts.CanNavigateBack)
	require.Nil(t, opts.CanNavigateForward)
	require.Nil(t, opts.OnNavigateSwipe)
	require.NotNil(t, opts.SelectionText)
	require.NotNil(t, opts.OnClipboardShortcut)
	require.Same(t, nativeWidget, wv.nativeWidget)
}

func TestWebViewHandleScrollInput_ForwardsOrdinaryHorizontalScroll(t *testing.T) {
	wv := &WebView{
		ctx: context.Background(),
		inputConfig: RuntimeInputConfig{
			TouchpadNavigationEnabled:          true,
			TouchpadNavigationMinDelta:         200,
			TouchpadNavigationMaxVerticalRatio: 0.5,
		},
		canGoBack: true,
	}

	// This verifies the bridge-level forwarding contract for ordinary scroll
	// updates. The nil native widget leaves view width unknown, so local history
	// gesture recognition fails closed and the event continues to CEF.
	decision := wv.handleScrollInput(cef2gtk.ScrollEvent{
		Phase:     cef2gtk.ScrollPhaseUpdate,
		X:         500,
		DX:        -240,
		Unit:      gdk.ScrollUnitSurfaceValue,
		UnitKnown: true,
	})

	require.Equal(t, cef2gtk.ScrollForwardToCEF, decision)
}

func TestWebViewHandleScrollInput_EmitsFinalGestureBeforeNavigation(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)
	events := make([]string, 0, 2)
	browser.EXPECT().GoBack().Run(func() {
		events = append(events, "go-back")
	}).Once()

	wv := &WebView{
		ctx: context.Background(),
		inputConfig: RuntimeInputConfig{
			TouchpadNavigationEnabled:          true,
			TouchpadNavigationMinDelta:         200,
			TouchpadNavigationMaxVerticalRatio: 0.5,
		},
		canGoBack: true,
		browser:   browser,
		callbacks: &port.WebViewCallbacks{
			OnTouchpadNavigationGesture: func(gesture dto.TouchpadNavigationGesture) {
				if !gesture.Active && gesture.ThresholdReached {
					events = append(events, "indicator-finished")
				}
			},
		},
	}

	begin := wv.handleScrollInput(cef2gtk.ScrollEvent{
		Phase:     cef2gtk.ScrollPhaseBegin,
		X:         0,
		Unit:      gdk.ScrollUnitSurfaceValue,
		UnitKnown: true,
	})
	require.Equal(t, cef2gtk.ScrollForwardToCEF, begin)

	// Avoid constructing GTK widgets in this package: the WebView integration
	// only needs a known gesture width after begin records the starting edge.
	wv.mu.Lock()
	require.NotNil(t, wv.touchpadNavigation)
	wv.touchpadNavigation.gestureViewWidth = 1000
	wv.mu.Unlock()

	update := wv.handleScrollInput(cef2gtk.ScrollEvent{
		Phase:     cef2gtk.ScrollPhaseUpdate,
		X:         0,
		DX:        -240,
		Unit:      gdk.ScrollUnitSurfaceValue,
		UnitKnown: true,
	})
	require.Equal(t, cef2gtk.ScrollForwardToCEF, update)

	end := wv.handleScrollInput(cef2gtk.ScrollEvent{
		Phase:     cef2gtk.ScrollPhaseEnd,
		X:         0,
		Unit:      gdk.ScrollUnitSurfaceValue,
		UnitKnown: true,
	})
	require.Equal(t, cef2gtk.ScrollForwardToCEF, end)
	require.Equal(t, []string{"indicator-finished", "go-back"}, events)
}
