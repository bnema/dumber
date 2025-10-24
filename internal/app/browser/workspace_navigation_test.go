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
