package cef2gtk

import (
	"errors"
	"math"

	"github.com/bnema/purego-cef/cef"
	"github.com/bnema/purego-cef2gtk/internal/gtkgl"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
)

var ErrInputNotAttached = errors.New("input bridge is not attached")
var ErrViewNotInitialized = errors.New("view not initialized")

// InputOptions configures GTK-to-CEF input forwarding.
type InputOptions struct {
	// Scale overrides the coordinate transform applied to pointer coordinates.
	// Values <= 0 let the view choose the correct transform for the current OSR
	// backing mode: logical coordinates normally, device coordinates when the
	// OSR backing-scale compatibility path is active.
	Scale float64
	// OnMiddleClick is invoked when GTK receives a middle-button press. Returning
	// true consumes the event before it is forwarded to CEF.
	OnMiddleClick func(x, y float64) bool
	// Scroll configures GTK scroll delta translation before forwarding to CEF.
	Scroll ScrollOptions
	// OnScroll is invoked for scroll begin/update/end/decelerate notifications.
	// Returning ScrollConsume for an update prevents forwarding that event to CEF.
	OnScroll func(ScrollEvent) ScrollDecision
	// NavigationSwipe configures browser-history swipe recognition from precise
	// horizontal touchpad scroll streams.
	NavigationSwipe NavigationSwipeOptions
	// CanNavigateBack reports whether a recognized back swipe may navigate.
	CanNavigateBack func() bool
	// CanNavigateForward reports whether a recognized forward swipe may navigate.
	CanNavigateForward func() bool
	// OnNavigateSwipe is invoked when a back/forward navigation swipe is recognized.
	OnNavigateSwipe func(NavigationSwipeAction)
	// SelectionText returns the current browser selection for explicit clipboard
	// shortcuts. When set with OnClipboardShortcut, Ctrl+C/Ctrl+X can be mirrored
	// to application-level clipboard orchestration before forwarding to CEF.
	SelectionText func() string
	// OnClipboardShortcut is invoked for explicit Ctrl+C/Ctrl+X shortcuts when
	// SelectionText returns non-empty text. action is "copy" or "cut".
	OnClipboardShortcut func(action, text string)
}

// ScrollPhase identifies the stage of a GTK scroll operation.
type ScrollPhase int

const (
	ScrollPhaseUnknown ScrollPhase = iota
	ScrollPhaseBegin
	ScrollPhaseUpdate
	ScrollPhaseEnd
	ScrollPhaseDecelerate
)

// ScrollDecision controls whether a scroll update should be forwarded to CEF.
type ScrollDecision int

const (
	ScrollForwardToCEF ScrollDecision = iota
	ScrollConsume
)

// ScrollOptions configures GTK scroll delta translation before forwarding to CEF.
// Wheel zero values keep legacy wheel behavior; precise zero values use a
// WebKitGTK-like touchpad/surface scale.
type ScrollOptions struct {
	WheelMultiplier      float64
	PreciseMultiplier    float64
	HorizontalMultiplier float64
	VerticalMultiplier   float64
	MaxDelta             int32
}

// ScrollEvent describes a GTK scroll event after CEF delta translation.
type ScrollEvent struct {
	Phase                ScrollPhase
	X, Y                 float64
	DX, DY               float64
	DeltaX, DeltaY       int32
	Modifiers            uint
	Unit                 gdk.ScrollUnit
	UnitKnown            bool
	VelocityX, VelocityY float64
}

// NavigationSwipeAction identifies a browser-history swipe action derived
// from precise horizontal touchpad scrolling.
type NavigationSwipeAction int

const (
	NavigationSwipeBack NavigationSwipeAction = iota
	NavigationSwipeForward
)

// NavigationSwipeOptions configures WebKitGTK-like back/forward swipe recognition.
type NavigationSwipeOptions struct {
	Enabled          bool
	MinDelta         float64
	MaxVerticalRatio float64
}

func (o InputOptions) normalizedScale(fallback float64) float64 {
	if o.Scale > 0 {
		return normalizeDeviceScale(o.Scale)
	}
	if fallback <= 0 {
		return 1
	}
	return inputScaleForOSRBacking(fallback)
}

func (v *View) setInputScaleOverride(scale float64) {
	if v == nil {
		return
	}
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		v.inputScaleOverride.Store(0)
		return
	}
	v.inputScaleOverride.Store(math.Float64bits(normalizeDeviceScale(scale)))
}

func (v *View) inputScaleForObservedScale(observedScale float64) float64 {
	if v == nil {
		return 1
	}
	if bits := v.inputScaleOverride.Load(); bits != 0 {
		return normalizeDeviceScale(math.Float64frombits(bits))
	}
	return inputScaleForOSRBacking(observedScale)
}

// AttachInput attaches GTK event controllers to the view's render widget and
// forwards input to host. Call from the GTK/main thread; GTK controller
// attachment is not goroutine-safe.
func (v *View) AttachInput(host cef.BrowserHost, opts InputOptions) error {
	return v.AttachInputToWidget(host, nil, opts)
}

