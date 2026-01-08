// Package entity contains domain entities representing core business concepts.
// These entities are pure Go types with no infrastructure dependencies.
package entity

import "time"

// PaneID uniquely identifies a pane within the browser.
type PaneID string

// SplitDirection indicates how a pane container splits its children.
type SplitDirection int

const (
	SplitNone       SplitDirection = iota // Leaf node or stacked container
	SplitHorizontal                       // Left/right split
	SplitVertical                         // Top/bottom split
)

// WindowType indicates the type of browser window.
type WindowType int

const (
	WindowMain  WindowType = iota // Regular browser tab
	WindowPopup                   // Popup window (OAuth, feature-restricted)
)

// Pane represents a single browsing context (a WebView container).
// This is the leaf-level entity that holds navigation state.
type Pane struct {
	ID         PaneID
	URI        string
	Title      string
	FaviconURL string
	WindowType WindowType
	ZoomFactor float64
	CanGoBack  bool
	CanForward bool
	IsLoading  bool
	CreatedAt  time.Time

	// Popup-specific fields
	IsRelated    bool    // Shares context with parent
	ParentPaneID *PaneID // Parent pane if this is a related popup
	AutoClose    bool    // Auto-close on OAuth success
	RequestID    string  // Request ID for popup tracking
}

// NewPane creates a new pane with default values.
func NewPane(id PaneID) *Pane {
	return &Pane{
		ID:         id,
		WindowType: WindowMain,
		ZoomFactor: 1.0,
		CreatedAt:  time.Now(),
	}
}

// PaneNode represents a node in the workspace pane tree structure.
// It can be either:
//   - Leaf node: Contains a single Pane
//   - Split node: Contains two children (left/right or top/bottom)
//   - Stacked node: Contains multiple panes in a tabbed view
type PaneNode struct {
	ID       string
	Pane     *Pane     // Non-nil for leaf nodes
	Parent   *PaneNode // nil for root
	Children []*PaneNode

	// Layout
	SplitDir   SplitDirection
	SplitRatio float64 // 0.0-1.0, position of divider

	// Stacked panes (alternative to split)
	IsStacked        bool
	ActiveStackIndex int
}

// IsLeaf returns true if this node contains a pane (no children).
func (n *PaneNode) IsLeaf() bool {
	return n.Pane != nil && len(n.Children) == 0 && !n.IsStacked
}

// IsContainer returns true if this is a split or stacked container.
func (n *PaneNode) IsContainer() bool {
	return !n.IsLeaf()
}

// IsSplit returns true if this node splits into two children.
func (n *PaneNode) IsSplit() bool {
	return n.SplitDir != SplitNone && len(n.Children) == 2
}

// Left returns the left/top child in a split node.
func (n *PaneNode) Left() *PaneNode {
	if len(n.Children) > 0 {
		return n.Children[0]
	}
	return nil
}

// Right returns the right/bottom child in a split node.
func (n *PaneNode) Right() *PaneNode {
	if len(n.Children) > 1 {
		return n.Children[1]
	}
	return nil
}

// StackedPanes returns the list of panes if this is a stacked container.
func (n *PaneNode) StackedPanes() []*PaneNode {
	if n.IsStacked {
		return n.Children
	}
	return nil
}

// ActivePane returns the currently visible pane in a stacked container.
func (n *PaneNode) ActivePane() *PaneNode {
	if n.IsStacked && n.ActiveStackIndex >= 0 && n.ActiveStackIndex < len(n.Children) {
		return n.Children[n.ActiveStackIndex]
	}
	return nil
}

// Walk traverses the tree calling fn for each node. Returns early if fn returns false.
func (n *PaneNode) Walk(fn func(*PaneNode) bool) {
	if !fn(n) {
		return
	}
	for _, child := range n.Children {
		child.Walk(fn)
	}
}

// FindPane searches the tree for a pane with the given ID.
func (n *PaneNode) FindPane(id PaneID) *PaneNode {
	var found *PaneNode
	n.Walk(func(node *PaneNode) bool {
		if node.Pane != nil && node.Pane.ID == id {
			found = node
			return false
		}
		return true
	})
	return found
}

// LeafCount returns the number of leaf nodes (panes) in the tree.
func (n *PaneNode) LeafCount() int {
	count := 0
	n.Walk(func(node *PaneNode) bool {
		if node.IsLeaf() {
			count++
		}
		return true
	})
	return count
}

// VisibleAreaCount returns the number of visible pane areas in the tree.
// A stacked node counts as 1 visible area (only one pane visible at a time).
// A leaf node counts as 1 visible area.
// A split node's visible areas are the sum of its children's visible areas.
func (n *PaneNode) VisibleAreaCount() int {
	switch {
	case n.IsLeaf():
		return 1
	case n.IsStacked:
		// Stacked panes: only one is visible at a time
		return 1
	case n.IsSplit():
		// Split: sum of both children's visible areas
		count := 0
		if left := n.Left(); left != nil {
			count += left.VisibleAreaCount()
		}
		if right := n.Right(); right != nil {
			count += right.VisibleAreaCount()
		}
		return count
	default:
		// Fallback for other container types
		if len(n.Children) == 0 {
			return 1
		}
		count := 0
		for _, child := range n.Children {
			count += child.VisibleAreaCount()
		}
		return count
	}
}
