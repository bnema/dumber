// workspace_tree_validator.go - Tree invariant validation for bulletproof binary tree operations
package browser

import (
	"fmt"
	"log"
	"sync"
	"time"
	"unsafe"
)

// TreeValidationError represents errors found during tree validation
type TreeValidationError struct {
	ErrorType   string
	NodePointer uintptr
	Message     string
	StackTrace  []string
}

func (e TreeValidationError) Error() string {
	return fmt.Sprintf("Tree validation error [%s]: %s (node: %p)", e.ErrorType, e.Message, e.NodePointer)
}

// TreeValidator provides comprehensive validation of binary tree invariants
type TreeValidator struct {
	enabled           bool
	debugMode         bool
	maxDepth          int
	validationHistory []TreeValidationResult
	mu                sync.RWMutex
}

// TreeValidationResult captures the result of a tree validation
type TreeValidationResult struct {
	Timestamp     time.Time
	Success       bool
	ErrorCount    int
	Errors        []TreeValidationError
	NodeCount     int
	MaxDepth      int
	BalanceFactor int
	Duration      time.Duration
}

// NewTreeValidator creates a new tree validator
func NewTreeValidator(enabled, debugMode bool) *TreeValidator {
	return &TreeValidator{
		enabled:           enabled,
		debugMode:         debugMode,
		maxDepth:          50, // Prevent infinite recursion
		validationHistory: make([]TreeValidationResult, 0, 100),
	}
}

// Enable turns on tree validation
func (tv *TreeValidator) Enable() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.enabled = true
}

// Disable turns off tree validation
func (tv *TreeValidator) Disable() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.enabled = false
}

// SetDebugMode enables/disables debug mode
func (tv *TreeValidator) SetDebugMode(debug bool) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.debugMode = debug
}

// ValidateTree performs comprehensive validation of the binary tree
func (tv *TreeValidator) ValidateTree(root *paneNode, operation string) error {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	if !tv.enabled {
		return nil
	}

	startTime := time.Now()
	result := TreeValidationResult{
		Timestamp: startTime,
		Errors:    make([]TreeValidationError, 0),
	}

	if tv.debugMode {
		log.Printf("[tree-validator] Starting validation after operation: %s", operation)
	}

	// Perform all validation checks
	visited := make(map[*paneNode]bool)
	nodeCount := 0
	maxDepth := 0

	// Check 1: Root validation
	if err := tv.validateRoot(root, &result); err.ErrorType != "" {
		result.Errors = append(result.Errors, err)
	}

	// Check 2: Tree structure validation
	if root != nil {
		if err := tv.validateTreeStructure(root, nil, visited, 0, &nodeCount, &maxDepth, &result); err != nil {
			result.Errors = append(result.Errors, err...)
		}
	}

	// Check 3: Cycle detection
	if err := tv.detectCycles(root, &result); err != nil {
		result.Errors = append(result.Errors, err...)
	}

	// Check 4: Parent-child consistency
	if err := tv.validateParentChildConsistency(root, &result); err != nil {
		result.Errors = append(result.Errors, err...)
	}

	// Check 5: Widget consistency
	if err := tv.validateWidgetConsistency(root, &result); err != nil {
		result.Errors = append(result.Errors, err...)
	}

	// Finalize result
	result.Duration = time.Since(startTime)
	result.NodeCount = nodeCount
	result.MaxDepth = maxDepth
	result.ErrorCount = len(result.Errors)
	result.Success = result.ErrorCount == 0

	// Calculate balance factor
	if root != nil {
		leftDepth := tv.calculateDepth(root.left)
		rightDepth := tv.calculateDepth(root.right)
		result.BalanceFactor = abs(leftDepth - rightDepth)
	}

	// Store result in history
	tv.addToHistory(result)

	if tv.debugMode {
		log.Printf("[tree-validator] Validation completed: success=%v, errors=%d, nodes=%d, depth=%d, balance=%d, duration=%v",
			result.Success, result.ErrorCount, result.NodeCount, result.MaxDepth, result.BalanceFactor, result.Duration)
	}

	if !result.Success {
		// Return first error as representative
		return result.Errors[0]
	}

	return nil
}

