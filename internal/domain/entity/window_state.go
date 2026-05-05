package entity

import "time"

// WindowID uniquely identifies a browser window.
type WindowID string

// WindowSnapshot captures the state of a single browser window.
type WindowSnapshot struct {
	ID             WindowID      `json:"id"`
	Tabs           []TabSnapshot `json:"tabs"`
	ActiveTabIndex int           `json:"active_tab_index"`
}

// WindowTabListState pairs a window ID with its live TabList.
type WindowTabListState struct {
	WindowID WindowID
	Tabs     *TabList
}

// SnapshotFromWindowTabLists creates a v2 SessionState from ordered windows.
func SnapshotFromWindowTabLists(sessionID SessionID, windows []WindowTabListState, activeWindowIndex int, savedAt time.Time) *SessionState {
	// Clamp active window index
	if activeWindowIndex < 0 || activeWindowIndex >= len(windows) {
		activeWindowIndex = 0
	}

	snapWindows := make([]WindowSnapshot, 0, len(windows))
	for _, w := range windows {
		snapWindows = append(snapWindows, windowToSnapshot(w.WindowID, w.Tabs))
	}

	return &SessionState{
		Version:           SessionStateVersion,
		SessionID:         sessionID,
		Windows:           snapWindows,
		ActiveWindowIndex: activeWindowIndex,
		SavedAt:           savedAt,
	}
}

// windowToSnapshot converts a live TabList to a WindowSnapshot.
func windowToSnapshot(id WindowID, tabs *TabList) WindowSnapshot {
	if tabs == nil {
		return WindowSnapshot{
			ID:   id,
			Tabs: []TabSnapshot{},
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

	return WindowSnapshot{
		ID:             id,
		Tabs:           snapTabs,
		ActiveTabIndex: activeTabIndex,
	}
}

// WindowTabListsFromSnapshot restores window/tab lists from a SessionState.
// For v2 states (with Windows), each WindowSnapshot is restored independently.
// For legacy v1 states (flat Tabs), returns a single window with empty WindowID.
func WindowTabListsFromSnapshot(state *SessionState, idGen IDGenerator) []WindowTabListState {
	if state == nil {
		return nil
	}

	// v2+: restore from Windows, including valid empty-window snapshots.
	if state.Version >= SessionStateVersion {
		result := make([]WindowTabListState, 0, len(state.Windows))
		for _, wSnap := range state.Windows {
			tabs := tabListFromWindowSnapshot(&wSnap, idGen)
			result = append(result, WindowTabListState{
				WindowID: wSnap.ID,
				Tabs:     tabs,
			})
		}
		return result
	}

	// v1: legacy flat tabs wrapped in a single window
	tabs := TabListFromSnapshot(state, idGen)
	return []WindowTabListState{
		{WindowID: "", Tabs: tabs},
	}
}

// tabListFromWindowSnapshot converts a single WindowSnapshot to a live TabList.
func tabListFromWindowSnapshot(snap *WindowSnapshot, idGen IDGenerator) *TabList {
	if snap == nil {
		return NewTabList()
	}

	return tabListFromSnapshots(snap.Tabs, snap.ActiveTabIndex, idGen)
}
