// Package component provides UI components for the browser.
package component

import (
	"sync"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/layout"
)

const (
	// CSS class applied to active pane's border overlay
	activePaneClass = "pane-active"
)

// PaneView is a container for a single WebView with active state indication.
// It uses an overlay to display a border around the active pane.
type PaneView struct {
	factory       layout.WidgetFactory
	overlay       layout.OverlayWidget
	webViewWidget layout.Widget    // The actual WebView widget
	borderBox     layout.BoxWidget // Border overlay for active indication
	paneID        entity.PaneID
	isActive      bool

	onFocusIn  func(paneID entity.PaneID)
	onFocusOut func(paneID entity.PaneID)

	mu sync.RWMutex
}

// NewPaneView creates a new pane view container for a WebView widget.
func NewPaneView(factory layout.WidgetFactory, paneID entity.PaneID, webViewWidget layout.Widget) *PaneView {
	overlay := factory.NewOverlay()
	overlay.SetHexpand(true)
	overlay.SetVexpand(true)
	overlay.SetVisible(true)

	// Set the WebView as the main child
	if webViewWidget != nil {
		webViewWidget.SetVisible(true)
		overlay.SetChild(webViewWidget)
	}

	// Create border overlay box (used for active pane indication)
	// This is an empty box that gets styled via CSS to show a border
	borderBox := factory.NewBox(layout.OrientationVertical, 0)
	borderBox.SetCanFocus(false)
	borderBox.SetCanTarget(false) // Don't intercept pointer events - let them pass through to WebView
	borderBox.AddCssClass("pane-border")
	// Position border to cover entire pane
	borderBox.SetHexpand(true)
	borderBox.SetVexpand(true)
	overlay.AddOverlay(borderBox)

	// Don't let border affect layout
	overlay.SetClipOverlay(borderBox, false)
	overlay.SetMeasureOverlay(borderBox, false)

	return &PaneView{
		factory:       factory,
		overlay:       overlay,
		webViewWidget: webViewWidget,
		borderBox:     borderBox,
		paneID:        paneID,
		isActive:      false,
	}
}

// SetActive updates the active state of the pane.
// Active panes display a visual border indicator.
func (pv *PaneView) SetActive(active bool) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	if pv.isActive == active {
		return
	}

	pv.isActive = active

	if active {
		pv.borderBox.AddCssClass(activePaneClass)
	} else {
		pv.borderBox.RemoveCssClass(activePaneClass)
	}
}

// IsActive returns whether this pane is currently active.
func (pv *PaneView) IsActive() bool {
	pv.mu.RLock()
	defer pv.mu.RUnlock()

	return pv.isActive
}

// PaneID returns the ID of the pane this view represents.
func (pv *PaneView) PaneID() entity.PaneID {
	return pv.paneID
}

// WebViewWidget returns the underlying WebView widget.
func (pv *PaneView) WebViewWidget() layout.Widget {
	pv.mu.RLock()
	defer pv.mu.RUnlock()

	return pv.webViewWidget
}

// SetWebViewWidget replaces the WebView widget.
func (pv *PaneView) SetWebViewWidget(widget layout.Widget) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	// Remove old widget from this overlay
	if pv.webViewWidget != nil {
		pv.overlay.SetChild(nil)
	}

	pv.webViewWidget = widget

	if widget != nil {
		// Unparent widget from any previous parent (critical for rebuild scenarios)
		// In GTK4, a widget can only have one parent at a time
		if widget.GetParent() != nil {
			widget.Unparent()
		}
		widget.SetVisible(true)
		pv.overlay.SetChild(widget)
	}
}

// GrabFocus attempts to focus the WebView.
// Returns true if focus was successfully grabbed.
func (pv *PaneView) GrabFocus() bool {
	pv.mu.RLock()
	wv := pv.webViewWidget
	pv.mu.RUnlock()

	if wv == nil {
		return false
	}
	return wv.GrabFocus()
}

// HasFocus returns whether the WebView currently has focus.
func (pv *PaneView) HasFocus() bool {
	pv.mu.RLock()
	wv := pv.webViewWidget
	pv.mu.RUnlock()

	if wv == nil {
		return false
	}
	return wv.HasFocus()
}

// SetOnFocusIn sets the callback for when the pane gains focus.
func (pv *PaneView) SetOnFocusIn(fn func(paneID entity.PaneID)) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	pv.onFocusIn = fn
}

// SetOnFocusOut sets the callback for when the pane loses focus.
func (pv *PaneView) SetOnFocusOut(fn func(paneID entity.PaneID)) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	pv.onFocusOut = fn
}

// Widget returns the underlying overlay widget for embedding in containers.
func (pv *PaneView) Widget() layout.Widget {
	return pv.overlay
}

// Overlay returns the underlying overlay widget for direct access.
func (pv *PaneView) Overlay() layout.OverlayWidget {
	return pv.overlay
}

// Show makes the pane visible.
func (pv *PaneView) Show() {
	pv.overlay.Show()
}

// Hide makes the pane invisible.
func (pv *PaneView) Hide() {
	pv.overlay.Hide()
}

// SetVisible sets the visibility of the pane.
func (pv *PaneView) SetVisible(visible bool) {
	pv.overlay.SetVisible(visible)
}

// IsVisible returns whether the pane is visible.
func (pv *PaneView) IsVisible() bool {
	return pv.overlay.IsVisible()
}

// AddCssClass adds a CSS class to the overlay.
func (pv *PaneView) AddCssClass(class string) {
	pv.overlay.AddCssClass(class)
}

// RemoveCssClass removes a CSS class from the overlay.
func (pv *PaneView) RemoveCssClass(class string) {
	pv.overlay.RemoveCssClass(class)
}

// AddOverlayWidget adds a widget as an overlay on this pane.
// The widget will appear above the WebView and border.
func (pv *PaneView) AddOverlayWidget(widget layout.Widget) {
	pv.overlay.AddOverlay(widget)
	pv.overlay.SetClipOverlay(widget, false)
	pv.overlay.SetMeasureOverlay(widget, false)
}

// RemoveOverlayWidget removes an overlay widget from this pane.
func (pv *PaneView) RemoveOverlayWidget(widget layout.Widget) {
	pv.overlay.RemoveOverlay(widget)
}

// GetContentDimensions returns the pane's allocated width and height.
func (pv *PaneView) GetContentDimensions() (width, height int) {
	return pv.overlay.GetAllocatedWidth(), pv.overlay.GetAllocatedHeight()
}
