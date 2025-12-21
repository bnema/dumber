// Package component provides UI components for the browser.
package component

import (
	"context"
	"errors"
	"sync"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/rs/zerolog"
)

// ErrNilWorkspace is returned when attempting to set a nil workspace.
var ErrNilWorkspace = errors.New("workspace is nil")

// ErrPaneNotFound is returned when a pane ID cannot be found.
var ErrPaneNotFound = errors.New("pane not found")

// WorkspaceView is the top-level container that renders a workspace's pane tree.
// It manages the widget tree and handles active pane state.
type WorkspaceView struct {
	factory      layout.WidgetFactory
	treeRenderer *layout.TreeRenderer
	container    layout.BoxWidget
	overlay      layout.OverlayWidget // Wraps container for mode borders
	rootWidget   layout.Widget        // Current root widget for removal on rebuild
	logger       zerolog.Logger

	// Mode border overlay slot
	modeBorderWidget layout.Widget

	// Omnibox lifecycle
	omnibox       *Omnibox      // Current omnibox (nil if not shown)
	omniboxWidget layout.Widget // Wrapped omnibox widget for overlay
	omniboxPaneID entity.PaneID // Which pane has the omnibox
	omniboxCfg    OmniboxConfig // Stored config for creating omniboxes

	// Find bar lifecycle
	findBar       *FindBar      // Current find bar (nil if not shown)
	findBarWidget layout.Widget // Wrapped find bar widget for overlay
	findBarPaneID entity.PaneID // Which pane has the find bar
	findBarCfg    FindBarConfig // Stored config for creating find bars

	workspace    *entity.Workspace
	paneViews    map[entity.PaneID]*PaneView
	activePaneID entity.PaneID

	onPaneFocused func(paneID entity.PaneID)

	mu sync.RWMutex
}

// paneViewFactoryAdapter adapts WorkspaceView to the PaneViewFactory interface
// so TreeRenderer can create PaneViews.
type paneViewFactoryAdapter struct {
	wv  *WorkspaceView
	ctx context.Context
}

func (a *paneViewFactoryAdapter) CreatePaneView(node *entity.PaneNode) layout.Widget {
	if node == nil || node.Pane == nil {
		return nil
	}

	// Create PaneView without WebView widget for now
	// WebView will be attached later by the application layer
	pv := NewPaneView(a.wv.factory, node.Pane.ID, nil)

	// Store in map for later lookup
	// Note: Caller (SetWorkspace) already holds the lock, so we access directly
	a.wv.paneViews[node.Pane.ID] = pv

	// Set up focus callback
	pv.SetOnFocusIn(func(paneID entity.PaneID) {
		a.wv.mu.RLock()
		callback := a.wv.onPaneFocused
		a.wv.mu.RUnlock()

		if callback != nil {
			callback(paneID)
		}
	})

	// Set up hover callback for focus-follows-mouse
	pv.SetOnHover(func(paneID entity.PaneID) {
		// Skip if this pane is already active
		if a.wv.GetActivePaneID() == paneID {
			return
		}

		// Activate the hovered pane and grab focus
		if err := a.wv.SetActivePaneID(paneID); err == nil {
			a.wv.FocusPane(paneID)
		}
	})

	// Attach hover handler with debouncing
	pv.AttachHoverHandler(a.ctx)

	return pv.Widget()
}

// NewWorkspaceView creates a new workspace view.
func NewWorkspaceView(ctx context.Context, factory layout.WidgetFactory) *WorkspaceView {
	log := logging.FromContext(ctx)

	container := factory.NewBox(layout.OrientationVertical, 0)
	container.SetHexpand(true)
	container.SetVexpand(true)
	container.SetVisible(true)

	// Wrap container in overlay for mode borders
	overlay := factory.NewOverlay()
	overlay.SetHexpand(true)
	overlay.SetVexpand(true)
	overlay.SetChild(container)
	overlay.SetVisible(true)

	wv := &WorkspaceView{
		factory:   factory,
		container: container,
		overlay:   overlay,
		logger:    log.With().Str("component", "workspace-view").Logger(),
		paneViews: make(map[entity.PaneID]*PaneView),
	}

	// Create tree renderer with our adapter as the pane view factory
	wv.treeRenderer = layout.NewTreeRenderer(ctx, factory, &paneViewFactoryAdapter{wv: wv, ctx: ctx})

	return wv
}

