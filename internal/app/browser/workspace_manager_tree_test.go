package browser

import (
	"fmt"
	"testing"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
)

// Test helper to create mock pane node
func newMockPaneNode(id string) *paneNode {
	container := uintptr(100 + len(id))

	return &paneNode{
		pane:      &BrowserPane{id: id},
		container: container,
		isLeaf:    true,
	}
}

// Test tree validation helper
func validateTreeStructure(t *testing.T, node *paneNode) {
	t.Helper()

	if node == nil {
		return
	}

	if node.isLeaf {
		// Leaf nodes should not have children
		if node.left != nil || node.right != nil {
			t.Errorf("Leaf node has children: left=%v, right=%v", node.left, node.right)
		}
		// Leaf nodes must have a pane
		if node.pane == nil {
			t.Error("Leaf node missing pane")
		}
	} else {
		// Branch nodes must have both children
		if node.left == nil || node.right == nil {
			t.Errorf("Branch node missing children: left=%v, right=%v", node.left, node.right)
		}
		// Branch nodes should not have a pane
		if node.pane != nil {
			t.Error("Branch node should not have pane")
		}
		// Validate children's parent pointers
		if node.left != nil && node.left.parent != node {
			t.Error("Left child's parent pointer incorrect")
		}
		if node.right != nil && node.right.parent != node {
			t.Error("Right child's parent pointer incorrect")
		}
		// Recursively validate subtrees
		validateTreeStructure(t, node.left)
		validateTreeStructure(t, node.right)
	}
}

// Count nodes in tree
func countNodes(node *paneNode) (leaves, branches int) {
	if node == nil {
		return 0, 0
	}
	if node.isLeaf {
		return 1, 0
	}
	leftLeaves, leftBranches := countNodes(node.left)
	rightLeaves, rightBranches := countNodes(node.right)
	return leftLeaves + rightLeaves, leftBranches + rightBranches + 1
}

func TestPaneNodeTreeInvariants(t *testing.T) {
	t.Run("Initial root is leaf", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		if !wm.root.isLeaf {
			t.Error("Initial root should be a leaf")
		}
		if wm.root.parent != nil {
			t.Error("Root should have no parent")
		}
		validateTreeStructure(t, wm.root)
	})

	t.Run("Tree structure after single split", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)
		original := wm.root

		// Split right
		newNode, err := wm.splitNode(original, "right")
		if err != nil {
			t.Fatalf("Split failed: %v", err)
		}

		// Validate new tree structure
		validateTreeStructure(t, wm.root)

		// Root should now be a branch
		if wm.root.isLeaf {
			t.Error("Root should be branch after split")
		}

		// Original and new nodes should be siblings
		if wm.root.left != original {
			t.Error("Original node should be left child")
		}
		if wm.root.right != newNode {
			t.Error("New node should be right child")
		}

		// Count nodes
		leaves, branches := countNodes(wm.root)
		if leaves != 2 {
			t.Errorf("Expected 2 leaves, got %d", leaves)
		}
		if branches != 1 {
			t.Errorf("Expected 1 branch, got %d", branches)
		}
	})
}

func TestPaneSplitDirections(t *testing.T) {
	testCases := []struct {
		name                     string
		direction                string
		expectedOrientation      webkit.Orientation
		expectedOriginalPosition string // "left" or "right" child
	}{
		{"SplitRight", "right", webkit.OrientationHorizontal, "left"},
		{"SplitLeft", "left", webkit.OrientationHorizontal, "right"},
		{"SplitDown", "down", webkit.OrientationVertical, "left"},
		{"SplitUp", "up", webkit.OrientationVertical, "right"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wm := newTestWorkspaceManagerWithMocksForTree(t)
			original := wm.root

			newNode, err := wm.splitNode(original, tc.direction)
			if err != nil {
				t.Fatalf("Split failed: %v", err)
			}

			parent := original.parent
			if parent.orientation != tc.expectedOrientation {
				t.Errorf("Expected orientation %v, got %v",
					tc.expectedOrientation, parent.orientation)
			}

			if tc.expectedOriginalPosition == "left" {
				if parent.left != original {
					t.Error("Original should be left child")
				}
				if parent.right != newNode {
					t.Error("New node should be right child")
				}
			} else {
				if parent.right != original {
					t.Error("Original should be right child")
				}
				if parent.left != newNode {
					t.Error("New node should be left child")
				}
			}

			validateTreeStructure(t, wm.root)
		})
	}
}

