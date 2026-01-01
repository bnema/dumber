// Package focus provides focus state management and geometric navigation for panes.
package focus

import (
	"context"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/layout"
)

// PaneGeometryProvider provides geometry information for panes.
// This interface allows the focus manager to work without direct component dependencies.
type PaneGeometryProvider interface {
	// GetPaneIDs returns all pane IDs in the provider.
	GetPaneIDs() []entity.PaneID
	// GetPaneWidget returns the widget for a pane, or nil if not found.
	GetPaneWidget(paneID entity.PaneID) layout.Widget
	// GetStackContainerWidget returns the stack container widget for a stacked pane.
	// Returns nil if the pane is not in a stack.
	GetStackContainerWidget(paneID entity.PaneID) layout.Widget
	// ContainerWidget returns the container widget for relative positioning.
	ContainerWidget() layout.Widget
}

// Manager handles focus state and geometric navigation.
type Manager struct {
	panesUC *usecase.ManagePanesUseCase
}

// NewManager creates a focus manager.
func NewManager(panesUC *usecase.ManagePanesUseCase) *Manager {
	return &Manager{panesUC: panesUC}
}

// CollectPaneRects gathers geometry from all visible panes.
// For panes in stacks, uses the stack container's geometry since individual
// collapsed panes have no allocated size.
// Only visible panes are included - for stacked panes, only the active one is visible.
func (m *Manager) CollectPaneRects(ctx context.Context, provider PaneGeometryProvider) []entity.PaneRect {
	log := logging.FromContext(ctx)
	var rects []entity.PaneRect

	container := provider.ContainerWidget()

	allPaneIDs := provider.GetPaneIDs()
	log.Debug().Int("total_panes", len(allPaneIDs)).Msg("CollectPaneRects: starting")

	for _, paneID := range allPaneIDs {
		paneWidget := provider.GetPaneWidget(paneID)

		// Skip panes that aren't visible (includes inactive stacked panes)
		if paneWidget == nil || !paneWidget.IsVisible() {
			log.Debug().
				Str("pane_id", string(paneID)).
				Bool("widget_nil", paneWidget == nil).
				Msg("CollectPaneRects: skipping invisible pane")
			continue
		}

		// For stacked panes, use stack container geometry (pane itself has no size)
		var widget layout.Widget
		if stackWidget := provider.GetStackContainerWidget(paneID); stackWidget != nil {
			widget = stackWidget
		} else {
			widget = paneWidget
		}

		if widget == nil || !widget.IsVisible() {
			continue
		}

		// Get position relative to container
		x, y, ok := widget.ComputePoint(container)
		if !ok {
			continue
		}

		w := widget.GetAllocatedWidth()
		h := widget.GetAllocatedHeight()

		// Skip widgets with no size (collapsed/hidden)
		if w <= 0 || h <= 0 {
			continue
		}

		log.Debug().
			Str("pane_id", string(paneID)).
			Int("x", int(x)).
			Int("y", int(y)).
			Int("w", w).
			Int("h", h).
			Msg("CollectPaneRects: added visible pane")

		rects = append(rects, entity.PaneRect{
			PaneID: paneID,
			X:      int(x),
			Y:      int(y),
			W:      w,
			H:      h,
		})
	}

	log.Debug().Int("visible_panes", len(rects)).Msg("CollectPaneRects: done")

	return rects
}

