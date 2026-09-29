package ui

import (
	"context"
	"fmt"
	"strings"
	"unsafe"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/puregotk/v4/gtk"
)

type floatingWorkspaceSession struct {
	paneID              entity.PaneID
	pane                *component.FloatingPane
	paneView            *component.PaneView
	webView             port.WebView
	overlay             layout.OverlayWidget
	widget              layout.Widget
	focusWidget         layout.Widget
	omnibox             *component.Omnibox
	omniboxWidget       layout.Widget
	resizeWatcherActive bool
	// resizeTickCallback keeps the purego callback live until its GTK source is
	// removed. It is cleared exactly once with resizeTickID.
	resizeTickCallback *gtk.TickCallback
	resizeTickID       uint
	appliedWidth       int
	appliedHeight      int
}

type floatingSessionKey struct {
	tabID     entity.TabID
	sessionID string
}

func normalizeFloatingSessionID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return floatingSessionIDDefault
	}
	return sessionID
}

func floatingPaneIDForSession(tabID entity.TabID, sessionID string) entity.PaneID {
	return entity.PaneID(floatingPaneIDPrefix + string(tabID) + ":" + normalizeFloatingSessionID(sessionID))
}

func floatingSessionMapKey(tabID entity.TabID, sessionID string) floatingSessionKey {
	return floatingSessionKey{tabID: tabID, sessionID: normalizeFloatingSessionID(sessionID)}
}

func decorateFloatingOverlay(overlay layout.OverlayWidget) {
	if overlay == nil {
		return
	}

	overlay.SetHexpand(false)
	overlay.SetVexpand(false)
	overlay.AddCssClass("floating-pane-container")
	overlay.AddCssClass("pane-border")
	overlay.AddCssClass("pane-active")
}

// showFloatingWidget makes a floating pane widget interactive and visible.
// Visibility is controlled by CSS class to animate opacity while keeping the
// widget mapped so WebKit can redraw without a hard pop.
func showFloatingWidget(w layout.Widget) {
	if w == nil {
		return
	}
	w.AddCssClass(floatingPaneVisibleClass)
	w.SetCanTarget(true)
	w.SetCanFocus(true)
}

// hideFloatingWidget makes a floating pane widget invisible and non-interactive.
// Removes the visible CSS class instead of SetVisible(false) so GTK keeps the
// widget mapped and WebKit retains its rendered surface in GPU memory.
func hideFloatingWidget(w layout.Widget) {
	if w == nil {
		return
	}
	w.RemoveCssClass(floatingPaneVisibleClass)
	w.SetCanTarget(false)
	w.SetCanFocus(false)
}

// setFloatingWidgetShown applies show or hide based on the visible flag.
func setFloatingWidgetShown(w layout.Widget, visible bool) {
	if visible {
		showFloatingWidget(w)
	} else {
		hideFloatingWidget(w)
	}
}

func configureFloatingOverlayMeasurement(workspaceOverlay layout.OverlayWidget, floatingOverlay layout.Widget) {
	if workspaceOverlay == nil || floatingOverlay == nil {
		return
	}

	workspaceOverlay.SetMeasureOverlay(floatingOverlay, false)
	workspaceOverlay.SetClipOverlay(floatingOverlay, false)
}

func floatingAllocationRect(overlayWidth, overlayHeight, desiredWidth, desiredHeight int) (x, y, width, height int, ok bool) {
	if overlayWidth <= 0 || overlayHeight <= 0 || desiredWidth <= 0 || desiredHeight <= 0 {
		return 0, 0, 0, 0, false
	}

	width = desiredWidth
	height = desiredHeight
	if width > overlayWidth {
		width = overlayWidth
	}
	if height > overlayHeight {
		height = overlayHeight
	}

	x = (overlayWidth - width) / 2
	y = (overlayHeight - height) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	return x, y, width, height, true
}

type gtkAllocationLayout [4]int32

const gtkAllocationLayoutSize = 16

var (
	_ [gtkAllocationLayoutSize - int(unsafe.Sizeof(gtkAllocationLayout{}))]struct{}
	_ [int(unsafe.Sizeof(gtkAllocationLayout{})) - gtkAllocationLayoutSize]struct{}
)