func TestComplexTreeOperations(t *testing.T) {
	t.Run("Multiple nested splits", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Create complex tree: split right, then split the right pane down
		node1, _ := wm.splitNode(wm.root, "right")
		node2, _ := wm.splitNode(node1, "down")

		validateTreeStructure(t, wm.root)

		// Count nodes
		leaves, branches := countNodes(wm.root)
		if leaves != 3 {
			t.Errorf("Expected 3 leaves, got %d", leaves)
		}
		if branches != 2 {
			t.Errorf("Expected 2 branches, got %d", branches)
		}

		// Verify node2 is correctly positioned
		if node2 == nil {
			t.Error("node2 should not be nil")
		}
	})

	t.Run("Deep nesting", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Create a chain of splits
		current := wm.root
		for i := 0; i < 5; i++ {
			newNode, err := wm.splitNode(current, "right")
			if err != nil {
				t.Fatalf("Split %d failed: %v", i, err)
			}
			current = newNode
		}

		validateTreeStructure(t, wm.root)

		leaves, branches := countNodes(wm.root)
		if leaves != 6 {
			t.Errorf("Expected 6 leaves, got %d", leaves)
		}
		if branches != 5 {
			t.Errorf("Expected 5 branches, got %d", branches)
		}
	})
}

func TestPaneDeletion(t *testing.T) {
	t.Run("Close leaf promotes sibling", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)
		original := wm.root

		// Split and then close the new pane
		newNode, _ := wm.splitNode(original, "right")

		err := wm.closePane(newNode)
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		// Original should be root again
		if wm.root != original {
			t.Error("Original should be promoted to root")
		}
		if original.parent != nil {
			t.Error("Promoted node should have no parent")
		}

		validateTreeStructure(t, wm.root)
	})

	t.Run("Close in complex tree", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Build tree: root splits right to A and B, then B splits down to C and D
		A := wm.root
		B, _ := wm.splitNode(A, "right")
		C, _ := wm.splitNode(B, "up")
		// D is the node that B became after split (positioned down)
		D := B.parent.right

		// Close C - D should be promoted to take the place of C's parent
		err := wm.closePane(C)
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		validateTreeStructure(t, wm.root)

		// Tree should now have A and D as siblings
		if wm.root.left != A && wm.root.right != A {
			t.Error("A should still be child of root")
		}
		if wm.root.left != D && wm.root.right != D {
			t.Error("D should be promoted as child of root")
		}

		leaves, branches := countNodes(wm.root)
		if leaves != 2 {
			t.Errorf("Expected 2 leaves after deletion, got %d", leaves)
		}
		if branches != 1 {
			t.Errorf("Expected 1 branch after deletion, got %d", branches)
		}
	})
}

func TestRootDelegation(t *testing.T) {
	t.Run("Closing root with siblings delegates correctly", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Create structure where root will need delegation
		originalRoot := wm.root
		rightNode, _ := wm.splitNode(originalRoot, "right")

		// Now originalRoot and rightNode are siblings under new root
		// Try to close the left child (originalRoot)
		err := wm.closePane(originalRoot)
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		// rightNode should now be the root
		if wm.root != rightNode {
			t.Error("Right sibling should be promoted to root")
		}
		if rightNode.parent != nil {
			t.Error("New root should have no parent")
		}

		validateTreeStructure(t, wm.root)
	})
}

func TestFindReplacementRoot(t *testing.T) {
	t.Run("Find replacement in complex tree", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Build a complex tree
		A := wm.root
		B, _ := wm.splitNode(A, "right")
		_, _ = wm.splitNode(B, "down")

		// Find replacement excluding A
		replacement := wm.findReplacementRoot(A)

		if replacement == nil {
			t.Error("Should find replacement root")
		}
		if replacement == A {
			t.Error("Replacement should not be excluded node")
		}
	})

	t.Run("No replacement when only one pane", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		replacement := wm.findReplacementRoot(wm.root)
		if replacement != nil {
			t.Error("Should not find replacement when only one pane exists")
		}
	})
}

