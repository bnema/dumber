// workspace_tree_rebalancer.go - Tree rebalancing for optimal split pane layout
package browser

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
)

// TreeRebalancer provides algorithms for rebalancing the binary tree after operations
type TreeRebalancer struct {
	wm            *WorkspaceManager
	treeValidator *TreeValidator
	maxImbalance  int  // Maximum allowed imbalance before rebalancing
	enabled       bool // Whether rebalancing is enabled
}

// RebalanceOperation represents a tree rebalancing operation
type RebalanceOperation struct {
	Type        RebalanceType
	Node        *paneNode
	Description string
}

// RebalanceType represents different types of rebalancing operations
type RebalanceType int

const (
	RebalanceRotateLeft RebalanceType = iota
	RebalanceRotateRight
	RebalancePromote
	RebalanceRestructure
)

func (rt RebalanceType) String() string {
	switch rt {
	case RebalanceRotateLeft:
		return "rotate_left"
	case RebalanceRotateRight:
		return "rotate_right"
	case RebalancePromote:
		return "promote"
	case RebalanceRestructure:
		return "restructure"
	default:
		return "unknown"
	}
}

// TreeMetrics contains metrics about the tree structure
type TreeMetrics struct {
	Height         int
	NodeCount      int
	LeafCount      int
	BalanceFactor  int
	MaxDepth       int
	AvgDepth       float64
	ImbalanceScore int
}

// NewTreeRebalancer creates a new tree rebalancer
func NewTreeRebalancer(wm *WorkspaceManager, treeValidator *TreeValidator) *TreeRebalancer {
	return &TreeRebalancer{
		wm:            wm,
		treeValidator: treeValidator,
		maxImbalance:  2, // Allow up to 2 levels of imbalance
		enabled:       true,
	}
}

// SetMaxImbalance sets the maximum allowed imbalance
func (tr *TreeRebalancer) SetMaxImbalance(maxImbalance int) {
	tr.maxImbalance = maxImbalance
}

// Enable enables tree rebalancing
func (tr *TreeRebalancer) Enable() {
	tr.enabled = true
	log.Printf("[tree-rebalancer] Tree rebalancing enabled")
}

// Disable disables tree rebalancing
func (tr *TreeRebalancer) Disable() {
	tr.enabled = false
	log.Printf("[tree-rebalancer] Tree rebalancing disabled")
}

// RebalanceAfterClose performs tree rebalancing after a pane close operation
func (tr *TreeRebalancer) RebalanceAfterClose(closedNode *paneNode, promotedNode *paneNode) error {
	if !tr.enabled {
		return nil
	}

	log.Printf("[tree-rebalancer] Analyzing tree for rebalancing after close operation")

	if promotedNode != nil {
		if err := tr.executePromotion(promotedNode); err != nil {
			return fmt.Errorf("promotion execution failed: %w", err)
		}

		if tr.wm != nil && tr.wm.geometryValidator != nil {
			tr.logInitialPromotionGeometry(promotedNode)
			tr.schedulePromotionValidation(promotedNode)
		}
	}

	// Calculate tree metrics
	metrics := tr.CalculateTreeMetrics(tr.wm.root)
	log.Printf("[tree-rebalancer] Tree metrics: height=%d, nodes=%d, balance=%d, imbalance=%d",
		metrics.Height, metrics.NodeCount, metrics.BalanceFactor, metrics.ImbalanceScore)

	// Check if rebalancing is needed
	if metrics.ImbalanceScore <= tr.maxImbalance {
		log.Printf("[tree-rebalancer] Tree is balanced, no rebalancing needed")
		return nil
	}

	// Find rebalancing operations
	operations := tr.findRebalancingOperations(tr.wm.root, metrics)
	if len(operations) == 0 {
		log.Printf("[tree-rebalancer] No rebalancing operations found")
		return nil
	}

	log.Printf("[tree-rebalancer] Found %d rebalancing operations", len(operations))

	// Execute rebalancing operations
	return tr.executeRebalancingOperations(operations)
}

