// workspace_navigation.go - Navigation and focus management for workspace panes
package browser

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	// focusThrottleInterval prevents rapid focus changes that could cause infinite loops
	focusThrottleInterval = 100 * time.Millisecond
)

// DispatchPaneFocusEvent sends a workspace focus event to a pane's webview
func (wm *WorkspaceManager) DispatchPaneFocusEvent(node *paneNode, active bool) {
	if node == nil || node.pane == nil || node.pane.webView == nil {
		return
	}

	detail := map[string]any{
		"active":    active,
		"webview":   fmt.Sprintf("%p", node.pane.webView),
		"webviewId": node.pane.webView.ID(),
		"hasGUI":    node.pane.HasGUI(),
		"timestamp": time.Now().UnixMilli(),
	}

	if err := node.pane.webView.DispatchCustomEvent("dumber:workspace-focus", detail); err != nil {
		log.Printf("[workspace] failed to dispatch focus event: %v", err)
	} else if wm.app.config != nil && wm.app.config.Debug.EnableWorkspaceDebug {
		log.Printf("[workspace] dispatched focus event for webview %d (active=%v)", node.pane.webView.ID(), active)
	}
}

// focusAfterPaneMode restores focus when a pane mode operation completes without
// stealing focus from the active pane inside a stack that just spawned a sibling.
func (wm *WorkspaceManager) focusAfterPaneMode(node *paneNode) {
	wm.focusRespectingStack(node, "focus-after-pane-mode")
}

// focusRespectingStack handles focus while respecting stacked pane hierarchies
func (wm *WorkspaceManager) focusRespectingStack(node *paneNode, reason string) {
	if node == nil {
		return
	}

	if stack := node.parent; stack != nil && stack.isStacked {
		activeIndex := stack.activeStackIndex
		if activeIndex >= 0 && activeIndex < len(stack.stackedPanes) {
			activePane := stack.stackedPanes[activeIndex]
			if activePane != nil && activePane != node {
				if reason != "" {
					log.Printf("[workspace] %s: preserving active stacked pane index=%d", reason, activeIndex)
				}
				wm.SetActivePane(activePane, SourceKeyboard)
				return
			}
		}
	}

	wm.SetActivePane(node, SourceKeyboard)
}

// focusByView changes focus to the pane containing the specified WebView
func (wm *WorkspaceManager) focusByView(view *webkit.WebView) {
	if wm == nil || view == nil {
		return
	}

	// Throttle focus changes to prevent infinite loops
	wm.focusThrottleMutex.Lock()
	if time.Since(wm.lastFocusChange) < focusThrottleInterval {
		wm.focusThrottleMutex.Unlock()
		return
	}
	wm.lastFocusChange = time.Now()
	wm.focusThrottleMutex.Unlock()

	if node, ok := wm.viewToNode[view]; ok {
		if wm.GetActiveNode() != node {
			wm.focusRespectingStack(node, "focus-by-view")
		}
	}
}

// Note: ensureHover and detachHover are now implemented in workspace_hover.go

// detachFocus removes the GTK focus controller attached to a pane, if any.
func (wm *WorkspaceManager) detachFocus(node *paneNode) {
	if wm == nil || node == nil || node.focusControllerToken == 0 {
		return
	}

	if wm.focusStateMachine != nil {
		wm.focusStateMachine.detachGTKController(node, node.focusControllerToken)
	}

	node.focusControllerToken = 0
}

// FocusNeighbor moves focus to the nearest pane in the requested direction using the
// actual widget geometry to determine adjacency. For stacked panes, DirectionUp and DirectionDown
// navigate within the stack.
func (wm *WorkspaceManager) FocusNeighbor(direction string) bool {
	if wm == nil {
		return false
	}
	switch strings.ToLower(direction) {
	case DirectionUp, DirectionDown:
		// Check if current pane is part of a stack and handle stack navigation
		if wm.stackedPaneManager.NavigateStack(strings.ToLower(direction)) {
			return true
		}
		// Fall back to regular adjacency navigation
		return wm.focusAdjacent(strings.ToLower(direction))
	case DirectionLeft, DirectionRight:
		return wm.focusAdjacent(strings.ToLower(direction))
	default:
		log.Printf("[workspace] invalid focus direction: %s (expected: up, down, left, right)", direction)
		return false
	}
}