// validateRoot checks root node constraints
func (tv *TreeValidator) validateRoot(root *paneNode, result *TreeValidationResult) TreeValidationError {
	if root == nil {
		return TreeValidationError{} // Empty tree is valid
	}

	if root.parent != nil {
		return TreeValidationError{
			ErrorType:   "ROOT_HAS_PARENT",
			NodePointer: uintptr(getNodePointer(root)),
			Message:     fmt.Sprintf("root node has parent: %p", root.parent),
		}
	}

	return TreeValidationError{} // No error
}

// validateTreeStructure performs recursive validation of tree structure
func (tv *TreeValidator) validateTreeStructure(node *paneNode, expectedParent *paneNode, visited map[*paneNode]bool, depth int, nodeCount *int, maxDepth *int, result *TreeValidationResult) []TreeValidationError {
	if node == nil {
		return nil
	}

	var errors []TreeValidationError

	// Check max depth to prevent infinite recursion
	if depth > tv.maxDepth {
		errors = append(errors, TreeValidationError{
			ErrorType:   "MAX_DEPTH_EXCEEDED",
			NodePointer: uintptr(getNodePointer(node)),
			Message:     fmt.Sprintf("tree depth exceeds maximum of %d", tv.maxDepth),
		})
		return errors
	}

	// Update counters
	*nodeCount++
	if depth > *maxDepth {
		*maxDepth = depth
	}

	// Check if already visited (cycle detection)
	if visited[node] {
		errors = append(errors, TreeValidationError{
			ErrorType:   "CYCLE_DETECTED",
			NodePointer: uintptr(getNodePointer(node)),
			Message:     "node already visited in traversal",
		})
		return errors
	}
	visited[node] = true

	// Check parent pointer consistency
	if node.parent != expectedParent {
		errors = append(errors, TreeValidationError{
			ErrorType:   "PARENT_MISMATCH",
			NodePointer: uintptr(getNodePointer(node)),
			Message:     fmt.Sprintf("parent mismatch: expected %p, got %p", expectedParent, node.parent),
		})
	}

	// Validate node type constraints
	if node.isLeaf {
		// Leaf nodes must have no children
		if node.left != nil {
			errors = append(errors, TreeValidationError{
				ErrorType:   "LEAF_HAS_LEFT_CHILD",
				NodePointer: uintptr(getNodePointer(node)),
				Message:     fmt.Sprintf("leaf node has left child: %p", node.left),
			})
		}
		if node.right != nil {
			errors = append(errors, TreeValidationError{
				ErrorType:   "LEAF_HAS_RIGHT_CHILD",
				NodePointer: uintptr(getNodePointer(node)),
				Message:     fmt.Sprintf("leaf node has right child: %p", node.right),
			})
		}

		// Leaf nodes must have a pane (unless it's a stack container)
		if !node.isStacked && node.pane == nil {
			errors = append(errors, TreeValidationError{
				ErrorType:   "LEAF_MISSING_PANE",
				NodePointer: uintptr(getNodePointer(node)),
				Message:     "leaf node missing pane",
			})
		}
	} else if node.isStacked {
		// Stacked container nodes live in the binary tree but do not use left/right children.
		if node.left != nil {
			errors = append(errors, TreeValidationError{
				ErrorType:   "STACK_CONTAINER_HAS_LEFT_CHILD",
				NodePointer: uintptr(getNodePointer(node)),
				Message:     "stack container should not have left child",
			})
		}
		if node.right != nil {
			errors = append(errors, TreeValidationError{
				ErrorType:   "STACK_CONTAINER_HAS_RIGHT_CHILD",
				NodePointer: uintptr(getNodePointer(node)),
				Message:     "stack container should not have right child",
			})
		}
	} else {
		// Branch nodes must have exactly 2 children
		if node.left == nil {
			errors = append(errors, TreeValidationError{
				ErrorType:   "BRANCH_MISSING_LEFT_CHILD",
				NodePointer: uintptr(getNodePointer(node)),
				Message:     "branch node missing left child",
			})
		}
		if node.right == nil {
			errors = append(errors, TreeValidationError{
				ErrorType:   "BRANCH_MISSING_RIGHT_CHILD",
				NodePointer: uintptr(getNodePointer(node)),
				Message:     "branch node missing right child",
			})
		}

		// Branch nodes should not have panes (except for stack containers)
		if !node.isStacked && node.pane != nil {
			errors = append(errors, TreeValidationError{
				ErrorType:   "BRANCH_HAS_PANE",
				NodePointer: uintptr(getNodePointer(node)),
				Message:     "branch node should not have pane",
			})
		}
	}

	// Validate stack-specific constraints
	if node.isStacked {
		if err := tv.validateStackConstraints(node); err.ErrorType != "" {
			errors = append(errors, err)
		}
	}

	// Recursively validate children
	if node.left != nil {
		leftErrors := tv.validateTreeStructure(node.left, node, visited, depth+1, nodeCount, maxDepth, result)
		errors = append(errors, leftErrors...)
	}
	if node.right != nil {
		rightErrors := tv.validateTreeStructure(node.right, node, visited, depth+1, nodeCount, maxDepth, result)
		errors = append(errors, rightErrors...)
	}

	return errors
}