// CalculateTreeMetrics calculates comprehensive metrics about the tree
func (tr *TreeRebalancer) CalculateTreeMetrics(root *paneNode) TreeMetrics {
	if root == nil {
		return TreeMetrics{}
	}

	metrics := TreeMetrics{}

	// Calculate basic metrics
	metrics.Height = tr.calculateHeight(root)
	metrics.NodeCount = tr.countNodes(root)
	metrics.LeafCount = tr.countLeaves(root)
	metrics.MaxDepth = tr.calculateMaxDepth(root)
	metrics.AvgDepth = tr.calculateAverageDepth(root)

	// Calculate balance factor
	leftHeight := tr.calculateHeight(root.left)
	rightHeight := tr.calculateHeight(root.right)
	metrics.BalanceFactor = rightHeight - leftHeight

	// Calculate imbalance score (how far from balanced the tree is)
	metrics.ImbalanceScore = tr.calculateImbalanceScore(root)

	return metrics
}

// calculateHeight calculates the height of a subtree
func (tr *TreeRebalancer) calculateHeight(node *paneNode) int {
	if node == nil {
		return 0
	}

	// Skip stacked nodes for height calculation (they don't affect layout balance)
	if node.isStacked {
		// For stacked nodes, height is determined by the active pane
		if len(node.stackedPanes) > 0 && node.activeStackIndex >= 0 && node.activeStackIndex < len(node.stackedPanes) {
			return tr.calculateHeight(node.stackedPanes[node.activeStackIndex])
		}
		return 1
	}

	if node.isLeaf {
		return 1
	}

	leftHeight := tr.calculateHeight(node.left)
	rightHeight := tr.calculateHeight(node.right)
	return 1 + max(leftHeight, rightHeight)
}

// countNodes counts the total number of nodes in the tree
func (tr *TreeRebalancer) countNodes(node *paneNode) int {
	if node == nil {
		return 0
	}

	count := 1
	if node.isStacked {
		// Count stacked panes
		count += len(node.stackedPanes)
	} else {
		count += tr.countNodes(node.left)
		count += tr.countNodes(node.right)
	}

	return count
}

// countLeaves counts the number of leaf nodes
func (tr *TreeRebalancer) countLeaves(node *paneNode) int {
	if node == nil {
		return 0
	}

	if node.isLeaf {
		return 1
	}

	if node.isStacked {
		return len(node.stackedPanes)
	}

	return tr.countLeaves(node.left) + tr.countLeaves(node.right)
}

// calculateMaxDepth calculates the maximum depth of any leaf
func (tr *TreeRebalancer) calculateMaxDepth(node *paneNode) int {
	return tr.calculateMaxDepthHelper(node, 0)
}

func (tr *TreeRebalancer) calculateMaxDepthHelper(node *paneNode, currentDepth int) int {
	if node == nil {
		return currentDepth
	}

	if node.isLeaf || node.isStacked {
		return currentDepth + 1
	}

	leftDepth := tr.calculateMaxDepthHelper(node.left, currentDepth+1)
	rightDepth := tr.calculateMaxDepthHelper(node.right, currentDepth+1)
	return max(leftDepth, rightDepth)
}

// calculateAverageDepth calculates the average depth of all leaves
func (tr *TreeRebalancer) calculateAverageDepth(node *paneNode) float64 {
	if node == nil {
		return 0
	}

	totalDepth, leafCount := tr.calculateTotalDepthAndCount(node, 0)
	if leafCount == 0 {
		return 0
	}

	return float64(totalDepth) / float64(leafCount)
}

func (tr *TreeRebalancer) calculateTotalDepthAndCount(node *paneNode, currentDepth int) (int, int) {
	if node == nil {
		return 0, 0
	}

	if node.isLeaf {
		return currentDepth + 1, 1
	}

	if node.isStacked {
		// All stacked panes have the same depth
		return (currentDepth + 1) * len(node.stackedPanes), len(node.stackedPanes)
	}

	leftDepth, leftCount := tr.calculateTotalDepthAndCount(node.left, currentDepth+1)
	rightDepth, rightCount := tr.calculateTotalDepthAndCount(node.right, currentDepth+1)

	return leftDepth + rightDepth, leftCount + rightCount
}

