package coordinator

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/focus"
	"github.com/bnema/dumber/internal/ui/layout"
)

// WorkspaceCoordinator manages pane operations within a workspace.
type WorkspaceCoordinator struct {
	panesUC        *usecase.ManagePanesUseCase
	focusMgr       *focus.Manager
	stackedPaneMgr *component.StackedPaneManager
	widgetFactory  layout.WidgetFactory
	contentCoord   *ContentCoordinator

	// Callbacks to avoid circular dependencies
	getActiveWS     func() (*entity.Workspace, *component.WorkspaceView)
	generateID      func() string
	onCloseLastPane func(ctx context.Context) error
}

// WorkspaceCoordinatorConfig holds configuration for WorkspaceCoordinator.
type WorkspaceCoordinatorConfig struct {
	PanesUC        *usecase.ManagePanesUseCase
	FocusMgr       *focus.Manager
	StackedPaneMgr *component.StackedPaneManager
	WidgetFactory  layout.WidgetFactory
	ContentCoord   *ContentCoordinator
	GetActiveWS    func() (*entity.Workspace, *component.WorkspaceView)
	GenerateID     func() string
}

// NewWorkspaceCoordinator creates a new WorkspaceCoordinator.
func NewWorkspaceCoordinator(ctx context.Context, cfg WorkspaceCoordinatorConfig) *WorkspaceCoordinator {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating workspace coordinator")

	return &WorkspaceCoordinator{
		panesUC:        cfg.PanesUC,
		focusMgr:       cfg.FocusMgr,
		stackedPaneMgr: cfg.StackedPaneMgr,
		widgetFactory:  cfg.WidgetFactory,
		contentCoord:   cfg.ContentCoord,
		getActiveWS:    cfg.GetActiveWS,
		generateID:     cfg.GenerateID,
	}
}

// SetOnCloseLastPane sets the callback for when the last pane is closed.
func (c *WorkspaceCoordinator) SetOnCloseLastPane(fn func(ctx context.Context) error) {
	c.onCloseLastPane = fn
}

// Split splits the active pane in the given direction.
func (c *WorkspaceCoordinator) Split(ctx context.Context, direction usecase.SplitDirection) error {
	log := logging.FromContext(ctx)

	if c.panesUC == nil {
		log.Warn().Msg("panes use case not available")
		return nil
	}

	ws, wsView := c.getActiveWS()
	if ws == nil {
		log.Warn().Msg("no active workspace")
		return nil
	}

	activePane := ws.ActivePane()
	if activePane == nil {
		log.Warn().Msg("no active pane to split")
		return nil
	}

	// Check if we're splitting from inside a stack (horizontal split around stack)
	isStackSplit := (direction == usecase.SplitLeft || direction == usecase.SplitRight) &&
		activePane.Parent != nil && activePane.Parent.IsStacked

	// Get the existing widget BEFORE domain changes (for incremental operation)
	var existingWidget layout.Widget
	if wsView != nil {
		if isStackSplit {
			// Stack split: get the StackedView widget
			tr := wsView.TreeRenderer()
			if tr != nil {
				stackedView := tr.GetStackedViewForPane(string(activePane.Pane.ID))
				if stackedView != nil {
					existingWidget = stackedView.Widget()
				}
			}
		} else {
			// Regular split: get the current root widget
			existingWidget = wsView.GetRootWidget()
		}
	}

	output, err := c.panesUC.Split(ctx, usecase.SplitPaneInput{
		Workspace:  ws,
		TargetPane: activePane,
		Direction:  direction,
	})
	if err != nil {
		log.Error().Err(err).Str("direction", string(direction)).Msg("failed to split pane")
		return err
	}

	// Remember old active pane before changing
	oldActivePaneID := activePane.Pane.ID

	// Set the new pane as active
	ws.ActivePaneID = output.NewPaneNode.Pane.ID

	// Update the workspace view
	if wsView != nil && existingWidget != nil {
		var splitErr error
		if isStackSplit {
			// Incremental stack split: reuse existing stack widget
			splitErr = c.doIncrementalStackSplit(ctx, wsView, ws, output, direction, existingWidget, oldActivePaneID)
		} else {
			// Incremental regular split: reuse existing root widget
			splitErr = c.doIncrementalSplit(ctx, wsView, ws, output, direction, existingWidget, oldActivePaneID)
		}

		if splitErr != nil {
			log.Warn().Err(splitErr).Msg("incremental split failed, falling back to rebuild")
			// Fallback to full rebuild
			if err := wsView.Rebuild(ctx); err != nil {
				log.Error().Err(err).Msg("failed to rebuild workspace view")
			}
			c.contentCoord.AttachToWorkspace(ctx, ws, wsView)
		}
		// Update workspace view's active pane tracking
		if err := wsView.SetActivePaneID(ws.ActivePaneID); err != nil {
			log.Warn().Err(err).Msg("failed to set active pane in workspace view")
		}
		wsView.FocusPane(ws.ActivePaneID)
	} else if wsView != nil {
		// No existing widget (shouldn't happen), fallback to rebuild
		if err := wsView.Rebuild(ctx); err != nil {
			log.Error().Err(err).Msg("failed to rebuild workspace view")
		}
		c.contentCoord.AttachToWorkspace(ctx, ws, wsView)
		// Update workspace view's active pane tracking
		if err := wsView.SetActivePaneID(ws.ActivePaneID); err != nil {
			log.Warn().Err(err).Msg("failed to set active pane in workspace view")
		}
		wsView.FocusPane(ws.ActivePaneID)
	}

	log.Info().Str("direction", string(direction)).Str("new_pane_id", string(output.NewPaneNode.Pane.ID)).Msg("pane split completed")

	return nil
}