// writeOverlayAllocation writes GTK-owned GtkAllocation fields in place.
// Assumes GTK4 C layout for GdkRectangle/GtkAllocation: four contiguous
// 32-bit signed integers {x, y, width, height} in native endianness.
// If GTK struct layout changes, revisit offsets and writes below.
//
//nolint:gosec // Intentional unsafe pointer math against GTK-owned allocation memory.
func writeOverlayAllocation(allocationPtr *uintptr, x, y, width, height int) bool {
	if allocationPtr == nil {
		return false
	}

	const (
		gtkAllocationOffsetX      = uintptr(0)
		gtkAllocationOffsetY      = uintptr(4)
		gtkAllocationOffsetWidth  = uintptr(8)
		gtkAllocationOffsetHeight = uintptr(12)
	)

	// GtkAllocation/GdkRectangle is four 32-bit signed integers in C.
	base := unsafe.Pointer(allocationPtr)
	*(*int32)(unsafe.Pointer(uintptr(base) + gtkAllocationOffsetX)) = int32(x)
	*(*int32)(unsafe.Pointer(uintptr(base) + gtkAllocationOffsetY)) = int32(y)
	*(*int32)(unsafe.Pointer(uintptr(base) + gtkAllocationOffsetWidth)) = int32(width)
	*(*int32)(unsafe.Pointer(uintptr(base) + gtkAllocationOffsetHeight)) = int32(height)
	return true
}

func (a *App) floatingAllocationForWidget(
	tabID entity.TabID,
	widgetPtr uintptr,
	overlayWidth, overlayHeight int,
) (x, y, width, height int, ok bool) {
	for _, session := range a.floatingSessions.forTab(tabID) {
		if session.widget == nil {
			continue
		}

		gtkWidget := session.widget.GtkWidget()
		if gtkWidget == nil || gtkWidget.GoPointer() != widgetPtr {
			continue
		}

		desiredWidth := session.appliedWidth
		desiredHeight := session.appliedHeight
		if (desiredWidth <= 0 || desiredHeight <= 0) && session.pane != nil {
			session.pane.Resize()
			desiredWidth, desiredHeight = session.pane.Dimensions()
		}

		return floatingAllocationRect(overlayWidth, overlayHeight, desiredWidth, desiredHeight)
	}

	return 0, 0, 0, 0, false
}

func (a *App) installFloatingOverlayPositioning(tabID entity.TabID, workspaceOverlay layout.OverlayWidget) {
	if workspaceOverlay == nil {
		return
	}

	workspaceWidget := workspaceOverlay.GtkWidget()
	if workspaceWidget == nil {
		return
	}

	gtkOverlay := gtk.OverlayNewFromInternalPtr(workspaceWidget.GoPointer())
	if gtkOverlay == nil {
		return
	}

	cb := func(_ gtk.Overlay, widgetPtr uintptr, allocationPtr *uintptr) bool {
		widget := gtk.WidgetNewFromInternalPtr(widgetPtr)
		if widget != nil {
			widgetPtr = widget.GoPointer()
		}
		overlayWidth := workspaceOverlay.GetAllocatedWidth()
		overlayHeight := workspaceOverlay.GetAllocatedHeight()
		x, y, width, height, ok := a.floatingAllocationForWidget(tabID, widgetPtr, overlayWidth, overlayHeight)
		if !ok {
			return false
		}
		return writeOverlayAllocation(allocationPtr, x, y, width, height)
	}

	gtkOverlay.ConnectGetChildPosition(&cb)
}

// currentFloatingWidthPct returns the configured floating pane width percentage.
func (a *App) currentFloatingWidthPct() float64 {
	return a.runtimeConfigSnapshot().UI.Workspace.FloatingPane.WidthPct
}

// currentFloatingHeightPct returns the configured floating pane height percentage.
func (a *App) currentFloatingHeightPct() float64 {
	return a.runtimeConfigSnapshot().UI.Workspace.FloatingPane.HeightPct
}