// SetWorkspace sets the workspace to render and builds the widget tree.
func (wv *WorkspaceView) SetWorkspace(ctx context.Context, ws *entity.Workspace) error {
	if ws == nil {
		return ErrNilWorkspace
	}

	wv.mu.Lock()
	defer wv.mu.Unlock()

	// Clear previous state
	wv.workspace = ws
	wv.paneViews = make(map[entity.PaneID]*PaneView)

	// Remove old root widget from container before building new tree
	if wv.rootWidget != nil {
		wv.container.Remove(wv.rootWidget)
		wv.rootWidget = nil
	}

	// Build new tree
	if ws.Root != nil {
		widget, err := wv.treeRenderer.Build(ctx, ws.Root)
		if err != nil {
			return err
		}

		if widget != nil {
			widget.SetVisible(true)
			wv.container.Append(widget)
			wv.rootWidget = widget
		}
	}

	// Set initial active pane
	if ws.ActivePaneID != "" {
		wv.setActivePaneIDInternal(ws.ActivePaneID)
	}

	return nil
}

// SetActivePaneID updates which pane is visually marked as active.
func (wv *WorkspaceView) SetActivePaneID(paneID entity.PaneID) error {
	wv.mu.Lock()
	defer wv.mu.Unlock()

	return wv.setActivePaneIDInternal(paneID)
}

// setActivePaneIDInternal updates active pane without locking.
func (wv *WorkspaceView) setActivePaneIDInternal(paneID entity.PaneID) error {
	// Destroy omnibox if active pane is changing
	if wv.activePaneID != paneID && wv.omnibox != nil {
		wv.hideOmniboxInternal()
	}
	// Destroy find bar if active pane is changing
	if wv.activePaneID != paneID && wv.findBar != nil {
		wv.hideFindBarInternal()
	}

	// Deactivate current active pane
	if wv.activePaneID != "" {
		if oldPV, ok := wv.paneViews[wv.activePaneID]; ok {
			oldPV.SetActive(false)
		}
	}

	// Activate new pane
	newPV, ok := wv.paneViews[paneID]
	if !ok {
		return ErrPaneNotFound
	}

	newPV.SetActive(true)
	wv.activePaneID = paneID

	return nil
}

// GetActivePaneID returns the ID of the currently active pane.
func (wv *WorkspaceView) GetActivePaneID() entity.PaneID {
	wv.mu.RLock()
	defer wv.mu.RUnlock()

	return wv.activePaneID
}

// GetPaneView returns the PaneView for a given pane ID.
// Returns nil if not found.
func (wv *WorkspaceView) GetPaneView(paneID entity.PaneID) *PaneView {
	wv.mu.RLock()
	defer wv.mu.RUnlock()

	return wv.paneViews[paneID]
}

// GetPaneIDs returns all pane IDs in this workspace view.
func (wv *WorkspaceView) GetPaneIDs() []entity.PaneID {
	wv.mu.RLock()
	defer wv.mu.RUnlock()

	ids := make([]entity.PaneID, 0, len(wv.paneViews))
	for id := range wv.paneViews {
		ids = append(ids, id)
	}
	return ids
}

// PaneCount returns the number of panes in the workspace view.
func (wv *WorkspaceView) PaneCount() int {
	wv.mu.RLock()
	defer wv.mu.RUnlock()

	return len(wv.paneViews)
}

// SetOnPaneFocused sets the callback for when a pane receives focus.
func (wv *WorkspaceView) SetOnPaneFocused(fn func(paneID entity.PaneID)) {
	wv.mu.Lock()
	defer wv.mu.Unlock()

	wv.onPaneFocused = fn
}

// Rebuild rebuilds the widget tree from the current workspace.
// Use this after structural changes like splits or closes.
func (wv *WorkspaceView) Rebuild(ctx context.Context) error {
	wv.mu.RLock()
	ws := wv.workspace
	wv.mu.RUnlock()

	if ws == nil {
		return ErrNilWorkspace
	}

	return wv.SetWorkspace(ctx, ws)
}

