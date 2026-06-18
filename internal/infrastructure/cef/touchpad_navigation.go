package cef

import (
	"math"

	"github.com/bnema/dumber/internal/application/port"
	cef2gtk "github.com/bnema/purego-cef2gtk"
	"github.com/bnema/puregotk/v4/gdk"
)

const (
	defaultTouchpadNavigationCommitDistance       = 320.0
	defaultTouchpadNavigationVerticalRatio        = 0.5
	touchpadNavigationIndicatorActivationFraction = 0.5
	touchpadNavigationEdgeFraction                = 0.15
	touchpadNavigationMaxEdgeDistance             = 96.0
)

type touchpadNavigationRecognizer struct {
	cumulativeDX        float64
	cumulativeDY        float64
	gestureStartX       float64
	gestureViewWidth    float64
	hasGestureStart     bool
	thresholdReached    bool
	verticalCanceled    bool
	indicatorShown      bool
	lastIndicatorAction port.TouchpadNavigationAction
}

type touchpadNavigationInput struct {
	Event        cef2gtk.ScrollEvent
	Config       RuntimeInputConfig
	CanGoBack    bool
	CanGoForward bool
	ViewWidth    float64
}

type touchpadNavigationResult struct {
	HasIndicator bool
	Indicator    port.TouchpadNavigationGesture
	HasAction    bool
	Action       cef2gtk.NavigationSwipeAction
}

func newTouchpadNavigationRecognizer() *touchpadNavigationRecognizer {
	return &touchpadNavigationRecognizer{}
}

func (r *touchpadNavigationRecognizer) Handle(input touchpadNavigationInput) touchpadNavigationResult {
	if r == nil || !input.Config.TouchpadNavigationEnabled {
		return touchpadNavigationResult{}
	}

	switch input.Event.Phase {
	case cef2gtk.ScrollPhaseBegin:
		r.reset()
		r.setGestureStart(input)
		return touchpadNavigationResult{}
	case cef2gtk.ScrollPhaseUpdate:
		if !isTouchpadNavigationPreciseEvent(input.Event) {
			return touchpadNavigationResult{}
		}
		return r.handleUpdate(input)
	case cef2gtk.ScrollPhaseEnd:
		return r.handleEnd(input)
	default:
		return touchpadNavigationResult{}
	}
}

func (r *touchpadNavigationRecognizer) handleUpdate(input touchpadNavigationInput) touchpadNavigationResult {
	if r.verticalCanceled {
		return touchpadNavigationResult{}
	}
	if !r.hasGestureStart {
		r.setGestureStart(input)
	}

	// GTK scroll deltas are inverted compared to browser-history direction:
	// negative horizontal surface deltas indicate a back swipe, positive indicate
	// forward. Match the prior bridge behavior while keeping recognition local so
	// normal scroll forwarding remains untouched.
	r.cumulativeDX += -input.Event.DX
	r.cumulativeDY += input.Event.DY
	if r.isTooVertical(input.Config) {
		return r.cancelAsTooVertical()
	}

	action, ok := r.indicatorAction(input)
	if !ok {
		return touchpadNavigationResult{}
	}

	threshold := normalizedTouchpadNavigationMinDelta(input.Config.TouchpadNavigationMinDelta)
	absDX := math.Abs(r.cumulativeDX)
	progress := clampFloat(absDX/threshold, 0, 1)
	r.thresholdReached = absDX > threshold
	if !r.indicatorShown && progress < touchpadNavigationIndicatorActivationFraction {
		return touchpadNavigationResult{}
	}
	r.indicatorShown = true
	r.lastIndicatorAction = action
	return touchpadNavigationResult{
		HasIndicator: true,
		Indicator: port.TouchpadNavigationGesture{
			Action:           action,
			Progress:         progress,
			ThresholdReached: r.thresholdReached,
			Active:           true,
		},
	}
}

func (r *touchpadNavigationRecognizer) handleEnd(input touchpadNavigationInput) touchpadNavigationResult {
	defer r.reset()
	if r.verticalCanceled || !r.thresholdReached || r.isTooVertical(input.Config) {
		return r.finishIndicator(input, false)
	}
	action, ok := r.navigationAction(input)
	if !ok {
		return r.finishIndicator(input, false)
	}
	result := r.finishIndicator(input, true)
	result.HasAction = true
	result.Action = action
	return result
}

