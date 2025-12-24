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
