package browser

import (
	"testing"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
)

func TestInfiniteRecursionProtection(t *testing.T) {
	t.Run("BoundaryFallback with deep tree", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Create a pathologically deep tree
		current := wm.root
		for i := 0; i < 20; i++ {
			newNode, err := wm.splitNode(current, "right")
			if err != nil {
				t.Fatalf("Split %d failed: %v", i, err)
			}
			current = newNode
		}

		// Test that boundaryFallback doesn't hang even with deep trees
		done := make(chan bool, 1)
		go func() {
			// This should complete quickly without infinite recursion
			result := wm.boundaryFallback(wm.root, "right")
			_ = result // We just care that it returns
			done <- true
		}()

		select {
		case <-done:
			// Good - function completed
		case <-time.After(1 * time.Second):
			t.Fatal("boundaryFallback took too long - likely infinite recursion")
		}
	})

	t.Run("CollectLeaves with complex tree", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Build a complex tree that could potentially cause issues
		leaves := []*paneNode{wm.root}
		for i := 0; i < 15; i++ {
			if len(leaves) > 0 {
				target := leaves[i%len(leaves)]
				newNode, err := wm.splitNode(target, "right")
				if err != nil {
					continue
				}
				leaves = append(leaves, newNode)
			}
		}

		// Test that collectLeaves doesn't hang
		done := make(chan []*paneNode, 1)
		go func() {
			result := wm.collectLeaves()
			done <- result
		}()

		select {
		case result := <-done:
			// Verify we got a reasonable result
			if len(result) == 0 {
				t.Error("Expected some leaves to be collected")
			}
			t.Logf("Collected %d leaves from complex tree", len(result))
		case <-time.After(1 * time.Second):
			t.Fatal("collectLeaves took too long - likely infinite recursion")
		}
	})

	t.Run("Focus calculation performance", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Create a tree with geometry
		right, err := wm.splitNode(wm.root, "right")
		if err != nil {
			t.Fatalf("Failed to split right: %v", err)
		}
		bottom, err := wm.splitNode(right, "down")
		if err != nil {
			t.Fatalf("Failed to split down: %v", err)
		}

		// Set widget bounds
		webkit.SetWidgetBoundsForTesting(wm.root.container, webkit.WidgetBounds{X: 0, Y: 0, Width: 100, Height: 100})
		if right != nil {
			webkit.SetWidgetBoundsForTesting(right.container, webkit.WidgetBounds{X: 120, Y: 0, Width: 100, Height: 100})
		}
		if bottom != nil {
			webkit.SetWidgetBoundsForTesting(bottom.container, webkit.WidgetBounds{X: 0, Y: 120, Width: 100, Height: 100})
		}

		// Test that focus neighbor calculations complete quickly
		start := time.Now()

		// Perform multiple focus operations
		for i := 0; i < 10; i++ {
			wm.FocusNeighbor("right")
			wm.FocusNeighbor("left")
			wm.FocusNeighbor("down")
			wm.FocusNeighbor("up")
		}

		elapsed := time.Since(start)
		if elapsed > 100*time.Millisecond {
			t.Errorf("Focus operations took too long: %v", elapsed)
		}
		t.Logf("40 focus operations completed in %v", elapsed)
	})

	t.Run("StructuralNeighbor depth protection", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Create nested structure
		node1, _ := wm.splitNode(wm.root, "right")
		node2, _ := wm.splitNode(node1, "down")
		node3, _ := wm.splitNode(node2, "right")

		// Set some bounds for the calculation
		webkit.SetWidgetBoundsForTesting(node3.container, webkit.WidgetBounds{X: 0, Y: 0, Width: 100, Height: 100})

		// Test structural neighbor calculation doesn't hang
		done := make(chan *paneNode, 1)
		go func() {
			result := wm.structuralNeighbor(node3, "left")
			done <- result
		}()

		select {
		case result := <-done:
			t.Logf("Structural neighbor calculation completed, result: %v", result != nil)
		case <-time.After(500 * time.Millisecond):
			t.Fatal("structuralNeighbor took too long")
		}
	})
}

func TestRecursionLimits(t *testing.T) {
	t.Run("MaxDepth protection", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Create an artificially corrupted node for testing
		// (This would never happen in normal operation)
		corruptedNode := &paneNode{
			isLeaf: false,
			left:   nil, // This creates a problematic state
			right:  nil, // No children but not a leaf
		}

		// Test that boundaryFallback handles this gracefully
		result := wm.boundaryFallback(corruptedNode, "right")
		if result != nil {
			t.Error("Expected nil result for corrupted node")
		}

		// The function should return quickly, not hang
		start := time.Now()
		for i := 0; i < 10; i++ {
			wm.boundaryFallback(corruptedNode, "left")
		}
		elapsed := time.Since(start)

		if elapsed > 10*time.Millisecond {
			t.Errorf("Corrupted node handling took too long: %v", elapsed)
		}
	})
}