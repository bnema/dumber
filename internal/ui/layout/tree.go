package layout

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/rs/zerolog"
)

// ErrNilRoot is returned when attempting to build from a nil root node.
var ErrNilRoot = errors.New("root node is nil")

// ErrNodeNotFound is returned when a node lookup fails.
var ErrNodeNotFound = errors.New("node not found")

// PaneViewFactory creates pane view widgets from pane nodes.
// This decouples the TreeRenderer from concrete PaneView implementation.
type PaneViewFactory interface {
	// CreatePaneView creates a widget for a leaf pane node.
	CreatePaneView(node *entity.PaneNode) Widget
}

// TreeRenderer builds GTK widget trees from domain PaneNode trees.
// It maps each node to its corresponding GTK widget and maintains
// a lookup table for finding widgets by node ID.
type TreeRenderer struct {
	factory          WidgetFactory
	paneViewFactory  PaneViewFactory
	logger           zerolog.Logger
	nodeToWidget     map[string]Widget
	splitOrientation map[string]Orientation
	// paneToStack maps pane IDs to their containing StackedView.
	// Every leaf pane is wrapped in a StackedView for easy stacking later.
	paneToStack map[string]*StackedView

	onSplitRatioChanged func(nodeID string, ratio float64)
	mu                  sync.RWMutex
}

// NewTreeRenderer creates a new tree renderer.
func NewTreeRenderer(ctx context.Context, factory WidgetFactory, paneViewFactory PaneViewFactory) *TreeRenderer {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating tree renderer")

	return &TreeRenderer{
		factory:          factory,
		paneViewFactory:  paneViewFactory,
		logger:           log.With().Str("component", "tree-renderer").Logger(),
		nodeToWidget:     make(map[string]Widget),
		splitOrientation: make(map[string]Orientation),
		paneToStack:      make(map[string]*StackedView),
	}
}

func (tr *TreeRenderer) SetOnSplitRatioChanged(fn func(nodeID string, ratio float64)) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tr.onSplitRatioChanged = fn
}

// Build constructs the entire widget tree from a root PaneNode.
// Returns the root widget that can be embedded in a container.
func (tr *TreeRenderer) Build(ctx context.Context, root *entity.PaneNode) (Widget, error) {
	if root == nil {
		return nil, ErrNilRoot
	}

	tr.mu.Lock()
	defer tr.mu.Unlock()

	// Clear previous mappings
	tr.nodeToWidget = make(map[string]Widget)
	tr.splitOrientation = make(map[string]Orientation)
	tr.paneToStack = make(map[string]*StackedView)

	return tr.renderNode(ctx, root)
}

// renderNode recursively renders a single node and its children.
// Must be called with lock held.
func (tr *TreeRenderer) renderNode(ctx context.Context, node *entity.PaneNode) (Widget, error) {
	if node == nil {
		return nil, nil
	}

	var widget Widget
	var err error

	switch {
	case node.IsLeaf():
		widget = tr.renderLeaf(ctx, node)
	case node.IsSplit():
		widget, err = tr.renderSplit(ctx, node)
	case node.IsStacked:
		widget, err = tr.renderStacked(ctx, node)
	default:
		// Unknown node type - treat as leaf if it has a pane
		if node.Pane != nil {
			widget = tr.renderLeaf(ctx, node)
		}
	}

	if err != nil {
		return nil, err
	}

	// Store mapping
	if widget != nil && node.ID != "" {
		tr.nodeToWidget[node.ID] = widget
	}

	return widget, nil
}

// renderLeaf creates a PaneView widget wrapped in a StackedView.
// Every pane is wrapped in a StackedView from the start, making stacking trivial.
// When there's only 1 pane, the StackedView shows no title bar.
func (tr *TreeRenderer) renderLeaf(ctx context.Context, node *entity.PaneNode) Widget {
	if tr.paneViewFactory == nil {
		return nil
	}

	paneWidget := tr.paneViewFactory.CreatePaneView(node)
	if paneWidget == nil {
		return nil
	}

	// Wrap the pane in a StackedView
	stackedView := NewStackedView(tr.factory)

	// Get title from pane if available
	title := "Untitled"
	paneID := ""
	if node.Pane != nil {
		paneID = string(node.Pane.ID)
		if node.Pane.Title != "" {
			title = node.Pane.Title
		}
	}

	tr.logger.Debug().
		Str("pane_id", paneID).
		Str("title", title).
		Msg("TreeRenderer.renderLeaf: wrapping pane in StackedView")

	stackedView.AddPane(ctx, paneID, title, "", paneWidget)

	// Track this pane's StackedView for later stacking operations
	if node.Pane != nil {
		tr.paneToStack[paneID] = stackedView
	}

	return stackedView.Widget()
}

// renderSplit creates a SplitView for a split node.
func (tr *TreeRenderer) renderSplit(ctx context.Context, node *entity.PaneNode) (Widget, error) {
	// Render children first
	leftWidget, err := tr.renderNode(ctx, node.Left())
	if err != nil {
		return nil, err
	}

	rightWidget, err := tr.renderNode(ctx, node.Right())
	if err != nil {
		return nil, err
	}

	// Determine orientation
	orientation := OrientationHorizontal
	if node.SplitDir == entity.SplitVertical {
		orientation = OrientationVertical
	}

	// Create split view
	splitView := NewSplitView(ctx, tr.factory, orientation, leftWidget, rightWidget, node.SplitRatio)
	nodeID := node.ID
	splitView.SetOnRatioChanged(func(ratio float64) {
		tr.mu.RLock()
		cb := tr.onSplitRatioChanged
		tr.mu.RUnlock()
		if cb != nil {
			cb(nodeID, ratio)
		}
	})
	tr.splitOrientation[node.ID] = orientation

	return splitView.Widget(), nil
}

