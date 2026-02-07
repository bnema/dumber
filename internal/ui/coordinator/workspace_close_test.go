package coordinator

import (
	"strings"
	"sync"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
)

func testLeafNode(id string) *entity.PaneNode {
	return &entity.PaneNode{
		ID:   id,
		Pane: entity.NewPane(entity.PaneID(id)),
	}
}

func testSplitNode(id string, left, right *entity.PaneNode) *entity.PaneNode {
	node := &entity.PaneNode{
		ID:       id,
		SplitDir: entity.SplitHorizontal,
		Children: []*entity.PaneNode{left, right},
	}
	if left != nil {
		left.Parent = node
	}
	if right != nil {
		right.Parent = node
	}
	return node
}

func TestDeriveIncrementalCloseTreeContext_ValidSplit(t *testing.T) {
	closing := testLeafNode("closing")
	sibling := testLeafNode("sibling")
	parent := testSplitNode("parent", closing, sibling)
	other := testLeafNode("other")
	grand := testSplitNode("grand", parent, other)

	ctx, err := deriveIncrementalCloseTreeContext(closing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.parentNode != parent {
		t.Fatalf("parent mismatch")
	}
	if ctx.siblingNode != sibling {
		t.Fatalf("sibling mismatch")
	}
	if ctx.grandparentNode != grand {
		t.Fatalf("grandparent mismatch")
	}
	if ctx.siblingIsStartChild {
		t.Fatalf("sibling should be end child")
	}
	if !ctx.parentIsStartInGrand {
		t.Fatalf("parent should be start child in grandparent")
	}
}

func TestDeriveIncrementalCloseTreeContext_MissingSibling(t *testing.T) {
	closing := testLeafNode("closing")
	parent := testSplitNode("parent", closing, nil)

	_, err := deriveIncrementalCloseTreeContext(closing)
	if err == nil {
		t.Fatalf("expected invariant error")
	}
	if !strings.Contains(err.Error(), "sibling") && !strings.Contains(err.Error(), "nil child") {
		t.Fatalf("expected sibling/nil-child error, got: %v", err)
	}

	if got := paneNodeID(parent); got != "parent" {
		t.Fatalf("unexpected parent id helper output: %s", got)
	}
}

func TestDeriveIncrementalCloseTreeContext_ParentDoesNotContainClosingPane(t *testing.T) {
	closing := testLeafNode("closing")
	left := testLeafNode("left")
	right := testLeafNode("right")
	parent := testSplitNode("parent", left, right)
	closing.Parent = parent

	_, err := deriveIncrementalCloseTreeContext(closing)
	if err == nil {
		t.Fatalf("expected invariant error")
	}
	if !strings.Contains(err.Error(), "not found under parent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCaptureIncrementalCloseContext_MissingSiblingSetsPrecheckReason(t *testing.T) {
	closing := testLeafNode("closing")
	testSplitNode("parent", closing, nil)

	coord := &WorkspaceCoordinator{}
	ctx := coord.captureIncrementalCloseContext(nil, closing)
	if ctx.precheckReason == "" {
		t.Fatalf("expected precheck reason")
	}
}

func TestDeriveIncrementalCloseTreeContext_ConcurrentPaneAndTabCloseSnapshots(t *testing.T) {
	paneCloseNode := testLeafNode("pane-close")
	paneCloseSibling := testLeafNode("pane-close-sibling")
	testSplitNode("pane-parent", paneCloseNode, paneCloseSibling)

	tabCloseNode := testLeafNode("tab-close")
	tabCloseSibling := testLeafNode("tab-close-sibling")
	testSplitNode("tab-parent", tabCloseSibling, tabCloseNode)

	run := func(node *entity.PaneNode, wg *sync.WaitGroup, errCh chan<- error) {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if _, err := deriveIncrementalCloseTreeContext(node); err != nil {
				errCh <- err
				return
			}
		}
	}

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go run(paneCloseNode, &wg, errCh)
	go run(tabCloseNode, &wg, errCh)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("unexpected concurrent derive error: %v", err)
		}
	}
}
