// workspace_geometry_validator.go - Geometry validation for split operations inspired by Zellij
package browser

import (
	"fmt"
	"log"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
)

// GeometryValidator validates pane geometry constraints for splits and operations
type GeometryValidator struct {
	minPaneWidth  int  // Minimum width for a pane in pixels
	minPaneHeight int  // Minimum height for a pane in pixels
	debugMode     bool // Enable debug logging
}

// PaneGeometry represents the geometry of a pane
type PaneGeometry struct {
	X       int
	Y       int
	Width   int
	Height  int
	IsValid bool
}

// SplitConstraints represents constraints for split operations
type SplitConstraints struct {
	MinResultingWidth  int
	MinResultingHeight int
	MaxSplitRatio      float64 // Maximum ratio for uneven splits (e.g., 0.8 = 80/20 split)
	MinSplitRatio      float64 // Minimum ratio for even splits (e.g., 0.3 = 30/70 split)
}

// ValidationResult represents the result of a geometry validation
type ValidationResult struct {
	IsValid              bool
	Reason               string
	Constraints          SplitConstraints
	Geometry             PaneGeometry
	RequiresRevalidation bool // True if validation should be retried after widget allocation
}

// NewGeometryValidator creates a new geometry validator with Zellij-inspired constraints
func NewGeometryValidator() *GeometryValidator {
	return &GeometryValidator{
		minPaneWidth:  300, // Inspired by Zellij's MIN_TERMINAL_WIDTH * character_width
		minPaneHeight: 200, // Inspired by Zellij's MIN_TERMINAL_HEIGHT * character_height
		debugMode:     false,
	}
}

// SetMinimumDimensions sets the minimum pane dimensions
func (gv *GeometryValidator) SetMinimumDimensions(width, height int) {
	gv.minPaneWidth = width
	gv.minPaneHeight = height
	log.Printf("[geometry] Updated minimum dimensions: %dx%d", width, height)
}

// SetDebugMode enables or disables debug logging
func (gv *GeometryValidator) SetDebugMode(debug bool) {
	gv.debugMode = debug
}

// GetPaneGeometry extracts geometry information from a pane node
// CRITICAL FIX: Use timeout to prevent indefinite blocking on GTK calls
func (gv *GeometryValidator) GetPaneGeometry(node *paneNode) PaneGeometry {
	geometry := PaneGeometry{IsValid: false}

	if node == nil || node.container == nil {
		if gv.debugMode {
			log.Printf("[geometry] Invalid node or missing container")
		}
		return geometry
	}

	// CRITICAL FIX: If not on main thread, marshal GTK calls to prevent deadlock
	if !webkit.IsMainThread() {
		// Use IdleAdd to marshal geometry query to main thread
		resultChan := make(chan PaneGeometry, 1)

		webkit.IdleAdd(func() bool {
			// Execute on main thread
			mainThreadGeometry := gv.getGeometryOnMainThread(node)
			resultChan <- mainThreadGeometry
			return false // Remove idle callback
		})

		// Wait for result with timeout
		select {
		case result := <-resultChan:
			return result
		case <-time.After(100 * time.Millisecond):
			log.Printf("[geometry] Geometry query timed out (100ms), assuming invalid")
			return geometry
		}
	}

	// We're on main thread, execute directly but with timeout protection
	return gv.getGeometryWithTimeout(node, 50*time.Millisecond)
}

// getGeometryOnMainThread executes geometry query on main thread (must be called from main thread)
func (gv *GeometryValidator) getGeometryOnMainThread(node *paneNode) PaneGeometry {
	geometry := PaneGeometry{IsValid: false}

	// Get geometry from GTK widget (now safe since we're on main thread)
	err := node.container.Execute(func(ptr uintptr) error {
		// Get widget allocation
		allocation := webkit.WidgetGetAllocation(ptr)

		geometry.X = allocation.X
		geometry.Y = allocation.Y
		geometry.Width = allocation.Width
		geometry.Height = allocation.Height
		geometry.IsValid = true

		if gv.debugMode {
			log.Printf("[geometry] Pane geometry: %dx%d at (%d,%d)",
				geometry.Width, geometry.Height, geometry.X, geometry.Y)
		}

		return nil
	})

	if err != nil {
		log.Printf("[geometry] Failed to get pane geometry: %v", err)
		geometry.IsValid = false
	}

	return geometry
}