// doIncrementalStackSplit performs an incremental split around an existing stacked pane.
func (c *WorkspaceCoordinator) doIncrementalStackSplit(
	ctx context.Context,
	wsView *component.WorkspaceView,
	ws *entity.Workspace,
	output *usecase.SplitPaneOutput,
	direction usecase.SplitDirection,
	existingStackWidget layout.Widget,
	oldActivePaneID entity.PaneID,
) error {
	log := logging.FromContext(ctx)
	factory := wsView.Factory()

	log.Debug().
		Str("direction", string(direction)).
		Str("new_pane_id", string(output.NewPaneNode.Pane.ID)).
		Str("old_active_pane_id", string(oldActivePaneID)).
		Msg("performing incremental stack split")

	// 1. Deactivate the old active pane (in the stack)
	if oldPaneView := wsView.GetPaneView(oldActivePaneID); oldPaneView != nil {
		oldPaneView.SetActive(false)
	}

	// 2. Unparent the existing stack widget from the workspace container
	// Clear rootWidget ref first so SetRootWidgetDirect won't double-remove
	wsView.ClearRootWidgetRef()
	wsView.Container().Remove(existingStackWidget)

	// 3. Create new PaneView for the new pane (without WebView - will attach later)
	newPaneView := component.NewPaneView(factory, output.NewPaneNode.Pane.ID, nil)

	// 4. Wrap the new PaneView in a StackedView
	newStackedView := layout.NewStackedView(factory)
	newStackedView.AddPane(ctx, output.NewPaneNode.Pane.Title, "", newPaneView.Widget())

	// 5. Create a SplitView with the appropriate orientation and child placement
	var splitView *layout.SplitView
	if direction == usecase.SplitLeft {
		// New pane goes on the left, existing stack on the right
		splitView = layout.NewSplitView(ctx, factory, layout.OrientationHorizontal,
			newStackedView.Widget(), existingStackWidget, output.SplitRatio)
	} else {
		// New pane goes on the right, existing stack on the left (SplitRight)
		splitView = layout.NewSplitView(ctx, factory, layout.OrientationHorizontal,
			existingStackWidget, newStackedView.Widget(), output.SplitRatio)
	}

	// 6. Replace the root widget in the workspace view
	wsView.SetRootWidgetDirect(splitView.Widget())

	// 7. Register the new pane in tracking maps and activate it
	wsView.RegisterPaneView(output.NewPaneNode.Pane.ID, newPaneView)
	newPaneView.SetActive(true)

	tr := wsView.TreeRenderer()
	if tr != nil {
		// Register the new pane's StackedView mapping
		tr.RegisterPaneInStack(string(output.NewPaneNode.Pane.ID), newStackedView)
		// Register the split node widget
		tr.RegisterWidget(output.ParentNode.ID, splitView.Widget())
	}

	// 8. Attach WebView only for the new pane
	wv, err := c.contentCoord.EnsureWebView(ctx, output.NewPaneNode.Pane.ID)
	if err != nil {
		log.Warn().Err(err).Str("pane_id", string(output.NewPaneNode.Pane.ID)).Msg("failed to ensure webview for new pane")
		return err
	}

	// Load the default URL for new pane
	newPaneEntity := output.NewPaneNode.Pane
	if newPaneEntity.URI != "" {
		if err := wv.LoadURI(ctx, newPaneEntity.URI); err != nil {
			log.Warn().Err(err).Str("uri", newPaneEntity.URI).Msg("failed to load URI for new pane")
		}
	}

	widget := c.contentCoord.WrapWidget(ctx, wv)
	if widget != nil {
		newPaneView.SetWebViewWidget(widget)
	}

	log.Debug().Msg("incremental stack split completed successfully")
	return nil
}

