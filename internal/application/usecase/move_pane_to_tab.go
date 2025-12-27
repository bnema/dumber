package usecase

import (
	"fmt"

	"github.com/bnema/dumber/internal/domain/entity"
)

// MovePaneToTabUseCase moves a pane from one tab's workspace to another.
//
// It is pure domain manipulation: it depends only on entities and an ID generator.
type MovePaneToTabUseCase struct {
	idGenerator IDGenerator
}

func NewMovePaneToTabUseCase(idGenerator IDGenerator) *MovePaneToTabUseCase {
	return &MovePaneToTabUseCase{idGenerator: idGenerator}
}

type MovePaneToTabInput struct {
	TabList      *entity.TabList
	SourceTabID  entity.TabID
	SourcePaneID entity.PaneID
	TargetTabID  entity.TabID // empty means create new tab
}

type MovePaneToTabOutput struct {
	TargetTab       *entity.Tab
	MovedPaneNode   *entity.PaneNode
	SourceTabClosed bool
	NewTabCreated   bool
}

func (uc *MovePaneToTabUseCase) Execute(input MovePaneToTabInput) (*MovePaneToTabOutput, error) {
	if err := validateMovePaneToTabInput(uc, input); err != nil {
		return nil, err
	}

	sourceTab, err := findSourceTab(input.TabList, input.SourceTabID)
	if err != nil {
		return nil, err
	}

	movedPane, sourceNode, err := findSourcePane(sourceTab.Workspace, input.SourcePaneID)
	if err != nil {
		return nil, err
	}

	if detachErr := detachPaneFromWorkspace(sourceTab.Workspace, sourceNode); detachErr != nil {
		return nil, detachErr
	}

	sourceTabClosed := closeSourceTabIfEmpty(input.TabList, sourceTab)

	targetTab, newTabCreated, err := uc.resolveTargetTab(input.TabList, input.TargetTabID, movedPane)
	if err != nil {
		return nil, err
	}
	if targetTab == nil || targetTab.Workspace == nil {
		return nil, fmt.Errorf("target tab/workspace is nil")
	}

	if newTabCreated {
		return &MovePaneToTabOutput{
			TargetTab:       targetTab,
			MovedPaneNode:   targetTab.Workspace.Root,
			SourceTabClosed: sourceTabClosed,
			NewTabCreated:   true,
		}, nil
	}

	movedNode, err := uc.insertIntoTargetWorkspace(targetTab.Workspace, movedPane)
	if err != nil {
		return nil, err
	}

	return &MovePaneToTabOutput{
		TargetTab:       targetTab,
		MovedPaneNode:   movedNode,
		SourceTabClosed: sourceTabClosed,
		NewTabCreated:   false,
	}, nil
}

func validateMovePaneToTabInput(uc *MovePaneToTabUseCase, input MovePaneToTabInput) error {
	if uc == nil {
		return fmt.Errorf("move pane to tab use case is nil")
	}
	if input.TabList == nil {
		return fmt.Errorf("tab list is required")
	}
	if input.SourceTabID == "" {
		return fmt.Errorf("source tab id is required")
	}
	if input.SourcePaneID == "" {
		return fmt.Errorf("source pane id is required")
	}
	if input.TargetTabID == input.SourceTabID {
		return fmt.Errorf("cannot move pane to same tab")
	}
	return nil
}

func findSourceTab(tl *entity.TabList, id entity.TabID) (*entity.Tab, error) {
	sourceTab := tl.Find(id)
	if sourceTab == nil {
		return nil, fmt.Errorf("source tab not found: %s", id)
	}
	if sourceTab.Workspace == nil {
		return nil, fmt.Errorf("source workspace is nil")
	}
	return sourceTab, nil
}