func TestTreeTraversal(t *testing.T) {
	t.Run("CollectLeaves gathers all leaf nodes", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Create tree with 4 leaves
		// Keep track of the original root which will become a leaf after split
		original := wm.root
		node1, _ := wm.splitNode(original, "right")
		_, _ = wm.splitNode(node1, "down")
		_, _ = wm.splitNode(original, "down") // Split the original leaf, not the new root branch

		leaves := wm.collectLeaves()

		if len(leaves) != 4 {
			t.Errorf("Expected 4 leaves, got %d", len(leaves))
		}

		// All collected nodes should be leaves
		for _, leaf := range leaves {
			if !leaf.isLeaf {
				t.Error("Non-leaf node in collectLeaves result")
			}
		}
	})

	t.Run("LeftmostLeaf finds correct node", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		original := wm.root
		right, _ := wm.splitNode(original, "right")
		_, _ = wm.splitNode(right, "right")

		leftmost := wm.leftmostLeaf(wm.root)
		if leftmost != original {
			t.Error("Leftmost should be original node")
		}
	})

	t.Run("LeftmostLeaf handles nil", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		leftmost := wm.leftmostLeaf(nil)
		if leftmost != nil {
			t.Error("LeftmostLeaf should return nil for nil input")
		}
	})
}

func TestErrorHandling(t *testing.T) {
	t.Run("Cannot split non-leaf", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Create a branch node
		_, err := wm.splitNode(wm.root, "right")
		if err != nil {
			t.Fatalf("Failed to create branch node: %v", err)
		}

		// Try to split the branch (root is now a branch)
		_, err = wm.splitNode(wm.root, "down")

		if err == nil {
			t.Error("Should not allow splitting branch nodes")
		}
	})

	t.Run("Cannot close non-leaf", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		_, err := wm.splitNode(wm.root, "right")
		if err != nil {
			t.Fatalf("Failed to create branch node: %v", err)
		}

		// Try to close the branch node
		err = wm.closePane(wm.root)

		if err == nil {
			t.Error("Should not allow closing branch nodes")
		}
	})

	t.Run("Cannot split nil node", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		_, err := wm.splitNode(nil, "right")

		if err == nil {
			t.Error("Should not allow splitting nil nodes")
		}
	})

	t.Run("Cannot close nil node", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		err := wm.closePane(nil)

		if err == nil {
			t.Error("Should not allow closing nil nodes")
		}
	})
}

func TestFocusManagement(t *testing.T) {
	t.Run("Focus transfers after closing active pane", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		original := wm.root
		newNode, _ := wm.splitNode(original, "right")

		// Set new node as active
		wm.active = newNode

		// Close the active node
		err := wm.closePane(newNode)
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		// Focus should transfer to original
		if wm.active != original {
			t.Error("Focus should transfer to remaining pane")
		}
	})

	t.Run("Focus preserved when closing non-active pane", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		original := wm.root
		newNode, _ := wm.splitNode(original, "right")

		// Keep original as active
		wm.active = original

		// Close the non-active node
		err := wm.closePane(newNode)
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		// Focus should remain on original
		if wm.active != original {
			t.Error("Focus should remain on original pane")
		}
	})
}

