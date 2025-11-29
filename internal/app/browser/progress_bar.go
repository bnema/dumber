package browser

import (
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// createProgressBar builds the shared loading indicator shown at the bottom of the window.
func (tm *TabManager) createProgressBar() gtk.Widgetter {
	bar := gtk.NewProgressBar()
	if bar == nil {
		logging.Error("[tabs] Failed to create tab progress bar")
		return nil
	}

	bar.SetHExpand(true)
	bar.SetVExpand(false)
	bar.SetShowText(false)
	bar.SetFraction(0.0)
	bar.AddCSSClass("tab-progress-bar")
	webkit.WidgetSetVisible(bar, false)
	return bar
}