// getGeometryWithTimeout gets geometry with timeout protection
func (gv *GeometryValidator) getGeometryWithTimeout(node *paneNode, timeout time.Duration) PaneGeometry {
	geometry := PaneGeometry{IsValid: false}

	// Use ExecuteWithTimeout to prevent indefinite blocking
	err := node.container.ExecuteWithTimeout(func(ptr uintptr) error {
		// Get widget allocation
		allocation := webkit.WidgetGetAllocation(ptr)

		geometry.X = allocation.X
		geometry.Y = allocation.Y
		geometry.Width = allocation.Width
		geometry.Height = allocation.Height
		geometry.IsValid = true

		if gv.debugMode {
			log.Printf("[geometry] Pane geometry: %dx%d at (%d,%d)",
				geometry.Width, geometry.Height, geometry.X, geometry.Y)
		}

		return nil
	}, timeout)

	if err != nil {
		log.Printf("[geometry] Failed to get pane geometry: %v", err)
		geometry.IsValid = false
	}

	return geometry
}

// ValidateHorizontalSplit validates if a pane can be split horizontally
func (gv *GeometryValidator) ValidateHorizontalSplit(node *paneNode) ValidationResult {
	geometry := gv.GetPaneGeometry(node)

	result := ValidationResult{
		Geometry: geometry,
		Constraints: SplitConstraints{
			MinResultingWidth:  gv.minPaneWidth,
			MinResultingHeight: gv.minPaneHeight,
			MaxSplitRatio:      0.8,
			MinSplitRatio:      0.2,
		},
	}

	if !geometry.IsValid {
		// Check if this is a zero allocation during startup
		if geometry.Width == 0 && geometry.Height == 0 {
			// Allow operation but flag for re-validation
			result.IsValid = true
			result.Reason = "geometry pending allocation, allowing horizontal split"
			result.RequiresRevalidation = true
			log.Printf("[geometry] Allowing horizontal split with zero allocation - will revalidate after first allocation")
			return result
		} else {
			result.IsValid = false
			result.Reason = "could not determine pane geometry"
			return result
		}
	}

	// Check if the pane is wide enough to split horizontally
	// Each resulting pane must be at least minPaneWidth wide
	requiredWidth := gv.minPaneWidth * 2

	if geometry.Width < requiredWidth {
		result.IsValid = false
		result.Reason = fmt.Sprintf("pane width %d is less than required %d for horizontal split",
			geometry.Width, requiredWidth)
		return result
	}

	// Check height constraints
	if geometry.Height < gv.minPaneHeight {
		result.IsValid = false
		result.Reason = fmt.Sprintf("pane height %d is less than minimum %d",
			geometry.Height, gv.minPaneHeight)
		return result
	}

	result.IsValid = true
	result.Reason = "horizontal split is valid"

	if gv.debugMode {
		log.Printf("[geometry] Horizontal split validation: %s (width=%d, required=%d)",
			result.Reason, geometry.Width, requiredWidth)
	}

	return result
}

// ValidateVerticalSplit validates if a pane can be split vertically
func (gv *GeometryValidator) ValidateVerticalSplit(node *paneNode) ValidationResult {
	geometry := gv.GetPaneGeometry(node)

	result := ValidationResult{
		Geometry: geometry,
		Constraints: SplitConstraints{
			MinResultingWidth:  gv.minPaneWidth,
			MinResultingHeight: gv.minPaneHeight,
			MaxSplitRatio:      0.8,
			MinSplitRatio:      0.2,
		},
	}

	if !geometry.IsValid {
		// Check if this is a zero allocation during startup
		if geometry.Width == 0 && geometry.Height == 0 {
			// Allow operation but flag for re-validation
			result.IsValid = true
			result.Reason = "geometry pending allocation, allowing vertical split"
			result.RequiresRevalidation = true
			log.Printf("[geometry] Allowing vertical split with zero allocation - will revalidate after first allocation")
			return result
		} else {
			result.IsValid = false
			result.Reason = "could not determine pane geometry"
			return result
		}
	}

	// Check if the pane is tall enough to split vertically
	// Each resulting pane must be at least minPaneHeight tall
	requiredHeight := gv.minPaneHeight * 2

	if geometry.Height < requiredHeight {
		result.IsValid = false
		result.Reason = fmt.Sprintf("pane height %d is less than required %d for vertical split",
			geometry.Height, requiredHeight)
		return result
	}

	// Check width constraints
	if geometry.Width < gv.minPaneWidth {
		result.IsValid = false
		result.Reason = fmt.Sprintf("pane width %d is less than minimum %d",
			geometry.Width, gv.minPaneWidth)
		return result
	}

	result.IsValid = true
	result.Reason = "vertical split is valid"

	if gv.debugMode {
		log.Printf("[geometry] Vertical split validation: %s (height=%d, required=%d)",
			result.Reason, geometry.Height, requiredHeight)
	}

	return result
}

