package component

import (
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"
)

const (
	touchpadNavigationIndicatorHideMs    = 220
	touchpadNavigationIndicatorSpacing   = 6
	touchpadNavigationIndicatorMargin    = 28
	touchpadNavigationIndicatorWidth     = 96
	touchpadNavigationIndicatorBarHeight = 4
)

// TouchpadNavigationIndicator is a lightweight overlay that makes CEF
// two-finger history navigation deliberate: it appears during the swipe,
// fills toward the commit threshold, and briefly shows the triggered state.
type TouchpadNavigationIndicator struct {
	container *gtk.Box
	label     *gtk.Label
	bar       *gtk.ProgressBar

	hideTimer uint
	mu        sync.Mutex
}

func NewTouchpadNavigationIndicator() *TouchpadNavigationIndicator {
	container := gtk.NewBox(gtk.OrientationVerticalValue, touchpadNavigationIndicatorSpacing)
	if container == nil {
		return nil
	}
	container.AddCssClass("touchpad-navigation-indicator")
	container.SetValign(gtk.AlignCenterValue)
	container.SetHalign(gtk.AlignStartValue)
	container.SetHexpand(false)
	container.SetVexpand(false)
	container.SetMarginStart(touchpadNavigationIndicatorMargin)
	container.SetMarginEnd(touchpadNavigationIndicatorMargin)
	container.SetCanTarget(false)
	container.SetCanFocus(false)
	container.SetVisible(false)

	text := ""
	label := gtk.NewLabel(&text)
	if label == nil {
		container.Unref()
		return nil
	}
	label.AddCssClass("touchpad-navigation-label")
	label.SetCanTarget(false)
	label.SetCanFocus(false)
	container.Append(&label.Widget)

	bar := gtk.NewProgressBar()
	if bar == nil {
		container.Unref()
		return nil
	}
	bar.AddCssClass("touchpad-navigation-progress")
	bar.SetSizeRequest(touchpadNavigationIndicatorWidth, touchpadNavigationIndicatorBarHeight)
	bar.SetFraction(0)
	bar.SetCanTarget(false)
	bar.SetCanFocus(false)
	container.Append(&bar.Widget)

	return &TouchpadNavigationIndicator{
		container: container,
		label:     label,
		bar:       bar,
	}
}

func (i *TouchpadNavigationIndicator) Widget() *gtk.Widget {
	if i == nil || i.container == nil {
		return nil
	}
	return &i.container.Widget
}

func (i *TouchpadNavigationIndicator) ShowGesture(gesture port.TouchpadNavigationGesture) {
	if i == nil || i.container == nil || i.label == nil || i.bar == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.cancelHideTimerLocked()

	if gesture.Action == port.TouchpadNavigationForward {
		i.container.SetHalign(gtk.AlignEndValue)
		i.bar.SetInverted(true)
	} else {
		i.container.SetHalign(gtk.AlignStartValue)
		i.bar.SetInverted(false)
	}

	if gesture.ThresholdReached {
		i.container.AddCssClass("threshold-reached")
	} else {
		i.container.RemoveCssClass("threshold-reached")
	}

	i.label.SetLabel(touchpadNavigationIndicatorLabel(gesture))
	i.bar.SetFraction(clampIndicatorProgress(gesture.Progress))
	i.container.SetVisible(true)

	if !gesture.Active {
		i.scheduleHideLocked(gesture.ThresholdReached)
	}
}

func (i *TouchpadNavigationIndicator) Hide() {
	if i == nil || i.container == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.hideLocked()
}

func (i *TouchpadNavigationIndicator) Destroy() {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.cancelHideTimerLocked()
	if i.container != nil {
		i.container.SetVisible(false)
		i.container.Unparent()
	}
	i.container = nil
	i.label = nil
	i.bar = nil
}

func (i *TouchpadNavigationIndicator) scheduleHideLocked(triggered bool) {
	delay := uint(0)
	if triggered {
		delay = touchpadNavigationIndicatorHideMs
	}
	cb := glib.SourceFunc(func(_ uintptr) bool {
		i.mu.Lock()
		defer i.mu.Unlock()
		i.hideTimer = 0
		i.hideLocked()
		return false
	})
	if delay == 0 {
		i.hideLocked()
		return
	}
	i.hideTimer = glib.TimeoutAdd(delay, &cb, 0)
}

func (i *TouchpadNavigationIndicator) hideLocked() {
	i.cancelHideTimerLocked()
	if i.container != nil {
		i.container.RemoveCssClass("threshold-reached")
		i.container.SetVisible(false)
	}
	if i.bar != nil {
		i.bar.SetFraction(0)
	}
}

func (i *TouchpadNavigationIndicator) cancelHideTimerLocked() {
	if i.hideTimer != 0 {
		glib.SourceRemove(i.hideTimer)
		i.hideTimer = 0
	}
}

func touchpadNavigationIndicatorLabel(gesture port.TouchpadNavigationGesture) string {
	suffix := "back"
	if gesture.Action == port.TouchpadNavigationForward {
		suffix = "forward"
	}
	if gesture.ThresholdReached {
		return "Release to go " + suffix
	}
	return "Slide to go " + suffix
}

func clampIndicatorProgress(progress float64) float64 {
	if progress < 0 {
		return 0
	}
	if progress > 1 {
		return 1
	}
	return progress
}
