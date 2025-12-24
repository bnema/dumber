package entity_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockIDGenerator creates a simple ID generator for testing.
func mockIDGenerator() entity.IDGenerator {
	var counter uint64
	return func() string {
		id := atomic.AddUint64(&counter, 1)
		return string(rune('a'-1+id)) + "_new"
	}
}

func TestTabListFromSnapshot_EmptyState(t *testing.T) {
	idGen := mockIDGenerator()

	// Nil state
	tabs := entity.TabListFromSnapshot(nil, idGen)
	require.NotNil(t, tabs)
	assert.Empty(t, tabs.Tabs)

	// Empty state
	state := &entity.SessionState{
		Version: 1,
		Tabs:    []entity.TabSnapshot{},
	}
	tabs = entity.TabListFromSnapshot(state, idGen)
	require.NotNil(t, tabs)
	assert.Empty(t, tabs.Tabs)
}

func TestTabListFromSnapshot_SingleTab(t *testing.T) {
	idGen := mockIDGenerator()

	state := &entity.SessionState{
		Version:        1,
		SessionID:      "test_session",
		ActiveTabIndex: 0,
		Tabs: []entity.TabSnapshot{
			{
				ID:       "old_tab_1",
				Name:     "Test Tab",
				Position: 0,
				IsPinned: true,
				Workspace: entity.WorkspaceSnapshot{
					ID:           "old_ws_1",
					ActivePaneID: "old_pane_1",
					Root: &entity.PaneNodeSnapshot{
						ID: "old_node_1",
						Pane: &entity.PaneSnapshot{
							ID:         "old_pane_1",
							URI:        "https://example.com",
							Title:      "Example",
							ZoomFactor: 1.25,
						},
					},
				},
			},
		},
		SavedAt: time.Now(),
	}

	tabs := entity.TabListFromSnapshot(state, idGen)

	require.NotNil(t, tabs)
	require.Len(t, tabs.Tabs, 1)

	tab := tabs.Tabs[0]
	assert.NotEqual(t, entity.TabID("old_tab_1"), tab.ID, "should generate new ID")
	assert.Equal(t, "Test Tab", tab.Name)
	assert.True(t, tab.IsPinned)

	require.NotNil(t, tab.Workspace)
	require.NotNil(t, tab.Workspace.Root)
	require.NotNil(t, tab.Workspace.Root.Pane)

	pane := tab.Workspace.Root.Pane
	assert.NotEqual(t, entity.PaneID("old_pane_1"), pane.ID, "should generate new pane ID")
	assert.Equal(t, "https://example.com", pane.URI)
	assert.Equal(t, "Example", pane.Title)
	assert.InDelta(t, 1.25, pane.ZoomFactor, 0.001)

	// Active tab should be set
	assert.Equal(t, tab.ID, tabs.ActiveTabID)
}

func TestTabListFromSnapshot_MultipleTabs(t *testing.T) {
	idGen := mockIDGenerator()

	state := &entity.SessionState{
		Version:        1,
		ActiveTabIndex: 1, // Second tab is active
		Tabs: []entity.TabSnapshot{
			{
				ID:   "tab_1",
				Name: "First",
				Workspace: entity.WorkspaceSnapshot{
					Root: &entity.PaneNodeSnapshot{
						Pane: &entity.PaneSnapshot{
							ID:  "pane_1",
							URI: "https://first.com",
						},
					},
				},
			},
			{
				ID:   "tab_2",
				Name: "Second",
				Workspace: entity.WorkspaceSnapshot{
					Root: &entity.PaneNodeSnapshot{
						Pane: &entity.PaneSnapshot{
							ID:  "pane_2",
							URI: "https://second.com",
						},
					},
				},
			},
		},
	}

	tabs := entity.TabListFromSnapshot(state, idGen)

	require.Len(t, tabs.Tabs, 2)
	assert.Equal(t, "First", tabs.Tabs[0].Name)
	assert.Equal(t, "Second", tabs.Tabs[1].Name)

	// Active tab should be the second one
	assert.Equal(t, tabs.Tabs[1].ID, tabs.ActiveTabID)
}