// NavigateGeometric performs geometric navigation and returns the target pane.
// It handles stack navigation internally: when navigating up/down and the current
// pane is in a stack, it will try to navigate within the stack first before
// escaping to external panes.
func (m *Manager) NavigateGeometric(
	ctx context.Context,
	ws *entity.Workspace,
	provider PaneGeometryProvider,
	direction usecase.NavigateDirection,
) (*entity.PaneNode, error) {
	log := logging.FromContext(ctx)

	if ws == nil {
		return nil, nil
	}

	// Get active pane node
	activeNode := ws.ActivePane()
	if activeNode == nil {
		log.Debug().Msg("no active pane for navigation")
		return nil, nil
	}

	// Check if we should navigate within a stack first
	if direction == usecase.NavUp || direction == usecase.NavDown {
		if activeNode.Parent != nil && activeNode.Parent.IsStacked {
			// Try to navigate within the stack
			stackNode := activeNode.Parent
			canNavigate, newPaneID := m.navigateWithinStack(stackNode, activeNode, direction)
			if canNavigate {
				ws.ActivePaneID = newPaneID
				return ws.FindPane(newPaneID), nil
			}
			// At stack boundary - fall through to geometric navigation
			log.Debug().Msg("at stack boundary, using geometric navigation")
		}
	}

	// Collect geometry from visible panes
	rects := m.CollectPaneRects(ctx, provider)
	if len(rects) == 0 {
		return nil, nil
	}

	// Run geometric algorithm
	output, err := m.panesUC.NavigateFocusGeometric(ctx, usecase.GeometricNavigationInput{
		ActivePaneID: ws.ActivePaneID,
		PaneRects:    rects,
		Direction:    direction,
	})
	if err != nil {
		return nil, err
	}
	if !output.Found {
		return nil, nil
	}

	// Update workspace
	ws.ActivePaneID = output.TargetPaneID

	// Find target node and update stack index if target is in a stack
	targetNode := ws.FindPane(output.TargetPaneID)
	if targetNode != nil && targetNode.Parent != nil && targetNode.Parent.IsStacked {
		// Update the stack's ActiveStackIndex to match the target pane
		oldIndex := targetNode.Parent.ActiveStackIndex
		for i, child := range targetNode.Parent.Children {
			if child == targetNode {
				targetNode.Parent.ActiveStackIndex = i
				log.Debug().
					Str("target_pane_id", string(output.TargetPaneID)).
					Int("old_stack_index", oldIndex).
					Int("new_stack_index", i).
					Int("num_children", len(targetNode.Parent.Children)).
					Msg("updated stack index for geometric nav into stack")
				break
			}
		}
	}

	return targetNode, nil
}

// navigateWithinStack tries to navigate within a stack.
// Returns (canNavigate, newPaneID). If canNavigate is false, the pane is at a
// boundary and navigation should escape the stack.
func (m *Manager) navigateWithinStack(
	stackNode, currentNode *entity.PaneNode, direction usecase.NavigateDirection,
) (bool, entity.PaneID) {
	if !stackNode.IsStacked || len(stackNode.Children) == 0 {
		return false, ""
	}

	currentIdx := m.findCurrentStackIndex(stackNode, currentNode)
	if currentIdx < 0 {
		return false, ""
	}

	newIdx, ok := m.calculateNewStackIndex(currentIdx, len(stackNode.Children), direction)
	if !ok {
		return false, ""
	}

	stackNode.ActiveStackIndex = newIdx
	return m.getPaneIDFromStackChild(stackNode.Children[newIdx])
}

// findCurrentStackIndex returns the current pane's index in the stack.
// Uses ActiveStackIndex from domain model, with fallback to pointer search.
func (*Manager) findCurrentStackIndex(stackNode, currentNode *entity.PaneNode) int {
	idx := stackNode.ActiveStackIndex
	if idx >= 0 && idx < len(stackNode.Children) {
		return idx
	}
	// Fallback to pointer search if ActiveStackIndex is invalid
	for i, child := range stackNode.Children {
		if child == currentNode {
			return i
		}
	}
	return -1
}

// calculateNewStackIndex computes the new index after navigation.
// Returns (newIndex, ok) where ok is false if at a boundary.
func (*Manager) calculateNewStackIndex(
	currentIdx, childCount int, direction usecase.NavigateDirection,
) (int, bool) {
	switch direction {
	case usecase.NavUp:
		if currentIdx <= 0 {
			return 0, false
		}
		return currentIdx - 1, true
	case usecase.NavDown:
		if currentIdx >= childCount-1 {
			return 0, false
		}
		return currentIdx + 1, true
	default:
		return 0, false
	}
}

// getPaneIDFromStackChild extracts the pane ID from a stack child node.
// If the child is not a leaf, finds the first leaf pane.
func (*Manager) getPaneIDFromStackChild(targetNode *entity.PaneNode) (bool, entity.PaneID) {
	if targetNode.Pane != nil {
		return true, targetNode.Pane.ID
	}

	var leafPaneID entity.PaneID
	targetNode.Walk(func(n *entity.PaneNode) bool {
		if n.IsLeaf() && n.Pane != nil {
			leafPaneID = n.Pane.ID
			return false
		}
		return true
	})

	if leafPaneID != "" {
		return true, leafPaneID
	}
	return false, ""
}
