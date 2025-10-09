// workspace_utils.go - Utility functions and helpers for workspace management
package browser

import (
	"fmt"
	"log"

	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// hasMultiplePanes returns true if there are multiple panes in the workspace
func (wm *WorkspaceManager) hasMultiplePanes() bool {
	return wm != nil && wm.app != nil && len(wm.app.panes) > 1
}

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
func mapDirection(direction string) (gtk.Orientation, bool) {
	switch direction {
	case "left":
		return gtk.OrientationHorizontal, false
	case "up":
		return gtk.OrientationVertical, false
	case "down":
		return gtk.OrientationVertical, true
	default:
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

// PaneBorderContext holds the context for applying borders to any pane type
type PaneBorderContext struct {
	webViewWidget   gtk.Widgetter // The WebView's native widget (for margin)
	borderContainer gtk.Widgetter // The container that gets background color
	cssClasses      []string      // Additional CSS classes to apply
	paneType        string        // Description for debugging
}

// determineBorderContext analyzes a pane node and determines the correct border context
func (wm *WorkspaceManager) determineBorderContext(node *paneNode) *PaneBorderContext {
	if node == nil {
		return nil
	}

	ctx := &PaneBorderContext{}

	// Step 1: Get WebView widget for margin (same for all pane types)
	// Use AsWidget() to get the actual WebView widget, not the container
	if node.pane != nil && node.pane.webView != nil {
		ctx.webViewWidget = node.pane.webView.AsWidget()
	}

	// Step 2: Determine the border container based on pane type
	switch {
	case node.windowType == webkit.WindowTypePopup:
		// Popup panes: Border on popup container
		ctx.borderContainer = node.container
		ctx.cssClasses = []string{activePaneClass}
		ctx.paneType = "popup"

	case node.parent != nil && node.parent.isStacked:
		// Pane in stack: Border on stack wrapper container
		ctx.borderContainer = node.parent.stackWrapper
		ctx.cssClasses = []string{activePaneClass}
		ctx.paneType = "stacked"

	default:
		// Regular pane: Border on pane container
		ctx.borderContainer = node.container
		ctx.cssClasses = []string{activePaneClass}
		ctx.paneType = "regular"
	}

	return ctx
}

// applyActivePaneBorder applies the active pane border using the border context
func (wm *WorkspaceManager) applyActivePaneBorder(ctx *PaneBorderContext) {
	if ctx == nil {
		log.Printf("[workspace] applyActivePaneBorder: nil context")
		return
	}

	// Skip applying border if there's only one pane
	if !wm.hasMultiplePanes() {
		log.Printf("[workspace] Skipping active border: only one pane exists")
		return
	}

	// Apply margin to WebView widget (creates space for border)
	if ctx.webViewWidget != nil {
		webkit.WidgetSetMargin(ctx.webViewWidget, 2)
	}

	// Apply CSS classes to border container
	if ctx.borderContainer != nil {
		for _, class := range ctx.cssClasses {
			webkit.WidgetAddCSSClass(ctx.borderContainer, class)
		}
	}

	log.Printf("[workspace] Applied active border: type=%s webView=%p container=%p",
		ctx.paneType, ctx.webViewWidget, ctx.borderContainer)
}

// removeActivePaneBorder removes the active pane border from a node
func (wm *WorkspaceManager) removeActivePaneBorder(node *paneNode) {
	if node == nil {
		return
	}

	ctx := wm.determineBorderContext(node)
	if ctx == nil {
		return
	}

	// Remove margin from WebView widget
	if ctx.webViewWidget != nil {
		webkit.WidgetSetMargin(ctx.webViewWidget, 0)
	}

	// Remove CSS classes from border container
	if ctx.borderContainer != nil {
		for _, class := range ctx.cssClasses {
			webkit.WidgetRemoveCSSClass(ctx.borderContainer, class)
		}
	}

	log.Printf("[workspace] Removed active border: type=%s webView=%p container=%p",
		ctx.paneType, ctx.webViewWidget, ctx.borderContainer)
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
			info += fmt.Sprintf(" webview=%s", node.pane.webView.ID())
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