// FocusPane attempts to give focus to a specific pane.
// Returns true if focus was successfully grabbed.
func (wv *WorkspaceView) FocusPane(paneID entity.PaneID) bool {
	wv.mu.RLock()
	pv, ok := wv.paneViews[paneID]
	wv.mu.RUnlock()

	if !ok || pv == nil {
		return false
	}

	return pv.GrabFocus()
}

// SetWebViewWidget attaches a WebView widget to a specific pane.
func (wv *WorkspaceView) SetWebViewWidget(paneID entity.PaneID, widget layout.Widget) error {
	wv.mu.RLock()
	pv, ok := wv.paneViews[paneID]
	wv.mu.RUnlock()

	if !ok {
		return ErrPaneNotFound
	}

	pv.SetWebViewWidget(widget)
	return nil
}

// Widget returns the overlay widget for embedding in the UI.
// The overlay wraps the pane container and allows mode borders to be displayed.
func (wv *WorkspaceView) Widget() layout.Widget {
	return wv.overlay
}

// Container returns the underlying BoxWidget for direct access.
func (wv *WorkspaceView) Container() layout.BoxWidget {
	return wv.container
}

// TreeRenderer returns the underlying tree renderer.
func (wv *WorkspaceView) TreeRenderer() *layout.TreeRenderer {
	return wv.treeRenderer
}

// Workspace returns the current workspace.
func (wv *WorkspaceView) Workspace() *entity.Workspace {
	wv.mu.RLock()
	defer wv.mu.RUnlock()

	return wv.workspace
}

// RegisterPaneView adds a PaneView to the tracking map without rebuilding.
// Use this for incremental operations like stacked panes.
func (wv *WorkspaceView) RegisterPaneView(paneID entity.PaneID, pv *PaneView) {
	wv.mu.Lock()
	defer wv.mu.Unlock()

	wv.paneViews[paneID] = pv
}

// UnregisterPaneView removes a PaneView from the tracking map.
// Use this when closing a pane incrementally.
func (wv *WorkspaceView) UnregisterPaneView(paneID entity.PaneID) {
	wv.mu.Lock()
	defer wv.mu.Unlock()

	delete(wv.paneViews, paneID)
}

// GetRootWidget returns the current root widget of the workspace.
// This is useful for incremental operations that need to modify the tree.
func (wv *WorkspaceView) GetRootWidget() layout.Widget {
	wv.mu.RLock()
	defer wv.mu.RUnlock()

	return wv.rootWidget
}

// SetRootWidgetDirect replaces the root widget without rebuilding the entire tree.
// Use this for incremental operations when converting to/from stacked panes.
func (wv *WorkspaceView) SetRootWidgetDirect(widget layout.Widget) {
	wv.mu.Lock()
	defer wv.mu.Unlock()

	// Remove old root if present
	if wv.rootWidget != nil {
		wv.container.Remove(wv.rootWidget)
	}

	// Add new root
	if widget != nil {
		widget.SetVisible(true)
		wv.container.Append(widget)
	}

	wv.rootWidget = widget
}

// ClearRootWidgetRef clears the stored root widget reference without removing it.
// Use this before SetRootWidgetDirect when the old root has already been removed
// through other means (e.g., GTK paned operations).
func (wv *WorkspaceView) ClearRootWidgetRef() {
	wv.mu.Lock()
	defer wv.mu.Unlock()
	wv.rootWidget = nil
}

// Factory returns the widget factory used by this workspace view.
func (wv *WorkspaceView) Factory() layout.WidgetFactory {
	return wv.factory
}