func (r *touchpadNavigationRecognizer) finishIndicator(input touchpadNavigationInput, triggered bool) touchpadNavigationResult {
	if !r.indicatorShown {
		return touchpadNavigationResult{}
	}
	action, ok := r.indicatorAction(input)
	if !ok {
		action = r.lastIndicatorAction
	}
	threshold := normalizedTouchpadNavigationMinDelta(input.Config.TouchpadNavigationMinDelta)
	return touchpadNavigationResult{
		HasIndicator: true,
		Indicator: port.TouchpadNavigationGesture{
			Action:           action,
			Progress:         clampFloat(math.Abs(r.cumulativeDX)/threshold, 0, 1),
			ThresholdReached: triggered,
			Active:           false,
		},
	}
}

func (r *touchpadNavigationRecognizer) indicatorAction(input touchpadNavigationInput) (port.TouchpadNavigationAction, bool) {
	return r.resolvedDirection(input)
}

func (r *touchpadNavigationRecognizer) cancelAsTooVertical() touchpadNavigationResult {
	var action port.TouchpadNavigationAction
	hasAction := r.indicatorShown
	if hasAction {
		action = r.lastIndicatorAction
	}
	r.reset()
	r.verticalCanceled = true
	if !hasAction {
		return touchpadNavigationResult{}
	}
	return touchpadNavigationResult{
		HasIndicator: true,
		Indicator: port.TouchpadNavigationGesture{
			Action: action,
			Active: false,
		},
	}
}

func (r *touchpadNavigationRecognizer) navigationAction(input touchpadNavigationInput) (cef2gtk.NavigationSwipeAction, bool) {
	action, ok := r.resolvedDirection(input)
	if !ok {
		return cef2gtk.NavigationSwipeBack, false
	}
	if action == port.TouchpadNavigationForward {
		return cef2gtk.NavigationSwipeForward, true
	}
	return cef2gtk.NavigationSwipeBack, true
}

func (r *touchpadNavigationRecognizer) resolvedDirection(input touchpadNavigationInput) (port.TouchpadNavigationAction, bool) {
	if r.cumulativeDX > 0 && input.CanGoBack && r.startedAtNavigationEdge(port.TouchpadNavigationBack) {
		return port.TouchpadNavigationBack, true
	}
	if r.cumulativeDX < 0 && input.CanGoForward && r.startedAtNavigationEdge(port.TouchpadNavigationForward) {
		return port.TouchpadNavigationForward, true
	}
	return port.TouchpadNavigationBack, false
}

func (r *touchpadNavigationRecognizer) setGestureStart(input touchpadNavigationInput) {
	r.gestureStartX = input.Event.X
	r.gestureViewWidth = input.ViewWidth
	r.hasGestureStart = true
}

func (r *touchpadNavigationRecognizer) startedAtNavigationEdge(action port.TouchpadNavigationAction) bool {
	if r.gestureViewWidth <= 0 {
		return false
	}
	edgeDistance := math.Min(r.gestureViewWidth*touchpadNavigationEdgeFraction, touchpadNavigationMaxEdgeDistance)
	if action == port.TouchpadNavigationForward {
		return r.gestureStartX >= r.gestureViewWidth-edgeDistance
	}
	return r.gestureStartX <= edgeDistance
}

func (r *touchpadNavigationRecognizer) isTooVertical(cfg RuntimeInputConfig) bool {
	absDX, absDY := math.Abs(r.cumulativeDX), math.Abs(r.cumulativeDY)
	return absDX == 0 || absDY >= absDX*normalizedTouchpadNavigationRatio(cfg.TouchpadNavigationMaxVerticalRatio)
}

func (r *touchpadNavigationRecognizer) reset() {
	r.cumulativeDX = 0
	r.cumulativeDY = 0
	r.gestureStartX = 0
	r.gestureViewWidth = 0
	r.hasGestureStart = false
	r.thresholdReached = false
	r.verticalCanceled = false
	r.indicatorShown = false
	r.lastIndicatorAction = port.TouchpadNavigationBack
}

func isTouchpadNavigationPreciseEvent(event cef2gtk.ScrollEvent) bool {
	return event.UnitKnown && event.Unit == gdk.ScrollUnitSurfaceValue
}

func normalizedTouchpadNavigationMinDelta(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return defaultTouchpadNavigationCommitDistance
	}
	return value
}

func normalizedTouchpadNavigationRatio(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return defaultTouchpadNavigationVerticalRatio
	}
	return value
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
