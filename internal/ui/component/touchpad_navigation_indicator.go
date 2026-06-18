package component

import (
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"
)

const (
	touchpadNavigationIndicatorHideMs       = 220
	touchpadNavigationIndicatorSpacing      = 8
	touchpadNavigationIndicatorHeaderGap    = 10
	touchpadNavigationIndicatorMargin       = 32
	touchpadNavigationIndicatorWidth        = 156
	touchpadNavigationIndicatorBarHeight    = 5
	touchpadNavigationIndicatorIconMinChars = 2
	touchpadNavigationIndicatorIconXAlign   = 0.5
)

// TouchpadNavigationIndicator is a lightweight overlay that makes CEF
// two-finger history navigation deliberate: it appears during the swipe,
// fills toward the commit threshold, and briefly shows the triggered state.
type TouchpadNavigationIndicator struct {
	container *gtk.Box
	icon      *gtk.Label
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

	icon, label, ok := appendTouchpadNavigationHeader(container)
	if !ok {
		container.Unref()
		return nil
	}

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
		icon:      icon,
		label:     label,
		bar:       bar,
	}
}

func appendTouchpadNavigationHeader(container *gtk.Box) (*gtk.Label, *gtk.Label, bool) {
	header := gtk.NewBox(gtk.OrientationHorizontalValue, touchpadNavigationIndicatorHeaderGap)
	if header == nil {
		return nil, nil, false
	}
	header.AddCssClass("touchpad-navigation-header")
	header.SetCanTarget(false)
	header.SetCanFocus(false)
	container.Append(&header.Widget)

	iconText := "←"
	icon := gtk.NewLabel(&iconText)
	if icon == nil {
		return nil, nil, false
	}
	icon.AddCssClass("touchpad-navigation-icon")
	icon.SetWidthChars(touchpadNavigationIndicatorIconMinChars)
	icon.SetXalign(touchpadNavigationIndicatorIconXAlign)
	icon.SetCanTarget(false)
	icon.SetCanFocus(false)
	header.Append(&icon.Widget)

	text := "Slide Back"
	label := gtk.NewLabel(&text)
	if label == nil {
		return nil, nil, false
	}
	label.AddCssClass("touchpad-navigation-label")
	label.SetXalign(0)
	label.SetCanTarget(false)
	label.SetCanFocus(false)
	header.Append(&label.Widget)

	return icon, label, true
}

func (i *TouchpadNavigationIndicator) Widget() *gtk.Widget {
	if i == nil || i.container == nil {
		return nil
	}
	return &i.container.Widget
}

func (i *TouchpadNavigationIndicator) ShowGesture(gesture port.TouchpadNavigationGesture) {
	if i == nil || i.container == nil || i.icon == nil || i.label == nil || i.bar == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.cancelHideTimerLocked()

	i.container.RemoveCssClass("back")
	i.container.RemoveCssClass("forward")
	if gesture.Action == port.TouchpadNavigationForward {
		i.container.AddCssClass("forward")
		i.container.SetHalign(gtk.AlignEndValue)
		i.bar.SetInverted(true)
	} else {
		i.container.AddCssClass("back")
		i.container.SetHalign(gtk.AlignStartValue)
		i.bar.SetInverted(false)
	}

	if gesture.ThresholdReached {
		i.container.AddCssClass("threshold-reached")
	} else {
		i.container.RemoveCssClass("threshold-reached")
	}

	i.icon.SetLabel(touchpadNavigationIndicatorIcon(gesture))
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
	i.icon = nil
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
		i.container.RemoveCssClass("back")
		i.container.RemoveCssClass("forward")
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

func touchpadNavigationIndicatorIcon(gesture port.TouchpadNavigationGesture) string {
	if gesture.Action == port.TouchpadNavigationForward {
		return "→"
	}
	return "←"
}

func touchpadNavigationIndicatorLabel(gesture port.TouchpadNavigationGesture) string {
	suffix := "Back"
	if gesture.Action == port.TouchpadNavigationForward {
		suffix = "Forward"
	}
	if gesture.ThresholdReached {
		return "Release for " + suffix
	}
	return "Slide " + suffix
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
