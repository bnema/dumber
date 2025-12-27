package component

import (
	"strings"

	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// LoadingSkeleton displays a themed loading placeholder.
// It is intended to be embedded as an overlay while primary content is loading.
type LoadingSkeleton struct {
	container layout.BoxWidget
	content   layout.BoxWidget
	spinner   layout.SpinnerWidget
	label     layout.LabelWidget
}

const loadingSkeletonSpacing = 6

func NewLoadingSkeleton(factory layout.WidgetFactory) *LoadingSkeleton {
	container := factory.NewBox(layout.OrientationVertical, 0)
	container.SetHexpand(true)
	container.SetVexpand(true)
	container.SetHalign(gtk.AlignFillValue)
	container.SetValign(gtk.AlignFillValue)
	container.SetCanFocus(false)
	container.SetCanTarget(false)
	container.AddCssClass("loading-skeleton")

	content := factory.NewBox(layout.OrientationVertical, loadingSkeletonSpacing)
	content.SetHalign(gtk.AlignCenterValue)
	content.SetValign(gtk.AlignCenterValue)
	content.SetCanFocus(false)
	content.SetCanTarget(false)
	content.AddCssClass("loading-skeleton-content")

	spinner := factory.NewSpinner()
	spinner.SetHalign(gtk.AlignCenterValue)
	spinner.SetValign(gtk.AlignCenterValue)
	spinner.SetCanFocus(false)
	spinner.SetCanTarget(false)
	spinner.SetSizeRequest(32, 32)
	spinner.AddCssClass("loading-skeleton-spinner")

	text := "Loading..."
	label := factory.NewLabel(text)
	label.SetHalign(gtk.AlignCenterValue)
	label.SetValign(gtk.AlignCenterValue)
	label.SetCanFocus(false)
	label.SetCanTarget(false)
	label.AddCssClass("loading-skeleton-text")

	content.Append(spinner)
	content.Append(label)
	container.Append(content)

	ls := &LoadingSkeleton{
		container: container,
		content:   content,
		spinner:   spinner,
		label:     label,
	}
	ls.SetVisible(true)
	return ls
}

func (ls *LoadingSkeleton) Widget() layout.Widget {
	if ls == nil {
		return nil
	}
	return ls.container
}

func (ls *LoadingSkeleton) SetText(text string) {
	if ls == nil || ls.label == nil {
		return
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		trimmed = "Loading..."
	}
	ls.label.SetText(trimmed)
}

func (ls *LoadingSkeleton) SetVisible(visible bool) {
	if ls == nil || ls.container == nil {
		return
	}
	ls.container.SetVisible(visible)
	if ls.spinner != nil {
		if visible {
			ls.spinner.Start()
		} else {
			ls.spinner.Stop()
		}
	}
}
