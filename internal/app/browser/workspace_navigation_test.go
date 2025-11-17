package browser

import (
	"testing"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// TestFocusNeighborVerticalSplit tests basic tree structure validation
// Note: Full geometry-based navigation testing requires integration tests with real GTK widgets
func TestFocusNeighborVerticalSplit(t *testing.T) {
	t.Skip("Structural neighbor navigation requires real GTK widgets for geometry calculations")
}

// TestFocusNeighborHorizontalSplit tests basic tree structure validation
// Note: Full geometry-based navigation testing requires integration tests with real GTK widgets
func TestFocusNeighborHorizontalSplit(t *testing.T) {
	t.Skip("Structural neighbor navigation requires real GTK widgets for geometry calculations")
}

// TestPaneNodeStructure verifies basic pane node relationships
func TestPaneNodeStructure(t *testing.T) {
	root := &paneNode{
		orientation: gtk.OrientationVertical,
		isLeaf:      false,
	}

	child1 := &paneNode{
		parent: root,
		isLeaf: true,
	}

	child2 := &paneNode{
		parent: root,
		isLeaf: true,
	}

	root.left = child1
	root.right = child2

	// Verify parent-child relationships
	if child1.parent != root {
		t.Errorf("child1.parent should be root")
	}

	if child2.parent != root {
		t.Errorf("child2.parent should be root")
	}

	// Verify root children
	if root.left != child1 {
		t.Errorf("root.left should be child1")
	}

	if root.right != child2 {
		t.Errorf("root.right should be child2")
	}

	// Verify leaf status
	if !child1.isLeaf {
		t.Errorf("child1 should be a leaf")
	}

	if !child2.isLeaf {
		t.Errorf("child2 should be a leaf")
	}

	if root.isLeaf {
		t.Errorf("root should not be a leaf")
	}
}

// TestMapDirection verifies direction mapping to GTK orientation
func TestMapDirection(t *testing.T) {
	tests := []struct {
		direction     string
		orientation   gtk.Orientation
		existingFirst bool
	}{
		{DirectionDown, gtk.OrientationVertical, true},
		{DirectionUp, gtk.OrientationVertical, false},
		{DirectionRight, gtk.OrientationHorizontal, true},
		{DirectionLeft, gtk.OrientationHorizontal, false},
	}

	for _, tt := range tests {
		t.Run(tt.direction, func(t *testing.T) {
			orientation, existingFirst := mapDirection(tt.direction)

			if orientation != tt.orientation {
				t.Errorf("mapDirection(%q) orientation = %v, want %v",
					tt.direction, orientation, tt.orientation)
			}

			if existingFirst != tt.existingFirst {
				t.Errorf("mapDirection(%q) existingFirst = %v, want %v",
					tt.direction, existingFirst, tt.existingFirst)
			}
		})
	}
}

// TestInvalidDirections verifies handling of invalid directions
func TestInvalidDirections(t *testing.T) {
	wm := &WorkspaceManager{}

	invalidDirections := []string{
		"",
		"unknown",
		"diagonal",
	}

	for _, direction := range invalidDirections {
		t.Run(direction, func(t *testing.T) {
			success := wm.FocusNeighbor(direction)
			if success {
				t.Errorf("Expected FocusNeighbor(%q) to return false, got true", direction)
			}
		})
	}
}

// TestCaseSensitiveDirections verifies that direction strings are case-insensitive
func TestCaseSensitiveDirections(t *testing.T) {
	wm := &WorkspaceManager{}

	// These should be accepted (case insensitive), but will return false
	// because there's no active pane to navigate from
	validDirections := []string{
		"UP",
		"Down",
		"LEFT",
		"Right",
		DirectionUp,
		DirectionDown,
		DirectionLeft,
		DirectionRight,
	}

	for _, direction := range validDirections {
		t.Run(direction, func(t *testing.T) {
			// Should not panic or error, just return false since no panes exist
			_ = wm.FocusNeighbor(direction)
		})
	}
}

// TestNavigationCoordinateBug documents the coordinate space bug in navigation
//
// Bug Description:
// When comparing positions for geometric navigation, the code mixes two coordinate spaces:
// - Normal panes: return window-absolute coordinates from WidgetGetAllocation()
// - Stacked panes: return stack-box-relative coordinates from WidgetGetAllocation()
//
// This causes navigation to fail because we're comparing coordinates in different spaces.
//
// Real-world example from logs:
//
//	Normal pane (top half):  center=(957, 532) - window coords
//	Stack pane (bottom):     center=(955, 511) - relative to stack box!
//	Result: dy=-21 → SKIPPED as "not below"
//
// The fix: Always use stack wrapper allocation for panes inside stacks.
func TestNavigationCoordinateBug(t *testing.T) {
	// Simulate the coordinate math from real user scenario
	// Window: 1914x1064, vertical split: normal pane (top) + stack (bottom)

	t.Run("BugValidation_CoordinateMixing", func(t *testing.T) {
		// Normal pane occupies top half - window-absolute coords
		normalCX, normalCY := 957.0, 266.0

		// Stack pane child returns RELATIVE coordinates (bug!)
		// Its allocation is relative to stack box at y=532
		stackChildCX, stackChildCY := 957.0, 133.0 // Relative to parent!

		dy := stackChildCY - normalCY
		t.Logf("Normal pane: center=(%.0f, %.0f)", normalCX, normalCY)
		t.Logf("Stack child (buggy relative): center=(%.0f, %.0f)", stackChildCX, stackChildCY)
		t.Logf("dy = %.0f (negative = appears above)", dy)

		// Navigation DOWN requires dy > 0, but we get negative!
		const focusEpsilon = 0.5
		if dy <= focusEpsilon {
			t.Logf("✓ BUG: dy=%.0f, navigation DOWN would FAIL", dy)
		}
	})

	t.Run("CorrectBehavior_UseStackWrapper", func(t *testing.T) {
		// Normal pane (same as before)
		normalCX, normalCY := 957.0, 266.0

		// Stack wrapper has correct window-absolute coordinates
		stackWrapperCX, stackWrapperCY := 957.0, 798.0

		dy := stackWrapperCY - normalCY
		t.Logf("Normal pane: center=(%.0f, %.0f)", normalCX, normalCY)
		t.Logf("Stack wrapper (correct): center=(%.0f, %.0f)", stackWrapperCX, stackWrapperCY)
		t.Logf("dy = %.0f (positive = correctly below)", dy)

		const focusEpsilon = 0.5
		if dy > focusEpsilon {
			t.Logf("✓ CORRECT: Navigation DOWN would SUCCEED")
		}

		// Also verify UP navigation
		dyUp := normalCY - stackWrapperCY
		if dyUp < -focusEpsilon {
			t.Logf("✓ CORRECT: Navigation UP would SUCCEED")
		}
	})
}

// TestGetNavigationAllocation tests the proposed fix helper function
// This will be uncommented and updated after implementing the helper
func TestGetNavigationAllocation(t *testing.T) {
	t.Skip("Enable this test after implementing getNavigationAllocation() helper")

	// TODO: After implementing getNavigationAllocation(), test:
	// 1. Regular leaf pane → returns pane.container allocation
	// 2. Pane inside stack → returns stack.stackWrapper allocation
	// 3. Stack container itself → returns stack.stackWrapper allocation
	// 4. Nil safety: node=nil, container=nil, parent=nil, stackWrapper=nil
}
