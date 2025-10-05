// workspace_utils.go - Utility functions and helpers for workspace management
package browser

import (
	"fmt"
	"log"

	"github.com/bnema/dumber/pkg/webkit"
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
func mapDirection(direction string) (webkit.Orientation, bool) {
	switch direction {
	case "left":
		return webkit.OrientationHorizontal, false
	case "up":
		return webkit.OrientationVertical, false
	case "down":
		return webkit.OrientationVertical, true
	default:
		return webkit.OrientationHorizontal, true
	}
}

// Widget management utilities

// initializePaneWidgets sets up widgets for a paneNode
func (wm *WorkspaceManager) initializePaneWidgets(node *paneNode, containerPtr uintptr) {
	node.container = containerPtr

	// Add base CSS class to the container
	if node.container != 0 && webkit.WidgetIsValid(node.container) {
		webkit.WidgetAddCSSClass(node.container, basePaneClass)
	}

	// Initialize other widget fields as zero (will be set when needed)
	node.titleBar = 0
	node.stackWrapper = 0

	// Initialize cleanup tracking fields
	node.widgetValid = true
	node.cleanupGeneration = 0
}

// setContainer sets the container widget pointer
func (wm *WorkspaceManager) setContainer(node *paneNode, ptr uintptr, typeInfo string) {
	_ = typeInfo // Type info kept for API compatibility but not used
	node.container = ptr

	// Mark widget validity based on container presence so idle guards run for stacks too
	if ptr != 0 {
		node.widgetValid = true
	} else {
		node.widgetValid = false
	}
}

// setTitleBar sets the titleBar widget pointer
func (wm *WorkspaceManager) setTitleBar(node *paneNode, ptr uintptr) {
	node.titleBar = ptr
}

// setStackWrapper sets the stackWrapper widget pointer
func (wm *WorkspaceManager) setStackWrapper(node *paneNode, ptr uintptr) {
	node.stackWrapper = ptr
}

// Centralized Active Pane Border Management System
// Handles all pane types: regular, popup, stacked, split panes

// PaneBorderContext holds the context for applying borders to any pane type
type PaneBorderContext struct {
	webViewWidget   uintptr  // The WebView's native widget (for margin)
	borderContainer uintptr  // The container that gets background color
	cssClasses      []string // Additional CSS classes to apply
	paneType        string   // Description for debugging
}

// determineBorderContext analyzes a pane node and determines the correct border context
func (wm *WorkspaceManager) determineBorderContext(node *paneNode) *PaneBorderContext {
	if node == nil {
		return nil
	}

	ctx := &PaneBorderContext{}

	// Step 1: Get WebView widget for margin (same for all pane types)
	if node.pane != nil && node.pane.webView != nil {
		ctx.webViewWidget = node.pane.webView.RootWidget()
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
	if ctx.webViewWidget != 0 && webkit.WidgetIsValid(ctx.webViewWidget) {
		webkit.WidgetSetMargin(ctx.webViewWidget, 2)
	}

	// Apply CSS classes to border container
	if ctx.borderContainer != 0 && webkit.WidgetIsValid(ctx.borderContainer) {
		for _, class := range ctx.cssClasses {
			webkit.WidgetAddCSSClass(ctx.borderContainer, class)
		}
	}

	log.Printf("[workspace] Applied active border: type=%s webView=%#x container=%#x",
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
	if ctx.webViewWidget != 0 && webkit.WidgetIsValid(ctx.webViewWidget) {
		webkit.WidgetSetMargin(ctx.webViewWidget, 0)
	}

	// Remove CSS classes from border container
	if ctx.borderContainer != 0 && webkit.WidgetIsValid(ctx.borderContainer) {
		for _, class := range ctx.cssClasses {
			webkit.WidgetRemoveCSSClass(ctx.borderContainer, class)
		}
	}

	log.Printf("[workspace] Removed active border: type=%s webView=%#x container=%#x",
		ctx.paneType, ctx.webViewWidget, ctx.borderContainer)
}

// Widget cleanup utilities

// cleanupWidget safely invalidates and cleans up a widget
func (wm *WorkspaceManager) cleanupWidget(ptr uintptr) {
	if ptr == 0 {
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
	handle = webkit.IdleAdd(func() bool {
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
	})
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
	webkit.IdleRemove(handle)
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
		info = fmt.Sprintf("leaf(container=%#x", node.container)
		if node.pane != nil && node.pane.webView != nil {
			info += fmt.Sprintf(" webview=%s", node.pane.webView.ID())
		}
		if node.windowType == webkit.WindowTypePopup {
			info += " popup"
		}
		info += ")"
	} else if node.isStacked {
		info = fmt.Sprintf("stack(panes=%d active=%d container=%#x)",
			len(node.stackedPanes), node.activeStackIndex, node.container)
	} else {
		info = fmt.Sprintf("branch(container=%#x orientation=%v)",
			node.container, node.orientation)
	}

	return info
}
