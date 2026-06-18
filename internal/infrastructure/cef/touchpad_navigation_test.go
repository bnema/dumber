package cef

import (
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
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
		ViewWidth:    1000,
	})
	require.False(t, begin.HasIndicator)
	require.False(t, begin.HasAction)

	ordinaryScroll := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -80, 0),
		Config:       cfg,
		CanGoBack:    true,
		CanGoForward: true,
	})
	require.False(t, ordinaryScroll.HasIndicator, "ordinary horizontal scrolling below the activation distance should not show navigation UI")
	require.False(t, ordinaryScroll.HasAction)

	progress := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -40, 0),
		Config:       cfg,
		CanGoBack:    true,
		CanGoForward: true,
	})
	require.True(t, progress.HasIndicator)
	require.Equal(t, entity.TouchpadNavigationBack, progress.Indicator.Action)
	require.True(t, progress.Indicator.Active)
	require.InDelta(t, 0.6, progress.Indicator.Progress, 0.001)
	require.False(t, progress.Indicator.ThresholdReached)
	require.False(t, progress.HasAction)

	armed := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -100, 0),
		Config:       cfg,
		CanGoBack:    true,
		CanGoForward: true,
	})
	require.True(t, armed.HasIndicator)
	require.Equal(t, entity.TouchpadNavigationBack, armed.Indicator.Action)
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

func TestTouchpadNavigationRecognizer_ReportsForwardProgressAndNavigatesOnReleaseAfterThreshold(t *testing.T) {
	recognizer := newTouchpadNavigationRecognizer()
	cfg := RuntimeInputConfig{
		TouchpadNavigationEnabled:          true,
		TouchpadNavigationMinDelta:         200,
		TouchpadNavigationMaxVerticalRatio: 0.5,
	}

	begin := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseBegin, 976, 0, 0),
		Config:       cfg,
		CanGoBack:    true,
		CanGoForward: true,
		ViewWidth:    1000,
	})
	require.False(t, begin.HasIndicator)
	require.False(t, begin.HasAction)

	armed := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseUpdate, 976, 240, 0),
		Config:       cfg,
		CanGoBack:    true,
		CanGoForward: true,
	})
	require.True(t, armed.HasIndicator)
	require.Equal(t, entity.TouchpadNavigationForward, armed.Indicator.Action)
	require.True(t, armed.Indicator.Active)
	require.InDelta(t, 1.0, armed.Indicator.Progress, 0.001)
	require.True(t, armed.Indicator.ThresholdReached)
	require.False(t, armed.HasAction)

	end := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseEnd, 976, 0, 0),
		Config:       cfg,
		CanGoBack:    true,
		CanGoForward: true,
	})
	require.True(t, end.HasIndicator)
	require.False(t, end.Indicator.Active)
	require.True(t, end.HasAction)
	require.Equal(t, cef2gtk.NavigationSwipeForward, end.Action)
}

func TestTouchpadNavigationRecognizer_DoesNotNavigateAtExactThreshold(t *testing.T) {
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
		ViewWidth: 1000,
	})
	exact := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -200, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.True(t, exact.HasIndicator)
	require.InDelta(t, 1.0, exact.Indicator.Progress, 0.001)
	require.False(t, exact.Indicator.ThresholdReached)

	end := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseEnd, 0, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.True(t, end.HasIndicator)
	require.False(t, end.Indicator.Active)
	require.False(t, end.Indicator.ThresholdReached)
	require.False(t, end.HasAction)
}

func TestTouchpadNavigationRecognizer_DoesNotShowIndicatorBelowActivationDistance(t *testing.T) {
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
		ViewWidth: 1000,
	})
	ordinaryScroll := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -80, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.False(t, ordinaryScroll.HasIndicator)
	require.False(t, ordinaryScroll.HasAction)

	end := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseEnd, 0, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.False(t, end.HasAction)
	require.False(t, end.HasIndicator)
}