// doIncrementalSplit performs an incremental split by reusing the existing widget tree.
// This avoids rebuilding the entire tree which would destroy existing WebView widgets.
func (c *WorkspaceCoordinator) doIncrementalSplit(
	ctx context.Context,
	wsView *component.WorkspaceView,
	ws *entity.Workspace,
	output *usecase.SplitPaneOutput,
	direction usecase.SplitDirection,
	existingRootWidget layout.Widget,
	oldActivePaneID entity.PaneID,
) error {
	log := logging.FromContext(ctx)
	factory := wsView.Factory()

	log.Debug().
		Str("direction", string(direction)).
		Str("new_pane_id", string(output.NewPaneNode.Pane.ID)).
		Str("old_active_pane_id", string(oldActivePaneID)).
		Int("pane_count", ws.PaneCount()).
		Msg("performing incremental split")

	// 1. Deactivate the old active pane
	if oldPaneView := wsView.GetPaneView(oldActivePaneID); oldPaneView != nil {
		oldPaneView.SetActive(false)
	}

	// 2. Get the active pane's StackedView widget (what we're actually splitting)
	tr := wsView.TreeRenderer()
	if tr == nil {
		return nil
	}

	activeStackedView := tr.GetStackedViewForPane(string(oldActivePaneID))
	if activeStackedView == nil {
		log.Warn().Msg("no stacked view found for active pane")
		return nil
	}
	activePaneWidget := activeStackedView.Widget()

	// 3. Determine if this is a root split by checking domain tree
	// output.ParentNode is the NEW split node (h0)
	// output.ParentNode's parent in domain tree is the ORIGINAL parent (f0) or nil if root
	grandparentNode := output.ParentNode.Parent
	isRootSplit := grandparentNode == nil

	log.Debug().
		Bool("is_root_split", isRootSplit).
		Str("grandparent_id", func() string {
			if grandparentNode != nil {
				return grandparentNode.ID
			}
			return "nil"
		}()).
		Msg("determined split type")

	// 4. Create new PaneView for the new pane
	newPaneView := component.NewPaneView(factory, output.NewPaneNode.Pane.ID, nil)

	// 5. Wrap the new PaneView in a StackedView
	newStackedView := layout.NewStackedView(factory)
	newStackedView.AddPane(ctx, output.NewPaneNode.Pane.Title, "", newPaneView.Widget())

	// 6. Determine orientation based on direction
	var orientation layout.Orientation
	var existingFirst bool
	switch direction {
	case usecase.SplitRight:
		orientation = layout.OrientationHorizontal
		existingFirst = true
	case usecase.SplitLeft:
		orientation = layout.OrientationHorizontal
		existingFirst = false
	case usecase.SplitDown:
		orientation = layout.OrientationVertical
		existingFirst = true
	case usecase.SplitUp:
		orientation = layout.OrientationVertical
		existingFirst = false
	default:
		return nil
	}

	// 7. Handle root vs non-root split differently
	if isRootSplit {
		// ROOT SPLIT: Remove from container, wrap in split, set as new root
		// Clear rootWidget ref first so SetRootWidgetDirect won't double-remove
		wsView.ClearRootWidgetRef()
		wsView.Container().Remove(existingRootWidget)

		var splitView *layout.SplitView
		if existingFirst {
			splitView = layout.NewSplitView(ctx, factory, orientation,
				existingRootWidget, newStackedView.Widget(), output.SplitRatio)
		} else {
			splitView = layout.NewSplitView(ctx, factory, orientation,
				newStackedView.Widget(), existingRootWidget, output.SplitRatio)
		}

		wsView.SetRootWidgetDirect(splitView.Widget())
		tr.RegisterWidget(output.ParentNode.ID, splitView.Widget())

		log.Debug().Msg("root split: replaced root with new split view")
	} else {
		// NON-ROOT SPLIT: Find grandparent's widget from TreeRenderer
		grandparentWidget := tr.Lookup(grandparentNode.ID)
		if grandparentWidget == nil {
			log.Warn().Str("grandparent_id", grandparentNode.ID).Msg("grandparent widget not found in TreeRenderer")
			return fmt.Errorf("grandparent widget not found")
		}

		// Cast to PanedWidget
		panedWidget, ok := grandparentWidget.(layout.PanedWidget)
		if !ok {
			log.Warn().Msg("grandparent widget is not a PanedWidget, falling back")
			return fmt.Errorf("grandparent is not a PanedWidget")
		}

		// Determine if active pane was in start or end position
		// The new split node (output.ParentNode) replaced the active pane's position
		// So we check if the new split node is Left or Right child of grandparent
		isStartChild := grandparentNode.Left() == output.ParentNode

		log.Debug().
			Bool("is_start_child", isStartChild).
			Str("grandparent_left_id", func() string {
				if grandparentNode.Left() != nil {
					return grandparentNode.Left().ID
				}
				return "nil"
			}()).
			Str("new_parent_id", output.ParentNode.ID).
			Msg("determined position in grandparent")

		// Remove the active pane widget from grandparent paned
		if isStartChild {
			panedWidget.SetStartChild(nil)
		} else {
			panedWidget.SetEndChild(nil)
		}

		// Create new split view with active pane + new pane
		var splitView *layout.SplitView
		if existingFirst {
			splitView = layout.NewSplitView(ctx, factory, orientation,
				activePaneWidget, newStackedView.Widget(), output.SplitRatio)
		} else {
			splitView = layout.NewSplitView(ctx, factory, orientation,
				newStackedView.Widget(), activePaneWidget, output.SplitRatio)
		}

		// Insert new split view in grandparent at same position
		if isStartChild {
			panedWidget.SetStartChild(splitView.Widget())
		} else {
			panedWidget.SetEndChild(splitView.Widget())
		}

		tr.RegisterWidget(output.ParentNode.ID, splitView.Widget())

		log.Debug().
			Bool("was_start_child", isStartChild).
			Msg("non-root split: replaced pane in grandparent with new split view")
	}

	// 8. Register the new pane in tracking maps and activate it
	wsView.RegisterPaneView(output.NewPaneNode.Pane.ID, newPaneView)
	newPaneView.SetActive(true)

	// Register the new pane's StackedView mapping
	tr.RegisterPaneInStack(string(output.NewPaneNode.Pane.ID), newStackedView)

	// 9. Attach WebView only for the new pane
	wv, err := c.contentCoord.EnsureWebView(ctx, output.NewPaneNode.Pane.ID)
	if err != nil {
		log.Warn().Err(err).Str("pane_id", string(output.NewPaneNode.Pane.ID)).Msg("failed to ensure webview for new pane")
		return err
	}

	// Load the default URL for new pane
	newPaneEntity := output.NewPaneNode.Pane
	if newPaneEntity.URI != "" {
		if err := wv.LoadURI(ctx, newPaneEntity.URI); err != nil {
			log.Warn().Err(err).Str("uri", newPaneEntity.URI).Msg("failed to load URI for new pane")
		}
	}

	widget := c.contentCoord.WrapWidget(ctx, wv)
	if widget != nil {
		newPaneView.SetWebViewWidget(widget)
	}

	log.Debug().Msg("incremental split completed successfully")
	return nil
}

