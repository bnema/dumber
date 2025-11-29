package browser

import (
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// createBorderOverlay builds a transparent overlay widget that displays as a border when visible.
// This allows showing visual borders without affecting layout or causing content shifts.
func (tm *TabManager) createBorderOverlay(cssClass string) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	if box == nil {
		logging.Error("[tabs] Failed to create border overlay for " + cssClass)
		return nil
	}

	box.AddCSSClass(cssClass)
	box.SetHExpand(true)
	box.SetVExpand(true)
	box.SetCanTarget(false) // Don't intercept mouse events - overlay is visual only

	return box
}

// wrapPaneInOverlay wraps a pane container widget in a gtk.Overlay with a border overlay.
// This allows showing active pane borders without affecting layout or causing content shifts.
// Returns the overlay (new container) and the border widget (to store in paneNode.borderOverlay).
func wrapPaneInOverlay(paneContainer gtk.Widgetter) (*gtk.Overlay, gtk.Widgetter) {
	// Create the overlay container
	overlay := gtk.NewOverlay()
	if overlay == nil {
		logging.Error("[workspace] Failed to create overlay for pane border")
		return nil, nil
	}
	overlay.SetHExpand(true)
	overlay.SetVExpand(true)

	// Set the pane container as the main child
	overlay.SetChild(paneContainer)

	// Create the border overlay widget (transparent box with border CSS)
	borderBox := gtk.NewBox(gtk.OrientationVertical, 0)
	if borderBox == nil {
		logging.Error("[workspace] Failed to create border overlay box")
		return overlay, nil
	}

	borderBox.AddCSSClass("pane-border-overlay")
	borderBox.SetHExpand(true)
	borderBox.SetVExpand(true)
	borderBox.SetCanTarget(false) // Don't intercept mouse events - overlay is visual only

	// Add the border overlay to the overlay (hidden by default)
	overlay.AddOverlay(borderBox)
	webkit.WidgetSetVisible(borderBox, false)

	return overlay, borderBox
}

// showPaneModeBorder displays the pane mode border overlay.
func (tm *TabManager) showPaneModeBorder() {
	if tm.paneModeOverlay == nil {
		return
	}
	webkit.WidgetSetVisible(tm.paneModeOverlay, true)
	logging.Debug("[pane-mode] Showing pane mode border overlay")
}

// hidePaneModeBorder hides the pane mode border overlay.
func (tm *TabManager) hidePaneModeBorder() {
	if tm.paneModeOverlay == nil {
		return
	}
	webkit.WidgetSetVisible(tm.paneModeOverlay, false)
	logging.Debug("[pane-mode] Hiding pane mode border overlay")
}

// showTabModeBorder displays the tab mode border overlay.
func (tm *TabManager) showTabModeBorder() {
	if tm.tabModeOverlay == nil {
		return
	}
	webkit.WidgetSetVisible(tm.tabModeOverlay, true)
	logging.Debug("[tab-mode] Showing tab mode border overlay")
}

// hideTabModeBorder hides the tab mode border overlay.
func (tm *TabManager) hideTabModeBorder() {
	if tm.tabModeOverlay == nil {
		return
	}
	webkit.WidgetSetVisible(tm.tabModeOverlay, false)
	logging.Debug("[tab-mode] Hiding tab mode border overlay")
}