// AttachInputToWidget attaches GTK event controllers to widget and forwards
// input to host. When widget is nil, the view's render widget is used. This is
// useful when the render widget is embedded inside another focusable container
// that should own GTK keyboard focus, such as a GtkOverlay wrapper.
func (v *View) AttachInputToWidget(host cef.BrowserHost, widget *gtk.Widget, opts InputOptions) error {
	if v == nil {
		return ErrNilView
	}
	if v.widget == nil {
		return ErrViewNotInitialized
	}
	if v.input != nil {
		v.input.Detach()
	}
	v.setInputScaleOverride(opts.Scale)
	scale := v.inputScaleForObservedScale(float64(v.DeviceScaleFactor()))
	targetWidget := widget
	if targetWidget == nil {
		targetWidget = v.widget
	}
	if targetWidget == nil {
		return ErrViewNotInitialized
	}
	v.input = gtkgl.NewInputBridge(host, scale)
	v.input.SetProfiler(v.profileRecorder())
	v.input.SetMiddleClickHandler(opts.OnMiddleClick)
	v.input.SetScrollOptions(toGTKGLScrollOptions(opts.Scroll), toGTKGLScrollHandler(opts.OnScroll))
	v.input.SetNavigationSwipeHandler(toGTKGLNavigationSwipeOptions(opts.NavigationSwipe), opts.CanNavigateBack, opts.CanNavigateForward, toGTKGLNavigationSwipeHandler(opts.OnNavigateSwipe))
	v.input.SetClipboardShortcutHandler(opts.SelectionText, opts.OnClipboardShortcut)
	v.input.AttachToWidget(targetWidget)
	v.inputWidget = targetWidget
	return nil
}

func toGTKGLScrollOptions(opts ScrollOptions) gtkgl.ScrollOptions {
	return gtkgl.ScrollOptions{
		WheelMultiplier:      opts.WheelMultiplier,
		PreciseMultiplier:    opts.PreciseMultiplier,
		HorizontalMultiplier: opts.HorizontalMultiplier,
		VerticalMultiplier:   opts.VerticalMultiplier,
		MaxDelta:             opts.MaxDelta,
	}
}

func toGTKGLScrollHandler(fn func(ScrollEvent) ScrollDecision) func(gtkgl.ScrollEvent) gtkgl.ScrollDecision {
	if fn == nil {
		return nil
	}
	return func(event gtkgl.ScrollEvent) gtkgl.ScrollDecision {
		if fn(ScrollEvent{
			Phase:     toPublicScrollPhase(event.Phase),
			X:         event.X,
			Y:         event.Y,
			DX:        event.DX,
			DY:        event.DY,
			DeltaX:    event.DeltaX,
			DeltaY:    event.DeltaY,
			Modifiers: event.Modifiers,
			Unit:      event.Unit,
			UnitKnown: event.UnitKnown,
			VelocityX: event.VelocityX,
			VelocityY: event.VelocityY,
		}) == ScrollConsume {
			return gtkgl.ScrollConsume
		}
		return gtkgl.ScrollForwardToCEF
	}
}

func toPublicScrollPhase(phase gtkgl.ScrollPhase) ScrollPhase {
	switch phase {
	case gtkgl.ScrollPhaseBegin:
		return ScrollPhaseBegin
	case gtkgl.ScrollPhaseUpdate:
		return ScrollPhaseUpdate
	case gtkgl.ScrollPhaseEnd:
		return ScrollPhaseEnd
	case gtkgl.ScrollPhaseDecelerate:
		return ScrollPhaseDecelerate
	default:
		return ScrollPhaseUnknown
	}
}

func toGTKGLNavigationSwipeOptions(opts NavigationSwipeOptions) gtkgl.NavigationSwipeOptions {
	return gtkgl.NavigationSwipeOptions{
		Enabled:          opts.Enabled,
		MinDelta:         opts.MinDelta,
		MaxVerticalRatio: opts.MaxVerticalRatio,
	}
}

func toGTKGLNavigationSwipeHandler(fn func(NavigationSwipeAction)) func(gtkgl.NavigationSwipeAction) {
	if fn == nil {
		return nil
	}
	return func(action gtkgl.NavigationSwipeAction) {
		switch action {
		case gtkgl.NavigationSwipeBack:
			fn(NavigationSwipeBack)
		case gtkgl.NavigationSwipeForward:
			fn(NavigationSwipeForward)
		}
	}
}

// DetachInput removes input controllers attached by AttachInput.
func (v *View) DetachInput() error {
	if v == nil {
		return ErrNilView
	}
	if v.input != nil {
		v.input.Detach()
		v.input = nil
	}
	v.inputWidget = nil
	v.setInputScaleOverride(0)
	return nil
}

// SetInputHost updates the CEF browser host used by the attached input bridge.
// Must be called on the GTK/main thread. Returns ErrInputNotAttached when v.input
// is nil. Callers must call AttachInput first.
func (v *View) SetInputHost(host cef.BrowserHost) error {
	if v == nil {
		return ErrNilView
	}
	if v.input == nil {
		return ErrInputNotAttached
	}
	v.input.SetHost(host)
	return nil
}