// ClosePane closes the active pane.
func (c *WorkspaceCoordinator) ClosePane(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if c.panesUC == nil {
		log.Warn().Msg("panes use case not available")
		return nil
	}

	ws, wsView := c.getActiveWS()
	if ws == nil {
		log.Warn().Msg("no active workspace")
		return nil
	}

	activePane := ws.ActivePane()
	if activePane == nil {
		log.Warn().Msg("no active pane to close")
		return nil
	}
	closingPaneID := activePane.Pane.ID

	log.Debug().Str("pane_id", string(closingPaneID)).Msg("closing pane")

	// Don't close the last pane - close the tab instead
	if ws.PaneCount() <= 1 {
		if c.onCloseLastPane != nil {
			return c.onCloseLastPane(ctx)
		}
		return nil
	}

	// BEFORE domain changes: capture widget info for incremental close
	var parentNode *entity.PaneNode
	var siblingNode *entity.PaneNode
	var grandparentNode *entity.PaneNode
	var parentWidget layout.Widget
	var siblingIsStartChild bool  // true if sibling is the start (left/top) child in parent
	var parentIsStartInGrand bool // true if parent is the start (left/top) child in grandparent

	if wsView != nil {
		tr := wsView.TreeRenderer()
		if tr != nil && activePane.Parent != nil {
			parentNode = activePane.Parent

			// Find sibling (the other child of parent)
			// If closing pane is start child, sibling is end child (and vice versa)
			if parentNode.Left() == activePane {
				siblingNode = parentNode.Right()
				siblingIsStartChild = false // sibling is end/right child
			} else {
				siblingNode = parentNode.Left()
				siblingIsStartChild = true // sibling is start/left child
			}

			grandparentNode = parentNode.Parent
			parentWidget = tr.Lookup(parentNode.ID)

			// Determine parent's position in grandparent (if grandparent exists)
			if grandparentNode != nil {
				parentIsStartInGrand = grandparentNode.Left() == parentNode
			}

			log.Debug().
				Str("parent_id", parentNode.ID).
				Str("sibling_id", func() string {
					if siblingNode != nil {
						return siblingNode.ID
					}
					return "nil"
				}()).
				Bool("sibling_is_start", siblingIsStartChild).
				Bool("parent_is_start_in_grand", parentIsStartInGrand).
				Bool("has_grandparent", grandparentNode != nil).
				Bool("has_parent_widget", parentWidget != nil).
				Msg("captured close context")
		}
	}

	// Now do domain changes
	_, err := c.panesUC.Close(ctx, ws, activePane)
	if err != nil {
		log.Error().Err(err).Msg("failed to close pane")
		return err
	}

	// Try incremental close if we have the context
	if wsView != nil && parentNode != nil {
		var closeErr error

		if parentNode.IsStacked {
			// Stacked close: remove pane from stack without rebuild
			closeErr = c.doIncrementalStackClose(ctx, wsView, closingPaneID, parentNode)
		} else if siblingNode != nil && parentWidget != nil {
			// Split close: promote sibling without rebuild
			closeErr = c.doIncrementalClose(ctx, wsView, ws, closingPaneID, siblingNode, parentNode, grandparentNode, parentWidget, siblingIsStartChild, parentIsStartInGrand)
		} else {
			closeErr = fmt.Errorf("missing context for incremental close")
		}

		if closeErr != nil {
			log.Warn().Err(closeErr).Msg("incremental close failed, falling back to rebuild")
			// Fallback to full rebuild
			if err := wsView.Rebuild(ctx); err != nil {
				log.Error().Err(err).Msg("failed to rebuild workspace view")
			}
			c.contentCoord.ReleaseWebView(ctx, closingPaneID)
			c.contentCoord.AttachToWorkspace(ctx, ws, wsView)
		}
		// Update workspace view's active pane tracking
		if err := wsView.SetActivePaneID(ws.ActivePaneID); err != nil {
			log.Warn().Err(err).Msg("failed to set active pane in workspace view")
		}
		wsView.FocusPane(ws.ActivePaneID)
	} else if wsView != nil {
		// Fallback to rebuild if we couldn't get context
		if err := wsView.Rebuild(ctx); err != nil {
			log.Error().Err(err).Msg("failed to rebuild workspace view")
		}
		c.contentCoord.ReleaseWebView(ctx, closingPaneID)
		c.contentCoord.AttachToWorkspace(ctx, ws, wsView)
		// Update workspace view's active pane tracking
		if err := wsView.SetActivePaneID(ws.ActivePaneID); err != nil {
			log.Warn().Err(err).Msg("failed to set active pane in workspace view")
		}
		wsView.FocusPane(ws.ActivePaneID)
	}

	log.Info().Msg("pane closed")
	return nil
}