// validateStackConstraints validates stack-specific invariants
func (tv *TreeValidator) validateStackConstraints(node *paneNode) TreeValidationError {
	if !node.isStacked {
		return TreeValidationError{} // Not a stack
	}

	// Stack nodes should have stackedPanes
	if len(node.stackedPanes) == 0 {
		return TreeValidationError{
			ErrorType:   "EMPTY_STACK",
			NodePointer: uintptr(getNodePointer(node)),
			Message:     "stacked node has no panes",
		}
	}

	// Active index should be valid
	if node.activeStackIndex < 0 || node.activeStackIndex >= len(node.stackedPanes) {
		return TreeValidationError{
			ErrorType:   "INVALID_STACK_INDEX",
			NodePointer: uintptr(getNodePointer(node)),
			Message:     fmt.Sprintf("active stack index %d out of range [0, %d)", node.activeStackIndex, len(node.stackedPanes)),
		}
	}

	// All panes in stack should have this node as parent
	for i, stackedPane := range node.stackedPanes {
		if stackedPane == nil {
			return TreeValidationError{
				ErrorType:   "NULL_STACK_PANE",
				NodePointer: uintptr(getNodePointer(node)),
				Message:     fmt.Sprintf("stacked pane at index %d is nil", i),
			}
		}
		if stackedPane.parent != node {
			return TreeValidationError{
				ErrorType:   "STACK_PANE_PARENT_MISMATCH",
				NodePointer: uintptr(getNodePointer(stackedPane)),
				Message:     fmt.Sprintf("stacked pane parent mismatch: expected %p, got %p", node, stackedPane.parent),
			}
		}
	}

	return TreeValidationError{} // No error
}

// detectCycles performs cycle detection using Floyd's algorithm
func (tv *TreeValidator) detectCycles(root *paneNode, result *TreeValidationResult) []TreeValidationError {
	if root == nil {
		return nil
	}

	var errors []TreeValidationError
	visited := make(map[*paneNode]bool)

	var detectCycleHelper func(*paneNode, map[*paneNode]bool) bool
	detectCycleHelper = func(node *paneNode, path map[*paneNode]bool) bool {
		if node == nil {
			return false
		}

		if path[node] {
			errors = append(errors, TreeValidationError{
				ErrorType:   "CYCLE_IN_PATH",
				NodePointer: uintptr(getNodePointer(node)),
				Message:     "cycle detected in tree path",
			})
			return true
		}

		if visited[node] {
			return false // Already checked this subtree
		}

		path[node] = true
		visited[node] = true

		hasCycle := detectCycleHelper(node.left, path) || detectCycleHelper(node.right, path)

		delete(path, node)
		return hasCycle
	}

	detectCycleHelper(root, make(map[*paneNode]bool))
	return errors
}

