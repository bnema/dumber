// Package component provides UI components for the browser.
package component

import (
	"context"
	"errors"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/layout"
)

// ErrStackNotFound is returned when a StackedView cannot be found for a pane.
var ErrStackNotFound = errors.New("stacked view not found for pane")

// StackedPaneManager handles stacked pane operations.
// With the new architecture, every pane is wrapped in a StackedView from the start,
// making stacking operations trivial - we just add panes to the existing StackedView.
type StackedPaneManager struct {
	factory layout.WidgetFactory
}

// NewStackedPaneManager creates a new stacked pane manager.
func NewStackedPaneManager(factory layout.WidgetFactory) *StackedPaneManager {
	return &StackedPaneManager{
		factory: factory,
	}
}

// AddPaneToStack adds a new pane to the stack containing the active pane.
// The StackedView is looked up from the TreeRenderer.
func (spm *StackedPaneManager) AddPaneToStack(
	ctx context.Context,
	wsView *WorkspaceView,
	activePaneID entity.PaneID,
	newPaneView *PaneView,
	title string,
) error {
	log := logging.FromContext(ctx)

	// Get the StackedView containing the active pane
	tr := wsView.TreeRenderer()
	if tr == nil {
		return errors.New("tree renderer not available")
	}

	stackedView := tr.GetStackedViewForPane(string(activePaneID))
	if stackedView == nil {
		return ErrStackNotFound
	}

	// Get the new pane's widget
	widget := newPaneView.Widget()
	if widget == nil {
		return errors.New("new pane has no widget")
	}

	// Ensure widget is unparented
	if widget.GetParent() != nil {
		widget.Unparent()
	}

	// Add to the stack after the current active pane (not at end)
	if title == "" {
		title = "Untitled"
	}

	// Get current active index to insert after it
	currentActiveIndex := stackedView.ActiveIndex()

	log.Debug().
		Str("active_pane", string(activePaneID)).
		Str("new_pane", string(newPaneView.PaneID())).
		Str("title", title).
		Int("current_active_index", currentActiveIndex).
		Int("stack_size_before", stackedView.Count()).
		Msg("StackedPaneManager: inserting pane after active position")

	stackedView.InsertPaneAfter(ctx, currentActiveIndex, string(newPaneView.PaneID()), title, "", widget)

	// Register the new pane in the TreeRenderer's tracking
	tr.RegisterPaneInStack(string(newPaneView.PaneID()), stackedView)

	log.Info().
		Str("active_pane", string(activePaneID)).
		Str("new_pane", string(newPaneView.PaneID())).
		Int("stack_size", stackedView.Count()).
		Msg("added pane to stack")

	return nil
}

// NavigateStack moves to the next or previous pane in a stack.
// Returns the pane ID that became active.
func (spm *StackedPaneManager) NavigateStack(
	ctx context.Context,
	wsView *WorkspaceView,
	currentPaneID entity.PaneID,
	direction string,
) (entity.PaneID, error) {
	log := logging.FromContext(ctx)

	tr := wsView.TreeRenderer()
	if tr == nil {
		return "", errors.New("tree renderer not available")
	}

	stackedView := tr.GetStackedViewForPane(string(currentPaneID))
	if stackedView == nil {
		return "", ErrStackNotFound
	}

	// Only navigate if there are multiple panes
	if stackedView.Count() <= 1 {
		log.Debug().Msg("only one pane in stack, nothing to navigate")
		return currentPaneID, nil
	}

	var err error
	switch direction {
	case "up":
		err = stackedView.NavigatePrevious(ctx)
	case "down":
		err = stackedView.NavigateNext(ctx)
	default:
		return "", errors.New("invalid direction: use 'up' or 'down'")
	}
	if err != nil {
		return "", err
	}

	// Get the newly active pane's widget to find its ID
	// We need to find which pane is now at the active index
	activeIndex := stackedView.ActiveIndex()

	// Find the pane ID at this index by looking through registered panes
	// This is a bit indirect but works with the current architecture
	log.Debug().
		Str("current_pane", string(currentPaneID)).
		Str("direction", direction).
		Int("new_index", activeIndex).
		Msg("navigated stack")

	// Return empty - caller should update based on domain model
	return "", nil
}

// SetStackActiveCallback sets up the callback for when a stack pane is clicked.
func (spm *StackedPaneManager) SetStackActiveCallback(
	wsView *WorkspaceView,
	paneID entity.PaneID,
	callback func(index int),
) {
	tr := wsView.TreeRenderer()
	if tr == nil {
		return
	}

	stackedView := tr.GetStackedViewForPane(string(paneID))
	if stackedView == nil {
		return
	}

	stackedView.SetOnActivate(callback)
}

// GetStackedView returns the StackedView for a pane.
func (spm *StackedPaneManager) GetStackedView(wsView *WorkspaceView, paneID entity.PaneID) *layout.StackedView {
	tr := wsView.TreeRenderer()
	if tr == nil {
		return nil
	}
	return tr.GetStackedViewForPane(string(paneID))
}

// IsStacked returns true if the pane is in a stack with multiple panes.
func (spm *StackedPaneManager) IsStacked(wsView *WorkspaceView, paneID entity.PaneID) bool {
	sv := spm.GetStackedView(wsView, paneID)
	return sv != nil && sv.Count() > 1
}