// ValidateSplit validates a split operation based on direction (inspired by Zellij's split logic)
func (gv *GeometryValidator) ValidateSplit(node *paneNode, direction string) ValidationResult {
	if node == nil {
		return ValidationResult{
			IsValid: false,
			Reason:  "node is nil",
		}
	}

	if node.isStacked {
		return ValidationResult{
			IsValid: false,
			Reason:  "cannot split stacked pane container directly",
		}
	}

	if !node.isLeaf {
		return ValidationResult{
			IsValid: false,
			Reason:  "can only split leaf panes",
		}
	}

	switch direction {
	case "left", "right":
		return gv.ValidateHorizontalSplit(node)
	case "up", "down":
		return gv.ValidateVerticalSplit(node)
	default:
		return ValidationResult{
			IsValid: false,
			Reason:  fmt.Sprintf("unknown split direction: %s", direction),
		}
	}
}

// DetermineBestSplitDirection determines the best split direction for a pane (like Zellij's algorithm)
func (gv *GeometryValidator) DetermineBestSplitDirection(node *paneNode) (string, ValidationResult) {
	geometry := gv.GetPaneGeometry(node)

	if !geometry.IsValid {
		return "", ValidationResult{
			IsValid: false,
			Reason:  "could not determine pane geometry",
		}
	}

	// Calculate aspect ratio and available space
	aspectRatio := float64(geometry.Width) / float64(geometry.Height)

	// Check which splits are possible
	horizontalValid := gv.ValidateHorizontalSplit(node)
	verticalValid := gv.ValidateVerticalSplit(node)

	if gv.debugMode {
		log.Printf("[geometry] Determining best split: aspect=%.2f, horizontal=%v, vertical=%v",
			aspectRatio, horizontalValid.IsValid, verticalValid.IsValid)
	}

	// If only one direction is valid, use it
	if horizontalValid.IsValid && !verticalValid.IsValid {
		return "right", horizontalValid
	}
	if verticalValid.IsValid && !horizontalValid.IsValid {
		return "down", verticalValid
	}

	// If neither is valid, return error
	if !horizontalValid.IsValid && !verticalValid.IsValid {
		return "", ValidationResult{
			IsValid: false,
			Reason:  "pane is too small for any split operation",
		}
	}

	// Both directions are valid - choose based on aspect ratio and space
	// Prefer horizontal split for wide panes, vertical for tall panes
	const aspectThreshold = 1.5

	if aspectRatio > aspectThreshold {
		// Wide pane - prefer horizontal split
		return "right", horizontalValid
	} else if aspectRatio < (1.0 / aspectThreshold) {
		// Tall pane - prefer vertical split
		return "down", verticalValid
	} else {
		// Nearly square - prefer the direction with more available space
		horizontalSpace := geometry.Width - (gv.minPaneWidth * 2)
		verticalSpace := geometry.Height - (gv.minPaneHeight * 2)

		if horizontalSpace > verticalSpace {
			return "right", horizontalValid
		} else {
			return "down", verticalValid
		}
	}
}

// ValidateStackOperation validates if a pane can be converted to a stack
func (gv *GeometryValidator) ValidateStackOperation(node *paneNode) ValidationResult {
	geometry := gv.GetPaneGeometry(node)

	result := ValidationResult{
		Geometry: geometry,
		Constraints: SplitConstraints{
			MinResultingWidth:  gv.minPaneWidth,
			MinResultingHeight: gv.minPaneHeight + 30, // Extra space for title bar
		},
	}

	if !geometry.IsValid {
		// Check if this is a zero allocation during startup
		if geometry.Width == 0 && geometry.Height == 0 {
			// Allow operation but flag for re-validation
			result.IsValid = true
			result.Reason = "geometry pending allocation, allowing stack operation"
			result.RequiresRevalidation = true
			log.Printf("[geometry] Allowing stack operation with zero allocation - will revalidate after first allocation")
			return result
		} else {
			result.IsValid = false
			result.Reason = "could not determine pane geometry"
			return result
		}
	}

	// Check if pane is already stacked
	if node.isStacked {
		result.IsValid = false
		result.Reason = "pane is already stacked"
		return result
	}

	// Check if pane is large enough for stacking (needs space for title bars)
	minStackHeight := gv.minPaneHeight + 30 // Extra space for title bar

	if geometry.Height < minStackHeight {
		result.IsValid = false
		result.Reason = fmt.Sprintf("pane height %d is less than required %d for stacking",
			geometry.Height, minStackHeight)
		return result
	}

	if geometry.Width < gv.minPaneWidth {
		result.IsValid = false
		result.Reason = fmt.Sprintf("pane width %d is less than minimum %d",
			geometry.Width, gv.minPaneWidth)
		return result
	}

	result.IsValid = true
	result.Reason = "stack operation is valid"

	if gv.debugMode {
		log.Printf("[geometry] Stack validation: %s", result.Reason)
	}

	return result
}

