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
	rootWidget   layout.Widget // Current root widget for removal on rebuild
	logger       zerolog.Logger

	workspace    *entity.Workspace
	paneViews    map[entity.PaneID]*PaneView
	activePaneID entity.PaneID

	onPaneFocused func(paneID entity.PaneID)

	mu sync.RWMutex
}

// paneViewFactoryAdapter adapts WorkspaceView to the PaneViewFactory interface
// so TreeRenderer can create PaneViews.
type paneViewFactoryAdapter struct {
	wv *WorkspaceView
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

	return pv.Widget()
}

// NewWorkspaceView creates a new workspace view.
func NewWorkspaceView(ctx context.Context, factory layout.WidgetFactory) *WorkspaceView {
	log := logging.FromContext(ctx)

	container := factory.NewBox(layout.OrientationVertical, 0)
	container.SetHexpand(true)
	container.SetVexpand(true)
	container.SetVisible(true)

	wv := &WorkspaceView{
		factory:   factory,
		container: container,
		logger:    log.With().Str("component", "workspace-view").Logger(),
		paneViews: make(map[entity.PaneID]*PaneView),
	}

	// Create tree renderer with our adapter as the pane view factory
	wv.treeRenderer = layout.NewTreeRenderer(ctx, factory, &paneViewFactoryAdapter{wv: wv})

	return wv
}

// SetWorkspace sets the workspace to render and builds the widget tree.
func (wv *WorkspaceView) SetWorkspace(ws *entity.Workspace) error {
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
		widget, err := wv.treeRenderer.Build(ws.Root)
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
func (wv *WorkspaceView) Rebuild() error {
	wv.mu.RLock()
	ws := wv.workspace
	wv.mu.RUnlock()

	if ws == nil {
		return ErrNilWorkspace
	}

	return wv.SetWorkspace(ws)
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

// Widget returns the underlying container widget for embedding in the UI.
func (wv *WorkspaceView) Widget() layout.Widget {
	return wv.container
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

// Factory returns the widget factory used by this workspace view.
func (wv *WorkspaceView) Factory() layout.WidgetFactory {
	return wv.factory
}