func TestTouchpadNavigationRecognizer_AllowsIntentionalEdgeSwipeWithViewWidth(t *testing.T) {
	recognizer := newTouchpadNavigationRecognizer()
	cfg := RuntimeInputConfig{
		TouchpadNavigationEnabled:          true,
		TouchpadNavigationMinDelta:         200,
		TouchpadNavigationMaxVerticalRatio: 0.5,
	}

	_ = recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseBegin, 24, 0, 0),
		Config:    cfg,
		CanGoBack: true,
		ViewWidth: 1000,
	})
	armed := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseUpdate, 24, -240, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.True(t, armed.HasIndicator)
	require.True(t, armed.Indicator.ThresholdReached)

	end := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseEnd, 24, 0, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.True(t, end.HasAction)
	require.Equal(t, cef2gtk.NavigationSwipeBack, end.Action)
}

func TestTouchpadNavigationRecognizer_DoesNotTreatInContentHorizontalScrollAsNavigation(t *testing.T) {
	recognizer := newTouchpadNavigationRecognizer()
	cfg := RuntimeInputConfig{
		TouchpadNavigationEnabled:          true,
		TouchpadNavigationMinDelta:         200,
		TouchpadNavigationMaxVerticalRatio: 0.5,
	}

	_ = recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseBegin, 500, 0, 0),
		Config:    cfg,
		CanGoBack: true,
		ViewWidth: 1000,
	})
	ordinaryScroll := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseUpdate, 500, -240, 0),
		Config:    cfg,
		CanGoBack: true,
		ViewWidth: 1000,
	})
	require.False(t, ordinaryScroll.HasIndicator)
	require.False(t, ordinaryScroll.HasAction)

	end := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseEnd, 500, 0, 0),
		Config:    cfg,
		CanGoBack: true,
		ViewWidth: 1000,
	})
	require.False(t, end.HasIndicator)
	require.False(t, end.HasAction)
}

func TestTouchpadNavigationRecognizer_IgnoresNonPreciseScrollEvents(t *testing.T) {
	tests := []struct {
		name  string
		event cef2gtk.ScrollEvent
	}{
		{
			name: "unknown unit",
			event: cef2gtk.ScrollEvent{
				Phase:     cef2gtk.ScrollPhaseUpdate,
				DX:        -240,
				Unit:      gdk.ScrollUnitSurfaceValue,
				UnitKnown: false,
			},
		},
		{
			name: "wheel unit",
			event: cef2gtk.ScrollEvent{
				Phase:     cef2gtk.ScrollPhaseUpdate,
				DX:        -240,
				Unit:      gdk.ScrollUnitWheelValue,
				UnitKnown: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				ViewWidth: 1000,
			})
			result := recognizer.Handle(touchpadNavigationInput{
				Event:     tt.event,
				Config:    cfg,
				CanGoBack: true,
			})
			require.False(t, result.HasIndicator)
			require.False(t, result.HasAction)
		})
	}
}

func TestTouchpadNavigationRecognizer_DisabledConfigSuppressesGestureRecognition(t *testing.T) {
	recognizer := newTouchpadNavigationRecognizer()
	cfg := RuntimeInputConfig{
		TouchpadNavigationEnabled:          false,
		TouchpadNavigationMinDelta:         200,
		TouchpadNavigationMaxVerticalRatio: 0.5,
	}

	begin := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseBegin, 0, 0),
		Config:    cfg,
		CanGoBack: true,
		ViewWidth: 1000,
	})
	require.False(t, begin.HasIndicator)
	require.False(t, begin.HasAction)

	update := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -240, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.False(t, update.HasIndicator)
	require.False(t, update.HasAction)

	end := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseEnd, 0, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.False(t, end.HasIndicator)
	require.False(t, end.HasAction)
}

func TestTouchpadNavigationRecognizer_BackCapabilitySuppressesBackGesture(t *testing.T) {
	recognizer := newTouchpadNavigationRecognizer()
	cfg := RuntimeInputConfig{
		TouchpadNavigationEnabled:          true,
		TouchpadNavigationMinDelta:         200,
		TouchpadNavigationMaxVerticalRatio: 0.5,
	}

	_ = recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseBegin, 0, 0),
		Config:    cfg,
		CanGoBack: false,
		ViewWidth: 1000,
	})
	update := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -240, 0),
		Config:    cfg,
		CanGoBack: false,
	})
	require.False(t, update.HasIndicator)
	require.False(t, update.HasAction)

	end := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseEnd, 0, 0),
		Config:    cfg,
		CanGoBack: false,
	})
	require.False(t, end.HasIndicator)
	require.False(t, end.HasAction)
}