// TestBugClosePaneWithBranchSibling reproduces the bug where closing a pane
// whose sibling is a branch (not a leaf) causes "unexpected tree state" error
func TestBugClosePaneWithBranchSibling(t *testing.T) {
	t.Run("Closing pane when sibling is branch with children", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Create the exact scenario from the bug:
		// 1. Start with root pane A
		A := wm.root

		// 2. Split down to create B (A is now top, B is bottom)
		B, err := wm.splitNode(A, "down")
		if err != nil {
			t.Fatalf("Split down failed: %v", err)
		}

		// After this split:
		// Root (vertical paned) has A (top) and B (bottom)

		// 3. Split B down again to create C
		C, err := wm.splitNode(B, "down")
		if err != nil {
			t.Fatalf("Split B down failed: %v", err)
		}

		// After this split, tree structure is:
		// Root (vertical) -> A (top), SubPaned (vertical, bottom)
		//                             SubPaned -> B (top), C (bottom)
		// Total: 3 leaf panes (A, B, C), 2 branch nodes

		// Update app.panes to reflect actual structure
		wm.app.panes = []*BrowserPane{A.pane, B.pane, C.pane}

		// Verify initial structure
		validateTreeStructure(t, wm.root)
		leaves, branches := countNodes(wm.root)
		if leaves != 3 {
			t.Errorf("Expected 3 leaves initially, got %d", leaves)
		}
		if branches != 2 {
			t.Errorf("Expected 2 branches initially, got %d", branches)
		}

		// Now close pane A - this should promote the SubPaned (containing B and C) to root
		// This is where the bug occurs: parent is root, but 2 panes remain (B and C)
		// The current code incorrectly thinks this is an error
		err = wm.closePane(A)

		// THIS IS THE BUG: The current implementation returns an error here
		// but it shouldn't - it's a valid operation
		if err != nil {
			// For now, we expect this to fail with the bug
			if err.Error() == "unexpected tree state during pane closure" {
				t.Logf("Bug reproduced: %v", err)
				t.Logf("This error should not occur - closing pane with branch sibling is valid")
				// Comment out the return to see what the final state should be
				// return
			} else {
				t.Fatalf("Unexpected error: %v", err)
			}
		}

		// What SHOULD happen after successful close:
		// The SubPaned (containing B and C) should become the new root
		validateTreeStructure(t, wm.root)

		// Verify final state (what we expect after fix)
		finalLeaves, finalBranches := countNodes(wm.root)
		if finalLeaves != 2 {
			t.Errorf("Expected 2 leaves after close, got %d", finalLeaves)
		}
		if finalBranches != 1 {
			t.Errorf("Expected 1 branch after close, got %d", finalBranches)
		}

		// The new root should be a branch (the old SubPaned)
		if wm.root.isLeaf {
			t.Error("Root should be a branch node after closing A")
		}

		// B and C should still be in the tree
		foundB := false
		foundC := false
		for _, leaf := range wm.collectLeaves() {
			if leaf == B {
				foundB = true
			}
			if leaf == C {
				foundC = true
			}
		}
		if !foundB {
			t.Error("Pane B should still be in tree")
		}
		if !foundC {
			t.Error("Pane C should still be in tree")
		}
	})

	t.Run("Alternative scenario - closing left child when right has grandchildren", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Create structure: A | (B / C)
		// where | is horizontal split and / is vertical split
		A := wm.root
		B, err := wm.splitNode(A, "right")
		if err != nil {
			t.Fatalf("Split right failed: %v", err)
		}

		C, err := wm.splitNode(B, "down")
		if err != nil {
			t.Fatalf("Split down failed: %v", err)
		}

		// Update app.panes
		wm.app.panes = []*BrowserPane{A.pane, B.pane, C.pane}

		// Tree is: Root(h) -> A, Paned(v) -> B, C
		validateTreeStructure(t, wm.root)

		// Close A - the Paned(v) containing B and C should become root
		err = wm.closePane(A)

		// Bug should occur here too
		if err != nil {
			if err.Error() == "unexpected tree state during pane closure" {
				t.Logf("Bug reproduced in alternative scenario: %v", err)
			}
		}

		// After fix, the vertical paned with B and C should be root
		validateTreeStructure(t, wm.root)
	})
}

func TestEdgeCases(t *testing.T) {
	t.Run("Closing last pane", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Mock the app.panes slice to have only one pane
		wm.app.panes = []*BrowserPane{wm.root.pane}

		// Closing the last pane should handle gracefully
		err := wm.closePane(wm.root)
		if err != nil {
			t.Logf("Expected behavior: closing last pane returned error: %v", err)
		}
	})

	t.Run("Split with invalid direction", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Test default behavior with invalid direction
		newNode, err := wm.splitNode(wm.root, "invalid")
		if err != nil {
			t.Fatalf("Split with invalid direction failed: %v", err)
		}

		// Should default to horizontal orientation
		if newNode.parent.orientation != webkit.OrientationHorizontal {
			t.Error("Invalid direction should default to horizontal")
		}
	})
}