// ValidateWorkspaceLayout validates the overall workspace layout
func (gv *GeometryValidator) ValidateWorkspaceLayout(root *paneNode) []ValidationResult {
	var results []ValidationResult

	if root == nil {
		results = append(results, ValidationResult{
			IsValid: false,
			Reason:  "workspace root is nil",
		})
		return results
	}

	// Validate each pane in the workspace
	leaves := gv.collectAllLeaves(root)

	for i, leaf := range leaves {
		geometry := gv.GetPaneGeometry(leaf)

		result := ValidationResult{
			Geometry: geometry,
		}

		if !geometry.IsValid {
			result.IsValid = false
			result.Reason = fmt.Sprintf("leaf pane %d has invalid geometry", i)
		} else if geometry.Width < gv.minPaneWidth {
			result.IsValid = false
			result.Reason = fmt.Sprintf("leaf pane %d width %d below minimum %d",
				i, geometry.Width, gv.minPaneWidth)
		} else if geometry.Height < gv.minPaneHeight {
			result.IsValid = false
			result.Reason = fmt.Sprintf("leaf pane %d height %d below minimum %d",
				i, geometry.Height, gv.minPaneHeight)
		} else {
			result.IsValid = true
			result.Reason = fmt.Sprintf("leaf pane %d geometry is valid", i)
		}

		results = append(results, result)
	}

	return results
}

// collectAllLeaves collects all leaf panes in the workspace
func (gv *GeometryValidator) collectAllLeaves(node *paneNode) []*paneNode {
	if node == nil {
		return nil
	}

	var leaves []*paneNode

	if node.isLeaf {
		leaves = append(leaves, node)
	} else if node.isStacked {
		// For stacked panes, only include the active pane
		if node.activeStackIndex >= 0 && node.activeStackIndex < len(node.stackedPanes) {
			leaves = append(leaves, node.stackedPanes[node.activeStackIndex])
		}
	} else {
		// Branch node - collect from children
		leaves = append(leaves, gv.collectAllLeaves(node.left)...)
		leaves = append(leaves, gv.collectAllLeaves(node.right)...)
	}

	return leaves
}

// GetOptimalSplitRatio calculates the optimal split ratio based on available space
func (gv *GeometryValidator) GetOptimalSplitRatio(geometry PaneGeometry, direction string) float64 {
	// Default to 50/50 split
	defaultRatio := 0.5

	switch direction {
	case "left", "right":
		// For horizontal splits, consider width
		availableWidth := geometry.Width - (gv.minPaneWidth * 2)
		if availableWidth <= 0 {
			return defaultRatio
		}
		// If there's plenty of space, use 50/50, otherwise bias toward existing content
		return defaultRatio

	case "up", "down":
		// For vertical splits, consider height
		availableHeight := geometry.Height - (gv.minPaneHeight * 2)
		if availableHeight <= 0 {
			return defaultRatio
		}
		// If there's plenty of space, use 50/50, otherwise bias toward existing content
		return defaultRatio

	default:
		return defaultRatio
	}
}

// GetGeometryStats returns statistics about the workspace geometry
func (gv *GeometryValidator) GetGeometryStats(root *paneNode) map[string]interface{} {
	results := gv.ValidateWorkspaceLayout(root)

	totalPanes := len(results)
	validPanes := 0
	totalArea := 0
	minWidth := int(^uint(0) >> 1) // Max int
	maxWidth := 0
	minHeight := int(^uint(0) >> 1) // Max int
	maxHeight := 0

	for _, result := range results {
		if result.IsValid {
			validPanes++
		}

		if result.Geometry.IsValid {
			totalArea += result.Geometry.Width * result.Geometry.Height

			if result.Geometry.Width < minWidth {
				minWidth = result.Geometry.Width
			}
			if result.Geometry.Width > maxWidth {
				maxWidth = result.Geometry.Width
			}
			if result.Geometry.Height < minHeight {
				minHeight = result.Geometry.Height
			}
			if result.Geometry.Height > maxHeight {
				maxHeight = result.Geometry.Height
			}
		}
	}

	if totalPanes == 0 {
		return map[string]interface{}{
			"total_panes": 0,
		}
	}

	return map[string]interface{}{
		"total_panes":     totalPanes,
		"valid_panes":     validPanes,
		"validity_rate":   float64(validPanes) / float64(totalPanes),
		"total_area":      totalArea,
		"min_width":       minWidth,
		"max_width":       maxWidth,
		"min_height":      minHeight,
		"max_height":      maxHeight,
		"min_pane_width":  gv.minPaneWidth,
		"min_pane_height": gv.minPaneHeight,
	}
}