// renderStacked creates a StackedView for a stacked node.
func (tr *TreeRenderer) renderStacked(ctx context.Context, node *entity.PaneNode) (Widget, error) {
	stackedView := NewStackedView(tr.factory)

	tr.logger.Debug().
		Int("child_count", len(node.Children)).
		Msg("TreeRenderer.renderStacked: creating stacked view")

	// Add each child pane to the stack
	for _, child := range node.Children {
		childWidget, err := tr.renderNode(ctx, child)
		if err != nil {
			return nil, err
		}

		// Get pane ID, title, and favicon from pane if available
		paneID := ""
		title := "Untitled"
		favicon := ""
		if child.Pane != nil {
			paneID = string(child.Pane.ID)
			if child.Pane.Title != "" {
				title = child.Pane.Title
			}
			favicon = child.Pane.FaviconURL
		}

		stackedView.AddPane(ctx, paneID, title, favicon, childWidget)

		// Override paneToStack to point to THIS stacked view, not the child's wrapper.
		// This ensures geometric navigation and stack sync work correctly.
		if child.Pane != nil {
			tr.paneToStack[paneID] = stackedView
		}
	}

	// Set active index
	if node.ActiveStackIndex >= 0 && node.ActiveStackIndex < stackedView.Count() {
		_ = stackedView.SetActive(ctx, node.ActiveStackIndex)
	}

	return stackedView.Widget(), nil
}

// Lookup finds the widget associated with a node ID.
// Returns nil if the node was not found or hasn't been rendered.
func (tr *TreeRenderer) Lookup(nodeID string) Widget {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	return tr.nodeToWidget[nodeID]
}

// LookupNode finds both the widget and verifies the node ID exists.
// Returns the widget and true if found, nil and false otherwise.
func (tr *TreeRenderer) LookupNode(nodeID string) (Widget, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	widget, ok := tr.nodeToWidget[nodeID]
	return widget, ok
}

// NodeCount returns the number of nodes currently tracked.
func (tr *TreeRenderer) NodeCount() int {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	return len(tr.nodeToWidget)
}

// Clear removes all node-to-widget mappings.
func (tr *TreeRenderer) Clear() {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tr.nodeToWidget = make(map[string]Widget)
	tr.splitOrientation = make(map[string]Orientation)
}

// RegisterWidget adds or updates a node-to-widget mapping.
// Use this for incremental updates when you don't want to rebuild the entire tree.
func (tr *TreeRenderer) RegisterWidget(nodeID string, widget Widget) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tr.nodeToWidget[nodeID] = widget
}

// RegisterSplit registers a split widget with its orientation.
func (tr *TreeRenderer) RegisterSplit(nodeID string, widget Widget, orientation Orientation) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tr.nodeToWidget[nodeID] = widget
	tr.splitOrientation[nodeID] = orientation
}

// UnregisterWidget removes a node-to-widget mapping.
// Use this when a node is removed or replaced during incremental operations.
func (tr *TreeRenderer) UnregisterWidget(nodeID string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	delete(tr.nodeToWidget, nodeID)
	delete(tr.splitOrientation, nodeID)
}

// UpdateSplitRatio updates the ratio of a split view by node ID.
// Returns an error if the node is not found or is not a split.
func (tr *TreeRenderer) UpdateSplitRatio(nodeID string, ratio float64) error {
	tr.mu.RLock()
	widget, ok := tr.nodeToWidget[nodeID]
	orientation, okOrient := tr.splitOrientation[nodeID]
	tr.mu.RUnlock()

	if !ok {
		return ErrNodeNotFound
	}

	paned, ok := widget.(PanedWidget)
	if !ok {
		return fmt.Errorf("widget is not a PanedWidget")
	}
	if !okOrient {
		return fmt.Errorf("split orientation not found")
	}

	totalSize := 0
	switch orientation {
	case OrientationHorizontal:
		totalSize = paned.GetAllocatedWidth()
	case OrientationVertical:
		totalSize = paned.GetAllocatedHeight()
	}
	if totalSize <= 0 {
		return nil
	}

	paned.SetPosition(int(float64(totalSize) * ratio))
	return nil
}

// GetNodeIDs returns all tracked node IDs.
func (tr *TreeRenderer) GetNodeIDs() []string {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	ids := make([]string, 0, len(tr.nodeToWidget))
	for id := range tr.nodeToWidget {
		ids = append(ids, id)
	}
	return ids
}

// GetStackedViewForPane returns the StackedView containing the given pane.
// Returns nil if the pane is not found.
func (tr *TreeRenderer) GetStackedViewForPane(paneID string) *StackedView {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	return tr.paneToStack[paneID]
}

// RegisterPaneInStack adds a pane to the paneToStack mapping.
// Use this when adding a pane to an existing stack without rebuild.
func (tr *TreeRenderer) RegisterPaneInStack(paneID string, stackedView *StackedView) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tr.paneToStack[paneID] = stackedView
}

// UnregisterPane removes a pane from the paneToStack mapping.
func (tr *TreeRenderer) UnregisterPane(paneID string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	delete(tr.paneToStack, paneID)
}
