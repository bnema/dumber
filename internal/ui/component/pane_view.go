// Package component provides UI components for the browser.
package component

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/input"
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
	webViewWidget layout.Widget      // The actual WebView widget
	borderBox     layout.BoxWidget   // Border overlay for active indication
	progressBar   *ProgressBar       // Loading progress indicator
	toaster       *Toaster           // Toast notification overlay
	linkStatus    *LinkStatusOverlay // Link hover URL overlay
	loading       *LoadingSkeleton   // Placeholder shown until WebView paints
	paneID        entity.PaneID
	isActive      bool

	onFocusIn  func(paneID entity.PaneID)
	onFocusOut func(paneID entity.PaneID)
	onHover    func(paneID entity.PaneID)

	hoverHandler *input.HoverHandler

	mu sync.RWMutex
}

// NewPaneView creates a new pane view container for a WebView widget.
func NewPaneView(factory layout.WidgetFactory, paneID entity.PaneID, webViewWidget layout.Widget) *PaneView {
	overlay := factory.NewOverlay()
	overlay.SetHexpand(true)
	overlay.SetVexpand(true)
	overlay.SetVisible(true)
	overlay.AddCssClass("pane-overlay") // Theme background prevents white flash

	// Set the WebView as the main child
	// Note: WebView may be hidden initially (see pool.go) - shown on LoadCommitted
	if webViewWidget != nil {
		overlay.SetChild(webViewWidget)
	}

	// Loading skeleton overlay (shown until WebView paints)
	loading := NewLoadingSkeleton(factory)
	loading.Widget().SetCanFocus(false)
	loading.Widget().SetCanTarget(false)
	overlay.AddOverlay(loading.Widget())
	// Don't let loading affect layout
	overlay.SetClipOverlay(loading.Widget(), false)
	overlay.SetMeasureOverlay(loading.Widget(), false)

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

	// Progress bar is created lazily on first use to avoid GTK measurement
	// issues with the internal progress gizmo before the overlay is realized

	return &PaneView{
		factory:       factory,
		overlay:       overlay,
		webViewWidget: webViewWidget,
		borderBox:     borderBox,
		progressBar:   nil, // Created lazily in ensureProgressBar()
		loading:       loading,
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

// SetOnHover sets the callback for when the pane is hovered.
func (pv *PaneView) SetOnHover(fn func(paneID entity.PaneID)) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	pv.onHover = fn
}

// AttachHoverHandler creates and attaches a hover handler for focus-follows-mouse behavior.
func (pv *PaneView) AttachHoverHandler(ctx context.Context) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	// Create hover handler
	pv.hoverHandler = input.NewHoverHandler(ctx, pv.paneID)

	// Wire up callback
	pv.hoverHandler.SetOnEnter(func(paneID entity.PaneID) {
		pv.mu.RLock()
		callback := pv.onHover
		pv.mu.RUnlock()

		if callback != nil {
			callback(paneID)
		}
	})

	// Attach to overlay widget
	gtkWidget := pv.overlay.GtkWidget()
	if gtkWidget != nil {
		pv.hoverHandler.AttachTo(gtkWidget)
	}
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

// ensureProgressBar creates the progress bar lazily on first use.
// This avoids GTK measurement issues with the internal progress gizmo
// that can occur when the widget is added before the overlay is realized.
// Must be called with write lock held.
func (pv *PaneView) ensureProgressBar() *ProgressBar {
	if pv.progressBar != nil {
		return pv.progressBar
	}

	pb := NewProgressBar(pv.factory)
	pv.overlay.AddOverlay(pb.Widget())
	pv.overlay.SetClipOverlay(pb.Widget(), false)
	pv.overlay.SetMeasureOverlay(pb.Widget(), false)
	pv.progressBar = pb
	return pb
}

// SetLoadProgress updates the progress bar with the current load progress.
// progress should be between 0.0 and 1.0.
func (pv *PaneView) SetLoadProgress(progress float64) {
	pv.mu.Lock()
	pb := pv.ensureProgressBar()
	pv.mu.Unlock()

	pb.SetProgress(progress)
}

// SetLoading shows or hides the progress bar.
func (pv *PaneView) SetLoading(loading bool) {
	pv.mu.Lock()
	pb := pv.ensureProgressBar()
	pv.mu.Unlock()

	if loading {
		pb.Show()
	} else {
		pb.Hide()
	}
}

// HideLoadingSkeleton hides the loading skeleton overlay (if present).
func (pv *PaneView) HideLoadingSkeleton() {
	pv.mu.Lock()
	loading := pv.loading
	pv.mu.Unlock()

	if loading != nil {
		loading.SetVisible(false)
	}
}

// ensureToaster creates the toaster lazily on first use.
// Must be called with write lock held.
func (pv *PaneView) ensureToaster() *Toaster {
	if pv.toaster != nil {
		return pv.toaster
	}

	t := NewToaster(pv.factory)
	pv.overlay.AddOverlay(t.Widget())
	pv.overlay.SetClipOverlay(t.Widget(), false)
	pv.overlay.SetMeasureOverlay(t.Widget(), false)
	pv.toaster = t
	return t
}

// ShowToast displays a toast notification with the given message and level.
// If a toast is already visible, updates the text and resets the dismiss timer.
func (pv *PaneView) ShowToast(ctx context.Context, message string, level ToastLevel) {
	pv.mu.Lock()
	t := pv.ensureToaster()
	pv.mu.Unlock()

	t.Show(ctx, message, level)
}

// ShowZoomToast displays a zoom level toast notification.
// Formats the zoom percentage with a % suffix.
func (pv *PaneView) ShowZoomToast(ctx context.Context, zoomPercent int) {
	pv.mu.Lock()
	t := pv.ensureToaster()
	pv.mu.Unlock()

	t.ShowZoom(ctx, zoomPercent)
}

// ensureLinkStatus creates the link status overlay lazily on first use.
// Must be called with write lock held.
func (pv *PaneView) ensureLinkStatus() *LinkStatusOverlay {
	if pv.linkStatus != nil {
		return pv.linkStatus
	}

	ls := NewLinkStatusOverlay(pv.factory)
	pv.overlay.AddOverlay(ls.Widget())
	pv.overlay.SetClipOverlay(ls.Widget(), false)
	pv.overlay.SetMeasureOverlay(ls.Widget(), false)
	pv.linkStatus = ls
	return ls
}

// ShowLinkStatus displays the link status overlay with the given URI.
// If uri is empty, hides the overlay instead.
func (pv *PaneView) ShowLinkStatus(uri string) {
	pv.mu.Lock()
	ls := pv.ensureLinkStatus()
	pv.mu.Unlock()

	ls.Show(uri)
}

// HideLinkStatus hides the link status overlay.
func (pv *PaneView) HideLinkStatus() {
	pv.mu.Lock()
	ls := pv.linkStatus
	pv.mu.Unlock()

	if ls != nil {
		ls.Hide()
	}
}

// Cleanup removes the WebView widget from the overlay and clears references.
// This must be called before destroying the WebView to ensure proper GTK cleanup.
// After calling Cleanup, the PaneView should not be reused.
func (pv *PaneView) Cleanup() {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	// Clear callbacks to prevent use-after-free
	pv.onFocusIn = nil
	pv.onFocusOut = nil
	pv.onHover = nil

	// Detach hover handler if present
	if pv.hoverHandler != nil {
		pv.hoverHandler.Detach()
		pv.hoverHandler = nil
	}

	// Remove WebView from overlay (unparents it from GTK hierarchy)
	if pv.webViewWidget != nil {
		pv.overlay.SetChild(nil)
		pv.webViewWidget = nil
	}

	// Clear other overlay children
	if pv.progressBar != nil {
		pv.overlay.RemoveOverlay(pv.progressBar.Widget())
		pv.progressBar = nil
	}
	if pv.toaster != nil {
		pv.overlay.RemoveOverlay(pv.toaster.Widget())
		pv.toaster = nil
	}
	if pv.linkStatus != nil {
		pv.linkStatus.Cleanup() // Cancel pending timers before removal
		pv.overlay.RemoveOverlay(pv.linkStatus.Widget())
		pv.linkStatus = nil
	}
}
