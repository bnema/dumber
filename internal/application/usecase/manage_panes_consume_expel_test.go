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

	stackNode := stack(leaf("a"), leaf("b"))
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

func TestManagePanesUseCase_ConsumeOrExpel_ConsumeLeft_InVerticalSplitWithStackedSibling_Noops(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	top := leaf("top")
	bottomStack := stack(leaf("a"), leaf("b"))
	root := split(entity.SplitVertical, top, bottomStack)
	ws := &entity.Workspace{Root: root, ActivePaneID: top.Pane.ID}

	res, err := uc.ConsumeOrExpel(ctx, ws, top, ConsumeOrExpelLeft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != "none" {
		t.Fatalf("result action=%v, want none", res)
	}
	if res.ErrorMessage != "No pane to the left" {
		t.Fatalf("error=%q, want %q", res.ErrorMessage, "No pane to the left")
	}
	if ws.Root != root {
		t.Fatalf("tree should be unchanged")
	}
}

func TestManagePanesUseCase_ConsumeOrExpel_Expel_DissolvesTwoPaneStack(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	stackNode := stack(leaf("a"), leaf("b"))
	b := stackNode.Children[1]
	ws := &entity.Workspace{Root: stackNode, ActivePaneID: b.Pane.ID}

	res, err := uc.ConsumeOrExpel(ctx, ws, b, ConsumeOrExpelRight)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != "expelled" {
		t.Fatalf("result action=%v, want expelled", res)
	}
	if ws.Root == nil || !ws.Root.IsSplit() || ws.Root.SplitDir != entity.SplitVertical {
		t.Fatalf("root should be a vertical split")
	}

	top := ws.Root.Left()
	bottom := ws.Root.Right()
	if top == nil || bottom == nil {
		t.Fatalf("split children should exist")
	}
	if !top.IsLeaf() || top.Pane == nil || top.Pane.ID != "a" {
		t.Fatalf("top child should be leaf a")
	}
	if !bottom.IsLeaf() || bottom.Pane == nil || bottom.Pane.ID != "b" {
		t.Fatalf("bottom child should be leaf b")
	}
	if ws.ActivePaneID != "b" {
		t.Fatalf("active=%s, want b", ws.ActivePaneID)
	}
}

func TestManagePanesUseCase_ConsumeOrExpel_ExpelThenConsumeRight_CyclesVerticalToHorizontal(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	stackNode := stack(leaf("a"), leaf("b"))
	b := stackNode.Children[1]
	ws := &entity.Workspace{Root: stackNode, ActivePaneID: b.Pane.ID}

	// First press (on stacked pane): stack -> vertical split with active on bottom.
	mustConsumeOrExpelAction(t, uc, ctx, ws, b, ConsumeOrExpelRight, "expelled")
	mustSplitRoot(t, ws, entity.SplitVertical)
	if string(ws.ActivePaneID) != "b" {
		t.Fatalf("active=%s, want b", ws.ActivePaneID)
	}

	active := ws.ActivePane()
	mustLeafPaneID(t, active, "b")

	// Second press (on bottom pane): vertical -> horizontal split with active on right.
	mustConsumeOrExpelAction(t, uc, ctx, ws, active, ConsumeOrExpelRight, "consumed")
	root := mustSplitRoot(t, ws, entity.SplitHorizontal)
	mustLeafPaneID(t, root.Left(), "a")
	mustLeafPaneID(t, root.Right(), "b")
	if string(ws.ActivePaneID) != "b" {
		t.Fatalf("active=%s, want b", ws.ActivePaneID)
	}
}

func TestManagePanesUseCase_ConsumeOrExpel_ExpelThenConsumeLeft_ReturnsToStack(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	stackNode := stack(leaf("a"), leaf("b"))
	b := stackNode.Children[1]
	ws := &entity.Workspace{Root: stackNode, ActivePaneID: b.Pane.ID}

	// First press (on stacked pane): stack -> vertical split with active on bottom.
	mustConsumeOrExpelAction(t, uc, ctx, ws, b, ConsumeOrExpelRight, "expelled")
	mustSplitRoot(t, ws, entity.SplitVertical)
	if string(ws.ActivePaneID) != "b" {
		t.Fatalf("active=%s, want b", ws.ActivePaneID)
	}

	active := ws.ActivePane()
	mustLeafPaneID(t, active, "b")

	// Second press (on bottom pane): vertical -> stack (return to original).
	mustConsumeOrExpelAction(t, uc, ctx, ws, active, ConsumeOrExpelLeft, "consumed")
	if ws.Root == nil || !ws.Root.IsStacked {
		t.Fatalf("root should be a stack")
	}
	if got := panesInOrder(ws.Root); got != "a,b" {
		t.Fatalf("stack panes=%s, want a,b", got)
	}
	if string(ws.ActivePaneID) != "b" {
		t.Fatalf("active=%s, want b", ws.ActivePaneID)
	}
}

func TestManagePanesUseCase_ConsumeOrExpel_Expel_StackRemainsWhenMoreThanTwo(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	stackNode := stack(leaf("a"), leaf("b"), leaf("c"))
	b := stackNode.Children[1]
	ws := &entity.Workspace{Root: stackNode, ActivePaneID: b.Pane.ID}

	res, err := uc.ConsumeOrExpel(ctx, ws, b, ConsumeOrExpelRight)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != "expelled" {
		t.Fatalf("result action=%v, want expelled", res)
	}
	if ws.Root == nil || !ws.Root.IsSplit() || ws.Root.SplitDir != entity.SplitVertical {
		t.Fatalf("root should be a vertical split")
	}
	top := ws.Root.Left()
	bottom := ws.Root.Right()
	if top == nil || !top.IsStacked {
		t.Fatalf("top child should be stack")
	}
	if panesInOrder(top) != "a,c" {
		t.Fatalf("remaining stack panes=%s, want a,c", panesInOrder(top))
	}
	if bottom == nil || !bottom.IsLeaf() || bottom.Pane.ID != "b" {
		t.Fatalf("bottom child should be expelled leaf b")
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

func mustConsumeOrExpelAction(
	t *testing.T,
	uc *ManagePanesUseCase,
	ctx context.Context,
	ws *entity.Workspace,
	node *entity.PaneNode,
	direction ConsumeOrExpelDirection,
	wantAction string,
) {
	t.Helper()
	res, err := uc.ConsumeOrExpel(ctx, ws, node, direction)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Action != wantAction {
		t.Fatalf("result action=%v, want %s", res, wantAction)
	}
}

func mustSplitRoot(t *testing.T, ws *entity.Workspace, dir entity.SplitDirection) *entity.PaneNode {
	t.Helper()
	if ws == nil || ws.Root == nil || !ws.Root.IsSplit() || ws.Root.SplitDir != dir {
		t.Fatalf("root should be a %v split", dir)
	}
	return ws.Root
}

func mustLeafPaneID(t *testing.T, node *entity.PaneNode, want string) {
	t.Helper()
	if node == nil || !node.IsLeaf() || node.Pane == nil || string(node.Pane.ID) != want {
		t.Fatalf("leaf pane=%v, want %s", node, want)
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

func stack(panes ...*entity.PaneNode) *entity.PaneNode {
	n := &entity.PaneNode{ID: "stack", IsStacked: true, ActiveStackIndex: 0}
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
