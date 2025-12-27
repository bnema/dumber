package usecase

import (
	"fmt"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

func TestMovePaneToTab_MoveToExistingTab(t *testing.T) {
	id := newTestIDGen()
	uc := NewMovePaneToTabUseCase(id)

	tabs := entity.NewTabList()

	paneA := entity.NewPane(entity.PaneID("pA"))
	tabA := entity.NewTab(entity.TabID("tA"), entity.WorkspaceID("wA"), paneA)
	tabs.Add(tabA)

	paneB := entity.NewPane(entity.PaneID("pB"))
	tabB := entity.NewTab(entity.TabID("tB"), entity.WorkspaceID("wB"), paneB)
	tabs.Add(tabB)
	// Ensure tabB active pane is paneB.
	tabB.Workspace.ActivePaneID = paneB.ID

	out, err := uc.Execute(MovePaneToTabInput{
		TabList:      tabs,
		SourceTabID:  tabA.ID,
		SourcePaneID: paneA.ID,
		TargetTabID:  tabB.ID,
	})
	require.NoError(t, err)
	require.True(t, out.SourceTabClosed)
	require.False(t, out.NewTabCreated)
	require.Equal(t, tabB.ID, out.TargetTab.ID)
	require.NotNil(t, out.MovedPaneNode)

	require.Nil(t, tabs.Find(tabA.ID))
	// Inserted as split in tabB.
	require.NotNil(t, tabB.Workspace.Root)
	require.True(t, tabB.Workspace.Root.IsSplit())
	require.Equal(t, paneB.ID, tabB.Workspace.Root.Left().Pane.ID)
	require.Equal(t, paneA.ID, tabB.Workspace.Root.Right().Pane.ID)
	// Active pane becomes moved pane.
	require.Equal(t, paneA.ID, tabB.Workspace.ActivePaneID)
}

func TestMovePaneToTab_MoveCreatesNewTabWhenOnlyOneTab(t *testing.T) {
	id := newTestIDGen()
	uc := NewMovePaneToTabUseCase(id)

	tabs := entity.NewTabList()
	paneA := entity.NewPane(entity.PaneID("pA"))
	tabA := entity.NewTab(entity.TabID("tA"), entity.WorkspaceID("wA"), paneA)
	tabs.Add(tabA)

	out, err := uc.Execute(MovePaneToTabInput{
		TabList:      tabs,
		SourceTabID:  tabA.ID,
		SourcePaneID: paneA.ID,
		TargetTabID:  "",
	})
	require.NoError(t, err)
	require.True(t, out.NewTabCreated)
	require.True(t, out.SourceTabClosed)
	require.Equal(t, 1, tabs.Count())
	require.Equal(t, paneA.ID, out.TargetTab.Workspace.ActivePaneID)
	require.NotNil(t, out.TargetTab.Workspace.Root)
	require.True(t, out.TargetTab.Workspace.Root.IsLeaf())
	require.Equal(t, paneA.ID, out.TargetTab.Workspace.Root.Pane.ID)
}

func TestMovePaneToTab_CannotMoveToSameTab(t *testing.T) {
	uc := NewMovePaneToTabUseCase(newTestIDGen())
	abs := entity.NewTabList()
	pane := entity.NewPane(entity.PaneID("pA"))
	tab := entity.NewTab(entity.TabID("tA"), entity.WorkspaceID("wA"), pane)
	abs.Add(tab)

	_, err := uc.Execute(MovePaneToTabInput{
		TabList:      abs,
		SourceTabID:  tab.ID,
		SourcePaneID: pane.ID,
		TargetTabID:  tab.ID,
	})
	require.Error(t, err)
}

func TestMovePaneToTab_SourcePaneNotFound(t *testing.T) {
	uc := NewMovePaneToTabUseCase(newTestIDGen())
	tabs := entity.NewTabList()
	pane := entity.NewPane(entity.PaneID("pA"))
	tab := entity.NewTab(entity.TabID("tA"), entity.WorkspaceID("wA"), pane)
	tabs.Add(tab)

	_, err := uc.Execute(MovePaneToTabInput{
		TabList:      tabs,
		SourceTabID:  tab.ID,
		SourcePaneID: entity.PaneID("missing"),
		TargetTabID:  entity.TabID("tB"),
	})
	require.Error(t, err)
}

func TestMovePaneToTab_MoveFromSplitClosesSourceTabIfLastPane(t *testing.T) {
	id := newTestIDGen()
	uc := NewMovePaneToTabUseCase(id)

	tabs := entity.NewTabList()

	// Source workspace: split with two panes; move one, leaving one.
	paneLeft := entity.NewPane(entity.PaneID("pL"))
	paneRight := entity.NewPane(entity.PaneID("pR"))
	source := entity.NewTab(entity.TabID("tA"), entity.WorkspaceID("wA"), paneLeft)
	// Build a split root manually.
	source.Workspace.Root = &entity.PaneNode{
		ID:         "root",
		SplitDir:   entity.SplitHorizontal,
		SplitRatio: 0.5,
		Children: []*entity.PaneNode{
			{ID: string(paneLeft.ID), Pane: paneLeft},
			{ID: string(paneRight.ID), Pane: paneRight},
		},
	}
	source.Workspace.Root.Children[0].Parent = source.Workspace.Root
	source.Workspace.Root.Children[1].Parent = source.Workspace.Root
	source.Workspace.ActivePaneID = paneRight.ID
	tabs.Add(source)

	targetPane := entity.NewPane(entity.PaneID("pT"))
	target := entity.NewTab(entity.TabID("tB"), entity.WorkspaceID("wB"), targetPane)
	tabs.Add(target)

	out, err := uc.Execute(MovePaneToTabInput{
		TabList:      tabs,
		SourceTabID:  source.ID,
		SourcePaneID: paneRight.ID,
		TargetTabID:  target.ID,
	})
	require.NoError(t, err)
	require.False(t, out.SourceTabClosed)
	// Source should now be single leaf.
	require.Equal(t, 1, source.Workspace.PaneCount())
	require.NotNil(t, source.Workspace.Root)
	require.True(t, source.Workspace.Root.IsLeaf())
	require.Equal(t, paneLeft.ID, source.Workspace.Root.Pane.ID)
}

func TestMovePaneToTab_InsertsRightOfActiveEvenWhenActiveInStack(t *testing.T) {
	id := newTestIDGen()
	uc := NewMovePaneToTabUseCase(id)

	tabs := entity.NewTabList()

	paneA := entity.NewPane(entity.PaneID("pA"))
	source := entity.NewTab(entity.TabID("tA"), entity.WorkspaceID("wA"), paneA)
	tabs.Add(source)

	// Target workspace: stacked container with two panes; active is second.
	pane1 := entity.NewPane(entity.PaneID("p1"))
	pane2 := entity.NewPane(entity.PaneID("p2"))
	target := entity.NewTab(entity.TabID("tB"), entity.WorkspaceID("wB"), pane1)
	stack := &entity.PaneNode{
		ID:               "stack",
		IsStacked:        true,
		ActiveStackIndex: 1,
		Children: []*entity.PaneNode{
			{ID: string(pane1.ID), Pane: pane1},
			{ID: string(pane2.ID), Pane: pane2},
		},
	}
	stack.Children[0].Parent = stack
	stack.Children[1].Parent = stack
	target.Workspace.Root = stack
	target.Workspace.ActivePaneID = pane2.ID
	tabs.Add(target)

	out, err := uc.Execute(MovePaneToTabInput{
		TabList:      tabs,
		SourceTabID:  source.ID,
		SourcePaneID: paneA.ID,
		TargetTabID:  target.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, out.TargetTab.Workspace.Root)
	require.True(t, out.TargetTab.Workspace.Root.IsSplit())
	// Left child should be the stack container.
	require.True(t, out.TargetTab.Workspace.Root.Left().IsStacked)
	// Right child is moved pane.
	require.Equal(t, paneA.ID, out.TargetTab.Workspace.Root.Right().Pane.ID)
}

func newTestIDGen() func() string {
	counter := 0
	return func() string {
		counter++
		return fmt.Sprintf("id%d", counter)
	}
}