// calculateImbalanceScore calculates how imbalanced the tree is
func (tr *TreeRebalancer) calculateImbalanceScore(node *paneNode) int {
	if node == nil {
		return 0
	}

	leftHeight := tr.calculateHeight(node.left)
	rightHeight := tr.calculateHeight(node.right)
	currentImbalance := int(math.Abs(float64(leftHeight - rightHeight)))

	// Recursively calculate imbalance for subtrees
	leftImbalance := tr.calculateImbalanceScore(node.left)
	rightImbalance := tr.calculateImbalanceScore(node.right)

	return max(currentImbalance, max(leftImbalance, rightImbalance))
}

// findRebalancingOperations identifies operations needed to rebalance the tree
func (tr *TreeRebalancer) findRebalancingOperations(root *paneNode, metrics TreeMetrics) []RebalanceOperation {
	var operations []RebalanceOperation

	// Find all imbalanced nodes
	imbalancedNodes := tr.findImbalancedNodes(root)

	for _, node := range imbalancedNodes {
		// Determine the best rebalancing operation for this node
		operation := tr.determineRebalancingOperation(node)
		if operation != nil {
			operations = append(operations, *operation)
		}
	}

	return operations
}

// findImbalancedNodes finds all nodes that are significantly imbalanced
func (tr *TreeRebalancer) findImbalancedNodes(node *paneNode) []*paneNode {
	if node == nil {
		return nil
	}

	var imbalanced []*paneNode

	leftHeight := tr.calculateHeight(node.left)
	rightHeight := tr.calculateHeight(node.right)
	balance := int(math.Abs(float64(leftHeight - rightHeight)))

	if balance > tr.maxImbalance {
		imbalanced = append(imbalanced, node)
	}

	// Recursively check children
	imbalanced = append(imbalanced, tr.findImbalancedNodes(node.left)...)
	imbalanced = append(imbalanced, tr.findImbalancedNodes(node.right)...)

	return imbalanced
}

// determineRebalancingOperation determines the best rebalancing operation for a node
func (tr *TreeRebalancer) determineRebalancingOperation(node *paneNode) *RebalanceOperation {
	if node == nil || node.isLeaf {
		return nil
	}

	leftHeight := tr.calculateHeight(node.left)
	rightHeight := tr.calculateHeight(node.right)

	if leftHeight > rightHeight+tr.maxImbalance {
		// Left-heavy, need right rotation
		return &RebalanceOperation{
			Type:        RebalanceRotateRight,
			Node:        node,
			Description: fmt.Sprintf("Right rotation on node %p (left=%d, right=%d)", node, leftHeight, rightHeight),
		}
	} else if rightHeight > leftHeight+tr.maxImbalance {
		// Right-heavy, need left rotation
		return &RebalanceOperation{
			Type:        RebalanceRotateLeft,
			Node:        node,
			Description: fmt.Sprintf("Left rotation on node %p (left=%d, right=%d)", node, leftHeight, rightHeight),
		}
	}

	return nil
}

// executeRebalancingOperations executes a series of rebalancing operations
func (tr *TreeRebalancer) executeRebalancingOperations(operations []RebalanceOperation) error {
	if len(operations) == 0 {
		return nil
	}

	log.Printf("[tree-rebalancer] Executing %d rebalancing operations", len(operations))

	// Execute each operation
	for i, operation := range operations {
		log.Printf("[tree-rebalancer] Executing operation %d/%d: %s", i+1, len(operations), operation.Description)

		var err error
		switch operation.Type {
		case RebalanceRotateLeft:
			err = tr.executeLeftRotation(operation.Node)
		case RebalanceRotateRight:
			err = tr.executeRightRotation(operation.Node)
		case RebalancePromote:
			err = tr.executePromotion(operation.Node)
		case RebalanceRestructure:
			err = tr.executeRestructure(operation.Node)
		default:
			err = fmt.Errorf("unknown rebalance operation type: %s", operation.Type)
		}

		if err != nil {
			log.Printf("[tree-rebalancer] Operation failed: %v", err)
			return fmt.Errorf("rebalancing operation failed: %w", err)
		}
	}

	// Validate tree after rebalancing
	if tr.treeValidator != nil {
		if err := tr.treeValidator.ValidateTree(tr.wm.root, "rebalance"); err != nil {
			log.Printf("[tree-rebalancer] Tree validation failed after rebalancing: %v", err)
		}
	}

	// Calculate new metrics
	newMetrics := tr.CalculateTreeMetrics(tr.wm.root)
	log.Printf("[tree-rebalancer] Rebalancing completed: new imbalance=%d",
		newMetrics.ImbalanceScore)

	return nil
}