func TestTabListFromSnapshot_SplitPanes(t *testing.T) {
	idGen := mockIDGenerator()

	// Horizontal split with two panes
	state := &entity.SessionState{
		Version: 1,
		Tabs: []entity.TabSnapshot{
			{
				ID:   "tab_1",
				Name: "Split Tab",
				Workspace: entity.WorkspaceSnapshot{
					Root: &entity.PaneNodeSnapshot{
						ID:         "container",
						SplitDir:   entity.SplitHorizontal,
						SplitRatio: 0.5,
						Children: []*entity.PaneNodeSnapshot{
							{
								ID: "left_node",
								Pane: &entity.PaneSnapshot{
									ID:  "left_pane",
									URI: "https://left.com",
								},
							},
							{
								ID: "right_node",
								Pane: &entity.PaneSnapshot{
									ID:  "right_pane",
									URI: "https://right.com",
								},
							},
						},
					},
				},
			},
		},
	}

	tabs := entity.TabListFromSnapshot(state, idGen)

	require.Len(t, tabs.Tabs, 1)
	ws := tabs.Tabs[0].Workspace
	require.NotNil(t, ws.Root)

	// Root should be a container with children
	assert.Nil(t, ws.Root.Pane)
	assert.Equal(t, entity.SplitHorizontal, ws.Root.SplitDir)
	assert.InDelta(t, 0.5, ws.Root.SplitRatio, 0.001)
	require.Len(t, ws.Root.Children, 2)

	// Check children
	leftPane := ws.Root.Children[0].Pane
	rightPane := ws.Root.Children[1].Pane
	require.NotNil(t, leftPane)
	require.NotNil(t, rightPane)
	assert.Equal(t, "https://left.com", leftPane.URI)
	assert.Equal(t, "https://right.com", rightPane.URI)
}

func TestTabListFromSnapshot_StackedPanes(t *testing.T) {
	idGen := mockIDGenerator()

	state := &entity.SessionState{
		Version: 1,
		Tabs: []entity.TabSnapshot{
			{
				ID: "tab_1",
				Workspace: entity.WorkspaceSnapshot{
					Root: &entity.PaneNodeSnapshot{
						ID:               "stack",
						IsStacked:        true,
						ActiveStackIndex: 1,
						Children: []*entity.PaneNodeSnapshot{
							{Pane: &entity.PaneSnapshot{ID: "p1", URI: "https://a.com"}},
							{Pane: &entity.PaneSnapshot{ID: "p2", URI: "https://b.com"}},
							{Pane: &entity.PaneSnapshot{ID: "p3", URI: "https://c.com"}},
						},
					},
				},
			},
		},
	}

	tabs := entity.TabListFromSnapshot(state, idGen)

	ws := tabs.Tabs[0].Workspace
	require.NotNil(t, ws.Root)
	assert.True(t, ws.Root.IsStacked)
	assert.Equal(t, 1, ws.Root.ActiveStackIndex)
	require.Len(t, ws.Root.Children, 3)
}

func TestTabList_ReplaceFrom(t *testing.T) {
	// Create original list
	original := entity.NewTabList()
	pane1 := entity.NewPane("pane1")
	pane1.URI = "https://old.com"
	tab1 := entity.NewTab("tab1", "ws1", pane1)
	original.Add(tab1)
	original.SetActive("tab1")

	// Create new list
	newList := entity.NewTabList()
	pane2 := entity.NewPane("pane2")
	pane2.URI = "https://new1.com"
	pane3 := entity.NewPane("pane3")
	pane3.URI = "https://new2.com"
	tab2 := entity.NewTab("tab2", "ws2", pane2)
	tab3 := entity.NewTab("tab3", "ws3", pane3)
	newList.Add(tab2)
	newList.Add(tab3)
	newList.SetActive("tab3")

	// Keep reference to original
	ref := original

	// Replace contents
	original.ReplaceFrom(newList)

	// Verify the reference still points to the modified list
	assert.Same(t, original, ref)
	assert.Len(t, ref.Tabs, 2)
	assert.Equal(t, entity.TabID("tab3"), ref.ActiveTabID)
	assert.Equal(t, "https://new1.com", ref.Tabs[0].Workspace.Root.Pane.URI)
}

func TestTabList_ReplaceFrom_Nil(t *testing.T) {
	original := entity.NewTabList()
	pane := entity.NewPane("pane1")
	tab := entity.NewTab("tab1", "ws1", pane)
	original.Add(tab)

	original.ReplaceFrom(nil)

	assert.Empty(t, original.Tabs)
	assert.Empty(t, original.ActiveTabID)
}