func (a *App) ensureFloatingSession(
	ctx context.Context,
	tabID entity.TabID,
	sessionID string,
	wsView *component.WorkspaceView,
) (*floatingWorkspaceSession, error) {
	key := floatingSessionMapKey(tabID, sessionID)
	if session, ok := a.floatingSessions[key]; ok && session != nil {
		if wsView == nil {
			return session, nil
		}
		session.pane.SetParentOverlay(wsView.WorkspaceOverlayWidget())
		if session.widget != nil {
			wsView.AddWorkspaceOverlayWidget(session.widget)
			configureFloatingOverlayMeasurement(wsView.WorkspaceOverlayWidget(), session.widget)
		}
		if session.paneView != nil {
			wsView.RegisterPaneView(session.paneID, session.paneView)
			if session.focusWidget == nil {
				session.focusWidget = session.paneView.WebViewWidget()
			}
		}
		return session, nil
	}

	if wsView == nil {
		return nil, fmt.Errorf("workspace view not found for tab %s", tabID)
	}
	if a.contentCoord == nil {
		return nil, fmt.Errorf("content coordinator not initialized")
	}
	if a.widgetFactory == nil {
		return nil, fmt.Errorf("widget factory not initialized")
	}

	paneID := floatingPaneIDForSession(tabID, sessionID)
	wv, err := a.contentCoord.EnsureWebView(ctx, paneID)
	if err != nil {
		return nil, fmt.Errorf("ensure floating webview: %w", err)
	}

	webViewWidget := a.contentCoord.WrapWidget(ctx, wv)
	if webViewWidget == nil {
		return nil, fmt.Errorf("wrap floating webview widget")
	}
	webViewWidget.SetHexpand(true)
	webViewWidget.SetVexpand(true)

	// Wrap the WebView in a PaneView so it gets progress bar, loading
	// skeleton, and link status overlays — the same widget hierarchy as
	// regular workspace panes. This allows the content coordinator's
	// existing callbacks (onLoadStarted, onProgressChanged, etc.) to find
	// and update the floating pane automatically.
	pv := component.NewPaneView(ctx, a.widgetFactory, paneID, webViewWidget)
	pv.SetActive(true)
	// Hide loading skeleton — floating panes have their own themed container.
	pv.HideLoadingSkeleton()
	wsView.RegisterPaneView(paneID, pv)

	// Use the PaneView's overlay directly as the floating overlay.
	// This avoids nesting two overlays which causes GTK4 allocation
	// issues (inner overlay expanding to fill the parent's allocation).
	pvOverlay := pv.Overlay()
	decorateFloatingOverlay(pvOverlay)
	pvOverlay.SetHalign(gtk.AlignCenterValue)
	pvOverlay.SetValign(gtk.AlignCenterValue)
	hideFloatingWidget(pvOverlay)
	wsView.AddWorkspaceOverlayWidget(pvOverlay)
	configureFloatingOverlayMeasurement(wsView.WorkspaceOverlayWidget(), pvOverlay)

	floatingPane := component.NewFloatingPane(wsView.WorkspaceOverlayWidget(), component.FloatingPaneOptions{
		WidthPct:       a.currentFloatingWidthPct(),
		HeightPct:      a.currentFloatingHeightPct(),
		FallbackWidth:  floatingPaneFallbackWidth,
		FallbackHeight: floatingPaneFallbackHeight,
		OnNavigate: func(navCtx context.Context, url string) error {
			return wv.LoadURI(navCtx, url)
		},
	})

	session := &floatingWorkspaceSession{
		paneID:      paneID,
		pane:        floatingPane,
		paneView:    pv,
		webView:     wv,
		overlay:     pvOverlay,
		widget:      pvOverlay,
		focusWidget: webViewWidget,
	}
	a.floatingSessions[key] = session
	return session, nil
}

func (a *App) releaseFloatingSessionsForTab(ctx context.Context, tabID entity.TabID) {
	// Snapshot: releaseFloatingSession deletes from the registry.
	sessions := a.floatingSessions.forTabSnapshot(tabID)
	if len(sessions) == 0 {
		return
	}

	for key, session := range sessions {
		a.releaseFloatingSession(ctx, key, session)
	}
	a.syncFloatingFocus()
}

func (a *App) activeFloatingSessionEntry() (floatingSessionKey, *floatingWorkspaceSession, bool) {
	activeTab := a.activeTabForBrowserWindow(a.lastFocusedBrowserWindow())
	if activeTab == nil {
		return floatingSessionKey{}, nil, false
	}
	return a.floatingSessions.visibleForTab(activeTab.ID)
}

func (a *App) activeFloatingSession() (*floatingWorkspaceSession, entity.TabID) {
	key, session, ok := a.activeFloatingSessionEntry()
	if !ok {
		return nil, key.tabID
	}
	return session, key.tabID
}

