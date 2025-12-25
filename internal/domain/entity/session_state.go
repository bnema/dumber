package entity

import "time"

// SessionStateVersion is the current schema version for session state.
// Increment when making breaking changes to the serialization format.
const SessionStateVersion = 1

// SessionState represents a complete snapshot of a browser session.
// This is serialized to JSON and stored in the database.
type SessionState struct {
	Version        int           `json:"version"`
	SessionID      SessionID     `json:"session_id"`
	Tabs           []TabSnapshot `json:"tabs"`
	ActiveTabIndex int           `json:"active_tab_index"`
	SavedAt        time.Time     `json:"saved_at"`
}

// TabSnapshot captures the state of a single tab.
type TabSnapshot struct {
	ID        TabID             `json:"id"`
	Name      string            `json:"name"`
	Position  int               `json:"position"`
	IsPinned  bool              `json:"is_pinned"`
	Workspace WorkspaceSnapshot `json:"workspace"`
}

// WorkspaceSnapshot captures the pane tree layout.
type WorkspaceSnapshot struct {
	ID           WorkspaceID       `json:"id"`
	Root         *PaneNodeSnapshot `json:"root"`
	ActivePaneID PaneID            `json:"active_pane_id"`
}

// PaneNodeSnapshot captures a node in the pane tree.
type PaneNodeSnapshot struct {
	ID               string              `json:"id"`
	Pane             *PaneSnapshot       `json:"pane,omitempty"`     // Non-nil for leaf nodes
	Children         []*PaneNodeSnapshot `json:"children,omitempty"` // Non-nil for containers
	SplitDir         SplitDirection      `json:"split_dir"`
	SplitRatio       float64             `json:"split_ratio"`
	IsStacked        bool                `json:"is_stacked"`
	ActiveStackIndex int                 `json:"active_stack_index"`
}

// PaneSnapshot captures the essential state of a pane.
type PaneSnapshot struct {
	ID         PaneID  `json:"id"`
	URI        string  `json:"uri"`
	Title      string  `json:"title"`
	ZoomFactor float64 `json:"zoom_factor"`
}

// SnapshotFromTabList creates a SessionState from a live TabList.
func SnapshotFromTabList(sessionID SessionID, tabs *TabList) *SessionState {
	if tabs == nil {
		return &SessionState{
			Version:   SessionStateVersion,
			SessionID: sessionID,
			Tabs:      []TabSnapshot{},
			SavedAt:   time.Now(),
		}
	}

	snapTabs := make([]TabSnapshot, 0, len(tabs.Tabs))
	activeTabIndex := 0

	for i, tab := range tabs.Tabs {
		if tab.ID == tabs.ActiveTabID {
			activeTabIndex = i
		}
		snapTabs = append(snapTabs, snapshotTab(tab))
	}

	return &SessionState{
		Version:        SessionStateVersion,
		SessionID:      sessionID,
		Tabs:           snapTabs,
		ActiveTabIndex: activeTabIndex,
		SavedAt:        time.Now(),
	}
}

func snapshotTab(tab *Tab) TabSnapshot {
	return TabSnapshot{
		ID:        tab.ID,
		Name:      tab.Name,
		Position:  tab.Position,
		IsPinned:  tab.IsPinned,
		Workspace: snapshotWorkspace(tab.Workspace),
	}
}

func snapshotWorkspace(ws *Workspace) WorkspaceSnapshot {
	if ws == nil {
		return WorkspaceSnapshot{}
	}
	return WorkspaceSnapshot{
		ID:           ws.ID,
		Root:         snapshotPaneNode(ws.Root),
		ActivePaneID: ws.ActivePaneID,
	}
}

func snapshotPaneNode(node *PaneNode) *PaneNodeSnapshot {
	if node == nil {
		return nil
	}

	snapshot := &PaneNodeSnapshot{
		ID:               node.ID,
		SplitDir:         node.SplitDir,
		SplitRatio:       node.SplitRatio,
		IsStacked:        node.IsStacked,
		ActiveStackIndex: node.ActiveStackIndex,
	}

	if node.Pane != nil {
		snapshot.Pane = &PaneSnapshot{
			ID:         node.Pane.ID,
			URI:        node.Pane.URI,
			Title:      node.Pane.Title,
			ZoomFactor: node.Pane.ZoomFactor,
		}
	}

	if len(node.Children) > 0 {
		snapshot.Children = make([]*PaneNodeSnapshot, 0, len(node.Children))
		for _, child := range node.Children {
			snapshot.Children = append(snapshot.Children, snapshotPaneNode(child))
		}
	}

	return snapshot
}

