package entity

import (
	"fmt"
	"time"
)

// TabID uniquely identifies a tab.
type TabID string

// Tab represents a browser tab containing a workspace.
// Tabs are the top-level container in the browser's tab bar.
type Tab struct {
	ID        TabID
	Name      string     // Display name (often derived from active pane title)
	Workspace *Workspace // The workspace this tab contains
	Position  int        // Position in the tab bar (0-indexed)
	IsPinned  bool       // Pinned tabs stay at the left
	CreatedAt time.Time
}

// NewTab creates a new tab with an initial pane.
func NewTab(tabID TabID, workspaceID WorkspaceID, initialPane *Pane) *Tab {
	return &Tab{
		ID:        tabID,
		Workspace: NewWorkspace(workspaceID, initialPane),
		Position:  0,
		CreatedAt: time.Now(),
	}
}

// Title returns the display title for the tab.
// Uses the tab's Name if set, otherwise returns "Tab N" based on position.
func (t *Tab) Title() string {
	if t.Name != "" {
		return t.Name
	}
	return fmt.Sprintf("Tab %d", t.Position+1)
}

// PaneCount returns the number of panes in this tab's workspace.
func (t *Tab) PaneCount() int {
	if t.Workspace == nil {
		return 0
	}
	return t.Workspace.PaneCount()
}

// TabList manages an ordered collection of tabs.
type TabList struct {
	Tabs                []*Tab
	ActiveTabID         TabID
	PreviousActiveTabID TabID // Tracks last active tab for Alt+Tab style switching
}

// NewTabList creates an empty tab list.
func NewTabList() *TabList {
	return &TabList{
		Tabs: make([]*Tab, 0),
	}
}

// Add appends a tab to the list.
func (tl *TabList) Add(tab *Tab) {
	tab.Position = len(tl.Tabs)
	tl.Tabs = append(tl.Tabs, tab)
	if tl.ActiveTabID == "" {
		tl.ActiveTabID = tab.ID
	}
}

// Remove removes a tab by ID and reindexes positions.
func (tl *TabList) Remove(id TabID) bool {
	for i, tab := range tl.Tabs {
		if tab.ID == id {
			tl.Tabs = append(tl.Tabs[:i], tl.Tabs[i+1:]...)
			// Reindex positions
			for j := i; j < len(tl.Tabs); j++ {
				tl.Tabs[j].Position = j
			}
			// Update active tab if needed
			if tl.ActiveTabID == id && len(tl.Tabs) > 0 {
				if i < len(tl.Tabs) {
					tl.ActiveTabID = tl.Tabs[i].ID
				} else {
					tl.ActiveTabID = tl.Tabs[len(tl.Tabs)-1].ID
				}
			}
			return true
		}
	}
	return false
}

// Find returns a tab by ID.
func (tl *TabList) Find(id TabID) *Tab {
	for _, tab := range tl.Tabs {
		if tab.ID == id {
			return tab
		}
	}
	return nil
}

// ActiveTab returns the currently active tab.
func (tl *TabList) ActiveTab() *Tab {
	return tl.Find(tl.ActiveTabID)
}

// Count returns the number of tabs.
func (tl *TabList) Count() int {
	return len(tl.Tabs)
}

// SetActive sets the active tab and updates the previous active tab.
func (tl *TabList) SetActive(id TabID) {
	if id != tl.ActiveTabID && tl.ActiveTabID != "" {
		tl.PreviousActiveTabID = tl.ActiveTabID
	}
	tl.ActiveTabID = id
}

// TabAt returns the tab at the given 0-based index.
func (tl *TabList) TabAt(index int) *Tab {
	if index < 0 || index >= len(tl.Tabs) {
		return nil
	}
	return tl.Tabs[index]
}

// Move moves a tab to a new position.
func (tl *TabList) Move(id TabID, newPos int) bool {
	if newPos < 0 || newPos >= len(tl.Tabs) {
		return false
	}
	var tab *Tab
	var oldPos int
	for i, t := range tl.Tabs {
		if t.ID == id {
			tab = t
			oldPos = i
			break
		}
	}
	if tab == nil {
		return false
	}
	// Remove from old position
	tl.Tabs = append(tl.Tabs[:oldPos], tl.Tabs[oldPos+1:]...)
	// Insert at new position
	tl.Tabs = append(tl.Tabs[:newPos], append([]*Tab{tab}, tl.Tabs[newPos:]...)...)
	// Reindex all
	for i := range tl.Tabs {
		tl.Tabs[i].Position = i
	}
	return true
}

// ReplaceFrom replaces this TabList's contents with those from another TabList.
// This modifies in-place so existing references to this TabList remain valid.
func (tl *TabList) ReplaceFrom(other *TabList) {
	if other == nil {
		tl.Tabs = make([]*Tab, 0)
		tl.ActiveTabID = ""
		tl.PreviousActiveTabID = ""
		return
	}
	tl.Tabs = other.Tabs
	tl.ActiveTabID = other.ActiveTabID
	tl.PreviousActiveTabID = other.PreviousActiveTabID
}
