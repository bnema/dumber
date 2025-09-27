// workspace_navigation.go - Navigation and focus management for workspace panes
package browser

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
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
		log.Printf("[workspace] dispatched focus event for webview %s (active=%v)", node.pane.webView.ID(), active)
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
	const focusThrottleInterval = 100 * time.Millisecond
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

// ensureHover sets up hover handlers for a pane node
func (wm *WorkspaceManager) ensureHover(node *paneNode) {
	if wm == nil || node == nil || !node.isLeaf {
		return
	}
	if node.container == nil || node.hoverToken != 0 {
		return
	}

	var token uintptr
	node.container.Execute(func(containerPtr uintptr) error {
		token = webkit.WidgetAddHoverHandler(containerPtr, func() {
			if wm == nil {
				return
			}
			wm.SetActivePane(node, SourceMouse)
		})
		return nil
	})
	node.hoverToken = token
	if token == 0 {
		log.Printf("[workspace] failed to attach hover handler")
	}
}

// detachHover removes hover handlers from a pane node
func (wm *WorkspaceManager) detachHover(node *paneNode) {
	if wm == nil || node == nil || node.hoverToken == 0 {
		return
	}
	node.container.Execute(func(containerPtr uintptr) error {
		webkit.WidgetRemoveHoverHandler(containerPtr, node.hoverToken)
		return nil
	})
	node.hoverToken = 0
}

// FocusNeighbor moves focus to the nearest pane in the requested direction using the
// actual widget geometry to determine adjacency. For stacked panes, "up" and "down"
// navigate within the stack.
func (wm *WorkspaceManager) FocusNeighbor(direction string) bool {
	if wm == nil {
		return false
	}
	switch strings.ToLower(direction) {
	case "up", "down":
		// Check if current pane is part of a stack and handle stack navigation
		if wm.stackedPaneManager.NavigateStack(strings.ToLower(direction)) {
			return true
		}
		// Fall back to regular adjacency navigation
		return wm.focusAdjacent(strings.ToLower(direction))
	case "left", "right":
		return wm.focusAdjacent(strings.ToLower(direction))
	default:
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
	case "up":
		newIndex = currentIndex - 1
		if newIndex < 0 {
			newIndex = len(stackNode.stackedPanes) - 1 // Wrap to last
		}
	case "down":
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
		wm.SetActivePane(neighbor, SourceKeyboard)
		return true
	}

	var currentBounds webkit.WidgetBounds
	var ok bool
	currentFocused.container.Execute(func(containerPtr uintptr) error {
		currentBounds, ok = webkit.WidgetGetBounds(containerPtr)
		return nil
	})
	if !ok {
		log.Printf("[workspace] unable to compute bounds for active pane")
		return false
	}

	cx := currentBounds.X + currentBounds.Width/2.0
	cy := currentBounds.Y + currentBounds.Height/2.0

	leaves := wm.collectLeaves()
	bestScore := math.MaxFloat64
	var best *paneNode
	var debugCandidates []string

	for _, candidate := range leaves {
		if candidate == nil || candidate == currentFocused || candidate.container == nil {
			continue
		}
		var bounds webkit.WidgetBounds
		var boundsOk bool
		candidate.container.Execute(func(containerPtr uintptr) error {
			bounds, boundsOk = webkit.WidgetGetBounds(containerPtr)
			return nil
		})
		ok := boundsOk
		if !ok {
			continue
		}
		tx := bounds.X + bounds.Width/2.0
		ty := bounds.Y + bounds.Height/2.0

		dx := tx - cx
		dy := ty - cy

		var score float64
		switch direction {
		case "left":
			if dx >= -focusEpsilon {
				continue
			}
			score = math.Abs(dx)*1000 + math.Abs(dy)
		case "right":
			if dx <= focusEpsilon {
				continue
			}
			score = math.Abs(dx)*1000 + math.Abs(dy)
		case "up":
			if dy >= -focusEpsilon {
				continue
			}
			score = math.Abs(dy)*1000 + math.Abs(dx)
		case "down":
			if dy <= focusEpsilon {
				continue
			}
			score = math.Abs(dy)*1000 + math.Abs(dx)
		}

		if direction == "up" || direction == "down" {
			debugCandidates = append(debugCandidates, fmt.Sprintf("cand=%#x dx=%.2f dy=%.2f score=%.2f", candidate.container, dx, dy, score))
		}

		if score < bestScore {
			bestScore = score
			best = candidate
		}
	}

	if best != nil {
		wm.SetActivePane(best, SourceKeyboard)
		return true
	}

	if len(debugCandidates) > 0 {
		log.Printf("[workspace] focusAdjacent no candidate direction=%s current=%#x candidates=%s", direction, currentFocused.container, strings.Join(debugCandidates, "; "))
	}
	return false
}