func (a *App) releaseFloatingSession(ctx context.Context, key floatingSessionKey, session *floatingWorkspaceSession) {
	if session == nil {
		return
	}

	a.stopFloatingResizeWatcher(session)
	if session.pane != nil {
		session.pane.Hide(ctx)
	}
	a.hideFloatingOmnibox(ctx, session)
	if wsView := a.workspaceViews[key.tabID]; wsView != nil {
		if session.widget != nil {
			wsView.RemoveWorkspaceOverlayWidget(session.widget)
		}
		if session.paneID != "" {
			wsView.UnregisterPaneView(session.paneID)
		}
	}
	if a.contentCoord != nil && session.paneID != "" {
		a.contentCoord.ReleaseWebView(ctx, session.paneID)
	}
	delete(a.floatingSessions, key)

	session.appliedWidth = 0
	session.appliedHeight = 0
	session.focusWidget = nil
	session.paneView = nil
	session.widget = nil
	session.overlay = nil
	session.webView = nil
	session.pane = nil
	// Session is fully released/zeroed; caller must drop this reference and not reuse it.
}

func (a *App) updateFloatingSessionURI(paneID entity.PaneID, url string) {
	if session := a.floatingSessionByPaneID(paneID); session != nil && session.pane != nil {
		session.pane.RecordLoadedURL(url)
	}
}

func (a *App) syncFloatingFocus() {
	for _, wsView := range a.workspaceViews {
		if wsView != nil {
			wsView.SetHoverFocusLocked(false)
		}
	}

	if a.contentCoord == nil {
		return
	}

	activeTab := a.activeTabForBrowserWindow(a.lastFocusedBrowserWindow())
	if activeTab == nil {
		a.contentCoord.ClearActivePaneOverride()
		return
	}

	if session, _ := a.activeFloatingSession(); session != nil {
		if wsView := a.workspaceViews[activeTab.ID]; wsView != nil {
			wsView.SetHoverFocusLocked(true)
		}
		a.startFloatingResizeWatcher(session)
		a.contentCoord.SetActivePaneOverride(session.paneID)
		return
	}

	a.contentCoord.ClearActivePaneOverride()
}

func (a *App) showFloatingOmnibox(ctx context.Context, session *floatingWorkspaceSession) {
	if session == nil || session.overlay == nil || a.widgetFactory == nil {
		return
	}

	if session.omnibox == nil {
		cfg := a.omniboxCfg
		cfg.OnNavigate = func(navCtx context.Context, url string) error {
			if session.pane == nil {
				return fmt.Errorf("floating pane is not available")
			}
			if navCtx == nil {
				navCtx = ctx
			}
			return session.pane.Navigate(navCtx, url)
		}
		cfg.OnToast = func(toastCtx context.Context, message string, level component.ToastLevel) {
			a.showToastOnLastFocusedBrowserWindow(toastCtx, message, level)
		}

		omnibox := component.NewOmnibox(ctx, cfg)
		if omnibox == nil {
			return
		}

		omnibox.SetParentOverlay(session.overlay)
		omniboxWidget := omnibox.WidgetAsLayout(a.widgetFactory)
		if omniboxWidget == nil {
			return
		}

		session.overlay.AddOverlay(omniboxWidget)
		session.overlay.SetClipOverlay(omniboxWidget, false)
		session.overlay.SetMeasureOverlay(omniboxWidget, false)

		session.omnibox = omnibox
		session.omniboxWidget = omniboxWidget
	}

	session.pane.SetOmniboxVisible(true)
	session.omnibox.Show(ctx, "")
}

func (a *App) hideFloatingOmnibox(ctx context.Context, session *floatingWorkspaceSession) {
	if session == nil || session.pane == nil {
		return
	}

	session.pane.SetOmniboxVisible(false)
	if session.omnibox == nil {
		return
	}

	session.omnibox.Hide(ctx)
	if session.overlay != nil && session.omniboxWidget != nil {
		if parent := session.omniboxWidget.GetParent(); parent == session.overlay {
			session.overlay.RemoveOverlay(session.omniboxWidget)
		} else if parent != nil {
			session.omniboxWidget.Unparent()
		}
	}
	session.omnibox = nil
	session.omniboxWidget = nil
}