// doIncrementalClose performs incremental close by promoting sibling without rebuild.
func (c *WorkspaceCoordinator) doIncrementalClose(
	ctx context.Context,
	wsView *component.WorkspaceView,
	ws *entity.Workspace,
	closingPaneID entity.PaneID,
	siblingNode *entity.PaneNode,
	parentNode *entity.PaneNode,
	grandparentNode *entity.PaneNode,
	parentWidget layout.Widget,
	siblingIsStartChild bool, // true if sibling is start/left child in parent
	parentIsStartInGrand bool, // true if parent is start/left child in grandparent
) error {
	log := logging.FromContext(ctx)
	tr := wsView.TreeRenderer()
	if tr == nil {
		return fmt.Errorf("tree renderer not available")
	}

	log.Debug().
		Str("closing_pane", string(closingPaneID)).
		Str("sibling_id", siblingNode.ID).
		Bool("sibling_is_leaf", siblingNode.IsLeaf()).
		Bool("sibling_is_start", siblingIsStartChild).
		Bool("parent_is_start_in_grand", parentIsStartInGrand).
		Msg("performing incremental close")

	// Get sibling's widget
	var siblingWidget layout.Widget
	if siblingNode.IsLeaf() && siblingNode.Pane != nil {
		stackedView := tr.GetStackedViewForPane(string(siblingNode.Pane.ID))
		if stackedView != nil {
			siblingWidget = stackedView.Widget()
		}
	} else {
		// Sibling is a split node
		siblingWidget = tr.Lookup(siblingNode.ID)
	}

	if siblingWidget == nil {
		return fmt.Errorf("sibling widget not found")
	}

	// Cast parent widget to PanedWidget
	panedWidget, ok := parentWidget.(layout.PanedWidget)
	if !ok {
		return fmt.Errorf("parent widget is not a PanedWidget")
	}

	// Remove BOTH children from parent paned before any reparenting
	// This is critical - GTK requires widgets to be unparented before reparenting
	// Order: unparent closing pane first, then sibling
	if siblingIsStartChild {
		// Sibling is start, closing pane is end
		panedWidget.SetEndChild(nil)   // Unparent closing pane
		panedWidget.SetStartChild(nil) // Unparent sibling
	} else {
		// Sibling is end, closing pane is start
		panedWidget.SetStartChild(nil) // Unparent closing pane
		panedWidget.SetEndChild(nil)   // Unparent sibling
	}

	if grandparentNode == nil {
		// Sibling becomes new root
		// Remove the old root (parent paned) from container manually since
		// ClearRootWidgetRef only clears the reference, not the actual widget
		wsView.Container().Remove(parentWidget)
		wsView.ClearRootWidgetRef()

		// Grab focus on sibling before reparenting to avoid GTK focus warnings
		siblingWidget.GrabFocus()

		// Ensure sibling expands to fill container (was constrained by paned before)
		siblingWidget.SetHexpand(true)
		siblingWidget.SetVexpand(true)

		wsView.SetRootWidgetDirect(siblingWidget)

		log.Debug().Msg("sibling promoted to root")
	} else {
		// Replace parent with sibling in grandparent
		grandparentWidget := tr.Lookup(grandparentNode.ID)
		if grandparentWidget == nil {
			return fmt.Errorf("grandparent widget not found")
		}

		grandPaned, ok := grandparentWidget.(layout.PanedWidget)
		if !ok {
			return fmt.Errorf("grandparent widget is not a PanedWidget")
		}

		// Grab focus on sibling before reparenting to avoid GTK focus warnings
		siblingWidget.GrabFocus()

		// Remove parent from grandparent using known position from domain tree
		if parentIsStartInGrand {
			grandPaned.SetStartChild(nil)
		} else {
			grandPaned.SetEndChild(nil)
		}

		// Insert sibling at same position
		if parentIsStartInGrand {
			grandPaned.SetStartChild(siblingWidget)
		} else {
			grandPaned.SetEndChild(siblingWidget)
		}

		log.Debug().Bool("was_start_child", parentIsStartInGrand).Msg("sibling promoted in grandparent")
	}

	// Clean up tracking
	tr.UnregisterWidget(parentNode.ID)
	wsView.UnregisterPaneView(closingPaneID)
	tr.UnregisterPane(string(closingPaneID))

	// Release the closing pane's webview
	c.contentCoord.ReleaseWebView(ctx, closingPaneID)

	// Activate sibling
	if siblingNode.IsLeaf() && siblingNode.Pane != nil {
		if siblingPaneView := wsView.GetPaneView(siblingNode.Pane.ID); siblingPaneView != nil {
			siblingPaneView.SetActive(true)
		}
	}

	log.Debug().Msg("incremental close completed successfully")
	return nil
}

