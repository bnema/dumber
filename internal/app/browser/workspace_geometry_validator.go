// workspace_geometry_validator.go - Simplified geometry validation for split operations
package browser

import (
	"fmt"
	"log"

	"github.com/bnema/dumber/pkg/webkit"
)

// GeometryValidator validates pane geometry constraints for splits and operations
type GeometryValidator struct {
	minPaneWidth  int
	minPaneHeight int
	debugMode     bool
}

// PaneGeometry represents the geometry of a pane
type PaneGeometry struct {
	X       int
	Y       int
	Width   int
	Height  int
	IsValid bool
}

// ValidationResult represents the result of a geometry validation
type ValidationResult struct {
	IsValid              bool
	Reason               string
	Geometry             PaneGeometry
	RequiresRevalidation bool
}

// NewGeometryValidator creates a new geometry validator
func NewGeometryValidator() *GeometryValidator {
	return &GeometryValidator{
		minPaneWidth:  300, // Minimum width for usable pane
		minPaneHeight: 200, // Minimum height for usable pane
		debugMode:     false,
	}
}

// SetDebugMode enables or disables debug logging
func (gv *GeometryValidator) SetDebugMode(debug bool) {
	gv.debugMode = debug
}

// GetPaneGeometry extracts geometry information from a pane node
func (gv *GeometryValidator) GetPaneGeometry(node *paneNode) PaneGeometry {
	geometry := PaneGeometry{IsValid: false}

	if node == nil || node.container == 0 {
		if gv.debugMode {
			log.Printf("[geometry] Invalid node or missing container")
		}
		return geometry
	}

	// Get widget allocation directly - GTK operations are always on main thread
	allocation := webkit.WidgetGetAllocation(node.container)

	geometry.X = allocation.X
	geometry.Y = allocation.Y
	geometry.Width = allocation.Width
	geometry.Height = allocation.Height
	geometry.IsValid = true

	if gv.debugMode {
		log.Printf("[geometry] Pane geometry: %dx%d at (%d,%d)",
			geometry.Width, geometry.Height, geometry.X, geometry.Y)
	}

	return geometry
}

// prepareGeometryResult centralizes the common geometry validation logic used by
// split/stack checks. It returns the computed geometry, a populated
// ValidationResult (for early exits), and a boolean that indicates whether the
// caller should continue with additional constraints.
func (gv *GeometryValidator) prepareGeometryResult(node *paneNode) (PaneGeometry, ValidationResult, bool) {
	geometry := gv.GetPaneGeometry(node)
	result := ValidationResult{Geometry: geometry}

	if !geometry.IsValid {
		if geometry.Width == 0 && geometry.Height == 0 {
			result.IsValid = true
			result.Reason = "geometry pending allocation"
			result.RequiresRevalidation = true
		} else {
			result.IsValid = false
			result.Reason = "could not determine pane geometry"
		}
		return geometry, result, false
	}

	return geometry, result, true
}

// ValidateHorizontalSplit validates if a pane can be split horizontally
func (gv *GeometryValidator) ValidateHorizontalSplit(node *paneNode) ValidationResult {
	geometry, result, ok := gv.prepareGeometryResult(node)
	if !ok {
		return result
	}

	requiredWidth := gv.minPaneWidth * 2
	if geometry.Width < requiredWidth {
		result.IsValid = false
		result.Reason = fmt.Sprintf("pane width %d < required %d for horizontal split",
			geometry.Width, requiredWidth)
		return result
	}

	if geometry.Height < gv.minPaneHeight {
		result.IsValid = false
		result.Reason = fmt.Sprintf("pane height %d < minimum %d",
			geometry.Height, gv.minPaneHeight)
		return result
	}

	result.IsValid = true
	result.Reason = "horizontal split is valid"
	return result
}

// ValidateVerticalSplit validates if a pane can be split vertically
func (gv *GeometryValidator) ValidateVerticalSplit(node *paneNode) ValidationResult {
	geometry, result, ok := gv.prepareGeometryResult(node)
	if !ok {
		return result
	}

	requiredHeight := gv.minPaneHeight * 2
	if geometry.Height < requiredHeight {
		result.IsValid = false
		result.Reason = fmt.Sprintf("pane height %d < required %d for vertical split",
			geometry.Height, requiredHeight)
		return result
	}

	if geometry.Width < gv.minPaneWidth {
		result.IsValid = false
		result.Reason = fmt.Sprintf("pane width %d < minimum %d",
			geometry.Width, gv.minPaneWidth)
		return result
	}

	result.IsValid = true
	result.Reason = "vertical split is valid"
	return result
}

// ValidateSplit validates a split operation based on direction
func (gv *GeometryValidator) ValidateSplit(node *paneNode, direction string) ValidationResult {
	if node == nil {
		return ValidationResult{IsValid: false, Reason: "node is nil"}
	}

	if node.isStacked {
		return ValidationResult{IsValid: false, Reason: "cannot split stacked pane container"}
	}

	if !node.isLeaf {
		return ValidationResult{IsValid: false, Reason: "can only split leaf panes"}
	}

	switch direction {
	case "left", "right":
		return gv.ValidateHorizontalSplit(node)
	case "up", "down":
		return gv.ValidateVerticalSplit(node)
	default:
		return ValidationResult{IsValid: false, Reason: fmt.Sprintf("unknown split direction: %s", direction)}
	}
}

// ValidateStackOperation validates if a pane can be converted to a stack
func (gv *GeometryValidator) ValidateStackOperation(node *paneNode) ValidationResult {
	geometry, result, ok := gv.prepareGeometryResult(node)
	if !ok {
		return result
	}

	if node.isStacked {
		result.IsValid = false
		result.Reason = "pane is already stacked"
		return result
	}

	minStackHeight := gv.minPaneHeight + 30 // Extra space for title bar
	if geometry.Height < minStackHeight {
		result.IsValid = false
		result.Reason = fmt.Sprintf("pane height %d < required %d for stacking",
			geometry.Height, minStackHeight)
		return result
	}

	if geometry.Width < gv.minPaneWidth {
		result.IsValid = false
		result.Reason = fmt.Sprintf("pane width %d < minimum %d",
			geometry.Width, gv.minPaneWidth)
		return result
	}

	result.IsValid = true
	result.Reason = "stack operation is valid"
	return result
}