func floatingFocusWidget(session *floatingWorkspaceSession) layout.Widget {
	if session == nil {
		return nil
	}
	if session.focusWidget != nil {
		return session.focusWidget
	}
	if session.paneView != nil {
		return session.paneView.WebViewWidget()
	}
	return nil
}

func focusFloatingSessionContent(session *floatingWorkspaceSession) {
	if session == nil || session.pane == nil {
		return
	}
	if !session.pane.IsVisible() || session.pane.IsOmniboxVisible() {
		return
	}
	w := floatingFocusWidget(session)
	if w == nil {
		return
	}
	w.GrabFocus()
}

func (a *App) resizeFloatingWidget(session *floatingWorkspaceSession) {
	if session == nil || session.pane == nil {
		return
	}
	session.pane.Resize()
	if session.widget == nil {
		return
	}
	width, height := session.pane.Dimensions()
	if session.appliedWidth == width && session.appliedHeight == height {
		return
	}
	session.widget.SetSizeRequest(width, height)
	session.appliedWidth = width
	session.appliedHeight = height
	if a.contentCoord != nil {
		syncCtx := context.Background()
		if a.deps != nil && a.deps.Ctx != nil {
			syncCtx = a.deps.Ctx
		}
		a.contentCoord.SyncWebViewViewport(syncCtx, session.paneID, "floating-resize")
	}
}

func (a *App) handleFloatingViewportTick(session *floatingWorkspaceSession) bool {
	if session == nil || session.pane == nil {
		return false
	}
	if !session.pane.IsVisible() {
		session.resizeWatcherActive = false
		session.resizeTickID = 0
		return false
	}

	a.resizeFloatingWidget(session)
	return true
}

func (a *App) floatingSessionByPaneID(paneID entity.PaneID) *floatingWorkspaceSession {
	_, session, _ := a.floatingSessions.byPaneID(paneID)
	return session
}

// popupOwnerWindowIDForPane scopes named contexts to the top-level owner. A
// floating pane is mapped through its session's tab, never by parsing PaneID.
func (a *App) popupOwnerWindowIDForPane(paneID entity.PaneID) (string, bool) {
	if key, _, ok := a.floatingSessions.byPaneID(paneID); ok {
		if bw := a.browserWindowForTab(key.tabID); bw != nil {
			return bw.id, true
		}
		return "", false
	}
	if bw := a.browserWindowForAnyPane(paneID); bw != nil {
		return bw.id, true
	}
	return "", false
}

func (a *App) startFloatingResizeWatcher(session *floatingWorkspaceSession) {
	if session == nil || session.overlay == nil || session.resizeWatcherActive {
		return
	}

	overlayWidget := session.overlay.GtkWidget()
	if overlayWidget == nil {
		return
	}

	session.resizeWatcherActive = true
	paneID := session.paneID
	callback := new(gtk.TickCallback)
	*callback = func(_ uintptr, _ uintptr, _ uintptr) bool {
		liveSession := a.floatingSessionByPaneID(paneID)
		if liveSession == nil {
			session.releaseResizeTickCallback()
			return false
		}

		keepRunning := a.handleFloatingViewportTick(liveSession)
		if !keepRunning {
			liveSession.releaseResizeTickCallback()
		}
		return keepRunning
	}
	session.resizeTickCallback = callback
	session.resizeTickID = overlayWidget.AddTickCallback(callback, 0, nil)
}

func (a *App) stopFloatingResizeWatcher(session *floatingWorkspaceSession) {
	if session == nil {
		return
	}

	tickID := session.resizeTickID
	session.releaseResizeTickCallback()
	if tickID != 0 && session.overlay != nil {
		if overlayWidget := session.overlay.GtkWidget(); overlayWidget != nil {
			overlayWidget.RemoveTickCallback(tickID)
		}
	}
}

// releaseResizeTickCallback releases the raw purego callback slot. It is safe
// to call after GTK has already removed a callback that returned false.
func (session *floatingWorkspaceSession) releaseResizeTickCallback() {
	if session == nil {
		return
	}
	callback := session.resizeTickCallback
	session.resizeTickID = 0
	session.resizeTickCallback = nil
	session.resizeWatcherActive = false
	if callback != nil {
		// AddTickCallback creates a purego function-pointer slot. Clearing the Go
		// pointer alone cannot return that finite runtime slot to the ledger.
		_ = unrefTickCallback(callback)
	}
}

