// workspace_operations.go - Workspace operation methods with validation and safety
package browser

import (
	"fmt"
	"log"

	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// SplitOptions configures how a pane split should be performed
type SplitOptions struct {
	Direction    string       // Required: left, right, up, down
	ExistingPane *BrowserPane // Optional: use existing pane instead of creating new
	MaxWidth     int          // Optional: constrain new pane width (0 = no limit)
	MaxHeight    int          // Optional: constrain new pane height (0 = no limit)
}

// SplitPane performs a split operation with validation and safety checks
func (wm *WorkspaceManager) SplitPane(target *paneNode, direction string) (*paneNode, error) {
	if wm == nil {
		return nil, fmt.Errorf("workspace manager is nil")
	}

	// Validate direction parameter
	validDirections := map[string]bool{
		DirectionLeft:  true,
		DirectionRight: true,
		DirectionUp:    true,
		DirectionDown:  true,
	}
	if !validDirections[direction] {
		return nil, fmt.Errorf("invalid split direction '%s', expected one of: left, right, up, down", direction)
	}

	log.Printf("[workspace] Starting split operation: target=%p direction=%s", target, direction)

	// Step 1: Validate geometry constraints (only in DebugBasic+)
	if wm.debugLevel >= DebugBasic {
		validation := wm.geometryValidator.ValidateSplit(target, direction)
		if !validation.IsValid {
			if wm.debugLevel == DebugFull {
				return nil, fmt.Errorf("split validation failed: %s", validation.Reason)
			}
			log.Printf("[workspace] WARNING: Split validation failed but allowing operation: %s", validation.Reason)
		}

		// Log if re-validation will be needed due to pending widget allocation
		if validation.RequiresRevalidation {
			log.Printf("[workspace] Split validation passed with pending allocation - operation will proceed")
		}

		// Step 3: Validate tree invariants before operation
		if err := wm.treeValidator.ValidateTree(wm.root, "before_split"); err != nil {
			if wm.debugLevel == DebugFull {
				return nil, fmt.Errorf("tree validation failed before split: %w", err)
			}
			log.Printf("[workspace] WARNING: Tree validation failed before split but allowing operation: %v", err)
		}
	}

	// Step 2: Execute directly if we're already on the GTK main thread
	if webkit.IsMainThread() {
		log.Printf("[workspace] Already on main thread, executing split directly")
		newNode, err := wm.splitNode(target, direction, nil, nil)
		if err != nil {
			return nil, err
		}

		if wm.debugLevel >= DebugBasic {
			if err := wm.treeValidator.ValidateTree(wm.root, "after_split"); err != nil {
				log.Printf("[workspace] Tree validation failed after split: %v", err)
			}
		}

		// Tree rebalancing is only needed after close promotions. Splits have correct allocation from GTK.

		log.Printf("[workspace] Split operation completed successfully (direct execution): newNode=%p", newNode)
		return newNode, nil
	}

	// Step 3: Not on main thread, marshal via IdleAdd
	log.Printf("[workspace] Not on main thread, marshalling split via IdleAdd")
	var newNode *paneNode
	var splitErr error
	done := make(chan struct{})

	_ = webkit.IdleAdd(func() bool {
		newNode, splitErr = wm.splitNode(target, direction, nil, nil)
		if splitErr == nil {
			if wm.debugLevel >= DebugBasic {
				if verr := wm.treeValidator.ValidateTree(wm.root, "after_split"); verr != nil {
					log.Printf("[workspace] Tree validation failed after split: %v", verr)
				}
			}
		}
		close(done)
		return false
	})

	<-done

	if splitErr != nil {
		return nil, splitErr
	}

	log.Printf("[workspace] Split operation completed successfully: newNode=%p", newNode)
	return newNode, nil
}

// SplitPaneWithPane performs a split using a pre-created pane (e.g., extension popup) while
// preserving the normal validation and bookkeeping logic.
func (wm *WorkspaceManager) SplitPaneWithPane(target *paneNode, direction string, existingPane *BrowserPane) (*paneNode, error) {
	if wm == nil {
		return nil, fmt.Errorf("workspace manager is nil")
	}
	if existingPane == nil {
		return nil, fmt.Errorf("existing pane is nil")
	}

	// Validate direction parameter
	validDirections := map[string]bool{
		DirectionLeft:  true,
		DirectionRight: true,
		DirectionUp:    true,
		DirectionDown:  true,
	}
	if !validDirections[direction] {
		return nil, fmt.Errorf("invalid split direction '%s', expected one of: left, right, up, down", direction)
	}

	log.Printf("[workspace] Starting split with existing pane: target=%p direction=%s pane=%p", target, direction, existingPane)

	if webkit.IsMainThread() {
		newNode, err := wm.splitNode(target, direction, existingPane, nil)
		if err != nil {
			return nil, err
		}
		if wm.debugLevel >= DebugBasic {
			if err := wm.treeValidator.ValidateTree(wm.root, "after_split_existing"); err != nil {
				log.Printf("[workspace] Tree validation failed after split with existing pane: %v", err)
			}
		}
		return newNode, nil
	}

	var newNode *paneNode
	var splitErr error
	done := make(chan struct{})
	_ = webkit.IdleAdd(func() bool {
		newNode, splitErr = wm.splitNode(target, direction, existingPane, nil)
		if splitErr == nil && wm.debugLevel >= DebugBasic {
			if err := wm.treeValidator.ValidateTree(wm.root, "after_split_existing"); err != nil {
				log.Printf("[workspace] Tree validation failed after split with existing pane: %v", err)
			}
		}
		close(done)
		return false
	})
	<-done
	return newNode, splitErr
}

// SplitPaneWithOptions performs a split with configurable options including max-width constraints.
// Useful for extension popups that need a fixed-width pane.
func (wm *WorkspaceManager) SplitPaneWithOptions(target *paneNode, opts SplitOptions) (*paneNode, error) {
	if wm == nil {
		return nil, fmt.Errorf("workspace manager is nil")
	}

	// Validate direction
	validDirections := map[string]bool{
		DirectionLeft:  true,
		DirectionRight: true,
		DirectionUp:    true,
		DirectionDown:  true,
	}
	if !validDirections[opts.Direction] {
		return nil, fmt.Errorf("invalid split direction '%s'", opts.Direction)
	}

	log.Printf("[workspace] SplitPaneWithOptions: direction=%s maxWidth=%d maxHeight=%d existingPane=%v",
		opts.Direction, opts.MaxWidth, opts.MaxHeight, opts.ExistingPane != nil)

	// Call splitNode directly with opts to pass size constraints through
	if webkit.IsMainThread() {
		newNode, err := wm.splitNode(target, opts.Direction, opts.ExistingPane, &opts)
		if err != nil {
			return nil, err
		}
		if wm.debugLevel >= DebugBasic {
			if err := wm.treeValidator.ValidateTree(wm.root, "after_split_options"); err != nil {
				log.Printf("[workspace] Tree validation failed after split with options: %v", err)
			}
		}
		return newNode, nil
	}

	var newNode *paneNode
	var splitErr error
	done := make(chan struct{})
	_ = webkit.IdleAdd(func() bool {
		newNode, splitErr = wm.splitNode(target, opts.Direction, opts.ExistingPane, &opts)
		if splitErr == nil && wm.debugLevel >= DebugBasic {
			if err := wm.treeValidator.ValidateTree(wm.root, "after_split_options"); err != nil {
				log.Printf("[workspace] Tree validation failed after split with options: %v", err)
			}
		}
		close(done)
		return false
	})
	<-done
	return newNode, splitErr
}

// applySizeConstraints sets max-width/height on a pane by adjusting the paned divider position
func (wm *WorkspaceManager) applySizeConstraints(node *paneNode, opts SplitOptions) {
	if node == nil || node.parent == nil {
		return
	}

	// Get the paned widget from the parent node
	paned, ok := node.parent.container.(*gtk.Paned)
	if !ok || paned == nil {
		log.Printf("[workspace] Cannot apply size constraints: parent is not a Paned")
		return
	}

	// Determine if this node is the start or end child
	isEndChild := node.parent.right == node

	// Set up position calculation based on direction and max-width/height
	if opts.MaxWidth > 0 {
		// For horizontal splits (left/right), we need to set the divider position
		// The position is the distance from the left edge to the divider
		wm.setPanedPositionFixed(paned, opts.MaxWidth, isEndChild, gtk.OrientationHorizontal)
	}
	if opts.MaxHeight > 0 {
		wm.setPanedPositionFixed(paned, opts.MaxHeight, isEndChild, gtk.OrientationVertical)
	}
}

// setPanedPositionFixed sets the paned divider to give a fixed size to one child
func (wm *WorkspaceManager) setPanedPositionFixed(paned *gtk.Paned, fixedSize int, isEndChild bool, orientation gtk.Orientation) {
	if paned == nil {
		return
	}

	const (
		minDimension = 32
		maxFrames    = 120
	)

	setPosition := func() bool {
		alloc := paned.Allocation()
		dimension := alloc.Width()
		if orientation == gtk.OrientationVertical {
			dimension = alloc.Height()
		}

		if dimension < minDimension {
			return false
		}

		var pos int
		if isEndChild {
			// New pane is on the right/bottom - position = total - fixedSize
			pos = dimension - fixedSize
		} else {
			// New pane is on the left/top - position = fixedSize
			pos = fixedSize
		}

		if pos <= 0 || pos >= dimension {
			pos = dimension / 2 // Fallback to 50% if calculation doesn't work
		}

		paned.SetPosition(pos)

		// Prevent the fixed-size pane from shrinking
		if isEndChild {
			paned.SetShrinkEndChild(false)
		} else {
			paned.SetShrinkStartChild(false)
		}

		log.Printf("[workspace] Set paned position for fixed size %d: pos=%d isEndChild=%v (dimension=%d)",
			fixedSize, pos, isEndChild, dimension)
		return true
	}

	// Try immediately
	if setPosition() {
		return
	}

	// Retry on tick callbacks until allocation is available
	var frames uint
	paned.AddTickCallback(func(widget gtk.Widgetter, _ gdk.FrameClocker) bool {
		frames++
		if setPosition() {
			return false // Stop callback
		}
		if frames >= maxFrames {
			log.Printf("[workspace] Unable to set fixed paned position after %d frames", frames)
			return false
		}
		return true // Continue
	})
}

// ClosePane performs a close operation with validation and safety checks
func (wm *WorkspaceManager) ClosePane(node *paneNode) error {
	if wm == nil {
		return fmt.Errorf("workspace manager is nil")
	}

	log.Printf("[workspace] Starting close operation: node=%p", node)
	wm.paneCloseLogf("start bulletproof close node=%p", node)
	wm.dumpTreeState("before_close")

	// Notify components about impending pane close (e.g., extensions overlay)
	// Must happen BEFORE we destroy the widget so overlay can detach cleanly
	if wm.app != nil && wm.app.tabManager != nil && wm.app.tabManager.extensionsOverlay != nil {
		wm.app.tabManager.extensionsOverlay.OnPaneClosing(node)
	}

	// Step 1: Validate tree invariants before operation (only in DebugBasic+)
	if wm.debugLevel >= DebugBasic {
		if err := wm.treeValidator.ValidateTree(wm.root, "before_close"); err != nil {
			if wm.debugLevel == DebugFull {
				return fmt.Errorf("tree validation failed before close: %w", err)
			}
			log.Printf("[workspace] WARNING: Tree validation failed before close but allowing operation: %v", err)
		}
	}

	// Step 3: Check if this is a stacked pane and use enhanced lifecycle management
	if node.parent != nil && node.parent.isStacked {
		log.Printf("[workspace] Using enhanced stack lifecycle management for close")
		return wm.stackLifecycleManager.CloseStackedPaneWithLifecycle(node)
	}

	// Step 4: Execute directly when already on the GTK main thread
	if webkit.IsMainThread() {
		log.Printf("[workspace] Already on main thread, executing close directly")

		wm.paneCloseLogf("invoking closePane node=%p", node)
		promoted, err := wm.closePane(node)
		if err != nil {
			wm.paneCloseLogf("closePane failed node=%p err=%v", node, err)
			wm.dumpTreeState("after_close_error")
			return err
		}

		if wm.debugLevel >= DebugBasic {
			if err := wm.treeValidator.ValidateTree(wm.root, "after_close"); err != nil {
				log.Printf("[workspace] Tree validation failed after close: %v", err)
			}
		}
		wm.paneCloseLogf("closePane succeeded node=%p promoted=%p root=%p", node, promoted, wm.root)
		wm.dumpTreeState("after_close_success")

		if wm.treeRebalancer != nil {
			if err := wm.treeRebalancer.RebalanceAfterClose(node, promoted); err != nil {
				log.Printf("[workspace] Tree rebalancing failed after close: %v", err)
			}
		}

		log.Printf("[workspace] Close operation completed successfully (direct execution)")
		return nil
	}

	// Step 5: Not on main thread, marshal via IdleAdd
	log.Printf("[workspace] Not on main thread, marshalling close via IdleAdd")
	var promoted *paneNode
	var closeErr error
	done := make(chan struct{})

	_ = webkit.IdleAdd(func() bool {
		wm.paneCloseLogf("invoking closePane node=%p", node)
		promoted, closeErr = wm.closePane(node)
		if closeErr != nil {
			wm.paneCloseLogf("closePane failed node=%p err=%v", node, closeErr)
			wm.dumpTreeState("after_close_error")
		} else {
			if wm.debugLevel >= DebugBasic {
				if verr := wm.treeValidator.ValidateTree(wm.root, "after_close"); verr != nil {
					log.Printf("[workspace] Tree validation failed after close: %v", verr)
				}
			}
			wm.paneCloseLogf("closePane succeeded node=%p promoted=%p root=%p", node, promoted, wm.root)
			wm.dumpTreeState("after_close_success")

			if wm.treeRebalancer != nil {
				if rerr := wm.treeRebalancer.RebalanceAfterClose(node, promoted); rerr != nil {
					log.Printf("[workspace] Tree rebalancing failed after close: %v", rerr)
				}
			}
		}
		close(done)
		return false
	})

	<-done

	if closeErr != nil {
		return closeErr
	}

	log.Printf("[workspace] Close operation completed successfully")
	return nil
}

// StackPane performs a stack operation with validation and safety checks
func (wm *WorkspaceManager) StackPane(target *paneNode) (*paneNode, error) {
	if wm == nil {
		return nil, fmt.Errorf("workspace manager is nil")
	}

	log.Printf("[workspace] Starting stack operation: target=%p", target)

	// Step 1: Validate stack operation constraints (only in DebugBasic+)
	if wm.debugLevel >= DebugBasic {
		validation := wm.geometryValidator.ValidateStackOperation(target)
		if !validation.IsValid {
			if wm.debugLevel == DebugFull {
				return nil, fmt.Errorf("stack validation failed: %s", validation.Reason)
			}
			log.Printf("[workspace] WARNING: Stack validation failed but allowing operation: %s", validation.Reason)
		}

		// Log if re-validation will be needed due to pending widget allocation
		if validation.RequiresRevalidation {
			log.Printf("[workspace] Stack validation passed with pending allocation - operation will proceed")
		}
	}

	// Step 2: Validate tree invariants before operation (only in DebugBasic+)
	if wm.debugLevel >= DebugBasic {
		if err := wm.treeValidator.ValidateTree(wm.root, "before_stack"); err != nil {
			if wm.debugLevel == DebugFull {
				return nil, fmt.Errorf("tree validation failed before stack: %w", err)
			}
			log.Printf("[workspace] WARNING: Tree validation failed before stack but allowing operation: %v", err)
		}
	}

	// Step 3: Execute directly if already on the GTK main thread
	if webkit.IsMainThread() {
		log.Printf("[workspace] Already on main thread, executing stack directly")
		newNode, err := wm.stackedPaneManager.StackPane(target)
		if err != nil {
			return nil, err
		}

		if wm.debugLevel >= DebugBasic {
			if err := wm.treeValidator.ValidateTree(wm.root, "after_stack"); err != nil {
				log.Printf("[workspace] Tree validation failed after stack: %v", err)
			}
		}

		log.Printf("[workspace] Stack operation completed successfully (direct execution): newNode=%p", newNode)
		return newNode, nil
	}

	// Step 4: Not on main thread, marshal via IdleAdd
	log.Printf("[workspace] Not on main thread, marshalling stack via IdleAdd")
	var newNode *paneNode
	var stackErr error
	done := make(chan struct{})

	_ = webkit.IdleAdd(func() bool {
		newNode, stackErr = wm.stackedPaneManager.StackPane(target)
		if stackErr == nil {
			if wm.debugLevel >= DebugBasic {
				if verr := wm.treeValidator.ValidateTree(wm.root, "after_stack"); verr != nil {
					log.Printf("[workspace] Tree validation failed after stack: %v", verr)
				}
			}
		}
		close(done)
		return false
	})

	<-done

	if stackErr != nil {
		return nil, stackErr
	}

	log.Printf("[workspace] Stack operation completed successfully: newNode=%p", newNode)
	return newNode, nil
}

// EnableEnhancedMode enables all enhanced validation features
func (wm *WorkspaceManager) EnableEnhancedMode() {
	if wm.treeValidator != nil {
		wm.treeValidator.Enable()
	}
	if wm.treeRebalancer != nil {
		wm.treeRebalancer.Enable()
	}
	log.Printf("[workspace] Enhanced validation mode enabled")
}

// DisableEnhancedMode disables enhanced features for performance
func (wm *WorkspaceManager) DisableEnhancedMode() {
	if wm.treeValidator != nil {
		wm.treeValidator.Disable()
	}
	if wm.treeRebalancer != nil {
		wm.treeRebalancer.Disable()
	}
	log.Printf("[workspace] Enhanced validation mode disabled")
}

// SetEnhancedDebugMode enables/disables debug mode for all enhanced validation components
func (wm *WorkspaceManager) SetEnhancedDebugMode(debug bool) {
	if wm.treeValidator != nil {
		wm.treeValidator.SetDebugMode(debug)
	}
	if wm.geometryValidator != nil {
		wm.geometryValidator.SetDebugMode(debug)
	}
	log.Printf("[workspace] Enhanced debug mode set to: %v", debug)
}

// GetEnhancedStats returns comprehensive statistics about all enhanced validation components
func (wm *WorkspaceManager) GetEnhancedStats() map[string]interface{} {
	stats := make(map[string]interface{})

	if wm.treeValidator != nil {
		stats["tree_validation"] = wm.treeValidator.GetValidationStats()
	}

	if wm.treeRebalancer != nil {
		stats["tree_rebalancing"] = wm.treeRebalancer.GetRebalancingStats()
	}

	// Geometry validator no longer tracks stats - simplified

	return stats
}

// ValidateWorkspaceIntegrity performs a comprehensive validation of the workspace
func (wm *WorkspaceManager) ValidateWorkspaceIntegrity() error {
	log.Printf("[workspace] Performing comprehensive workspace integrity check")

	// Tree structure validation
	if wm.treeValidator != nil {
		if err := wm.treeValidator.ValidateTree(wm.root, "integrity_check"); err != nil {
			return fmt.Errorf("tree validation failed: %w", err)
		}
	}

	// Geometry validation simplified - ValidateWorkspaceLayout removed

	log.Printf("[workspace] Workspace integrity check completed successfully")
	return nil
}

// ShutdownEnhancedComponents gracefully shuts down all enhanced validation components
func (wm *WorkspaceManager) ShutdownEnhancedComponents() {
	log.Printf("[workspace] Shutting down enhanced validation components")

	// Other components don't require explicit shutdown currently
	log.Printf("[workspace] Enhanced validation components shutdown complete")
}