// SessionInfo provides summary information for the session manager UI.
type SessionInfo struct {
	Session   *Session
	State     *SessionState
	TabCount  int
	PaneCount int
	IsActive  bool      // Has active lock file
	IsCurrent bool      // Is the current session
	UpdatedAt time.Time // When the state was last saved
}

// CountPanes returns the total number of panes in the session state.
func (s *SessionState) CountPanes() int {
	count := 0
	for _, tab := range s.Tabs {
		count += countPanesInNode(tab.Workspace.Root)
	}
	return count
}

func countPanesInNode(node *PaneNodeSnapshot) int {
	if node == nil {
		return 0
	}
	if node.Pane != nil {
		return 1
	}
	count := 0
	for _, child := range node.Children {
		count += countPanesInNode(child)
	}
	return count
}

// IDGenerator is a function that generates unique IDs.
type IDGenerator func() string

// TabListFromSnapshot reconstructs a TabList from a SessionState snapshot.
// Generates new IDs for all entities using the provided generator.
// This is the inverse of SnapshotFromTabList.
func TabListFromSnapshot(state *SessionState, idGen IDGenerator) *TabList {
	if state == nil {
		return NewTabList()
	}

	tabs := NewTabList()

	for i, tabSnap := range state.Tabs {
		tab := tabFromSnapshot(&tabSnap, idGen)
		if tab == nil {
			continue
		}
		tabs.Tabs = append(tabs.Tabs, tab)
		tab.Position = len(tabs.Tabs) - 1

		if i == state.ActiveTabIndex {
			tabs.ActiveTabID = tab.ID
		}
	}

	// Ensure we have an active tab
	if tabs.ActiveTabID == "" && len(tabs.Tabs) > 0 {
		tabs.ActiveTabID = tabs.Tabs[0].ID
	}

	return tabs
}

func tabFromSnapshot(snap *TabSnapshot, idGen IDGenerator) *Tab {
	if snap == nil {
		return nil
	}

	ws := workspaceFromSnapshot(&snap.Workspace, idGen)
	if ws == nil {
		return nil
	}

	return &Tab{
		ID:        TabID(idGen()),
		Name:      snap.Name,
		Workspace: ws,
		Position:  snap.Position,
		IsPinned:  snap.IsPinned,
		CreatedAt: time.Now(),
	}
}

func workspaceFromSnapshot(snap *WorkspaceSnapshot, idGen IDGenerator) *Workspace {
	if snap == nil {
		return nil
	}

	root := paneNodeFromSnapshot(snap.Root, nil, idGen)
	if root == nil {
		return nil
	}

	// Find the active pane ID in the restored tree
	// We need to map the snapshot's ActivePaneID to the new pane ID
	var activePaneID PaneID
	root.Walk(func(node *PaneNode) bool {
		if node.Pane != nil {
			// Use the first pane as active if we can't find the original
			if activePaneID == "" {
				activePaneID = node.Pane.ID
			}
		}
		return true
	})

	return &Workspace{
		ID:           WorkspaceID(idGen()),
		Root:         root,
		ActivePaneID: activePaneID,
		CreatedAt:    time.Now(),
	}
}

func paneNodeFromSnapshot(snap *PaneNodeSnapshot, parent *PaneNode, idGen IDGenerator) *PaneNode {
	if snap == nil {
		return nil
	}

	node := &PaneNode{
		ID:               idGen(),
		Parent:           parent,
		SplitDir:         snap.SplitDir,
		SplitRatio:       snap.SplitRatio,
		IsStacked:        snap.IsStacked,
		ActiveStackIndex: snap.ActiveStackIndex,
	}

	// Restore pane if this is a leaf node
	if snap.Pane != nil {
		node.Pane = paneFromSnapshot(snap.Pane, idGen)
	}

	// Restore children recursively
	if len(snap.Children) > 0 {
		node.Children = make([]*PaneNode, 0, len(snap.Children))
		for _, childSnap := range snap.Children {
			child := paneNodeFromSnapshot(childSnap, node, idGen)
			if child != nil {
				node.Children = append(node.Children, child)
			}
		}
	}

	return node
}

func paneFromSnapshot(snap *PaneSnapshot, idGen IDGenerator) *Pane {
	if snap == nil {
		return nil
	}

	return &Pane{
		ID:         PaneID(idGen()),
		URI:        snap.URI,
		Title:      snap.Title,
		ZoomFactor: snap.ZoomFactor,
		WindowType: WindowMain,
		CreatedAt:  time.Now(),
	}
}
