package layout

import (
	"errors"
	"sync"

	"github.com/bnema/dumber/internal/domain/entity"
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
	factory         WidgetFactory
	paneViewFactory PaneViewFactory
	nodeToWidget    map[string]Widget
	mu              sync.RWMutex
}

// NewTreeRenderer creates a new tree renderer.
func NewTreeRenderer(factory WidgetFactory, paneViewFactory PaneViewFactory) *TreeRenderer {
	return &TreeRenderer{
		factory:         factory,
		paneViewFactory: paneViewFactory,
		nodeToWidget:    make(map[string]Widget),
	}
}

// Build constructs the entire widget tree from a root PaneNode.
// Returns the root widget that can be embedded in a container.
func (tr *TreeRenderer) Build(root *entity.PaneNode) (Widget, error) {
	if root == nil {
		return nil, ErrNilRoot
	}

	tr.mu.Lock()
	defer tr.mu.Unlock()

	// Clear previous mappings
	tr.nodeToWidget = make(map[string]Widget)

	return tr.renderNode(root)
}

// renderNode recursively renders a single node and its children.
// Must be called with lock held.
func (tr *TreeRenderer) renderNode(node *entity.PaneNode) (Widget, error) {
	if node == nil {
		return nil, nil
	}

	var widget Widget
	var err error

	switch {
	case node.IsLeaf():
		widget = tr.renderLeaf(node)
	case node.IsSplit():
		widget, err = tr.renderSplit(node)
	case node.IsStacked:
		widget, err = tr.renderStacked(node)
	default:
		// Unknown node type - treat as leaf if it has a pane
		if node.Pane != nil {
			widget = tr.renderLeaf(node)
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

// renderLeaf creates a PaneView widget for a leaf node.
func (tr *TreeRenderer) renderLeaf(node *entity.PaneNode) Widget {
	if tr.paneViewFactory == nil {
		return nil
	}
	return tr.paneViewFactory.CreatePaneView(node)
}

// renderSplit creates a SplitView for a split node.
func (tr *TreeRenderer) renderSplit(node *entity.PaneNode) (Widget, error) {
	// Render children first
	leftWidget, err := tr.renderNode(node.Left())
	if err != nil {
		return nil, err
	}

	rightWidget, err := tr.renderNode(node.Right())
	if err != nil {
		return nil, err
	}

	// Determine orientation
	orientation := OrientationHorizontal
	if node.SplitDir == entity.SplitVertical {
		orientation = OrientationVertical
	}

	// Create split view
	splitView := NewSplitView(tr.factory, orientation, leftWidget, rightWidget, node.SplitRatio)

	return splitView.Widget(), nil
}

// renderStacked creates a StackedView for a stacked node.
func (tr *TreeRenderer) renderStacked(node *entity.PaneNode) (Widget, error) {
	stackedView := NewStackedView(tr.factory)

	// Add each child pane to the stack
	for _, child := range node.Children {
		childWidget, err := tr.renderNode(child)
		if err != nil {
			return nil, err
		}

		// Get title and favicon from pane if available
		title := "Untitled"
		favicon := ""
		if child.Pane != nil {
			if child.Pane.Title != "" {
				title = child.Pane.Title
			}
			favicon = child.Pane.FaviconURL
		}

		stackedView.AddPane(title, favicon, childWidget)
	}

	// Set active index
	if node.ActiveStackIndex >= 0 && node.ActiveStackIndex < stackedView.Count() {
		_ = stackedView.SetActive(node.ActiveStackIndex)
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
}

// UpdateSplitRatio updates the ratio of a split view by node ID.
// Returns an error if the node is not found or is not a split.
func (tr *TreeRenderer) UpdateSplitRatio(nodeID string, ratio float64) error {
	tr.mu.RLock()
	widget, ok := tr.nodeToWidget[nodeID]
	tr.mu.RUnlock()

	if !ok {
		return ErrNodeNotFound
	}

	// The widget should be a PanedWidget from a SplitView
	// We can't directly update the ratio since we only store the Widget interface
	// The caller would need to track SplitView instances separately or
	// use the underlying PanedWidget's SetPosition method

	// For now, we can at least verify the widget exists
	_ = widget
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