// executeLeftRotation performs a left rotation on the given node
func (tr *TreeRebalancer) executeLeftRotation(node *paneNode) error {
	if node == nil || node.right == nil {
		return fmt.Errorf("cannot perform left rotation: invalid node structure")
	}

	log.Printf("[tree-rebalancer] Performing left rotation on node %p", node)

	// Store original structure
	rightChild := node.right
	rightLeftChild := rightChild.left
	parent := node.parent

	// Update tree structure
	rightChild.left = node
	node.right = rightLeftChild
	rightChild.parent = parent
	node.parent = rightChild

	if rightLeftChild != nil {
		rightLeftChild.parent = node
	}

	// Update parent's child pointer or root
	if parent == nil {
		tr.wm.root = rightChild
	} else if parent.left == node {
		parent.left = rightChild
	} else {
		parent.right = rightChild
	}

	// Update widget hierarchy for the rotation
	return tr.updateRotationWidgets(node, rightChild, "left")
}

// executeRightRotation performs a right rotation on the given node
func (tr *TreeRebalancer) executeRightRotation(node *paneNode) error {
	if node == nil || node.left == nil {
		return fmt.Errorf("cannot perform right rotation: invalid node structure")
	}

	log.Printf("[tree-rebalancer] Performing right rotation on node %p", node)

	// Store original structure
	leftChild := node.left
	leftRightChild := leftChild.right
	parent := node.parent

	// Update tree structure
	leftChild.right = node
	node.left = leftRightChild
	leftChild.parent = parent
	node.parent = leftChild

	if leftRightChild != nil {
		leftRightChild.parent = node
	}

	// Update parent's child pointer or root
	if parent == nil {
		tr.wm.root = leftChild
	} else if parent.left == node {
		parent.left = leftChild
	} else {
		parent.right = leftChild
	}

	// Update widget hierarchy for the rotation
	return tr.updateRotationWidgets(node, leftChild, "right")
}

// updateRotationWidgets updates widget hierarchy after tree rotation
func (tr *TreeRebalancer) updateRotationWidgets(oldRoot, newRoot *paneNode, rotationType string) error {
	// The widget hierarchy needs to be updated to match the new tree structure
	// This is complex because GTK widgets need to be reparented correctly

	if oldRoot.container == nil || newRoot.container == nil {
		return fmt.Errorf("rotation nodes missing containers")
	}

	log.Printf("[tree-rebalancer] Updating widget hierarchy for %s rotation", rotationType)

	// Force a layout update
	if newRoot.container != nil {
		return newRoot.container.Execute(func(ptr uintptr) error {
			if ptr == 0 || !webkit.WidgetIsValid(ptr) {
				return fmt.Errorf("rotation widget update: invalid widget pointer")
			}
			webkit.WidgetQueueAllocate(ptr)
			return nil
		})
	}
	return nil
}