// doIncrementalStackClose removes a pane from a stack without rebuilding the entire tree.
func (c *WorkspaceCoordinator) doIncrementalStackClose(
	ctx context.Context,
	wsView *component.WorkspaceView,
	closingPaneID entity.PaneID,
	stackNode *entity.PaneNode,
) error {
	log := logging.FromContext(ctx)
	tr := wsView.TreeRenderer()
	if tr == nil {
		return fmt.Errorf("tree renderer not available")
	}

	log.Debug().
		Str("closing_pane", string(closingPaneID)).
		Str("stack_node", stackNode.ID).
		Int("stack_children", len(stackNode.Children)).
		Msg("performing incremental stack close")

	// Get the StackedView for this pane
	stackedView := tr.GetStackedViewForPane(string(closingPaneID))
	if stackedView == nil {
		return fmt.Errorf("stacked view not found for pane %s", closingPaneID)
	}

	// Find the index of the closing pane in the stack
	closingIndex := -1
	for i, child := range stackNode.Children {
		if child.Pane != nil && child.Pane.ID == closingPaneID {
			closingIndex = i
			break
		}
	}

	if closingIndex < 0 {
		return fmt.Errorf("pane %s not found in stack", closingPaneID)
	}

	// Remove the pane from the StackedView widget
	if err := stackedView.RemovePane(ctx, closingIndex); err != nil {
		return fmt.Errorf("failed to remove pane from stacked view: %w", err)
	}

	// Clean up tracking
	wsView.UnregisterPaneView(closingPaneID)
	tr.UnregisterPane(string(closingPaneID))

	// Release the closing pane's webview
	c.contentCoord.ReleaseWebView(ctx, closingPaneID)

	log.Debug().
		Int("removed_index", closingIndex).
		Msg("incremental stack close completed successfully")
	return nil
}

// FocusPane navigates focus to an adjacent pane.
func (c *WorkspaceCoordinator) FocusPane(ctx context.Context, direction usecase.NavigateDirection) error {
	log := logging.FromContext(ctx)

	ws, wsView := c.getActiveWS()
	if ws == nil {
		log.Warn().Msg("no active workspace")
		return nil
	}

	if wsView == nil {
		log.Warn().Msg("no active workspace view")
		return nil
	}

	// Use geometric navigation if focus manager is available
	var newPane *entity.PaneNode
	var err error

	if c.focusMgr != nil {
		newPane, err = c.focusMgr.NavigateGeometric(ctx, ws, wsView, direction)
	} else if c.panesUC != nil {
		// Fallback to structural navigation
		newPane, err = c.panesUC.NavigateFocus(ctx, ws, direction)
	} else {
		log.Warn().Msg("no navigation manager available")
		return nil
	}

	if err != nil {
		log.Error().Err(err).Str("direction", string(direction)).Msg("failed to navigate focus")
		return err
	}

	if newPane == nil {
		log.Debug().Str("direction", string(direction)).Msg("no pane in that direction")
		return nil
	}

	// Update the workspace view's active pane
	if err := wsView.SetActivePaneID(newPane.Pane.ID); err != nil {
		log.Warn().Err(err).Msg("failed to update active pane in view")
	} else {
		wsView.FocusPane(newPane.Pane.ID)
	}

	// Sync StackedView visibility if new pane is in a stack
	if newPane.Parent != nil && newPane.Parent.IsStacked {
		c.syncStackedViewActive(ctx, wsView, newPane)
	}

	log.Debug().Str("direction", string(direction)).Str("new_pane_id", newPane.ID).Msg("focus navigated")

	return nil
}