func findSourcePane(ws *entity.Workspace, paneID entity.PaneID) (*entity.Pane, *entity.PaneNode, error) {
	if ws == nil {
		return nil, nil, fmt.Errorf("workspace is required")
	}
	sourceNode := ws.FindPane(paneID)
	if sourceNode == nil || sourceNode.Pane == nil {
		return nil, nil, fmt.Errorf("source pane not found: %s", paneID)
	}
	return sourceNode.Pane, sourceNode, nil
}

func closeSourceTabIfEmpty(tl *entity.TabList, sourceTab *entity.Tab) bool {
	if tl == nil || sourceTab == nil || sourceTab.Workspace == nil {
		return false
	}
	if sourceTab.Workspace.PaneCount() != 0 {
		return false
	}
	return tl.Remove(sourceTab.ID)
}

func (uc *MovePaneToTabUseCase) resolveTargetTab(
	tl *entity.TabList,
	targetID entity.TabID,
	movedPane *entity.Pane,
) (*entity.Tab, bool, error) {
	if tl == nil {
		return nil, false, fmt.Errorf("tab list is required")
	}
	if movedPane == nil {
		return nil, false, fmt.Errorf("moved pane is required")
	}

	if targetID != "" {
		if targetTab := tl.Find(targetID); targetTab != nil {
			return targetTab, false, nil
		}
		// Treat missing as "create new".
	}

	if uc.idGenerator == nil {
		return nil, false, fmt.Errorf("id generator is required to create new tab")
	}
	tabID := entity.TabID(uc.idGenerator())
	wsID := entity.WorkspaceID(uc.idGenerator())
	targetTab := entity.NewTab(tabID, wsID, movedPane)
	tl.Add(targetTab)
	return targetTab, true, nil
}

func (uc *MovePaneToTabUseCase) insertIntoTargetWorkspace(ws *entity.Workspace, movedPane *entity.Pane) (*entity.PaneNode, error) {
	if ws == nil {
		return nil, fmt.Errorf("workspace is required")
	}
	if movedPane == nil {
		return nil, fmt.Errorf("moved pane is required")
	}

	movedNode := &entity.PaneNode{ID: string(movedPane.ID), Pane: movedPane}

	if ws.Root == nil {
		ws.Root = movedNode
		ws.ActivePaneID = movedPane.ID
		return movedNode, nil
	}

	targetActive := ws.ActivePane()
	if targetActive == nil || targetActive.Pane == nil {
		return nil, fmt.Errorf("target tab has no active pane")
	}

	if err := insertPaneRightOfActive(ws, targetActive, movedNode, uc.idGenerator); err != nil {
		return nil, err
	}
	ws.ActivePaneID = movedPane.ID
	return movedNode, nil
}

func detachPaneFromWorkspace(ws *entity.Workspace, leaf *entity.PaneNode) error {
	if ws == nil {
		return fmt.Errorf("workspace is required")
	}
	if leaf == nil || leaf.Pane == nil {
		return fmt.Errorf("pane node is required")
	}

	detachedFromStack, err := detachFromStackIfNeeded(ws, leaf)
	if err != nil {
		return err
	}
	if detachedFromStack {
		return nil
	}

	if !leaf.IsLeaf() {
		return fmt.Errorf("can only move leaf panes")
	}
	return detachLeafFromWorkspace(ws, leaf)
}