func TestInfiniteSiblingsClosingIssue(t *testing.T) {
	t.Run("Infinite siblings issue reproduction", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Reproduce the scenario: Create multiple siblings by repeatedly splitting right
		original := wm.root
		rightPane1, err := wm.splitNode(original, "right")
		if err != nil {
			t.Fatalf("First split failed: %v", err)
		}

		// Split the right pane down to create a nested structure
		downPane, err := wm.splitNode(rightPane1, "down")
		if err != nil {
			t.Fatalf("Down split failed: %v", err)
		}

		// Verify tree structure is correct
		validateTreeStructure(t, wm.root)

		// Count initial panes
		initialLeaves, initialBranches := countNodes(wm.root)
		if initialLeaves != 3 {
			t.Errorf("Expected 3 leaves after splits, got %d", initialLeaves)
		}
		if initialBranches != 2 {
			t.Errorf("Expected 2 branches after splits, got %d", initialBranches)
		}

		// Update app.panes to reflect the actual structure
		wm.app.panes = []*BrowserPane{original.pane, rightPane1.pane, downPane.pane}

		// First close the downPane (the non-direct sibling)
		err = wm.closePane(downPane)
		if err != nil {
			t.Fatalf("Close downPane failed: %v", err)
		}

		validateTreeStructure(t, wm.root)

		// Now try to close rightPane1 (which should leave only original)
		// This should cause the promotion of original to root
		err = wm.closePane(rightPane1)
		if err != nil {
			t.Fatalf("Close rightPane1 failed: %v", err)
		}

		validateTreeStructure(t, wm.root)

		// Final tree should have only original as root
		if wm.root != original {
			t.Error("Original should be promoted to root after closing siblings")
		}
		if !wm.root.isLeaf {
			t.Error("Root should be a leaf after closing all siblings")
		}

		finalLeaves, finalBranches := countNodes(wm.root)
		if finalLeaves != 1 {
			t.Errorf("Expected 1 leaf after closing siblings, got %d", finalLeaves)
		}
		if finalBranches != 0 {
			t.Errorf("Expected 0 branches after closing siblings, got %d", finalBranches)
		}
	})

	t.Run("Closing direct sibling of root reproduces GTK error", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Create a simple two-pane layout (root + sibling)
		original := wm.root
		sibling, err := wm.splitNode(original, "right")
		if err != nil {
			t.Fatalf("Split failed: %v", err)
		}

		// Update app.panes
		wm.app.panes = []*BrowserPane{original.pane, sibling.pane}

		validateTreeStructure(t, wm.root)

		// Now close the sibling - this should promote original back to root
		// and should NOT cause GTK-CRITICAL errors
		err = wm.closePane(sibling)
		if err != nil {
			t.Fatalf("Close sibling failed: %v", err)
		}

		validateTreeStructure(t, wm.root)

		// Verify the tree is properly cleaned up
		if wm.root != original {
			t.Error("Original should be promoted to root")
		}
		if !wm.root.isLeaf {
			t.Error("Root should be a leaf after closing sibling")
		}
		if wm.root.parent != nil {
			t.Error("Root should have no parent")
		}
	})

	t.Run("Complex closing sequence without direct root sibling", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		// Create a more complex tree that can test the issue where
		// closing requires closing until only the direct sibling of root remains

		// Start with root (A)
		A := wm.root

		// Split right to create B
		B, err := wm.splitNode(A, "right")
		if err != nil {
			t.Fatalf("Split A->B failed: %v", err)
		}

		// Split B down to create C (B becomes internal node, C is new leaf)
		C, err := wm.splitNode(B, "down")
		if err != nil {
			t.Fatalf("Split B->C failed: %v", err)
		}
		// After this split, the tree structure is:
		// Root(horizontal) -> A (left), Parent(vertical) (right)
		//                           -> B (top), C (bottom)

		// Split C right to create D
		D, err := wm.splitNode(C, "right")
		if err != nil {
			t.Fatalf("Split C->D failed: %v", err)
		}

		// Update app.panes to reflect actual panes
		wm.app.panes = []*BrowserPane{A.pane, B.pane, C.pane, D.pane}

		validateTreeStructure(t, wm.root)

		// Count nodes to verify structure
		leaves, branches := countNodes(wm.root)
		if leaves != 4 {
			t.Errorf("Expected 4 leaves, got %d", leaves)
		}
		if branches != 3 {
			t.Errorf("Expected 3 branches, got %d", branches)
		}

		// Now start closing panes in a sequence that would eventually
		// require promoting siblings and potentially trigger the GTK error

		// Close D first
		err = wm.closePane(D)
		if err != nil {
			t.Fatalf("Close D failed: %v", err)
		}
		validateTreeStructure(t, wm.root)

		// Close C next
		err = wm.closePane(C)
		if err != nil {
			t.Fatalf("Close C failed: %v", err)
		}
		validateTreeStructure(t, wm.root)

		// Close B next - this should leave only A and trigger the root sibling issue
		err = wm.closePane(B)
		if err != nil {
			t.Fatalf("Close B failed: %v", err)
		}
		validateTreeStructure(t, wm.root)

		// Verify final state
		if wm.root != A {
			t.Error("A should be the final root")
		}
		if !wm.root.isLeaf {
			t.Error("Final root should be a leaf")
		}

		finalLeaves, finalBranches := countNodes(wm.root)
		if finalLeaves != 1 {
			t.Errorf("Expected 1 final leaf, got %d", finalLeaves)
		}
		if finalBranches != 0 {
			t.Errorf("Expected 0 final branches, got %d", finalBranches)
		}
	})
}