func TestTouchpadNavigationRecognizer_ForwardCapabilitySuppressesForwardGesture(t *testing.T) {
	recognizer := newTouchpadNavigationRecognizer()
	cfg := RuntimeInputConfig{
		TouchpadNavigationEnabled:          true,
		TouchpadNavigationMinDelta:         200,
		TouchpadNavigationMaxVerticalRatio: 0.5,
	}

	_ = recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseBegin, 976, 0, 0),
		Config:       cfg,
		CanGoForward: false,
		ViewWidth:    1000,
	})
	update := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseUpdate, 976, 240, 0),
		Config:       cfg,
		CanGoForward: false,
	})
	require.False(t, update.HasIndicator)
	require.False(t, update.HasAction)

	end := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseEnd, 976, 0, 0),
		Config:       cfg,
		CanGoForward: false,
	})
	require.False(t, end.HasIndicator)
	require.False(t, end.HasAction)
}

func TestTouchpadNavigationRecognizer_HidesIndicatorWithoutNavigatingWhenReleasedAfterActivationBelowThreshold(t *testing.T) {
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
		ViewWidth: 1000,
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

func TestTouchpadNavigationRecognizer_ResetsBetweenConsecutiveGestures(t *testing.T) {
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
		ViewWidth: 1000,
	})
	firstUpdate := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -240, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.True(t, firstUpdate.HasIndicator)
	firstEnd := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseEnd, 0, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.True(t, firstEnd.HasAction)
	require.Equal(t, cef2gtk.NavigationSwipeBack, firstEnd.Action)

	_ = recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseBegin, 976, 0, 0),
		Config:       cfg,
		CanGoForward: true,
		ViewWidth:    1000,
	})
	secondUpdate := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseUpdate, 976, 240, 0),
		Config:       cfg,
		CanGoForward: true,
	})
	require.True(t, secondUpdate.HasIndicator)
	require.Equal(t, entity.TouchpadNavigationForward, secondUpdate.Indicator.Action)
	secondEnd := recognizer.Handle(touchpadNavigationInput{
		Event:        touchpadNavigationScrollEventAt(cef2gtk.ScrollPhaseEnd, 976, 0, 0),
		Config:       cfg,
		CanGoForward: true,
	})
	require.True(t, secondEnd.HasAction)
	require.Equal(t, cef2gtk.NavigationSwipeForward, secondEnd.Action)
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
		ViewWidth: 1000,
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

func TestTouchpadNavigationRecognizer_HidesVisibleIndicatorOnVerticalCancelAndResets(t *testing.T) {
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
		ViewWidth: 1000,
	})
	shown := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -120, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.True(t, shown.HasIndicator)
	require.True(t, shown.Indicator.Active)

	canceled := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, 0, 80),
		Config:    cfg,
		CanGoBack: true,
	})
	require.True(t, canceled.HasIndicator)
	require.False(t, canceled.Indicator.Active)
	require.False(t, canceled.HasAction)

	end := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseEnd, 0, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.False(t, end.HasIndicator)
	require.False(t, end.HasAction)

	_ = recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseBegin, 0, 0),
		Config:    cfg,
		CanGoBack: true,
		ViewWidth: 1000,
	})
	valid := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseUpdate, -240, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.True(t, valid.HasIndicator)
	validEnd := recognizer.Handle(touchpadNavigationInput{
		Event:     touchpadNavigationScrollEvent(cef2gtk.ScrollPhaseEnd, 0, 0),
		Config:    cfg,
		CanGoBack: true,
	})
	require.True(t, validEnd.HasAction)
	require.Equal(t, cef2gtk.NavigationSwipeBack, validEnd.Action)
}

func touchpadNavigationScrollEvent(phase cef2gtk.ScrollPhase, dx, dy float64) cef2gtk.ScrollEvent {
	return touchpadNavigationScrollEventAt(phase, 0, dx, dy)
}

// ViewWidth is provided separately on touchpadNavigationInput; the recognizer
// caches it from the initial Begin event for subsequent Update and End phases.
func touchpadNavigationScrollEventAt(phase cef2gtk.ScrollPhase, x, dx, dy float64) cef2gtk.ScrollEvent {
	return cef2gtk.ScrollEvent{
		Phase:     phase,
		X:         x,
		DX:        dx,
		DY:        dy,
		Unit:      gdk.ScrollUnitSurfaceValue,
		UnitKnown: true,
	}
}