func detachFromStackIfNeeded(ws *entity.Workspace, leaf *entity.PaneNode) (bool, error) {
	if ws == nil || leaf == nil {
		return false, nil
	}
	if leaf.Parent == nil || !leaf.Parent.IsStacked {
		return false, nil
	}

	stackNode := leaf.Parent

	idx := -1
	for i, child := range stackNode.Children {
		if child == leaf {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false, fmt.Errorf("pane not found in stack")
	}

	stackNode.Children = append(stackNode.Children[:idx], stackNode.Children[idx+1:]...)
	if stackNode.ActiveStackIndex >= len(stackNode.Children) {
		stackNode.ActiveStackIndex = len(stackNode.Children) - 1
	}
	if stackNode.ActiveStackIndex < 0 {
		stackNode.ActiveStackIndex = 0
	}

	// If only one pane remains in stack, dissolve it into a leaf node.
	if len(stackNode.Children) == 1 {
		remaining := stackNode.Children[0]
		stackNode.Pane = remaining.Pane
		stackNode.Children = nil
		stackNode.IsStacked = false
		stackNode.ActiveStackIndex = 0
		if stackNode.Pane != nil {
			ws.ActivePaneID = stackNode.Pane.ID
		} else {
			ws.ActivePaneID = ""
		}
		return true, nil
	}

	// Otherwise, set workspace active pane to the stack's current active.
	if stackNode.ActivePane() != nil && stackNode.ActivePane().Pane != nil {
		ws.ActivePaneID = stackNode.ActivePane().Pane.ID
	}
	return true, nil
}

func detachLeafFromWorkspace(ws *entity.Workspace, leaf *entity.PaneNode) error {
	parent := leaf.Parent
	if parent == nil {
		ws.Root = nil
		ws.ActivePaneID = ""
		return nil
	}
	if !parent.IsSplit() {
		return fmt.Errorf("pane parent is not a split")
	}

	sibling := findSibling(parent, leaf)
	if sibling == nil {
		return fmt.Errorf("no sibling found")
	}

	promoteSibling(ws, parent, sibling)
	ws.ActivePaneID = findFirstLeafPaneID(sibling)
	return nil
}

func findSibling(parent, leaf *entity.PaneNode) *entity.PaneNode {
	if parent == nil {
		return nil
	}
	for _, child := range parent.Children {
		if child != leaf {
			return child
		}
	}
	return nil
}

func promoteSibling(ws *entity.Workspace, parent, sibling *entity.PaneNode) {
	if ws == nil || sibling == nil {
		return
	}

	grandparent := parent.Parent
	if grandparent == nil {
		ws.Root = sibling
		sibling.Parent = nil
		return
	}
	for i, child := range grandparent.Children {
		if child == parent {
			grandparent.Children[i] = sibling
			break
		}
	}
	sibling.Parent = grandparent
}

func findFirstLeafPaneID(node *entity.PaneNode) entity.PaneID {
	if node == nil {
		return ""
	}
	var active entity.PaneID
	node.Walk(func(n *entity.PaneNode) bool {
		if n.IsLeaf() && n.Pane != nil {
			active = n.Pane.ID
			return false
		}
		return true
	})
	return active
}

func insertPaneRightOfActive(ws *entity.Workspace, activeNode, newLeaf *entity.PaneNode, idGen IDGenerator) error {
	if ws == nil {
		return fmt.Errorf("workspace is required")
	}
	if activeNode == nil {
		return fmt.Errorf("active pane is required")
	}
	if newLeaf == nil || newLeaf.Pane == nil {
		return fmt.Errorf("new pane node is required")
	}
	if idGen == nil {
		return fmt.Errorf("id generator is required")
	}

	targetNode := activeNode
	// Split around stacks when inserting left/right.
	if targetNode.Parent != nil && targetNode.Parent.IsStacked {
		targetNode = targetNode.Parent
	}

	parentID := idGen()
	splitParent := &entity.PaneNode{
		ID:         parentID,
		SplitDir:   entity.SplitHorizontal,
		SplitRatio: 0.5,
		Children:   make([]*entity.PaneNode, 2),
	}

	splitParent.Children[0] = targetNode
	splitParent.Children[1] = newLeaf

	newLeaf.Parent = splitParent
	oldParent := targetNode.Parent
	targetNode.Parent = splitParent

	if oldParent == nil {
		ws.Root = splitParent
	} else {
		for i, child := range oldParent.Children {
			if child == targetNode {
				oldParent.Children[i] = splitParent
				break
			}
		}
		splitParent.Parent = oldParent
	}

	return nil
}
