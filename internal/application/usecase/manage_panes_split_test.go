package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
)

func TestManagePanesUseCase_Split_ValidDirections(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" }, nil)
	ctx := context.Background()

	dirs := []SplitDirection{SplitLeft, SplitRight, SplitUp, SplitDown}
	for _, dir := range dirs {
		t.Run(string(dir), func(t *testing.T) {
			target := leaf("target")
			ws := &entity.Workspace{Root: target, ActivePaneID: "target"}
			target.Parent = nil // root

			out, err := uc.Split(ctx, SplitPaneInput{
				Workspace:  ws,
				TargetPane: target,
				Direction:  dir,
			})
			if err != nil {
				t.Fatalf("unexpected error for direction %s: %v", dir, err)
			}
			if out == nil {
				t.Fatalf("output is nil for direction %s", dir)
			}
			if out.NewPaneNode == nil {
				t.Fatalf("new pane node is nil for direction %s", dir)
			}
			if out.ParentNode == nil {
				t.Fatalf("parent node is nil for direction %s", dir)
			}
			if !out.ParentNode.IsSplit() {
				t.Fatalf("parent node should be a split for direction %s, got SplitDir=%v", dir, out.ParentNode.SplitDir)
			}
			if ws.Root != out.ParentNode {
				t.Fatalf("workspace root should be the parent node for direction %s", dir)
			}
			// Verify the tree has 2 leaf panes
			if ws.PaneCount() != 2 {
				t.Fatalf("workspace should have 2 panes for direction %s, got %d", dir, ws.PaneCount())
			}
		})
	}
}

func TestManagePanesUseCase_Split_InvalidDirection_ReturnsError(t *testing.T) {
	uc := NewManagePanesUseCase(func() string { return "id" }, nil)
	ctx := context.Background()

	target := leaf("target")
	ws := &entity.Workspace{Root: target, ActivePaneID: "target"}

	// Save initial state
	initialRoot := ws.Root
	initialActive := ws.ActivePaneID

	_, err := uc.Split(ctx, SplitPaneInput{
		Workspace:  ws,
		TargetPane: target,
		Direction:  SplitDirection("diagonal"),
	})
	if err == nil {
		t.Fatalf("expected error for invalid direction, got nil")
	}
	if !strings.Contains(err.Error(), "invalid split direction: diagonal") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Verify workspace state is unchanged
	if ws.Root != initialRoot {
		t.Fatalf("workspace root was modified")
	}
	if ws.ActivePaneID != initialActive {
		t.Fatalf("workspace active pane was modified")
	}
	// Verify target pane parent is still nil (was root)
	if target.Parent != nil {
		t.Fatalf("target pane parent was modified")
	}
	// Verify pane count is still 1
	if ws.PaneCount() != 1 {
		t.Fatalf("workspace pane count changed: got %d, want 1", ws.PaneCount())
	}
}