func TestSnapshotRoundTrip(t *testing.T) {
	// Create a complex tab structure
	idGen := mockIDGenerator()

	pane1 := entity.NewPane("p1")
	pane1.URI = "https://google.com"
	pane1.Title = "Google"
	pane1.ZoomFactor = 1.5

	pane2 := entity.NewPane("p2")
	pane2.URI = "https://github.com"
	pane2.Title = "GitHub"

	tab1 := entity.NewTab("t1", "ws1", pane1)
	tab1.Name = "Search"
	tab1.IsPinned = true

	tab2 := entity.NewTab("t2", "ws2", pane2)
	tab2.Name = "Code"

	original := entity.NewTabList()
	original.Add(tab1)
	original.Add(tab2)
	original.SetActive("t2")

	// Snapshot
	sessionID := entity.SessionID("test_session")
	state := entity.SnapshotFromTabList(sessionID, original)

	require.NotNil(t, state)
	assert.Equal(t, sessionID, state.SessionID)
	assert.Len(t, state.Tabs, 2)
	assert.Equal(t, 1, state.ActiveTabIndex) // Second tab active

	// Restore
	restored := entity.TabListFromSnapshot(state, idGen)

	require.NotNil(t, restored)
	require.Len(t, restored.Tabs, 2)

	// Check first tab
	restoredTab1 := restored.Tabs[0]
	assert.Equal(t, "Search", restoredTab1.Name)
	assert.True(t, restoredTab1.IsPinned)
	require.NotNil(t, restoredTab1.Workspace.Root.Pane)
	assert.Equal(t, "https://google.com", restoredTab1.Workspace.Root.Pane.URI)
	assert.Equal(t, "Google", restoredTab1.Workspace.Root.Pane.Title)
	assert.InDelta(t, 1.5, restoredTab1.Workspace.Root.Pane.ZoomFactor, 0.001)

	// Check second tab
	restoredTab2 := restored.Tabs[1]
	assert.Equal(t, "Code", restoredTab2.Name)
	assert.False(t, restoredTab2.IsPinned)
	assert.Equal(t, "https://github.com", restoredTab2.Workspace.Root.Pane.URI)

	// Active tab should be second
	assert.Equal(t, restoredTab2.ID, restored.ActiveTabID)
}

func TestFindPaneAcrossTabs(t *testing.T) {
	// Create tabs with panes
	pane1 := entity.NewPane("pane_in_tab1")
	pane1.URI = "https://tab1.com"
	tab1 := entity.NewTab("t1", "ws1", pane1)

	pane2 := entity.NewPane("pane_in_tab2")
	pane2.URI = "https://tab2.com"
	tab2 := entity.NewTab("t2", "ws2", pane2)

	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)

	// Helper function to find pane across all tabs (mirrors App.updatePaneURIInAllTabs logic)
	findPaneInTabs := func(tabs *entity.TabList, paneID entity.PaneID) *entity.Pane {
		for _, tab := range tabs.Tabs {
			if tab.Workspace == nil {
				continue
			}
			paneNode := tab.Workspace.FindPane(paneID)
			if paneNode != nil && paneNode.Pane != nil {
				return paneNode.Pane
			}
		}
		return nil
	}

	// Find pane in first tab
	found := findPaneInTabs(tabs, "pane_in_tab1")
	require.NotNil(t, found)
	assert.Equal(t, "https://tab1.com", found.URI)

	// Find pane in second tab
	found = findPaneInTabs(tabs, "pane_in_tab2")
	require.NotNil(t, found)
	assert.Equal(t, "https://tab2.com", found.URI)

	// Update URI and verify
	found.URI = "https://updated.com"
	foundAgain := findPaneInTabs(tabs, "pane_in_tab2")
	assert.Equal(t, "https://updated.com", foundAgain.URI)

	// Non-existent pane
	notFound := findPaneInTabs(tabs, "non_existent")
	assert.Nil(t, notFound)
}

func TestFindPaneInNestedStructure(t *testing.T) {
	// Create a tab with split panes
	pane1 := entity.NewPane("left_pane")
	pane1.URI = "https://left.com"

	pane2 := entity.NewPane("right_pane")
	pane2.URI = "https://right.com"

	// Build a split structure manually
	tab := entity.NewTab("t1", "ws1", pane1)

	// Add second pane as sibling (simulating a horizontal split)
	rightNode := &entity.PaneNode{
		ID:   "right_node",
		Pane: pane2,
	}

	// Create container node
	containerNode := &entity.PaneNode{
		ID:       "container",
		SplitDir: entity.SplitHorizontal,
		Children: []*entity.PaneNode{
			tab.Workspace.Root, // left pane
			rightNode,          // right pane
		},
	}
	tab.Workspace.Root = containerNode

	tabs := entity.NewTabList()
	tabs.Add(tab)

	// Find left pane
	leftNode := tab.Workspace.FindPane("left_pane")
	require.NotNil(t, leftNode)
	assert.Equal(t, "https://left.com", leftNode.Pane.URI)

	// Find right pane
	rightNodeFound := tab.Workspace.FindPane("right_pane")
	require.NotNil(t, rightNodeFound)
	assert.Equal(t, "https://right.com", rightNodeFound.Pane.URI)

	// Update URI on nested pane
	rightNodeFound.Pane.URI = "https://right-updated.com"

	// Verify update persisted
	rightNodeAgain := tab.Workspace.FindPane("right_pane")
	assert.Equal(t, "https://right-updated.com", rightNodeAgain.Pane.URI)
}