// validateParentChildConsistency checks bidirectional parent-child relationships
func (tv *TreeValidator) validateParentChildConsistency(root *paneNode, result *TreeValidationResult) []TreeValidationError {
	if root == nil {
		return nil
	}

	var errors []TreeValidationError

	var validate func(*paneNode)
	validate = func(node *paneNode) {
		if node == nil {
			return
		}

		// Check left child consistency
		if node.left != nil {
			if node.left.parent != node {
				errors = append(errors, TreeValidationError{
					ErrorType:   "LEFT_CHILD_PARENT_MISMATCH",
					NodePointer: uintptr(getNodePointer(node.left)),
					Message:     fmt.Sprintf("left child parent points to %p instead of %p", node.left.parent, node),
				})
			}
		}

		// Check right child consistency
		if node.right != nil {
			if node.right.parent != node {
				errors = append(errors, TreeValidationError{
					ErrorType:   "RIGHT_CHILD_PARENT_MISMATCH",
					NodePointer: uintptr(getNodePointer(node.right)),
					Message:     fmt.Sprintf("right child parent points to %p instead of %p", node.right.parent, node),
				})
			}
		}

		validate(node.left)
		validate(node.right)
	}

	validate(root)
	return errors
}

// validateWidgetConsistency checks widget-related invariants
func (tv *TreeValidator) validateWidgetConsistency(root *paneNode, result *TreeValidationResult) []TreeValidationError {
	if root == nil {
		return nil
	}

	var errors []TreeValidationError

	var validate func(*paneNode)
	validate = func(node *paneNode) {
		if node == nil {
			return
		}

		// Every node should have a container
		if node.container == nil {
			errors = append(errors, TreeValidationError{
				ErrorType:   "MISSING_CONTAINER",
				NodePointer: uintptr(getNodePointer(node)),
				Message:     "node missing container widget",
			})
		} else if !node.container.IsValid() {
			errors = append(errors, TreeValidationError{
				ErrorType:   "INVALID_CONTAINER",
				NodePointer: uintptr(getNodePointer(node)),
				Message:     fmt.Sprintf("node has invalid container: %s", node.container.String()),
			})
		}

		// Stack containers should have stackWrapper
		if node.isStacked && node.stackWrapper == nil {
			errors = append(errors, TreeValidationError{
				ErrorType:   "STACK_MISSING_WRAPPER",
				NodePointer: uintptr(getNodePointer(node)),
				Message:     "stacked node missing stackWrapper",
			})
		}

		validate(node.left)
		validate(node.right)
	}

	validate(root)
	return errors
}

// calculateDepth calculates the depth of a subtree
func (tv *TreeValidator) calculateDepth(node *paneNode) int {
	if node == nil {
		return 0
	}
	leftDepth := tv.calculateDepth(node.left)
	rightDepth := tv.calculateDepth(node.right)
	return 1 + max(leftDepth, rightDepth)
}

// addToHistory adds a validation result to the history
func (tv *TreeValidator) addToHistory(result TreeValidationResult) {
	// Keep only the last 100 results
	if len(tv.validationHistory) >= 100 {
		tv.validationHistory = tv.validationHistory[1:]
	}
	tv.validationHistory = append(tv.validationHistory, result)
}

// GetValidationHistory returns the validation history
func (tv *TreeValidator) GetValidationHistory() []TreeValidationResult {
	tv.mu.RLock()
	defer tv.mu.RUnlock()

	// Return a copy to prevent race conditions
	history := make([]TreeValidationResult, len(tv.validationHistory))
	copy(history, tv.validationHistory)
	return history
}

// GetValidationStats returns summary statistics
func (tv *TreeValidator) GetValidationStats() map[string]interface{} {
	tv.mu.RLock()
	defer tv.mu.RUnlock()

	if len(tv.validationHistory) == 0 {
		return map[string]interface{}{
			"total_validations": 0,
		}
	}

	totalValidations := len(tv.validationHistory)
	successfulValidations := 0
	totalErrors := 0
	avgDuration := time.Duration(0)

	for _, result := range tv.validationHistory {
		if result.Success {
			successfulValidations++
		}
		totalErrors += result.ErrorCount
		avgDuration += result.Duration
	}

	avgDuration = avgDuration / time.Duration(totalValidations)

	return map[string]interface{}{
		"total_validations":      totalValidations,
		"successful_validations": successfulValidations,
		"success_rate":           float64(successfulValidations) / float64(totalValidations),
		"total_errors":           totalErrors,
		"avg_duration_ms":        avgDuration.Milliseconds(),
		"enabled":                tv.enabled,
		"debug_mode":             tv.debugMode,
	}
}

// Helper functions

func getNodePointer(node *paneNode) unsafe.Pointer {
	if node == nil {
		return nil
	}
	return unsafe.Pointer(node)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
