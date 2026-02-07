package component

import (
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// LoadingSkeleton displays a themed loading placeholder.
// It is intended to be embedded as an overlay while primary content is loading.
// Shows a faded app logo, a discrete spinner, and the version below.
type LoadingSkeleton struct {
	container layout.BoxWidget
	content   layout.BoxWidget
	spinner   layout.SpinnerWidget
	logo      layout.ImageWidget
	version   layout.LabelWidget
}

const (
	loadingSkeletonSpacing     = 6
	loadingSkeletonLogoSize    = 256
	loadingSkeletonSpinnerSize = 32
)

// skeletonVersion is set via SetSkeletonVersion during startup.
var skeletonVersion string

// SetSkeletonVersion sets the version string shown on loading skeletons.
func SetSkeletonVersion(v string) {
	skeletonVersion = v
}

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

	// Faded app logo
	logo := factory.NewImage()
	logo.SetHalign(gtk.AlignCenterValue)
	logo.SetValign(gtk.AlignCenterValue)
	logo.SetCanFocus(false)
	logo.SetCanTarget(false)
	logo.SetSizeRequest(loadingSkeletonLogoSize, loadingSkeletonLogoSize)
	logo.SetPixelSize(loadingSkeletonLogoSize)
	logo.AddCssClass("loading-skeleton-logo")

	// Set logo texture if available
	if logoTexture := GetLogoTexture(); logoTexture != nil {
		logo.SetFromPaintable(logoTexture)
	}

	// Discrete spinner between logo and version
	spinner := factory.NewSpinner()
	spinner.SetHalign(gtk.AlignCenterValue)
	spinner.SetValign(gtk.AlignCenterValue)
	spinner.SetCanFocus(false)
	spinner.SetCanTarget(false)
	spinner.SetSizeRequest(loadingSkeletonSpinnerSize, loadingSkeletonSpinnerSize)
	spinner.AddCssClass("loading-skeleton-spinner")

	// Version label below spinner
	versionText := skeletonVersion
	if versionText == "" {
		versionText = "dev"
	}
	versionLabel := factory.NewLabel(versionText)
	versionLabel.SetHalign(gtk.AlignCenterValue)
	versionLabel.SetValign(gtk.AlignCenterValue)
	versionLabel.SetCanFocus(false)
	versionLabel.SetCanTarget(false)
	versionLabel.AddCssClass("loading-skeleton-version")

	// Layout: logo, spinner, version (vertically centered)
	content.Append(logo)
	content.Append(spinner)
	content.Append(versionLabel)
	container.Append(content)

	ls := &LoadingSkeleton{
		container: container,
		content:   content,
		spinner:   spinner,
		logo:      logo,
		version:   versionLabel,
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