// syncStackedViewActive updates the StackedView's visibility to match the domain model.
func (c *WorkspaceCoordinator) syncStackedViewActive(ctx context.Context, wsView *component.WorkspaceView, paneNode *entity.PaneNode) {
	log := logging.FromContext(ctx)

	if paneNode == nil || paneNode.Parent == nil || !paneNode.Parent.IsStacked {
		return
	}

	tr := wsView.TreeRenderer()
	if tr == nil {
		return
	}

	// Get the StackedView for this pane
	stackedView := tr.GetStackedViewForPane(string(paneNode.Pane.ID))
	if stackedView == nil {
		log.Warn().Str("pane_id", string(paneNode.Pane.ID)).Msg("stacked view not found for pane")
		return
	}

	// Use the parent's ActiveStackIndex which was set by the focus manager
	stackIndex := paneNode.Parent.ActiveStackIndex

	log.Debug().
		Str("pane_id", string(paneNode.Pane.ID)).
		Int("stack_index", stackIndex).
		Msg("syncing stacked view visibility")

	if err := stackedView.SetActive(ctx, stackIndex); err != nil {
		log.Warn().Err(err).Msg("failed to set stacked view active index")
	}
}

// StackPane adds a new pane stacked on top of the active pane.
func (c *WorkspaceCoordinator) StackPane(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if c.stackedPaneMgr == nil {
		log.Warn().Msg("stacked pane manager not available")
		return nil
	}

	ws, wsView := c.getActiveWS()
	if ws == nil {
		log.Warn().Msg("no active workspace")
		return nil
	}

	activeNode := ws.ActivePane()
	if activeNode == nil || activeNode.Pane == nil {
		log.Warn().Msg("no active pane")
		return nil
	}

	if wsView == nil {
		log.Warn().Msg("no workspace view")
		return nil
	}

	activePaneID := activeNode.Pane.ID

	// Create a new pane entity
	newPaneID := entity.PaneID(c.generateID())
	newPane := entity.NewPane(newPaneID)
	newPane.URI = "about:blank"
	newPane.Title = "Untitled"

	// Get the original pane's current title
	originalTitle := c.contentCoord.GetTitle(activePaneID)
	if originalTitle == "" {
		originalTitle = activeNode.Pane.Title
	}
	if originalTitle == "" {
		originalTitle = "Untitled"
	}

	// Update domain model: convert leaf to stacked if needed, add new pane
	var stackNode *entity.PaneNode
	var needsFirstPaneTitleUpdate bool
	if activeNode.IsStacked {
		// Already stacked, just add to it
		stackNode = activeNode
	} else {
		// Convert leaf node to stacked container
		originalPane := activeNode.Pane
		originalPane.Title = originalTitle
		originalPaneChild := &entity.PaneNode{
			ID:     activeNode.ID + "_0",
			Pane:   originalPane,
			Parent: activeNode,
		}

		activeNode.Pane = nil
		activeNode.IsStacked = true
		activeNode.Children = []*entity.PaneNode{originalPaneChild}
		stackNode = activeNode
		needsFirstPaneTitleUpdate = true

		log.Debug().
			Str("node_id", activeNode.ID).
			Str("original_pane", string(originalPane.ID)).
			Str("original_title", originalTitle).
			Msg("converted leaf to stacked node")
	}

	// Create new child node for the new pane
	newChildNode := &entity.PaneNode{
		ID:     stackNode.ID + "_" + string(newPaneID),
		Pane:   newPane,
		Parent: stackNode,
	}
	stackNode.Children = append(stackNode.Children, newChildNode)
	stackNode.ActiveStackIndex = len(stackNode.Children) - 1

	log.Debug().
		Int("stack_size", len(stackNode.Children)).
		Int("active_index", stackNode.ActiveStackIndex).
		Msg("domain tree updated")

	// Create PaneView for the new pane
	newPaneView := component.NewPaneView(c.widgetFactory, newPaneID, nil)
	wsView.RegisterPaneView(newPaneID, newPaneView)

	// Add to the UI StackedView
	if err := c.stackedPaneMgr.AddPaneToStack(ctx, wsView, activePaneID, newPaneView, "Untitled"); err != nil {
		log.Error().Err(err).Msg("failed to add pane to stack")
		return err
	}

	// Update the first pane's title if we just converted from leaf to stacked
	if needsFirstPaneTitleUpdate {
		tr := wsView.TreeRenderer()
		if tr != nil {
			stackedView := tr.GetStackedViewForPane(string(activePaneID))
			if stackedView != nil {
				if err := stackedView.UpdateTitle(0, originalTitle); err != nil {
					log.Warn().Err(err).Str("title", originalTitle).Msg("failed to update first pane title")
				} else {
					log.Debug().Str("title", originalTitle).Msg("updated first pane title in StackedView")
				}
			}
		}
	}

	// Get WebView and attach
	wv, err := c.contentCoord.EnsureWebView(ctx, newPaneID)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get webview for new pane")
	} else {
		widget := c.contentCoord.WrapWidget(ctx, wv)
		if widget != nil {
			newPaneView.SetWebViewWidget(widget)
		}
		// Load blank page
		if err := wv.LoadURI(ctx, "about:blank"); err != nil {
			log.Warn().Err(err).Msg("failed to load blank page")
		}
	}

	// Update workspace active pane ID
	ws.ActivePaneID = newPaneID

	// Update workspace view
	if err := wsView.SetActivePaneID(newPaneID); err != nil {
		log.Warn().Err(err).Msg("failed to set active pane")
	}

	// Set up title bar click callback
	tr := wsView.TreeRenderer()
	if tr != nil {
		stackedView := tr.GetStackedViewForPane(string(activePaneID))
		if stackedView != nil {
			capturedStackNode := stackNode
			stackedView.SetOnActivate(func(index int) {
				c.onTitleBarClick(ctx, capturedStackNode, stackedView, index)
			})
		}
	}

	log.Info().
		Str("original_pane", string(activePaneID)).
		Str("new_pane", string(newPaneID)).
		Int("stack_size", len(stackNode.Children)).
		Msg("stacked new pane")

	return nil
}

