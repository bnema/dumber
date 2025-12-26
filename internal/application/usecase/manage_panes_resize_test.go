package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
)

func TestManagePanesUseCase_Resize_Errors(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })

	ctx := context.Background()

	err := uc.Resize(ctx, nil, nil, ResizeIncreaseDown, 5, 10)
	if err == nil {
		t.Fatalf("expected error when workspace is nil")
	}

	ws := &entity.Workspace{}
	err = uc.Resize(ctx, ws, nil, ResizeIncreaseDown, 5, 10)
	if err == nil {
		t.Fatalf("expected error when pane node is nil")
	}

	// Root nil should return ErrNothingToResize.
	leaf := &entity.PaneNode{ID: "p1", Pane: &entity.Pane{ID: "p1"}}
	err = uc.Resize(ctx, ws, leaf, ResizeIncreaseDown, 5, 10)
	if !errors.Is(err, ErrNothingToResize) {
		t.Fatalf("expected ErrNothingToResize, got %v", err)
	}
}

func TestManagePanesUseCase_Resize_VerticalDividerMove(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	top := &entity.PaneNode{ID: "top", Pane: &entity.Pane{ID: "top"}}
	bottom := &entity.PaneNode{ID: "bottom", Pane: &entity.Pane{ID: "bottom"}}
	root := &entity.PaneNode{
		ID:         "split",
		SplitDir:   entity.SplitVertical,
		SplitRatio: 0.5,
		Children:   []*entity.PaneNode{top, bottom},
	}
	top.Parent = root
	bottom.Parent = root

	ws := &entity.Workspace{Root: root, ActivePaneID: "bottom"}

	// Moving divider down increases the first child's ratio.
	if err := uc.Resize(ctx, ws, bottom, ResizeIncreaseDown, 5.0, 10.0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := root.SplitRatio, 0.55; got != want {
		t.Fatalf("split ratio = %v, want %v", got, want)
	}

	// Moving divider up decreases the first child's ratio.
	if err := uc.Resize(ctx, ws, bottom, ResizeIncreaseUp, 5.0, 10.0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := root.SplitRatio, 0.5; got != want {
		t.Fatalf("split ratio = %v, want %v", got, want)
	}
}

func TestManagePanesUseCase_Resize_SmartResizeGrowsAndShrinksActivePane(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	top := &entity.PaneNode{ID: "top", Pane: &entity.Pane{ID: "top"}}
	bottom := &entity.PaneNode{ID: "bottom", Pane: &entity.Pane{ID: "bottom"}}
	root := &entity.PaneNode{
		ID:         "split",
		SplitDir:   entity.SplitVertical,
		SplitRatio: 0.5,
		Children:   []*entity.PaneNode{top, bottom},
	}
	top.Parent = root
	bottom.Parent = root

	ws := &entity.Workspace{Root: root, ActivePaneID: "bottom"}

	// Smart increase should grow the active (bottom) pane: decrease SplitRatio.
	if err := uc.Resize(ctx, ws, bottom, ResizeIncrease, 5.0, 10.0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := root.SplitRatio, 0.45; got != want {
		t.Fatalf("split ratio = %v, want %v", got, want)
	}

	// Smart decrease should shrink the active pane: increase SplitRatio.
	if err := uc.Resize(ctx, ws, bottom, ResizeDecrease, 5.0, 10.0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := root.SplitRatio, 0.5; got != want {
		t.Fatalf("split ratio = %v, want %v", got, want)
	}
}

func TestManagePanesUseCase_Resize_ClampsToMinPanePercent(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	left := &entity.PaneNode{ID: "left", Pane: &entity.Pane{ID: "left"}}
	right := &entity.PaneNode{ID: "right", Pane: &entity.Pane{ID: "right"}}
	root := &entity.PaneNode{
		ID:         "split",
		SplitDir:   entity.SplitHorizontal,
		SplitRatio: 0.9,
		Children:   []*entity.PaneNode{left, right},
	}
	left.Parent = root
	right.Parent = root

	ws := &entity.Workspace{Root: root, ActivePaneID: "left"}

	// maxRatio = 1 - (minPanePercent/100) = 0.9
	if err := uc.Resize(ctx, ws, left, ResizeIncreaseRight, 5.0, 10.0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := root.SplitRatio, 0.9; got != want {
		t.Fatalf("split ratio = %v, want %v", got, want)
	}
}

func TestManagePanesUseCase_Resize_TargetsStackContainer(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" })
	ctx := context.Background()

	stackLeaf1 := &entity.PaneNode{ID: "s1", Pane: &entity.Pane{ID: "s1"}}
	stackLeaf2 := &entity.PaneNode{ID: "s2", Pane: &entity.Pane{ID: "s2"}}
	stack := &entity.PaneNode{
		ID:               "stack",
		IsStacked:        true,
		ActiveStackIndex: 0,
		Children:         []*entity.PaneNode{stackLeaf1, stackLeaf2},
	}
	stackLeaf1.Parent = stack
	stackLeaf2.Parent = stack

	right := &entity.PaneNode{ID: "right", Pane: &entity.Pane{ID: "right"}}
	root := &entity.PaneNode{
		ID:         "split",
		SplitDir:   entity.SplitHorizontal,
		SplitRatio: 0.5,
		Children:   []*entity.PaneNode{stack, right},
	}
	stack.Parent = root
	right.Parent = root

	ws := &entity.Workspace{Root: root, ActivePaneID: "s1"}

	// Pass the leaf inside the stack; Resize should act on the split above the stack.
	if err := uc.Resize(ctx, ws, stackLeaf1, ResizeIncreaseRight, 5.0, 10.0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := root.SplitRatio, 0.55; got != want {
		t.Fatalf("split ratio = %v, want %v", got, want)
	}
}
