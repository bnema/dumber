// Package entity defines domain entities for the browser.
package entity

// PaneRect represents a pane's screen position and size.
// Used for geometric navigation to find adjacent panes by position.
type PaneRect struct {
	PaneID PaneID
	X, Y   int // Top-left position relative to workspace container
	W, H   int // Width and height
}

// Center returns the center point of the rectangle.
func (r PaneRect) Center() (cx, cy int) {
	return r.X + r.W/2, r.Y + r.H/2
}

// OverlapsVertically returns true if the two rectangles overlap in the Y axis.
// Used for left/right navigation to prefer panes at the same vertical level.
func (r PaneRect) OverlapsVertically(other PaneRect) bool {
	// r spans [r.Y, r.Y+r.H), other spans [other.Y, other.Y+other.H)
	return r.Y < other.Y+other.H && other.Y < r.Y+r.H
}

// OverlapsHorizontally returns true if the two rectangles overlap in the X axis.
// Used for up/down navigation to prefer panes at the same horizontal level.
func (r PaneRect) OverlapsHorizontally(other PaneRect) bool {
	// r spans [r.X, r.X+r.W), other spans [other.X, other.X+other.W)
	return r.X < other.X+other.W && other.X < r.X+r.W
}
