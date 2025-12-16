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
