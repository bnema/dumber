package entity

import "time"

// WorkspaceID uniquely identifies a workspace.
type WorkspaceID string

// Workspace represents a collection of panes arranged in a tree layout.
// Each tab contains exactly one workspace.
type Workspace struct {
	ID           WorkspaceID
	Name         string
	Root         *PaneNode // Root of the pane tree
	ActivePaneID PaneID    // Currently focused pane
	CreatedAt    time.Time
}

// NewWorkspace creates a new workspace with an initial pane.
func NewWorkspace(id WorkspaceID, initialPane *Pane) *Workspace {
	root := &PaneNode{
		ID:   string(initialPane.ID),
		Pane: initialPane,
	}
	return &Workspace{
		ID:           id,
		Root:         root,
		ActivePaneID: initialPane.ID,
		CreatedAt:    time.Now(),
	}
}

// PaneCount returns the number of panes in the workspace.
func (w *Workspace) PaneCount() int {
	if w.Root == nil {
		return 0
	}
	return w.Root.LeafCount()
}

// FindPane searches for a pane by ID in the workspace.
func (w *Workspace) FindPane(id PaneID) *PaneNode {
	if w.Root == nil {
		return nil
	}
	return w.Root.FindPane(id)
}

// ActivePane returns the currently active pane node.
func (w *Workspace) ActivePane() *PaneNode {
	return w.FindPane(w.ActivePaneID)
}

// AllPanes returns all leaf panes in the workspace.
func (w *Workspace) AllPanes() []*Pane {
	var panes []*Pane
	if w.Root == nil {
		return panes
	}
	w.Root.Walk(func(node *PaneNode) bool {
		if node.Pane != nil {
			panes = append(panes, node.Pane)
		}
		return true
	})
	return panes
}

// VisibleAreaCount returns the number of visible pane areas.
// Stacked panes count as 1 (only one visible at a time).
func (w *Workspace) VisibleAreaCount() int {
	if w.Root == nil {
		return 0
	}
	return w.Root.VisibleAreaCount()
}
