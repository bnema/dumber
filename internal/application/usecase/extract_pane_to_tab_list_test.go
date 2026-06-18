package usecase

import (
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

func TestExtractPaneToTabList_FromThreePaneStack(t *testing.T) {
	uc := NewExtractPaneToTabListUseCase(newTestIDGen())
	sourceTabs := entity.NewTabList()
	targetTabs := entity.NewTabList()

	paneA := entity.NewPane(entity.PaneID("pA"))
	paneB := entity.NewPane(entity.PaneID("pB"))
	paneC := entity.NewPane(entity.PaneID("pC"))
	sourceTab := newStackedTab("tA", "wA", paneA, paneB, paneC)
	sourceTab.Workspace.ActivePaneID = paneB.ID
	sourceTabs.Add(sourceTab)

	out, err := uc.Execute(ExtractPaneToTabListInput{
		SourceTabs:   sourceTabs,
		SourceTabID:  sourceTab.ID,
		SourcePaneID: paneB.ID,
		TargetTabs:   targetTabs,
	})
	require.NoError(t, err)
	require.False(t, out.SourceTabClosed)
	require.NotNil(t, out.NewTab)
	require.NotNil(t, out.MovedPaneNode)

	require.True(t, sourceTab.Workspace.Root.IsStacked)
	require.Len(t, sourceTab.Workspace.Root.Children, 2)
	require.Equal(t, paneA.ID, sourceTab.Workspace.Root.Children[0].Pane.ID)
	require.Equal(t, paneC.ID, sourceTab.Workspace.Root.Children[1].Pane.ID)
	require.Contains(t, []entity.PaneID{paneA.ID, paneC.ID}, sourceTab.Workspace.ActivePaneID)
	require.Same(t, sourceTab.Workspace.Root, sourceTab.Workspace.Root.Children[0].Parent)
	require.Same(t, sourceTab.Workspace.Root, sourceTab.Workspace.Root.Children[1].Parent)

	require.Equal(t, 1, targetTabs.Count())
	require.Same(t, out.NewTab, targetTabs.TabAt(0))
	require.Same(t, paneB, out.NewTab.Workspace.Root.Pane)
	require.Nil(t, out.NewTab.Workspace.Root.Parent)
	require.Nil(t, out.MovedPaneNode.Parent)
}

func TestExtractPaneToTabList_FromThreePaneStackUsesWorkspaceActivePaneWhenStackIndexIsStale(t *testing.T) {
	uc := NewExtractPaneToTabListUseCase(newTestIDGen())
	sourceTabs := entity.NewTabList()
	targetTabs := entity.NewTabList()

	paneA := entity.NewPane(entity.PaneID("pA"))
	paneB := entity.NewPane(entity.PaneID("pB"))
	paneC := entity.NewPane(entity.PaneID("pC"))
	sourceTab := newStackedTab("tA", "wA", paneA, paneB, paneC)
	// ActivePaneID is the workspace source of truth, but stacked UI state can lag.
	// Eject should remove pB deterministically and activate the pane that takes its slot.
	sourceTab.Workspace.ActivePaneID = paneB.ID
	sourceTab.Workspace.Root.ActiveStackIndex = 0
	sourceTabs.Add(sourceTab)

	out, err := uc.Execute(ExtractPaneToTabListInput{
		SourceTabs:   sourceTabs,
		SourceTabID:  sourceTab.ID,
		SourcePaneID: paneB.ID,
		TargetTabs:   targetTabs,
	})
	require.NoError(t, err)
	require.False(t, out.SourceTabClosed)

	require.True(t, sourceTab.Workspace.Root.IsStacked)
	require.Len(t, sourceTab.Workspace.Root.Children, 2)
	require.Equal(t, paneA.ID, sourceTab.Workspace.Root.Children[0].Pane.ID)
	require.Equal(t, paneC.ID, sourceTab.Workspace.Root.Children[1].Pane.ID)
	require.Equal(t, 1, sourceTab.Workspace.Root.ActiveStackIndex)
	require.Equal(t, paneC.ID, sourceTab.Workspace.ActivePaneID)
	require.Same(t, paneB, out.NewTab.Workspace.Root.Pane)
}

func TestExtractPaneToTabList_FromTwoPaneStackDissolvesSource(t *testing.T) {
	uc := NewExtractPaneToTabListUseCase(newTestIDGen())
	sourceTabs := entity.NewTabList()
	targetTabs := entity.NewTabList()

	paneA := entity.NewPane(entity.PaneID("pA"))
	paneB := entity.NewPane(entity.PaneID("pB"))
	sourceTab := newStackedTab("tA", "wA", paneA, paneB)
	sourceTab.Workspace.ActivePaneID = paneB.ID
	sourceTabs.Add(sourceTab)

	out, err := uc.Execute(ExtractPaneToTabListInput{
		SourceTabs:   sourceTabs,
		SourceTabID:  sourceTab.ID,
		SourcePaneID: paneB.ID,
		TargetTabs:   targetTabs,
	})
	require.NoError(t, err)
	require.False(t, out.SourceTabClosed)

	require.NotNil(t, sourceTab.Workspace.Root)
	require.True(t, sourceTab.Workspace.Root.IsLeaf())
	require.Same(t, paneA, sourceTab.Workspace.Root.Pane)
	require.Nil(t, sourceTab.Workspace.Root.Parent)
	require.Same(t, paneB, out.NewTab.Workspace.Root.Pane)
	require.Nil(t, out.NewTab.Workspace.Root.Parent)
	require.Nil(t, out.MovedPaneNode.Parent)
}

func TestExtractPaneToTabList_LastPaneClosesSourceTab(t *testing.T) {
	uc := NewExtractPaneToTabListUseCase(newTestIDGen())
	sourceTabs := entity.NewTabList()
	targetTabs := entity.NewTabList()

	pane := entity.NewPane(entity.PaneID("pA"))
	sourceTab := entity.NewTab(entity.TabID("tA"), entity.WorkspaceID("wA"), pane)
	sourceTabs.Add(sourceTab)

	out, err := uc.Execute(ExtractPaneToTabListInput{
		SourceTabs:   sourceTabs,
		SourceTabID:  sourceTab.ID,
		SourcePaneID: pane.ID,
		TargetTabs:   targetTabs,
	})
	require.NoError(t, err)
	require.True(t, out.SourceTabClosed)
	require.Nil(t, sourceTabs.Find(sourceTab.ID))
	require.Equal(t, 0, sourceTabs.Count())
	require.Equal(t, 1, targetTabs.Count())
	require.Same(t, pane, out.NewTab.Workspace.Root.Pane)
}

func TestExtractPaneToTabList_FromSplitSeversMovedNode(t *testing.T) {
	uc := NewExtractPaneToTabListUseCase(newTestIDGen())
	sourceTabs := entity.NewTabList()
	targetTabs := entity.NewTabList()

	paneA := entity.NewPane(entity.PaneID("pA"))
	paneB := entity.NewPane(entity.PaneID("pB"))
	sourceTab := entity.NewTab(entity.TabID("tA"), entity.WorkspaceID("wA"), paneA)
	removedSplit := &entity.PaneNode{ID: "split", SplitDir: entity.SplitHorizontal, SplitRatio: 0.5}
	movedLeaf := &entity.PaneNode{ID: "pA", Pane: paneA, Parent: removedSplit}
	otherLeaf := &entity.PaneNode{ID: "pB", Pane: paneB, Parent: removedSplit}
	removedSplit.Children = []*entity.PaneNode{movedLeaf, otherLeaf}
	sourceTab.Workspace.Root = removedSplit
	sourceTab.Workspace.ActivePaneID = paneA.ID
	sourceTabs.Add(sourceTab)

	out, err := uc.Execute(ExtractPaneToTabListInput{
		SourceTabs:   sourceTabs,
		SourceTabID:  sourceTab.ID,
		SourcePaneID: paneA.ID,
		TargetTabs:   targetTabs,
	})
	require.NoError(t, err)
	require.False(t, out.SourceTabClosed)

	require.Same(t, otherLeaf, sourceTab.Workspace.Root)
	require.Nil(t, sourceTab.Workspace.Root.Parent)
	require.Nil(t, out.MovedPaneNode.Parent)
	require.Empty(t, removedSplit.Children)
	assertParentPointers(t, sourceTab.Workspace.Root, nil)
	assertParentPointers(t, out.NewTab.Workspace.Root, nil)
}

func TestExtractPaneToTabList_InvalidSourceTabID(t *testing.T) {
	uc := NewExtractPaneToTabListUseCase(newTestIDGen())
	sourceTabs := entity.NewTabList()
	sourceTabs.Add(entity.NewTab("source-tab", "source-ws", entity.NewPane("pane-a")))

	_, err := uc.Execute(ExtractPaneToTabListInput{
		SourceTabs:   sourceTabs,
		SourceTabID:  "missing-tab",
		SourcePaneID: "pane-a",
		TargetTabs:   entity.NewTabList(),
	})
	require.Error(t, err)
}

func TestExtractPaneToTabList_InvalidSourcePaneID(t *testing.T) {
	uc := NewExtractPaneToTabListUseCase(newTestIDGen())
	sourceTabs := entity.NewTabList()
	sourceTab := entity.NewTab("source-tab", "source-ws", entity.NewPane("pane-a"))
	sourceTabs.Add(sourceTab)

	_, err := uc.Execute(ExtractPaneToTabListInput{
		SourceTabs:   sourceTabs,
		SourceTabID:  sourceTab.ID,
		SourcePaneID: "missing-pane",
		TargetTabs:   entity.NewTabList(),
	})
	require.Error(t, err)
}

func TestExtractPaneToTabList_NilInputs(t *testing.T) {
	uc := NewExtractPaneToTabListUseCase(newTestIDGen())

	_, err := uc.Execute(ExtractPaneToTabListInput{
		SourceTabID:  "source-tab",
		SourcePaneID: "pane-a",
		TargetTabs:   entity.NewTabList(),
	})
	require.Error(t, err)

	_, err = uc.Execute(ExtractPaneToTabListInput{
		SourceTabs:   entity.NewTabList(),
		SourceTabID:  "source-tab",
		SourcePaneID: "pane-a",
	})
	require.Error(t, err)
}

func TestExtractPaneToTabList_ParentPointerTreeIntegrity(t *testing.T) {
	uc := NewExtractPaneToTabListUseCase(newTestIDGen())
	sourceTabs := entity.NewTabList()
	targetTabs := entity.NewTabList()

	paneA := entity.NewPane(entity.PaneID("pA"))
	paneB := entity.NewPane(entity.PaneID("pB"))
	paneC := entity.NewPane(entity.PaneID("pC"))
	sourceTab := newStackedTab("tA", "wA", paneA, paneB, paneC)
	sourceTabs.Add(sourceTab)

	out, err := uc.Execute(ExtractPaneToTabListInput{
		SourceTabs:   sourceTabs,
		SourceTabID:  sourceTab.ID,
		SourcePaneID: paneB.ID,
		TargetTabs:   targetTabs,
	})
	require.NoError(t, err)

	assertParentPointers(t, sourceTab.Workspace.Root, nil)
	assertParentPointers(t, out.NewTab.Workspace.Root, nil)
	require.Nil(t, out.MovedPaneNode.Parent)
}

func newStackedTab(tabID entity.TabID, workspaceID entity.WorkspaceID, panes ...*entity.Pane) *entity.Tab {
	tab := entity.NewTab(tabID, workspaceID, panes[0])
	stack := &entity.PaneNode{ID: "stack", IsStacked: true, ActiveStackIndex: 0}
	for _, pane := range panes {
		child := &entity.PaneNode{ID: string(pane.ID), Pane: pane, Parent: stack}
		stack.Children = append(stack.Children, child)
	}
	tab.Workspace.Root = stack
	tab.Workspace.ActivePaneID = panes[0].ID
	return tab
}

func assertParentPointers(t *testing.T, node, wantParent *entity.PaneNode) {
	t.Helper()
	if node == nil {
		return
	}
	require.Same(t, wantParent, node.Parent)
	for _, child := range node.Children {
		assertParentPointers(t, child, node)
	}
}