// structuralNeighbor finds neighbors based on the tree structure rather than geometry
func (wm *WorkspaceManager) structuralNeighbor(node *paneNode, direction string) *paneNode {
	if node == nil || node.container == nil {
		return nil
	}

	var refBounds webkit.WidgetBounds
	var ok bool
	node.container.Execute(func(containerPtr uintptr) error {
		refBounds, ok = webkit.WidgetGetBounds(containerPtr)
		return nil
	})
	if !ok {
		return nil
	}
	cx := refBounds.X + refBounds.Width/2.0
	cy := refBounds.Y + refBounds.Height/2.0
	axisVertical := direction == "up" || direction == "down"

	for parent := node.parent; parent != nil; parent = parent.parent {
		switch direction {
		case "up":
			if axisVertical && parent.orientation == webkit.OrientationVertical && parent.right == node {
				if leaf := wm.closestLeafFromSubtree(parent.left, cx, cy, direction); leaf != nil {
					return leaf
				}
			}
		case "down":
			if axisVertical && parent.orientation == webkit.OrientationVertical && parent.left == node {
				if leaf := wm.closestLeafFromSubtree(parent.right, cx, cy, direction); leaf != nil {
					return leaf
				}
			}
		case "left":
			if !axisVertical && parent.orientation == webkit.OrientationHorizontal && parent.right == node {
				if leaf := wm.closestLeafFromSubtree(parent.left, cx, cy, direction); leaf != nil {
					return leaf
				}
			}
		case "right":
			if !axisVertical && parent.orientation == webkit.OrientationHorizontal && parent.left == node {
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
	leaves := wm.collectLeavesFrom(node)
	bestScore := math.MaxFloat64
	var best *paneNode
	for _, leaf := range leaves {
		if leaf == nil || leaf.container == nil {
			continue
		}
		var bounds webkit.WidgetBounds
		var boundsOk bool
		leaf.container.Execute(func(containerPtr uintptr) error {
			bounds, boundsOk = webkit.WidgetGetBounds(containerPtr)
			return nil
		})
		ok := boundsOk
		if !ok {
			continue
		}
		tx := bounds.X + bounds.Width/2.0
		ty := bounds.Y + bounds.Height/2.0
		dx := tx - cx
		dy := ty - cy
		var score float64
		switch direction {
		case "left":
			if dx >= -focusEpsilon {
				continue
			}
			score = math.Abs(dx)*1000 + math.Abs(dy)
		case "right":
			if dx <= focusEpsilon {
				continue
			}
			score = math.Abs(dx)*1000 + math.Abs(dy)
		case "up":
			if dy >= -focusEpsilon {
				continue
			}
			score = math.Abs(dy)*1000 + math.Abs(dx)
		case "down":
			if dy <= focusEpsilon {
				continue
			}
			score = math.Abs(dy)*1000 + math.Abs(dx)
		default:
			continue
		}
		if score < bestScore {
			bestScore = score
			best = leaf
		}
	}
	if best == nil {
		return wm.boundaryFallback(node, direction)
	}
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
	case "up":
		if leaf := wm.boundaryFallbackWithDepth(node.right, direction, depth+1); leaf != nil {
			return leaf
		}
		return wm.boundaryFallbackWithDepth(node.left, direction, depth+1)
	case "down":
		if leaf := wm.boundaryFallbackWithDepth(node.left, direction, depth+1); leaf != nil {
			return leaf
		}
		return wm.boundaryFallbackWithDepth(node.right, direction, depth+1)
	case "left":
		if leaf := wm.boundaryFallbackWithDepth(node.right, direction, depth+1); leaf != nil {
			return leaf
		}
		return wm.boundaryFallbackWithDepth(node.left, direction, depth+1)
	case "right":
		if leaf := wm.boundaryFallbackWithDepth(node.left, direction, depth+1); leaf != nil {
			return leaf
		}
		return wm.boundaryFallbackWithDepth(node.right, direction, depth+1)
	default:
		return nil
	}
}
