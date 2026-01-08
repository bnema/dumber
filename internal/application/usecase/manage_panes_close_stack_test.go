package usecase

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
)

func TestManagePanesUseCase_CloseStackedPane_RemovesFromStackWithMoreThanTwo(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	// Create a stack with 3 panes
	a := leaf("a")
	b := leaf("b")
	c := leaf("c")
	stackNode := stack(a, b, c)
	stackNode.ActiveStackIndex = 1 // b is active

	ws := &entity.Workspace{Root: stackNode, ActivePaneID: b.Pane.ID}

	// Close pane b (the middle one)
	result, err := uc.Close(ctx, ws, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Stack should remain with 2 panes
	if !ws.Root.IsStacked {
		t.Fatalf("root should still be a stack")
	}
	if len(ws.Root.Children) != 2 {
		t.Fatalf("stack should have 2 children, got %d", len(ws.Root.Children))
	}
	if got := panesInOrder(ws.Root); got != "a,c" {
		t.Fatalf("remaining panes=%s, want a,c", got)
	}

	// Result should be the stack node
	if result != stackNode {
		t.Fatalf("expected stack node to be returned")
	}
}

func TestManagePanesUseCase_CloseStackedPane_DissolvesStackWithTwo(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	// Create a stack with 2 panes
	a := leaf("a")
	b := leaf("b")
	stackNode := stack(a, b)
	stackNode.ActiveStackIndex = 1 // b is active

	ws := &entity.Workspace{Root: stackNode, ActivePaneID: b.Pane.ID}

	// Close pane b
	result, err := uc.Close(ctx, ws, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Stack should be dissolved, remaining pane becomes root
	if ws.Root.IsStacked {
		t.Fatalf("root should no longer be a stack")
	}
	if !ws.Root.IsLeaf() {
		t.Fatalf("root should be a leaf")
	}
	if ws.Root.Pane.ID != "a" {
		t.Fatalf("remaining pane should be a, got %s", ws.Root.Pane.ID)
	}

	// Result should be the remaining pane
	if result.Pane.ID != "a" {
		t.Fatalf("expected remaining pane a to be returned")
	}

	// Active pane should update to remaining pane
	if ws.ActivePaneID != "a" {
		t.Fatalf("active pane should be a, got %s", ws.ActivePaneID)
	}
}

func TestManagePanesUseCase_CloseStackedPane_UpdatesActivePaneWhenClosingActive(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	// Create a stack with 3 panes, middle one is active
	a := leaf("a")
	b := leaf("b")
	c := leaf("c")
	stackNode := stack(a, b, c)
	stackNode.ActiveStackIndex = 1 // b is active

	ws := &entity.Workspace{Root: stackNode, ActivePaneID: b.Pane.ID}

	// Close pane b (the active one)
	_, err := uc.Close(ctx, ws, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Active pane should be updated (to the pane at the new active index)
	if ws.ActivePaneID == "b" {
		t.Fatalf("active pane should not be the closed pane b")
	}
	// Should be either a or c depending on new ActiveStackIndex
	if ws.ActivePaneID != "a" && ws.ActivePaneID != "c" {
		t.Fatalf("active pane should be a or c, got %s", ws.ActivePaneID)
	}
}

func TestManagePanesUseCase_CloseStackedPane_PreservesActivePaneWhenClosingNonActive(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	// Create a stack with 3 panes, first one is active
	a := leaf("a")
	b := leaf("b")
	c := leaf("c")
	stackNode := stack(a, b, c)
	stackNode.ActiveStackIndex = 0 // a is active

	ws := &entity.Workspace{Root: stackNode, ActivePaneID: a.Pane.ID}

	// Close pane c (not the active one)
	_, err := uc.Close(ctx, ws, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Active pane should remain a
	if ws.ActivePaneID != "a" {
		t.Fatalf("active pane should still be a, got %s", ws.ActivePaneID)
	}
}

func TestManagePanesUseCase_CloseStackedPane_DissolvesNestedStack(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	// Create a split with a stack on the left
	a := leaf("a")
	b := leaf("b")
	stackNode := stack(a, b)
	stackNode.ActiveStackIndex = 0

	c := leaf("c")
	root := split(entity.SplitHorizontal, stackNode, c)

	ws := &entity.Workspace{Root: root, ActivePaneID: a.Pane.ID}

	// Close pane a from the stack
	_, err := uc.Close(ctx, ws, a)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Root should still be a split
	if !ws.Root.IsSplit() {
		t.Fatalf("root should still be a split")
	}

	// Left child should now be the remaining pane b (not a stack)
	left := ws.Root.Left()
	if left.IsStacked {
		t.Fatalf("left child should no longer be a stack")
	}
	if !left.IsLeaf() || left.Pane.ID != "b" {
		t.Fatalf("left child should be leaf b, got %v", left)
	}

	// Right child should still be c
	right := ws.Root.Right()
	if !right.IsLeaf() || right.Pane.ID != "c" {
		t.Fatalf("right child should be leaf c")
	}

	// Active pane should update to b
	if ws.ActivePaneID != "b" {
		t.Fatalf("active pane should be b, got %s", ws.ActivePaneID)
	}
}

func TestManagePanesUseCase_CloseStackedPane_ClosingFirstPaneUpdatesIndex(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	// Create a stack with 3 panes, first one is active
	a := leaf("a")
	b := leaf("b")
	c := leaf("c")
	stackNode := stack(a, b, c)
	stackNode.ActiveStackIndex = 0 // a is active

	ws := &entity.Workspace{Root: stackNode, ActivePaneID: a.Pane.ID}

	// Close pane a (the first and active one)
	_, err := uc.Close(ctx, ws, a)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Stack should have 2 panes remaining
	if len(ws.Root.Children) != 2 {
		t.Fatalf("stack should have 2 children, got %d", len(ws.Root.Children))
	}
	if got := panesInOrder(ws.Root); got != "b,c" {
		t.Fatalf("remaining panes=%s, want b,c", got)
	}

	// Active pane should be updated
	if ws.ActivePaneID == "a" {
		t.Fatalf("active pane should not be the closed pane a")
	}
}

func TestManagePanesUseCase_CloseStackedPane_ClosingLastPaneUpdatesIndex(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	// Create a stack with 3 panes, last one is active
	a := leaf("a")
	b := leaf("b")
	c := leaf("c")
	stackNode := stack(a, b, c)
	stackNode.ActiveStackIndex = 2 // c is active

	ws := &entity.Workspace{Root: stackNode, ActivePaneID: c.Pane.ID}

	// Close pane c (the last and active one)
	_, err := uc.Close(ctx, ws, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Stack should have 2 panes remaining
	if len(ws.Root.Children) != 2 {
		t.Fatalf("stack should have 2 children, got %d", len(ws.Root.Children))
	}
	if got := panesInOrder(ws.Root); got != "a,b" {
		t.Fatalf("remaining panes=%s, want a,b", got)
	}

	// Active pane should be updated to the new last pane
	if ws.ActivePaneID == "c" {
		t.Fatalf("active pane should not be the closed pane c")
	}
}
