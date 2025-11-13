// workspace_utils.go - Utility functions and helpers for workspace management
package browser

import (
	"fmt"
	"log"

	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// UpdateTitleBar updates the title bar label for a WebView in stacked panes
func (wm *WorkspaceManager) UpdateTitleBar(webView *webkit.WebView, title string) {
	// Delegate to StackedPaneManager which has the correct logic
	if wm.stackedPaneManager != nil {
		wm.stackedPaneManager.UpdateTitleBar(webView, title)
	}
}

// ensureGUIInPane lazily loads GUI components into a pane when it gains focus
func (wm *WorkspaceManager) ensureGUIInPane(pane *BrowserPane) {
	if pane == nil {
		return
	}

	// GUI is already injected globally via WebKit's enableUserContentManager
	if !pane.HasGUI() {
		pane.SetHasGUI(true)
		pane.SetGUIComponent("manager", true)
		pane.SetGUIComponent("omnibox", true)
	}
}

// mapDirection maps a direction string to GTK orientation and positioning
// The bool return value indicates whether the existing pane should be the first child (start)
// In GTK Paned with Vertical orientation: start=top, end=bottom
// In GTK Paned with Horizontal orientation: start=left, end=right
func mapDirection(direction string) (gtk.Orientation, bool) {
	switch direction {
	case DirectionLeft:
		// New pane goes to left, existing to right
		return gtk.OrientationHorizontal, false
	case DirectionUp:
		// New pane goes to top (start), existing to bottom (end)
		// So existingFirst = false
		return gtk.OrientationVertical, false
	case DirectionDown:
		// New pane goes to bottom (end), existing to top (start)
		// So existingFirst = true
		return gtk.OrientationVertical, true
	default:
		// Right: new pane to right, existing to left
		return gtk.OrientationHorizontal, true
	}
}

// Widget management utilities

// initializePaneWidgets sets up widgets for a paneNode
func (wm *WorkspaceManager) initializePaneWidgets(node *paneNode, widget gtk.Widgetter) {
	node.container = widget

	// Add base CSS class to the container
	if node.container != nil {
		webkit.WidgetAddCSSClass(node.container, basePaneClass)
	}

	// Initialize other widget fields as nil (will be set when needed)
	node.titleBar = nil
	node.stackWrapper = nil

	// Initialize cleanup tracking fields
	node.widgetValid = true
	node.cleanupGeneration = 0
}

// setContainer sets the container widget
func (wm *WorkspaceManager) setContainer(node *paneNode, widget gtk.Widgetter, typeInfo string) {
	_ = typeInfo // Type info kept for API compatibility but not used
	node.container = widget

	// Mark widget validity based on container presence so idle guards run for stacks too
	if widget != nil {
		node.widgetValid = true
	} else {
		node.widgetValid = false
	}
}

// setTitleBar sets the titleBar widget
func (wm *WorkspaceManager) setTitleBar(node *paneNode, widget gtk.Widgetter) {
	node.titleBar = widget
}

// setStackWrapper sets the stackWrapper widget
func (wm *WorkspaceManager) setStackWrapper(node *paneNode, widget gtk.Widgetter) {
	node.stackWrapper = widget
}

// Centralized Active Pane Border Management System
// Handles all pane types: regular, popup, stacked, split panes

// borderTargets returns the widgets that should reflect the pane border state.
func (wm *WorkspaceManager) borderTargets(node *paneNode) []gtk.Widgetter {
	if node == nil {
		return nil
	}

	targets := make([]gtk.Widgetter, 0, 4)
	seen := make(map[*gtk.Widget]struct{})
	add := func(widget gtk.Widgetter) {
		if widget == nil {
			return
		}
		if base := gtk.BaseWidget(widget); base != nil {
			if _, exists := seen[base]; exists {
				return
			}
			seen[base] = struct{}{}
		}
		targets = append(targets, widget)
	}

	// Primary container for this node.
	add(node.container)
	add(node.stackWrapper)

	// Highlight stacks via their outer container as well.
	if node.parent != nil && node.parent.isStacked {
		add(node.parent.container)
		add(node.parent.stackWrapper)
	}

	// Fallback to the WebView root widget to cover any edge cases where the
	// container reference is temporarily nil.
	if node.pane != nil && node.pane.webView != nil {
		add(node.pane.webView.RootWidget())
	}

	if len(targets) == 0 {
		return nil
	}
	return targets
}

// applyActivePaneBorder marks the target widgets as active for CSS styling.
func (wm *WorkspaceManager) applyActivePaneBorder(node *paneNode) {
	if wm == nil || node == nil {
		return
	}

	// Only show active border if there are 2+ panes
	leaves := wm.collectLeaves()
	if len(leaves) < 2 {
		return
	}

	targets := wm.borderTargets(node)
	for _, widget := range targets {
		if widget == nil {
			continue
		}
		// Only add CSS class if it doesn't already exist to prevent bloom filter errors
		if !webkit.WidgetHasCSSClass(widget, activePaneClass) {
			webkit.WidgetAddCSSClass(widget, activePaneClass)
			log.Printf("[workspace] Applied active border to pane %p (widget=%p)", node, widget)
		}
	}
}

// removeActivePaneBorder clears the active styling from a pane.
func (wm *WorkspaceManager) removeActivePaneBorder(node *paneNode) {
	if wm == nil || node == nil {
		return
	}

	targets := wm.borderTargets(node)
	for _, widget := range targets {
		if widget == nil {
			continue
		}
		// Only remove CSS class if it exists to prevent bloom filter errors
		if webkit.WidgetHasCSSClass(widget, activePaneClass) {
			webkit.WidgetRemoveCSSClass(widget, activePaneClass)
			log.Printf("[workspace] Removed active border from pane %p (widget=%p)", node, widget)
		}
	}
}

// Widget cleanup utilities

// cleanupWidget safely invalidates and cleans up a widget
func (wm *WorkspaceManager) cleanupWidget(widget gtk.Widgetter) {
	if widget == nil {
		return
	}

	// Widget will be destroyed by GTK when unparented/removed
	// No manual cleanup needed - GTK4 handles reference counting
}

// scheduleIdleGuarded queues a callback to run on the GTK main loop and tracks it against
// the provided pane nodes so it can be cancelled safely if the nodes are destroyed.
func (wm *WorkspaceManager) scheduleIdleGuarded(fn func() bool, nodes ...*paneNode) uintptr {
	if wm == nil || fn == nil {
		return 0
	}
	guards := uniquePaneNodes(nodes...)
	var handle uintptr
	handle = uintptr(glib.IdleAdd(func() bool {
		defer wm.releaseIdleHandle(handle)
		for _, node := range guards {
			if node == nil {
				continue
			}
			if !node.widgetValid {
				return false
			}
		}
		return fn()
	}))
	if handle != 0 {
		wm.registerIdleHandle(handle, guards...)
	}
	return handle
}

func uniquePaneNodes(nodes ...*paneNode) []*paneNode {
	if len(nodes) == 0 {
		return nil
	}
	seen := make(map[*paneNode]struct{}, len(nodes))
	unique := make([]*paneNode, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if _, ok := seen[node]; ok {
			continue
		}
		seen[node] = struct{}{}
		unique = append(unique, node)
	}
	return unique
}

func (wm *WorkspaceManager) registerIdleHandle(handle uintptr, nodes ...*paneNode) {
	if wm == nil || handle == 0 {
		return
	}
	unique := uniquePaneNodes(nodes...)
	if len(unique) == 0 {
		return
	}
	if wm.pendingIdle == nil {
		wm.pendingIdle = make(map[uintptr][]*paneNode)
	}
	wm.pendingIdle[handle] = unique
	for _, node := range unique {
		if node.pendingIdleHandles == nil {
			node.pendingIdleHandles = make(map[uintptr]struct{})
		}
		node.pendingIdleHandles[handle] = struct{}{}
	}
}

func (wm *WorkspaceManager) releaseIdleHandle(handle uintptr) {
	if wm == nil || handle == 0 {
		return
	}
	if wm.pendingIdle == nil {
		return
	}
	nodes, ok := wm.pendingIdle[handle]
	if !ok {
		return
	}
	delete(wm.pendingIdle, handle)
	for _, node := range nodes {
		if node == nil || node.pendingIdleHandles == nil {
			continue
		}
		delete(node.pendingIdleHandles, handle)
		if len(node.pendingIdleHandles) == 0 {
			node.pendingIdleHandles = nil
		}
	}
}

func (wm *WorkspaceManager) cancelIdleHandle(handle uintptr) {
	if handle == 0 {
		return
	}
	wm.releaseIdleHandle(handle)
	glib.SourceRemove(glib.SourceHandle(handle))
}

func (wm *WorkspaceManager) cancelIdleHandles(node *paneNode) {
	if wm == nil || node == nil || len(node.pendingIdleHandles) == 0 {
		return
	}
	handles := make([]uintptr, 0, len(node.pendingIdleHandles))
	for handle := range node.pendingIdleHandles {
		handles = append(handles, handle)
	}
	for _, handle := range handles {
		wm.cancelIdleHandle(handle)
	}
}

// Note: nextCleanupGeneration, updateMainPane, dumpTreeState, paneCloseLogf
// are defined in workspace_pane_ops.go and workspace_debug.go

// formatPaneInfo returns a debug string for a pane node
func formatPaneInfo(node *paneNode) string {
	if node == nil {
		return "nil"
	}

	var info string
	if node.isLeaf {
		info = fmt.Sprintf("leaf(container=%p", node.container)
		if node.pane != nil && node.pane.webView != nil {
			info += fmt.Sprintf(" webview=%d", node.pane.webView.ID())
		}
		if node.windowType == webkit.WindowTypePopup {
			info += " popup"
		}
		info += ")"
	} else if node.isStacked {
		info = fmt.Sprintf("stack(panes=%d active=%d container=%p)",
			len(node.stackedPanes), node.activeStackIndex, node.container)
	} else {
		info = fmt.Sprintf("branch(container=%p orientation=%v)",
			node.container, node.orientation)
	}

	return info
}