func TestViewToNodeMapping(t *testing.T) {
	t.Run("ViewToNode map updates correctly", func(t *testing.T) {
		wm := newTestWorkspaceManagerWithMocksForTree(t)

		originalNode := wm.root
		originalView := wm.root.pane.webView

		// Split should add new mapping
		newNode, _ := wm.splitNode(wm.root, "right")
		newView := newNode.pane.webView

		// Check mapping exists for new view
		if mapped, ok := wm.viewToNode[newView]; !ok || mapped != newNode {
			t.Error("New webview should be mapped to new node")
		}

		// Original mapping should still exist - note that wm.root may have changed after split
		if mapped, ok := wm.viewToNode[originalView]; !ok || mapped != originalNode {
			t.Error("Original webview mapping should be preserved")
		}

		// Close should remove mapping
		err := wm.closePane(newNode)
		if err != nil {
			t.Fatalf("Failed to close pane: %v", err)
		}

		if _, ok := wm.viewToNode[newView]; ok {
			t.Error("Closed pane's webview should be removed from mapping")
		}
	})
}

// Helper to create WorkspaceManager with minimal mocked dependencies
func newTestWorkspaceManagerWithMocksForTree(t *testing.T) *WorkspaceManager {
	t.Helper()
	webkit.ResetWidgetStubsForTesting()

	// Create a real WebView for testing (stub implementation)
	rootView, err := webkit.NewWebView(&webkit.Config{})
	if err != nil {
		t.Fatalf("failed to create root webview: %v", err)
	}

	rootPane := &BrowserPane{
		webView: rootView,
		id:      "root",
	}

	// Create mock window shortcut handler
	mockWindowShortcutHandler := &mockWindowShortcutHandler{}

	app := &BrowserApp{
		panes:                 []*BrowserPane{rootPane},
		activePane:            rootPane,
		webView:               rootView,
		windowShortcutHandler: mockWindowShortcutHandler,
	}

	wm := &WorkspaceManager{
		app:          app,
		window:       rootView.Window(),
		viewToNode:   make(map[*webkit.WebView]*paneNode),
		lastSplitMsg: make(map[*webkit.WebView]time.Time),
		lastExitMsg:  make(map[*webkit.WebView]time.Time),
	}

	// Set up mock factories
	wm.createWebViewFn = func() (*webkit.WebView, error) {
		return webkit.NewWebView(&webkit.Config{})
	}

	wm.createPaneFn = func(view *webkit.WebView) (*BrowserPane, error) {
		return &BrowserPane{
			webView: view,
			id:      fmt.Sprintf("pane-%p", view),
		}, nil
	}

	// Initialize root
	root := &paneNode{
		pane:      rootPane,
		container: rootView.RootWidget(),
		isLeaf:    true,
	}
	wm.root = root
	wm.active = root
	wm.mainPane = root
	wm.viewToNode[rootView] = root

	return wm
}