// SetModeBorderOverlay attaches a mode border overlay widget.
// The widget will be displayed on top of the pane container when modes are active.
func (wv *WorkspaceView) SetModeBorderOverlay(widget layout.Widget) {
	wv.mu.Lock()
	defer wv.mu.Unlock()

	// Remove old overlay if present
	if wv.modeBorderWidget != nil && wv.overlay != nil {
		wv.overlay.RemoveOverlay(wv.modeBorderWidget)
	}

	wv.modeBorderWidget = widget

	// Add new overlay
	if widget != nil && wv.overlay != nil {
		wv.overlay.AddOverlay(widget)
		// Don't clip or measure the overlay - it should fill the entire area
		wv.overlay.SetClipOverlay(widget, false)
		wv.overlay.SetMeasureOverlay(widget, false)
	}
}

// GetPaneWidget returns the widget for a pane ID.
// Implements focus.PaneGeometryProvider.
func (wv *WorkspaceView) GetPaneWidget(paneID entity.PaneID) layout.Widget {
	wv.mu.RLock()
	defer wv.mu.RUnlock()

	pv, ok := wv.paneViews[paneID]
	if !ok || pv == nil {
		return nil
	}
	return pv.Widget()
}

// GetStackContainerWidget returns the stack container widget for a stacked pane.
// Returns nil if the pane is not in a stack.
// Implements focus.PaneGeometryProvider.
func (wv *WorkspaceView) GetStackContainerWidget(paneID entity.PaneID) layout.Widget {
	if wv.treeRenderer == nil {
		return nil
	}

	stackedView := wv.treeRenderer.GetStackedViewForPane(string(paneID))
	if stackedView == nil {
		return nil
	}

	return stackedView.Widget()
}

// ContainerWidget returns the container widget for relative positioning.
// Implements focus.PaneGeometryProvider.
func (wv *WorkspaceView) ContainerWidget() layout.Widget {
	return wv.container
}

// SetOmniboxConfig stores the omnibox configuration for later use.
func (wv *WorkspaceView) SetOmniboxConfig(cfg OmniboxConfig) {
	wv.mu.Lock()
	defer wv.mu.Unlock()
	wv.omniboxCfg = cfg
}

// SetFindBarConfig stores the find bar configuration for later use.
func (wv *WorkspaceView) SetFindBarConfig(cfg FindBarConfig) {
	wv.mu.Lock()
	defer wv.mu.Unlock()
	wv.findBarCfg = cfg
}

// ShowOmnibox creates and shows the omnibox in the active pane.
func (wv *WorkspaceView) ShowOmnibox(ctx context.Context, query string) {
	wv.mu.Lock()
	defer wv.mu.Unlock()

	// If omnibox already exists, just show it
	if wv.omnibox != nil {
		wv.omnibox.Show(ctx, query)
		return
	}

	// Get the active pane view
	pv := wv.paneViews[wv.activePaneID]
	if pv == nil {
		wv.logger.Warn().Str("paneID", string(wv.activePaneID)).Msg("cannot show omnibox: active pane not found")
		return
	}

	// Create omnibox with pane-specific toast callback
	cfg := wv.omniboxCfg
	cfg.OnToast = func(message string) {
		pv.ShowToast(ctx, message, ToastSuccess)
	}
	omnibox := NewOmnibox(ctx, cfg)
	if omnibox == nil {
		wv.logger.Error().Msg("failed to create omnibox")
		return
	}

	// Set parent overlay for sizing
	omnibox.SetParentOverlay(pv.Overlay())

	// Wrap widget for layout system
	omniboxWidget := omnibox.WidgetAsLayout(wv.factory)
	if omniboxWidget == nil {
		wv.logger.Error().Msg("failed to wrap omnibox widget")
		return
	}

	// Add to pane overlay
	pv.AddOverlayWidget(omniboxWidget)

	// Store references
	wv.omnibox = omnibox
	wv.omniboxWidget = omniboxWidget
	wv.omniboxPaneID = wv.activePaneID

	// Show the omnibox
	omnibox.Show(ctx, query)

	wv.logger.Debug().Str("paneID", string(wv.activePaneID)).Msg("omnibox shown")
}

// HideOmnibox hides and destroys the current omnibox.
func (wv *WorkspaceView) HideOmnibox() {
	wv.mu.Lock()
	defer wv.mu.Unlock()

	wv.hideOmniboxInternal()
}