// navigateStack handles navigation within a stacked pane container.
func (wm *WorkspaceManager) navigateStack(direction string) bool {
	currentFocused := wm.GetActiveNode()
	if currentFocused == nil {
		return false
	}

	// Find the stack container this pane belongs to
	var stackNode *paneNode
	current := currentFocused

	// Check if current pane is directly in a stack
	if current.parent != nil && current.parent.isStacked {
		stackNode = current.parent
	} else {
		// Current pane might be the stack container itself if it was the first pane converted to stack
		if current.isStacked {
			stackNode = current
		}
	}

	if stackNode == nil || !stackNode.isStacked || len(stackNode.stackedPanes) <= 1 {
		return false // Not in a stack or stack has only one pane
	}

	// Find current pane's index in the stack
	currentIndex := -1
	for i, pane := range stackNode.stackedPanes {
		if pane == current || (pane.pane != nil && current.pane != nil && pane.pane.webView == current.pane.webView) {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 {
		log.Printf("[workspace] navigateStack: current pane not found in stack")
		return false
	}

	// Calculate new index based on direction
	var newIndex int
	switch direction {
	case DirectionUp:
		newIndex = currentIndex - 1
		if newIndex < 0 {
			newIndex = len(stackNode.stackedPanes) - 1 // Wrap to last
		}
	case DirectionDown:
		newIndex = currentIndex + 1
		if newIndex >= len(stackNode.stackedPanes) {
			newIndex = 0 // Wrap to first
		}
	default:
		return false
	}

	if newIndex == currentIndex {
		return false // No change
	}

	// Update active stack index and visibility
	stackNode.activeStackIndex = newIndex
	wm.stackedPaneManager.UpdateStackVisibility(stackNode)

	// Focus the new active pane
	newActivePane := stackNode.stackedPanes[newIndex]
	wm.SetActivePane(newActivePane, SourceStackNav)

	log.Printf("[workspace] navigated stack: direction=%s from=%d to=%d stackSize=%d",
		direction, currentIndex, newIndex, len(stackNode.stackedPanes))
	return true
}

// focusAdjacent uses geometry-based calculations to find and focus adjacent panes
func (wm *WorkspaceManager) focusAdjacent(direction string) bool {
	currentFocused := wm.GetActiveNode()
	if currentFocused == nil || !currentFocused.isLeaf || currentFocused.container == nil {
		return false
	}

	if neighbor := wm.structuralNeighbor(currentFocused, direction); neighbor != nil {
		log.Printf("[workspace] focusAdjacent: using structural neighbor for direction=%s", direction)
		wm.SetActivePane(neighbor, SourceKeyboard)
		return true
	}

	curX, curY, curWidth, curHeight := wm.getNavigationAllocation(currentFocused)
	cx := float64(curX) + float64(curWidth)/2.0
	cy := float64(curY) + float64(curHeight)/2.0
	log.Printf("[workspace] focusAdjacent: current pane center=(%.0f, %.0f) direction=%s", cx, cy, direction)

	leaves := wm.collectLeaves()
	bestScore := math.MaxFloat64
	var best *paneNode
	var debugCandidates []string

	for _, candidate := range leaves {
		if candidate == nil || candidate == currentFocused || candidate.container == nil {
			continue
		}

		x, y, width, height := wm.getNavigationAllocation(candidate)
		tx := float64(x) + float64(width)/2.0
		ty := float64(y) + float64(height)/2.0

		dx := tx - cx
		dy := ty - cy

		var score float64
		switch direction {
		case DirectionLeft:
			if dx >= -focusEpsilon {
				continue
			}
			score = math.Abs(dx)*1000 + math.Abs(dy)
		case DirectionRight:
			if dx <= focusEpsilon {
				continue
			}
			score = math.Abs(dx)*1000 + math.Abs(dy)
		case DirectionUp:
			if dy >= -focusEpsilon {
				debugCandidates = append(debugCandidates, fmt.Sprintf("SKIPPED cand=%p center=(%.0f,%.0f) dy=%.0f (not above)", candidate.container, tx, ty, dy))
				continue
			}
			score = math.Abs(dy)*1000 + math.Abs(dx)
			debugCandidates = append(debugCandidates, fmt.Sprintf("cand=%p center=(%.0f,%.0f) dy=%.0f score=%.0f", candidate.container, tx, ty, dy, score))
		case DirectionDown:
			if dy <= focusEpsilon {
				debugCandidates = append(debugCandidates, fmt.Sprintf("SKIPPED cand=%p center=(%.0f,%.0f) dy=%.0f (not below)", candidate.container, tx, ty, dy))
				continue
			}
			score = math.Abs(dy)*1000 + math.Abs(dx)
			debugCandidates = append(debugCandidates, fmt.Sprintf("cand=%p center=(%.0f,%.0f) dy=%.0f score=%.0f", candidate.container, tx, ty, dy, score))
		}

		if score < bestScore {
			bestScore = score
			best = candidate
		}
	}

	if best != nil {
		bx, by, _, _ := webkit.WidgetGetAllocation(best.container)
		log.Printf("[workspace] focusAdjacent: found best candidate at pos=(%d,%d) for direction=%s", bx, by, direction)
		wm.SetActivePane(best, SourceKeyboard)
		return true
	}

	if len(debugCandidates) > 0 {
		log.Printf("[workspace] focusAdjacent: NO candidate found for direction=%s current=%p center=(%.0f,%.0f) candidates=[%s]", direction, currentFocused.container, cx, cy, strings.Join(debugCandidates, "; "))
	}
	return false
}

// getNavigationAllocation returns window-absolute coordinates for navigation geometric checks.
// Uses ComputeBounds relative to the window root to get actual screen positions.
// For panes inside a stack, returns the stack wrapper's bounds.
// For regular panes, returns the pane's own bounds.
func (wm *WorkspaceManager) getNavigationAllocation(node *paneNode) (x, y, width, height int) {
	if node == nil || node.container == nil {
		return 0, 0, 0, 0
	}

	// If this pane is inside a stack, use the stack wrapper's window bounds
	if node.parent != nil && node.parent.isStacked && node.parent.stackWrapper != nil {
		return webkit.WidgetGetWindowBounds(node.parent.stackWrapper)
	}

	// For regular panes or stack containers themselves, use their own window bounds
	return webkit.WidgetGetWindowBounds(node.container)
}

// structuralNeighbor finds neighbors based on the tree structure rather than geometry
func (wm *WorkspaceManager) structuralNeighbor(node *paneNode, direction string) *paneNode {
	if node == nil || node.container == nil {
		return nil
	}

	refX, refY, refWidth, refHeight := wm.getNavigationAllocation(node)
	cx := float64(refX) + float64(refWidth)/2.0
	cy := float64(refY) + float64(refHeight)/2.0
	axisVertical := direction == DirectionUp || direction == DirectionDown

	log.Printf("[workspace] structuralNeighbor: node=%p pos=(%d,%d) direction=%s", node.container, refX, refY, direction)

	// Start from the node's parent, but skip stack containers for vertical navigation
	// Stack containers are transparent to vertical navigation - we want to navigate
	// from stack to external panes, not within the stack (that's handled by NavigateStack)
	startParent := node.parent
	if startParent != nil && startParent.isStacked && axisVertical {
		log.Printf("[workspace] structuralNeighbor: skipping stack container parent, using stack's parent instead")
		node = startParent // Treat the stack container as the navigation node
		startParent = startParent.parent
	}

	for parent := startParent; parent != nil; parent = parent.parent {
		isLeft := parent.left == node
		isRight := parent.right == node
		log.Printf("[workspace] structuralNeighbor: checking parent orientation=%v isLeft=%v isRight=%v", parent.orientation, isLeft, isRight)

		switch direction {
		case DirectionUp:
			if axisVertical && parent.orientation == gtk.OrientationVertical && parent.right == node {
				log.Printf("[workspace] structuralNeighbor: DirectionUp - we are RIGHT child, looking in LEFT subtree")
				if leaf := wm.closestLeafFromSubtree(parent.left, cx, cy, direction); leaf != nil {
					lx, ly, _, _ := webkit.WidgetGetAllocation(leaf.container)
					log.Printf("[workspace] structuralNeighbor: found leaf at pos=(%d,%d)", lx, ly)
					return leaf
				}
			}
		case DirectionDown:
			if axisVertical && parent.orientation == gtk.OrientationVertical && parent.left == node {
				log.Printf("[workspace] structuralNeighbor: DirectionDown - we are LEFT child, looking in RIGHT subtree")
				if leaf := wm.closestLeafFromSubtree(parent.right, cx, cy, direction); leaf != nil {
					lx, ly, _, _ := wm.getNavigationAllocation(leaf)
					log.Printf("[workspace] structuralNeighbor: found leaf at pos=(%d,%d)", lx, ly)
					return leaf
				} else {
					log.Printf("[workspace] structuralNeighbor: closestLeafFromSubtree returned nil for DirectionDown")
				}
			}
		case DirectionLeft:
			if !axisVertical && parent.orientation == gtk.OrientationHorizontal && parent.right == node {
				if leaf := wm.closestLeafFromSubtree(parent.left, cx, cy, direction); leaf != nil {
					return leaf
				}
			}
		case DirectionRight:
			if !axisVertical && parent.orientation == gtk.OrientationHorizontal && parent.left == node {
				if leaf := wm.closestLeafFromSubtree(parent.right, cx, cy, direction); leaf != nil {
					return leaf
				}
			}
		}
		node = parent
	}
	return nil
}

// closestLeafFromSubtree finds the closest leaf node in a subtree based on direction
func (wm *WorkspaceManager) closestLeafFromSubtree(node *paneNode, cx, cy float64, direction string) *paneNode {
	leaves := wm.collectLeavesFromWithDirection(node, direction)
	log.Printf("[workspace] closestLeafFromSubtree: found %d leaves for direction=%s from cx=%.0f cy=%.0f", len(leaves), direction, cx, cy)
	bestScore := math.MaxFloat64
	var best *paneNode
	for _, leaf := range leaves {
		if leaf == nil || leaf.container == nil {
			continue
		}

		x, y, width, height := wm.getNavigationAllocation(leaf)
		tx := float64(x) + float64(width)/2.0
		ty := float64(y) + float64(height)/2.0
		dx := tx - cx
		dy := ty - cy
		log.Printf("[workspace] closestLeafFromSubtree: leaf=%p pos=(%d,%d) center=(%.0f,%.0f) dx=%.0f dy=%.0f", leaf.container, x, y, tx, ty, dx, dy)
		var score float64
		switch direction {
		case DirectionLeft:
			if dx >= -focusEpsilon {
				log.Printf("[workspace] closestLeafFromSubtree: SKIPPED (dx=%.0f not left)", dx)
				continue
			}
			score = math.Abs(dx)*1000 + math.Abs(dy)
		case DirectionRight:
			if dx <= focusEpsilon {
				log.Printf("[workspace] closestLeafFromSubtree: SKIPPED (dx=%.0f not right)", dx)
				continue
			}
			score = math.Abs(dx)*1000 + math.Abs(dy)
		case DirectionUp:
			if dy >= -focusEpsilon {
				log.Printf("[workspace] closestLeafFromSubtree: SKIPPED (dy=%.0f not above)", dy)
				continue
			}
			score = math.Abs(dy)*1000 + math.Abs(dx)
		case DirectionDown:
			if dy <= focusEpsilon {
				log.Printf("[workspace] closestLeafFromSubtree: SKIPPED (dy=%.0f not below)", dy)
				continue
			}
			score = math.Abs(dy)*1000 + math.Abs(dx)
		default:
			continue
		}
		log.Printf("[workspace] closestLeafFromSubtree: ACCEPTED with score=%.0f", score)
		if score < bestScore {
			bestScore = score
			best = leaf
		}
	}
	if best == nil {
		log.Printf("[workspace] closestLeafFromSubtree: no match found, trying boundaryFallback")
		return wm.boundaryFallback(node, direction)
	}
	log.Printf("[workspace] closestLeafFromSubtree: returning best=%p", best.container)
	return best
}

// boundaryFallback provides fallback navigation when geometric search fails
func (wm *WorkspaceManager) boundaryFallback(node *paneNode, direction string) *paneNode {
	return wm.boundaryFallbackWithDepth(node, direction, 0)
}

// boundaryFallbackWithDepth provides recursive fallback with depth protection
func (wm *WorkspaceManager) boundaryFallbackWithDepth(node *paneNode, direction string, depth int) *paneNode {
	// Prevent infinite recursion - max tree depth should be reasonable
	const maxDepth = 50
	if depth > maxDepth {
		log.Printf("[workspace] boundaryFallback: max depth exceeded, possible tree corruption")
		return nil
	}

	if node == nil {
		return nil
	}
	if node.isLeaf {
		return node
	}
	switch direction {
	case DirectionUp:
		if leaf := wm.boundaryFallbackWithDepth(node.right, direction, depth+1); leaf != nil {
			return leaf
		}
		return wm.boundaryFallbackWithDepth(node.left, direction, depth+1)
	case DirectionDown:
		if leaf := wm.boundaryFallbackWithDepth(node.left, direction, depth+1); leaf != nil {
			return leaf
		}
		return wm.boundaryFallbackWithDepth(node.right, direction, depth+1)
	case DirectionLeft:
		if leaf := wm.boundaryFallbackWithDepth(node.right, direction, depth+1); leaf != nil {
			return leaf
		}
		return wm.boundaryFallbackWithDepth(node.left, direction, depth+1)
	case DirectionRight:
		if leaf := wm.boundaryFallbackWithDepth(node.left, direction, depth+1); leaf != nil {
			return leaf
		}
		return wm.boundaryFallbackWithDepth(node.right, direction, depth+1)
	default:
		return nil
	}
}
