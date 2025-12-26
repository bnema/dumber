package usecase

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
)

func TestManagePanesUseCase_ConsumeOrExpel_ConsumeLeft_CyclesHorizontalToVerticalThenStack(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	left := leaf("left")
	right := leaf("right")
	root := split(entity.SplitHorizontal, left, right)
	ws := &entity.Workspace{Root: root, ActivePaneID: right.Pane.ID}

	// First press: horizontal -> vertical split (left on top, right on bottom).
	res, err := uc.ConsumeOrExpel(ctx, ws, right, ConsumeOrExpelLeft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != "consumed" {
		t.Fatalf("result action=%v, want consumed", res)
	}
	if ws.Root == nil || !ws.Root.IsSplit() || ws.Root.SplitDir != entity.SplitVertical {
		t.Fatalf("root should be a vertical split")
	}
	if ws.Root.Left() == nil || ws.Root.Left().Pane.ID != "left" {
		t.Fatalf("top pane=%v, want left", ws.Root.Left())
	}
	if ws.Root.Right() == nil || ws.Root.Right().Pane.ID != "right" {
		t.Fatalf("bottom pane=%v, want right", ws.Root.Right())
	}
	if ws.ActivePaneID != "right" {
		t.Fatalf("active=%s, want right", ws.ActivePaneID)
	}

	// Second press: vertical split -> stack.
	active := ws.ActivePane()
	if active == nil {
		t.Fatalf("expected active pane")
	}
	res, err = uc.ConsumeOrExpel(ctx, ws, active, ConsumeOrExpelLeft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != "consumed" {
		t.Fatalf("result action=%v, want consumed", res)
	}
	if ws.Root == nil || !ws.Root.IsStacked {
		t.Fatalf("root should be a stack")
	}
	if got := panesInOrder(ws.Root); got != "left,right" {
		t.Fatalf("stack panes=%s, want left,right", got)
	}
	if ws.ActivePaneID != "right" {
		t.Fatalf("active=%s, want right", ws.ActivePaneID)
	}
}

func TestManagePanesUseCase_ConsumeOrExpel_ConsumeRight_CyclesHorizontalToVerticalThenStack(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	left := leaf("left")
	right := leaf("right")
	root := split(entity.SplitHorizontal, left, right)
	ws := &entity.Workspace{Root: root, ActivePaneID: left.Pane.ID}

	// First press: horizontal -> vertical split (right on top, left on bottom).
	res, err := uc.ConsumeOrExpel(ctx, ws, left, ConsumeOrExpelRight)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != "consumed" {
		t.Fatalf("result action=%v, want consumed", res)
	}
	if ws.Root == nil || !ws.Root.IsSplit() || ws.Root.SplitDir != entity.SplitVertical {
		t.Fatalf("root should be a vertical split")
	}
	if ws.Root.Left() == nil || ws.Root.Left().Pane.ID != "right" {
		t.Fatalf("top pane=%v, want right", ws.Root.Left())
	}
	if ws.Root.Right() == nil || ws.Root.Right().Pane.ID != "left" {
		t.Fatalf("bottom pane=%v, want left", ws.Root.Right())
	}
	if ws.ActivePaneID != "left" {
		t.Fatalf("active=%s, want left", ws.ActivePaneID)
	}

	// Second press: vertical split -> stack.
	active := ws.ActivePane()
	if active == nil {
		t.Fatalf("expected active pane")
	}
	res, err = uc.ConsumeOrExpel(ctx, ws, active, ConsumeOrExpelRight)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != "consumed" {
		t.Fatalf("result action=%v, want consumed", res)
	}
	if ws.Root == nil || !ws.Root.IsStacked {
		t.Fatalf("root should be a stack")
	}
	if got := panesInOrder(ws.Root); got != "right,left" {
		t.Fatalf("stack panes=%s, want right,left", got)
	}
	if ws.ActivePaneID != "left" {
		t.Fatalf("active=%s, want left", ws.ActivePaneID)
	}
}

func TestManagePanesUseCase_ConsumeOrExpel_ConsumeIntoExistingStack_AppendsToEnd(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	stackNode := stack("stack", leaf("a"), leaf("b"))
	c := leaf("c")
	root := split(entity.SplitHorizontal, stackNode, c)
	ws := &entity.Workspace{Root: root, ActivePaneID: c.Pane.ID}

	res, err := uc.ConsumeOrExpel(ctx, ws, c, ConsumeOrExpelLeft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != "consumed" {
		t.Fatalf("result action=%v, want consumed", res)
	}
	if ws.Root == nil || !ws.Root.IsStacked {
		t.Fatalf("root should be a stack")
	}
	if got := panesInOrder(ws.Root); got != "a,b,c" {
		t.Fatalf("stack panes=%s, want a,b,c", got)
	}
	if ws.ActivePaneID != "c" {
		t.Fatalf("active=%s, want c", ws.ActivePaneID)
	}
}

func TestManagePanesUseCase_ConsumeOrExpel_Expel_DissolvesTwoPaneStack(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	stackNode := stack("stack", leaf("a"), leaf("b"))
	b := stackNode.Children[1]
	ws := &entity.Workspace{Root: stackNode, ActivePaneID: b.Pane.ID}

	res, err := uc.ConsumeOrExpel(ctx, ws, b, ConsumeOrExpelRight)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != "expelled" {
		t.Fatalf("result action=%v, want expelled", res)
	}
	if ws.Root == nil || !ws.Root.IsSplit() || ws.Root.SplitDir != entity.SplitHorizontal {
		t.Fatalf("root should be a horizontal split")
	}

	left := ws.Root.Left()
	right := ws.Root.Right()
	if left == nil || right == nil {
		t.Fatalf("split children should exist")
	}
	if !left.IsLeaf() || left.Pane == nil || left.Pane.ID != "a" {
		t.Fatalf("left child should be leaf a")
	}
	if !right.IsLeaf() || right.Pane == nil || right.Pane.ID != "b" {
		t.Fatalf("right child should be leaf b")
	}
	if ws.ActivePaneID != "b" {
		t.Fatalf("active=%s, want b", ws.ActivePaneID)
	}
}

func TestManagePanesUseCase_ConsumeOrExpel_Expel_StackRemainsWhenMoreThanTwo(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	stackNode := stack("stack", leaf("a"), leaf("b"), leaf("c"))
	b := stackNode.Children[1]
	ws := &entity.Workspace{Root: stackNode, ActivePaneID: b.Pane.ID}

	res, err := uc.ConsumeOrExpel(ctx, ws, b, ConsumeOrExpelRight)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != "expelled" {
		t.Fatalf("result action=%v, want expelled", res)
	}
	if ws.Root == nil || !ws.Root.IsSplit() {
		t.Fatalf("root should be a split")
	}
	left := ws.Root.Left()
	right := ws.Root.Right()
	if left == nil || !left.IsStacked {
		t.Fatalf("left child should be stack")
	}
	if panesInOrder(left) != "a,c" {
		t.Fatalf("remaining stack panes=%s, want a,c", panesInOrder(left))
	}
	if right == nil || !right.IsLeaf() || right.Pane.ID != "b" {
		t.Fatalf("right child should be expelled leaf b")
	}
	if ws.ActivePaneID != "b" {
		t.Fatalf("active=%s, want b", ws.ActivePaneID)
	}
}

func TestManagePanesUseCase_ConsumeOrExpel_NoSibling_ReturnsWarningAndNoop(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	left := leaf("left")
	right := leaf("right")
	root := split(entity.SplitHorizontal, left, right)
	ws := &entity.Workspace{Root: root, ActivePaneID: left.Pane.ID}

	res, err := uc.ConsumeOrExpel(ctx, ws, left, ConsumeOrExpelLeft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != "none" {
		t.Fatalf("result action=%v, want none", res)
	}
	if res.ErrorMessage == "" {
		t.Fatalf("expected warning message")
	}
	if ws.Root != root {
		t.Fatalf("tree should be unchanged")
	}
}

func TestManagePanesUseCase_ConsumeOrExpel_NestedSplits_ConsumesImmediateSiblingOnly(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	a := leaf("a")
	b := leaf("b")
	top := split(entity.SplitHorizontal, a, b)
	c := leaf("c")
	root := split(entity.SplitVertical, top, c)
	ws := &entity.Workspace{Root: root, ActivePaneID: a.Pane.ID}

	res, err := uc.ConsumeOrExpel(ctx, ws, a, ConsumeOrExpelDown)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != "consumed" {
		t.Fatalf("result action=%v, want consumed", res)
	}
	if ws.Root == nil || !ws.Root.IsSplit() || ws.Root.SplitDir != entity.SplitVertical {
		t.Fatalf("root should remain a vertical split")
	}
	if ws.Root.Right() == nil || !ws.Root.Right().IsStacked {
		t.Fatalf("expected bottom pane to become a stack")
	}
	if panesInOrder(ws.Root.Right()) != "c,a" {
		t.Fatalf("bottom stack panes=%s, want c,a", panesInOrder(ws.Root.Right()))
	}
}

func TestManagePanesUseCase_ConsumeOrExpel_OnlyOnePane(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	only := leaf("only")
	ws := &entity.Workspace{Root: only, ActivePaneID: only.Pane.ID}

	res, err := uc.ConsumeOrExpel(ctx, ws, only, ConsumeOrExpelLeft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != "none" {
		t.Fatalf("result action=%v, want none", res)
	}
	if res.ErrorMessage == "" {
		t.Fatalf("expected warning message")
	}
}

func leaf(id string) *entity.PaneNode {
	return &entity.PaneNode{ID: id, Pane: &entity.Pane{ID: entity.PaneID(id)}}
}

func split(dir entity.SplitDirection, left, right *entity.PaneNode) *entity.PaneNode {
	root := &entity.PaneNode{ID: "split", SplitDir: dir, SplitRatio: 0.5, Children: []*entity.PaneNode{left, right}}
	left.Parent = root
	right.Parent = root
	return root
}

func stack(id string, panes ...*entity.PaneNode) *entity.PaneNode {
	n := &entity.PaneNode{ID: id, IsStacked: true, ActiveStackIndex: 0}
	children := make([]*entity.PaneNode, 0, len(panes))
	for _, p := range panes {
		p.Parent = n
		children = append(children, p)
	}
	n.Children = children
	return n
}

func panesInOrder(stackNode *entity.PaneNode) string {
	if stackNode == nil || len(stackNode.Children) == 0 {
		return ""
	}
	out := ""
	for i, child := range stackNode.Children {
		if child == nil || child.Pane == nil {
			continue
		}
		if i > 0 {
			out += ","
		}
		out += string(child.Pane.ID)
	}
	return out
}