func (a *App) hideFloatingSession(ctx context.Context, session *floatingWorkspaceSession) {
	if session == nil || session.pane == nil {
		return
	}

	a.stopFloatingResizeWatcher(session)
	session.pane.Hide(ctx)
	a.hideFloatingOmnibox(ctx, session)
	hideFloatingWidget(session.widget)
	// Force a fresh size request on next show so WebKit gets a new
	// allocation cycle and repaints content after rapid toggles.
	session.appliedWidth = 0
	session.appliedHeight = 0
}

func (a *App) hideVisibleFloatingSessions(ctx context.Context, tabID entity.TabID, except *floatingWorkspaceSession) {
	for _, session := range a.floatingSessions.forTab(tabID) {
		if session == except || session.pane == nil {
			continue
		}
		if !session.pane.IsVisible() {
			continue
		}
		a.hideFloatingSession(ctx, session)
	}
}

func (a *App) closeActiveFloatingPane(ctx context.Context) bool {
	session, _ := a.activeFloatingSession()
	if session == nil {
		return false
	}
	a.hideFloatingSession(ctx, session)
	a.syncFloatingFocus()
	return true
}

func (a *App) handleGlobalEscape(ctx context.Context) bool {
	return a.closeActiveFloatingPane(ctx)
}

func (a *App) closeAndReleaseActiveFloatingPane(ctx context.Context) bool {
	key, session, ok := a.activeFloatingSessionEntry()
	if !ok {
		return false
	}
	a.releaseFloatingSession(ctx, key, session)
	a.syncFloatingFocus()
	return true
}

// ToggleFloatingPane toggles the active workspace floating pane visibility.
func (a *App) ToggleFloatingPane(ctx context.Context) error {
	activeTab := a.activeTabForBrowserWindow(a.lastFocusedBrowserWindow())
	if activeTab == nil {
		return nil
	}
	if a.closeActiveFloatingPane(ctx) {
		return nil
	}
	return a.openFloatingPaneSession(ctx, floatingSessionIDDefault)
}

// OpenFloatingPaneURL opens the active workspace floating pane directly to a URL.
func (a *App) OpenFloatingPaneURL(ctx context.Context, url string) error {
	return a.openFloatingPaneSession(ctx, floatingSessionIDDefault, url)
}

// OpenFloatingPaneProfileURL opens a named floating pane profile session directly to a URL.
func (a *App) OpenFloatingPaneProfileURL(ctx context.Context, sessionID, url string) error {
	return a.openFloatingPaneSession(ctx, sessionID, url)
}

func (a *App) openFloatingPaneSession(ctx context.Context, sessionID string, url ...string) error {
	activeTab := a.activeTabForBrowserWindow(a.lastFocusedBrowserWindow())
	if activeTab == nil {
		return nil
	}

	wsView := a.workspaceViews[activeTab.ID]
	session, err := a.ensureFloatingSession(ctx, activeTab.ID, sessionID, wsView)
	if err != nil {
		return err
	}
	hasURL := len(url) > 0 && strings.TrimSpace(url[0]) != ""
	isProfileSession := normalizeFloatingSessionID(sessionID) != floatingSessionIDDefault

	if hasURL && isProfileSession && session.pane.IsVisible() {
		a.hideFloatingSession(ctx, session)
		a.syncFloatingFocus()
		return nil
	}

	a.hideVisibleFloatingSessions(ctx, activeTab.ID, session)

	if hasURL {
		if isProfileSession && session.pane.SessionStarted() {
			session.pane.Show()
		} else {
			if err := session.pane.ShowURL(ctx, url[0]); err != nil {
				return err
			}
		}
	} else {
		if err := session.pane.ShowToggle(ctx); err != nil {
			return err
		}
	}

	a.resizeFloatingWidget(session)
	setFloatingWidgetShown(session.widget, session.pane.IsVisible())
	if session.pane.IsOmniboxVisible() {
		a.showFloatingOmnibox(ctx, session)
	} else {
		a.hideFloatingOmnibox(ctx, session)
		focusFloatingSessionContent(session)
	}
	a.startFloatingResizeWatcher(session)
	a.syncFloatingFocus()

	return nil
}