// NavigateStack navigates up or down within a stacked pane container.
func (c *WorkspaceCoordinator) NavigateStack(ctx context.Context, direction string) error {
	log := logging.FromContext(ctx)

	if c.stackedPaneMgr == nil {
		return nil
	}

	ws, wsView := c.getActiveWS()
	if ws == nil || wsView == nil {
		return nil
	}

	activePane := ws.ActivePane()
	if activePane == nil || activePane.Pane == nil {
		return nil
	}

	currentPaneID := activePane.Pane.ID

	// Check if we're actually in a multi-pane stack
	if !c.stackedPaneMgr.IsStacked(wsView, currentPaneID) {
		log.Debug().Msg("current pane is not in a multi-pane stack")
		return nil
	}

	// Navigate within the stack
	_, err := c.stackedPaneMgr.NavigateStack(ctx, wsView, currentPaneID, direction)
	if err != nil {
		log.Warn().Err(err).Str("direction", direction).Msg("failed to navigate stack")
		return err
	}

	log.Debug().
		Str("direction", direction).
		Str("current_pane", string(currentPaneID)).
		Msg("navigated stack")

	return nil
}

// onTitleBarClick handles clicks on title bars to switch the active pane in a stack.
func (c *WorkspaceCoordinator) onTitleBarClick(ctx context.Context, stackNode *entity.PaneNode, sv *layout.StackedView, clickedIndex int) {
	log := logging.FromContext(ctx)

	if stackNode == nil || sv == nil {
		return
	}

	// Validate index
	if clickedIndex < 0 || clickedIndex >= len(stackNode.Children) {
		log.Warn().Int("index", clickedIndex).Int("children", len(stackNode.Children)).Msg("invalid stack index clicked")
		return
	}

	// Get the current active index
	currentIndex := sv.ActiveIndex()
	if clickedIndex == currentIndex {
		log.Debug().Int("index", clickedIndex).Msg("clicked pane is already active")
		return
	}

	// Update the outgoing pane's title bar with its current webpage title
	if currentIndex >= 0 && currentIndex < len(stackNode.Children) {
		outgoingChild := stackNode.Children[currentIndex]
		if outgoingChild.Pane != nil {
			outgoingPaneID := outgoingChild.Pane.ID
			outgoingTitle := c.contentCoord.GetTitle(outgoingPaneID)
			if outgoingTitle == "" {
				outgoingTitle = outgoingChild.Pane.Title
			}
			if outgoingTitle != "" {
				if err := sv.UpdateTitle(currentIndex, outgoingTitle); err != nil {
					log.Warn().Err(err).Msg("failed to update outgoing pane title")
				}
			}
		}
	}

	// Get the pane ID at the clicked index
	clickedChild := stackNode.Children[clickedIndex]
	if clickedChild.Pane == nil {
		log.Warn().Int("index", clickedIndex).Msg("clicked child has no pane")
		return
	}
	clickedPaneID := clickedChild.Pane.ID

	// Update StackedView active index
	if err := sv.SetActive(ctx, clickedIndex); err != nil {
		log.Warn().Err(err).Int("index", clickedIndex).Msg("failed to set active pane in stack")
		return
	}

	// Update domain model
	stackNode.ActiveStackIndex = clickedIndex

	// Update workspace active pane
	ws, wsView := c.getActiveWS()
	if ws != nil {
		ws.ActivePaneID = clickedPaneID
	}

	// Update workspace view
	if wsView != nil {
		if err := wsView.SetActivePaneID(clickedPaneID); err != nil {
			log.Warn().Err(err).Msg("failed to set active pane in workspace view")
		}
	}

	log.Info().
		Int("from_index", currentIndex).
		Int("to_index", clickedIndex).
		Str("pane_id", string(clickedPaneID)).
		Msg("switched active pane via title bar click")
}