// executePromotion promotes a child node to replace its parent
func (tr *TreeRebalancer) executePromotion(node *paneNode) error {
	if node == nil {
		return fmt.Errorf("promotion target is nil")
	}
	if node.container == nil {
		return fmt.Errorf("promotion target missing container")
	}
	if !node.container.IsValid() {
		return fmt.Errorf("promotion target container %s invalid", node.container.String())
	}

	log.Printf("[tree-rebalancer] Promoting node %p (parent=%p)", node, node.parent)
	promotionStart := time.Now()

	// Ensure the promoted widget can expand to occupy available space
	log.Printf("[tree-rebalancer] promotion expand for %p executing at +%s", node, time.Since(promotionStart).Round(time.Millisecond))
	if err := node.container.Execute(func(ptr uintptr) error {
		if ptr == 0 || !webkit.WidgetIsValid(ptr) {
			return fmt.Errorf("promotion expand: invalid widget pointer")
		}
		webkit.WidgetResetSizeRequest(ptr)
		webkit.WidgetSetHExpand(ptr, true)
		webkit.WidgetSetVExpand(ptr, true)
		webkit.WidgetQueueAllocate(ptr)
		return nil
	}); err != nil {
		return fmt.Errorf("failed to expand promoted widget: %w", err)
	}

	parent := node.parent
	if parent == nil {
		// Attach to window root
		if tr.wm != nil && tr.wm.window != nil {
			log.Printf("[tree-rebalancer] promotion window attach for %p executing at +%s", node, time.Since(promotionStart).Round(time.Millisecond))
			if err := node.container.Execute(func(ptr uintptr) error {
				if ptr == 0 || !webkit.WidgetIsValid(ptr) {
					return fmt.Errorf("promotion attach window: invalid widget pointer")
				}

				currentParent := webkit.WidgetGetParent(ptr)
				if currentParent != 0 {
					// Check if parent is a container widget (paned or box) that we need to unparent from
					if webkit.IsPaned(currentParent) {
						log.Printf("[tree-rebalancer] unparenting widget from paned %#x", currentParent)
						webkit.WidgetUnparent(ptr)
					} else if webkit.IsBox(currentParent) {
						log.Printf("[tree-rebalancer] unparenting widget from box (stack) %#x", currentParent)
						webkit.WidgetUnparent(ptr)
					} else {
						// Parent is likely a window or other non-container widget
						// For safety, check if it's a toplevel window by attempting unparent
						log.Printf("[tree-rebalancer] widget %#x has non-container parent %#x, checking if unparent needed", ptr, currentParent)
						if webkit.WidgetIsValid(currentParent) {
							// If parent is valid but not a container, try unparent for safety
							webkit.WidgetUnparent(ptr)
							log.Printf("[tree-rebalancer] unparented widget %#x from non-container parent %#x", ptr, currentParent)
						}
					}
				}

				// Ensure widget is configured for window child
				webkit.WidgetSetHExpand(ptr, true)
				webkit.WidgetSetVExpand(ptr, true)

				// Set as window child
				tr.wm.window.SetChild(ptr)
				webkit.WidgetQueueAllocate(ptr)
				webkit.WidgetShow(ptr)

				// Verify the attachment worked
				finalParent := webkit.WidgetGetParent(ptr)
				if finalParent == 0 {
					log.Printf("[tree-rebalancer] WARNING: SetChild failed, widget still has no parent")
					// Try one more time
					tr.wm.window.SetChild(ptr)
					webkit.WidgetQueueAllocate(ptr)
					finalParent = webkit.WidgetGetParent(ptr)
					if finalParent == 0 {
						return fmt.Errorf("failed to attach widget %#x to window after multiple attempts", ptr)
					}
				}

				log.Printf("[tree-rebalancer] promotion window attach successful: widget %#x now has parent %#x", ptr, finalParent)
				return nil
			}); err != nil {
				return fmt.Errorf("failed to attach widget to window: %w", err)
			}
		}
	} else {
		// Attach to parent container
		if parent.container == nil {
			return fmt.Errorf("promotion parent missing container")
		}
		if !parent.container.IsValid() {
			return fmt.Errorf("promotion parent container %s invalid", parent.container.String())
		}

		log.Printf("[tree-rebalancer] promotion reparent for %p executing at +%s", node, time.Since(promotionStart).Round(time.Millisecond))
		if err := parent.container.Execute(func(parentPtr uintptr) error {
			if parentPtr == 0 || !webkit.WidgetIsValid(parentPtr) {
				return fmt.Errorf("promotion reparent: invalid parent widget")
			}
			return node.container.Execute(func(childPtr uintptr) error {
				if childPtr == 0 || !webkit.WidgetIsValid(childPtr) {
					return fmt.Errorf("promotion reparent: invalid child widget")
				}

				// Check if widget is already correctly parented (closePane.swapContainers already did the work)
				currentParent := webkit.WidgetGetParent(childPtr)
				if currentParent == parentPtr {
					log.Printf("[tree-rebalancer] widget %#x already correctly parented to %#x, skipping reparent", childPtr, parentPtr)
					// Just ensure properties are set correctly
					webkit.WidgetResetSizeRequest(childPtr)
					webkit.WidgetSetHExpand(childPtr, true)
					webkit.WidgetSetVExpand(childPtr, true)
					webkit.WidgetQueueAllocate(childPtr)
					webkit.WidgetQueueAllocate(parentPtr)
					return nil
				}

				// Widget needs reparenting - unparent from wrong parent if needed
				if currentParent != 0 {
					log.Printf("[tree-rebalancer] unparenting widget %#x from incorrect parent %#x", childPtr, currentParent)
					webkit.WidgetUnparent(childPtr)
				}

				// Reattach promoted widget to the GtkPaned
				if parent.left == node {
					webkit.PanedSetStartChild(parentPtr, childPtr)
				} else {
					webkit.PanedSetEndChild(parentPtr, childPtr)
				}

				// Verify attachment succeeded before setting properties
				finalParent := webkit.WidgetGetParent(childPtr)
				if finalParent != parentPtr {
					return fmt.Errorf("failed to attach widget %#x to parent %#x (current parent: %#x)", childPtr, parentPtr, finalParent)
				}

				webkit.WidgetResetSizeRequest(childPtr)
				webkit.WidgetSetHExpand(childPtr, true)
				webkit.WidgetSetVExpand(childPtr, true)
				webkit.WidgetQueueAllocate(childPtr)
				webkit.WidgetQueueAllocate(parentPtr)
				return nil
			})
		}); err != nil {
			return fmt.Errorf("failed to reparent promoted widget: %w", err)
		}
	}

	// Propagate allocation updates up the ancestor chain so GTK recalculates sizes
	ancestorPtrs := tr.collectAncestorContainers(node)
	if len(ancestorPtrs) > 0 {
		log.Printf("[tree-rebalancer] promotion allocation queue for %p executing at +%s (ancestors=%d)", node, time.Since(promotionStart).Round(time.Millisecond), len(ancestorPtrs))
		for _, ancestorPtr := range ancestorPtrs {
			if ancestorPtr == 0 || !webkit.WidgetIsValid(ancestorPtr) {
				continue
			}
			webkit.WidgetQueueAllocate(ancestorPtr)
		}
	}

	// Validate allocation after promotion for debugging purposes
	log.Printf("[tree-rebalancer] promotion immediate validation for %p executing at +%s", node, time.Since(promotionStart).Round(time.Millisecond))
	_ = node.container.Execute(func(ptr uintptr) error {
		if ptr == 0 || !webkit.WidgetIsValid(ptr) {
			log.Printf("[tree-rebalancer] promotion validate: invalid widget pointer")
			return nil
		}
		bounds, ok := webkit.WidgetGetBounds(ptr)
		if !ok {
			log.Printf("[tree-rebalancer] promotion validation: failed to read bounds for %#x", ptr)
			return nil
		}
		if bounds.Width <= 1 || bounds.Height <= 1 {
			log.Printf("[tree-rebalancer] WARNING: promoted pane %#x has tiny allocation %.1fx%.1f", ptr, bounds.Width, bounds.Height)
		} else {
			log.Printf("[tree-rebalancer] Promotion allocation verified for %#x: %.1fx%.1f", ptr, bounds.Width, bounds.Height)
		}
		return nil
	})

	return nil
}

