package cef

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	cef2gtk "github.com/bnema/purego-cef2gtk"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/stretchr/testify/require"
)

func TestTouchpadNavigationRecognizer_ReportsProgressAndNavigatesOnReleaseAfterThreshold(t *testing.T) {
	recognizer := newTouchpadNavigationRecognizer()
	cfg := RuntimeInputConfig{
		TouchpadNavigationEnabled:          true,
		TouchpadNavigationMinDelta:         200,
		TouchpadNavigationMaxVerticalRatio: 0.5,
	}

	begin := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseBegin, 0, 0),
		Config:       cfg,
		CanGoBack:    true,
		CanGoForward: true,
	})
	require.False(t, begin.HasIndicator)
	require.False(t, begin.HasAction)

	progress := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -80, 0),
		Config:       cfg,
		CanGoBack:    true,
		CanGoForward: true,
	})
	require.True(t, progress.HasIndicator)
	require.Equal(t, port.TouchpadNavigationBack, progress.Indicator.Action)
	require.True(t, progress.Indicator.Active)
	require.InDelta(t, 0.4, progress.Indicator.Progress, 0.001)
	require.False(t, progress.Indicator.ThresholdReached)
	require.False(t, progress.HasAction)

	armed := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -140, 0),
		Config:       cfg,
		CanGoBack:    true,
		CanGoForward: true,
	})
	require.True(t, armed.HasIndicator)
	require.Equal(t, port.TouchpadNavigationBack, armed.Indicator.Action)
	require.True(t, armed.Indicator.Active)
	require.InDelta(t, 1.0, armed.Indicator.Progress, 0.001)
	require.True(t, armed.Indicator.ThresholdReached)
	require.False(t, armed.HasAction, "navigation waits for gesture release so the indicator is visible first")

	end := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseEnd, 0, 0),
		Config:       cfg,
		CanGoBack:    true,
		CanGoForward: true,
	})
	require.True(t, end.HasIndicator)
	require.False(t, end.Indicator.Active)
	require.True(t, end.HasAction)
	require.Equal(t, cef2gtk.NavigationSwipeBack, end.Action)
}

func TestTouchpadNavigationRecognizer_DoesNotNavigateBelowThreshold(t *testing.T) {
	recognizer := newTouchpadNavigationRecognizer()
	cfg := RuntimeInputConfig{
		TouchpadNavigationEnabled:          true,
		TouchpadNavigationMinDelta:         200,
		TouchpadNavigationMaxVerticalRatio: 0.5,
	}

	_ = recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseBegin, 0, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	progress := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -120, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.True(t, progress.HasIndicator)
	require.InDelta(t, 0.6, progress.Indicator.Progress, 0.001)
	require.False(t, progress.Indicator.ThresholdReached)

	end := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseEnd, 0, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.False(t, end.HasAction)
	require.True(t, end.HasIndicator)
	require.False(t, end.Indicator.Active)
}

func TestTouchpadNavigationRecognizer_CancelsVerticalGestures(t *testing.T) {
	recognizer := newTouchpadNavigationRecognizer()
	cfg := RuntimeInputConfig{
		TouchpadNavigationEnabled:          true,
		TouchpadNavigationMinDelta:         200,
		TouchpadNavigationMaxVerticalRatio: 0.5,
	}

	_ = recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseBegin, 0, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	vertical := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -60, 80),
		Config:    cfg,
		CanGoBack: true,
	})
	require.False(t, vertical.HasIndicator)
	require.False(t, vertical.HasAction)

	continued := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -400, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.False(t, continued.HasIndicator)
	require.False(t, continued.HasAction)

	end := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseEnd, 0, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.False(t, end.HasAction)
}

func touchpadNavigationScrollEvent(phase cef2gtk.ScrollPhase, dx, dy float64) cef2gtk.ScrollEvent {
	return cef2gtk.ScrollEvent{
		Phase:     phase,
		DX:        dx,
		DY:        dy,
		Unit:      gdk.ScrollUnitSurfaceValue,
		UnitKnown: true,
	}
}