// hideOmniboxInternal destroys the omnibox without locking.
func (wv *WorkspaceView) hideOmniboxInternal() {
	if wv.omnibox == nil {
		return
	}

	// Hide the omnibox first (use Background context since this is internal cleanup)
	wv.omnibox.Hide(context.Background())

	// Remove from pane overlay
	if pv := wv.paneViews[wv.omniboxPaneID]; pv != nil && wv.omniboxWidget != nil {
		pv.RemoveOverlayWidget(wv.omniboxWidget)
	}

	// Clear references
	wv.omnibox = nil
	wv.omniboxWidget = nil
	wv.omniboxPaneID = ""

	wv.logger.Debug().Msg("omnibox hidden and destroyed")
}

// ShowFindBar creates and shows the find bar in the active pane.
func (wv *WorkspaceView) ShowFindBar(ctx context.Context) {
	wv.mu.Lock()
	defer wv.mu.Unlock()

	// If find bar already exists, just show it
	if wv.findBar != nil {
		wv.findBar.Show()
		return
	}

	pv := wv.paneViews[wv.activePaneID]
	if pv == nil {
		wv.logger.Warn().Str("paneID", string(wv.activePaneID)).Msg("cannot show find bar: active pane not found")
		return
	}

	cfg := wv.findBarCfg
	cfg.OnClose = func() {
		wv.HideFindBar()
	}
	findBar := NewFindBar(ctx, cfg)
	if findBar == nil {
		wv.logger.Error().Msg("failed to create find bar")
		return
	}

	findBarWidget := findBar.WidgetAsLayout(wv.factory)
	if findBarWidget == nil {
		wv.logger.Error().Msg("failed to wrap find bar widget")
		return
	}

	pv.AddOverlayWidget(findBarWidget)

	if cfg.GetFindController != nil {
		controller := cfg.GetFindController(wv.activePaneID)
		findBar.SetFindController(controller)
	}

	wv.findBar = findBar
	wv.findBarWidget = findBarWidget
	wv.findBarPaneID = wv.activePaneID

	findBar.Show()

	wv.logger.Debug().Str("paneID", string(wv.activePaneID)).Msg("find bar shown")
}

// HideFindBar hides and destroys the current find bar.
func (wv *WorkspaceView) HideFindBar() {
	wv.mu.Lock()
	defer wv.mu.Unlock()

	wv.hideFindBarInternal()
}

// FindNext triggers the next match in the find bar if available.
func (wv *WorkspaceView) FindNext() {
	wv.mu.RLock()
	findBar := wv.findBar
	wv.mu.RUnlock()

	if findBar != nil {
		findBar.FindNext()
	}
}

// FindPrevious triggers the previous match in the find bar if available.
func (wv *WorkspaceView) FindPrevious() {
	wv.mu.RLock()
	findBar := wv.findBar
	wv.mu.RUnlock()

	if findBar != nil {
		findBar.FindPrevious()
	}
}

// IsFindBarVisible returns whether the find bar is currently visible.
func (wv *WorkspaceView) IsFindBarVisible() bool {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.findBar != nil && wv.findBar.IsVisible()
}

// hideFindBarInternal destroys the find bar without locking.
func (wv *WorkspaceView) hideFindBarInternal() {
	if wv.findBar == nil {
		return
	}

	wv.findBar.Hide()

	if pv := wv.paneViews[wv.findBarPaneID]; pv != nil && wv.findBarWidget != nil {
		pv.RemoveOverlayWidget(wv.findBarWidget)
	}

	wv.findBar = nil
	wv.findBarWidget = nil
	wv.findBarPaneID = ""

	wv.logger.Debug().Msg("find bar hidden and destroyed")
}

// GetOmnibox returns the current omnibox if visible.
func (wv *WorkspaceView) GetOmnibox() *Omnibox {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.omnibox
}

// IsOmniboxVisible returns whether the omnibox is currently visible.
func (wv *WorkspaceView) IsOmniboxVisible() bool {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.omnibox != nil && wv.omnibox.IsVisible()
}

// GetActivePaneView returns the PaneView for the current active pane.
func (wv *WorkspaceView) GetActivePaneView() *PaneView {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.paneViews[wv.activePaneID]
}