// executeRestructure performs a complete restructure of a subtree
func (tr *TreeRebalancer) executeRestructure(node *paneNode) error {
	// This would be used for more complex rebalancing scenarios
	log.Printf("[tree-rebalancer] Restructuring subtree at node %p", node)

	// TODO: Implement restructure logic
	return fmt.Errorf("restructure operation not yet implemented")
}

func (tr *TreeRebalancer) logInitialPromotionGeometry(node *paneNode) {
	if tr == nil || tr.wm == nil || tr.wm.geometryValidator == nil || node == nil {
		return
	}
	if node.container == nil || !node.container.IsValid() {
		log.Printf("[tree-rebalancer] Promotion geometry skipped: container invalid for %p", node)
		return
	}
	tr.ensureRootAttachment(node)
	geom := tr.wm.geometryValidator.GetPaneGeometry(node)
	if !geom.IsValid || geom.Width <= 0 || geom.Height <= 0 {
		log.Printf("[tree-rebalancer] Promotion geometry pending after commit: valid=%v size=%dx%d", geom.IsValid, geom.Width, geom.Height)
		return
	}
	log.Printf("[tree-rebalancer] Promotion geometry validated immediately: %dx%d", geom.Width, geom.Height)
}

func (tr *TreeRebalancer) schedulePromotionValidation(node *paneNode) {
	if tr == nil || tr.wm == nil || tr.wm.geometryValidator == nil || node == nil {
		return
	}

	const maxAttempts = 5
	start := time.Now()
	attempt := 0

	var retry func() bool
	retry = func() bool {
		attempt++
		elapsed := time.Since(start).Round(time.Millisecond)
		if node.container == nil || !node.container.IsValid() {
			log.Printf("[tree-rebalancer] Deferred promotion geometry aborted after %s (attempt %d): container invalid", elapsed, attempt)
			return false
		}
		tr.ensureRootAttachment(node)
		geom := tr.wm.geometryValidator.GetPaneGeometry(node)
		if geom.IsValid && geom.Width > 0 && geom.Height > 0 {
			log.Printf("[tree-rebalancer] Deferred promotion geometry validated after %s (attempt %d): %dx%d", elapsed, attempt, geom.Width, geom.Height)
			return false
		}

		if attempt >= maxAttempts {
			log.Printf("[tree-rebalancer] Deferred promotion geometry still invalid after %s (attempt %d): valid=%v size=%dx%d", elapsed, attempt, geom.IsValid, geom.Width, geom.Height)
			return false
		}

		log.Printf("[tree-rebalancer] Promotion geometry still pending after %s (attempt %d); rescheduling", elapsed, attempt)
		webkit.IdleAdd(retry)
		return false
	}

	webkit.IdleAdd(retry)
}

func (tr *TreeRebalancer) ensureRootAttachment(node *paneNode) {
	if tr == nil || tr.wm == nil || node == nil || node.parent != nil {
		return
	}
	if node.container == nil || !node.container.IsValid() {
		return
	}

	_ = node.container.Execute(func(ptr uintptr) error {
		parent := webkit.WidgetGetParent(ptr)
		if parent != 0 {
			// Widget is properly attached, no action needed
			return nil
		}
		if tr.wm.window == nil {
			log.Printf("[tree-rebalancer] Root pane %#x missing parent but window unavailable", ptr)
			return nil
		}

		// This should not happen if promotion worked correctly - log as error
		log.Printf("[tree-rebalancer] ERROR: Root pane %#x lost window attachment (this indicates a promotion bug); attempting recovery", ptr)

		// Recovery attempt
		tr.wm.window.SetChild(ptr)
		webkit.WidgetQueueAllocate(ptr)
		webkit.WidgetShow(ptr)

		// Verify recovery
		recoveredParent := webkit.WidgetGetParent(ptr)
		if recoveredParent == 0 {
			log.Printf("[tree-rebalancer] CRITICAL: Failed to recover window attachment for %#x", ptr)
		} else {
			log.Printf("[tree-rebalancer] Recovery successful: widget %#x now has parent %#x", ptr, recoveredParent)
		}
		return nil
	})
}

// collectAncestorContainers returns the widget pointers for the promoted node and all ancestors up to the root.
func (tr *TreeRebalancer) collectAncestorContainers(node *paneNode) []uintptr {
	var containers []uintptr
	visited := make(map[uintptr]bool)

	current := node
	for current != nil {
		if current.container != nil && current.container.IsValid() {
			ptr := current.container.Ptr()
			if ptr != 0 && !visited[ptr] && webkit.WidgetIsValid(ptr) {
				containers = append(containers, ptr)
				visited[ptr] = true
			}
		}
		current = current.parent
	}

	return containers
}

// GetRebalancingStats returns statistics about tree rebalancing
func (tr *TreeRebalancer) GetRebalancingStats() map[string]interface{} {
	metrics := tr.CalculateTreeMetrics(tr.wm.root)

	return map[string]interface{}{
		"enabled":         tr.enabled,
		"max_imbalance":   tr.maxImbalance,
		"current_height":  metrics.Height,
		"node_count":      metrics.NodeCount,
		"leaf_count":      metrics.LeafCount,
		"balance_factor":  metrics.BalanceFactor,
		"imbalance_score": metrics.ImbalanceScore,
		"avg_depth":       metrics.AvgDepth,
		"needs_rebalance": metrics.ImbalanceScore > tr.maxImbalance,
	}
}

// Helper functions
