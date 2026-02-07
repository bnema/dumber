package coordinator

import (
	"context"
	"errors"
	"fmt"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	domainurl "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/focus"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/rs/zerolog"
)

const (
	defaultPaneTitle = "Untitled"
	nilString        = "nil"
)

// WorkspaceCoordinator manages pane operations within a workspace.
type WorkspaceCoordinator struct {
	panesUC        *usecase.ManagePanesUseCase
	focusMgr       *focus.Manager
	stackedPaneMgr *component.StackedPaneManager
	widgetFactory  layout.WidgetFactory
	contentCoord   *ContentCoordinator

	// Callbacks to avoid circular dependencies
	getActiveWS      func() (*entity.Workspace, *component.WorkspaceView)
	generateID       func() string
	onCloseLastPane  func(ctx context.Context) error
	onCreatePopupTab func(ctx context.Context, input InsertPopupInput) error // For tabbed popup behavior
	onStateChanged   func()                                                  // For session snapshots
	onPaneClosed     func(paneID entity.PaneID)                              // For pane-specific cleanup hooks
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

type splitContext struct {
	ws             *entity.Workspace
	wsView         *component.WorkspaceView
	activePane     *entity.PaneNode
	existingWidget layout.Widget
	isStackSplit   bool
}

type stackPaneContext struct {
	ws            *entity.Workspace
	wsView        *component.WorkspaceView
	activeNode    *entity.PaneNode
	activePaneID  entity.PaneID
	originalTitle string
}

type incrementalCloseContext struct {
	parentNode           *entity.PaneNode
	siblingNode          *entity.PaneNode
	grandparentNode      *entity.PaneNode
	parentWidget         layout.Widget
	siblingIsStartChild  bool
	parentIsStartInGrand bool
	precheckReason       string
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

// SetOnStateChanged sets the callback for when workspace state changes (for session snapshots).
func (c *WorkspaceCoordinator) SetOnStateChanged(fn func()) {
	c.onStateChanged = fn
}

// SetOnPaneClosed sets a callback invoked after a pane is successfully closed.
func (c *WorkspaceCoordinator) SetOnPaneClosed(fn func(paneID entity.PaneID)) {
	c.onPaneClosed = fn
}

// notifyStateChanged triggers the state changed callback if set.
func (c *WorkspaceCoordinator) notifyStateChanged() {
	if c.onStateChanged != nil {
		c.onStateChanged()
	}
}

// setupPaneViewHover configures hover-to-focus behavior on a PaneView.
func setupPaneViewHover(ctx context.Context, pv *component.PaneView, wsView *component.WorkspaceView) {
	pv.SetOnHover(func(paneID entity.PaneID) {
		// Skip if this pane is already active
		if wsView.GetActivePaneID() == paneID {
			return
		}

		// Activate the hovered pane and grab focus
		if err := wsView.SetActivePaneID(paneID); err == nil {
			wsView.FocusPane(paneID)
		}
	})

	pv.AttachHoverHandler(ctx)
}

// Split splits the active pane in the given direction.
func (c *WorkspaceCoordinator) Split(ctx context.Context, direction usecase.SplitDirection) error {
	log := logging.FromContext(ctx)

	splitCtx, ok := c.prepareSplit(ctx, direction)
	if !ok {
		return nil
	}

	output, err := c.panesUC.Split(ctx, usecase.SplitPaneInput{
		Workspace:  splitCtx.ws,
		TargetPane: splitCtx.activePane,
		Direction:  direction,
		InitialURL: domainurl.Normalize(config.Get().Workspace.NewPaneURL),
	})
	if err != nil {
		log.Error().Err(err).Str("direction", string(direction)).Msg("failed to split pane")
		return err
	}

	// Remember old active pane before changing
	oldActivePaneID := splitCtx.activePane.Pane.ID

	// Set the new pane as active
	splitCtx.ws.ActivePaneID = output.NewPaneNode.Pane.ID

	// Update the workspace view
	if splitCtx.wsView != nil {
		c.applySplitToView(ctx, splitCtx.wsView, splitCtx.ws, output, direction, splitCtx.existingWidget, splitCtx.isStackSplit, oldActivePaneID)
	}

	if splitCtx.wsView != nil {
		splitCtx.wsView.NotifyNewPaneCreated(ctx)
	}

	// Notify state change for session snapshots
	c.notifyStateChanged()

	log.Info().Str("direction", string(direction)).Str("new_pane_id", string(output.NewPaneNode.Pane.ID)).Msg("pane split completed")

	return nil
}

func (c *WorkspaceCoordinator) prepareSplit(
	ctx context.Context,
	direction usecase.SplitDirection,
) (*splitContext, bool) {
	log := logging.FromContext(ctx)
	if c.panesUC == nil {
		log.Warn().Msg("panes use case not available")
		return nil, false
	}

	ws, wsView := c.getActiveWS()
	if ws == nil {
		log.Warn().Msg("no active workspace")
		return nil, false
	}

	activePane := ws.ActivePane()
	if activePane == nil {
		log.Warn().Msg("no active pane to split")
		return nil, false
	}

	// Check if active pane is inside a stack
	isInStack := activePane.Parent != nil && activePane.Parent.IsStacked
	// isStackSplit is only true for horizontal splits - these use a special code path
	isStackSplit := (direction == usecase.SplitLeft || direction == usecase.SplitRight) && isInStack

	var existingWidget layout.Widget
	if wsView != nil {
		existingWidget = c.resolveSplitWidget(wsView, activePane, isInStack)
	}

	return &splitContext{
		ws:             ws,
		wsView:         wsView,
		activePane:     activePane,
		existingWidget: existingWidget,
		isStackSplit:   isStackSplit,
	}, true
}

func (c *WorkspaceCoordinator) resolveSplitWidget(
	wsView *component.WorkspaceView,
	activePane *entity.PaneNode,
	isInStack bool,
) layout.Widget {
	if wsView == nil {
		return nil
	}

	// When splitting from inside a stack (any direction), we need the stack widget
	// because the domain model promotes the split to happen around the stack container
	if isInStack {
		tr := wsView.TreeRenderer()
		if tr == nil {
			return nil
		}

		stackedView := tr.GetStackedViewForPane(string(activePane.Pane.ID))
		if stackedView == nil {
			return nil
		}

		return stackedView.Widget()
	}

	return wsView.GetRootWidget()
}

func (c *WorkspaceCoordinator) applySplitToView(
	ctx context.Context,
	wsView *component.WorkspaceView,
	ws *entity.Workspace,
	output *usecase.SplitPaneOutput,
	direction usecase.SplitDirection,
	existingWidget layout.Widget,
	isStackSplit bool,
	oldActivePaneID entity.PaneID,
) {
	log := logging.FromContext(ctx)
	needsAttach := false

	if existingWidget != nil {
		var splitErr error
		if isStackSplit {
			splitErr = c.doIncrementalStackSplit(ctx, wsView, output, direction, existingWidget, oldActivePaneID)
		} else {
			splitErr = c.doIncrementalSplit(ctx, wsView, ws, output, direction, existingWidget, oldActivePaneID)
		}

		if splitErr != nil {
			log.Warn().Err(splitErr).Msg("incremental split failed, falling back to rebuild")
			if err := wsView.Rebuild(ctx); err != nil {
				log.Error().Err(err).Msg("failed to rebuild workspace view")
			}
			needsAttach = true
		}
	} else {
		if err := wsView.Rebuild(ctx); err != nil {
			log.Error().Err(err).Msg("failed to rebuild workspace view")
		}
		needsAttach = true
	}

	if needsAttach {
		c.contentCoord.AttachToWorkspace(ctx, ws, wsView)
		c.SetupStackedPaneCallbacks(ctx, ws, wsView)
	}
	if err := wsView.SetActivePaneID(ws.ActivePaneID); err != nil {
		log.Warn().Err(err).Msg("failed to set active pane in workspace view")
	}
	wsView.FocusPane(ws.ActivePaneID)
}

// doIncrementalStackSplit performs an incremental split around an existing stacked pane.
func (c *WorkspaceCoordinator) doIncrementalStackSplit(
	ctx context.Context,
	wsView *component.WorkspaceView,
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

	// 2. Determine if this is a root split or non-root split
	// The grandparent of the new split node tells us where the stack currently sits
	grandparentNode := output.ParentNode.Parent
	isRootSplit := grandparentNode == nil

	log.Debug().
		Bool("is_root_split", isRootSplit).
		Str("grandparent_id", func() string {
			if grandparentNode != nil {
				return grandparentNode.ID
			}
			return nilString
		}()).
		Msg("stack split: determined split type")

	// 3. Create new PaneView for the new pane (without WebView - will attach later)
	newPaneView := component.NewPaneView(factory, output.NewPaneNode.Pane.ID, nil)
	setupPaneViewHover(ctx, newPaneView, wsView)

	// 4. Wrap the new PaneView in a StackedView
	newStackedView := layout.NewStackedView(factory)
	newStackedView.AddPane(ctx, string(output.NewPaneNode.Pane.ID), output.NewPaneNode.Pane.Title, "", newPaneView.Widget())

	tr := wsView.TreeRenderer()

	// 5. Handle root vs non-root split differently
	if isRootSplit {
		// Root split: stack is direct child of container
		wsView.ClearRootWidgetRef()
		wsView.Container().Remove(existingStackWidget)

		// Create a SplitView with the appropriate orientation and child placement
		var splitView *layout.SplitView
		if direction == usecase.SplitLeft {
			splitView = layout.NewSplitView(ctx, factory, layout.OrientationHorizontal,
				newStackedView.Widget(), existingStackWidget, output.SplitRatio)
		} else {
			splitView = layout.NewSplitView(ctx, factory, layout.OrientationHorizontal,
				existingStackWidget, newStackedView.Widget(), output.SplitRatio)
		}
		c.wireSplitRatioPersistence(ctx, splitView, output.ParentNode.ID)

		wsView.SetRootWidgetDirect(splitView.Widget())

		if tr != nil {
			tr.RegisterSplit(output.ParentNode.ID, splitView.Widget(), layout.OrientationHorizontal)
		}
	} else {
		// Non-root split: stack is inside another split, need to replace in grandparent
		if tr == nil {
			return fmt.Errorf("tree renderer not available for non-root stack split")
		}

		grandparentWidget := tr.Lookup(grandparentNode.ID)
		if grandparentWidget == nil {
			log.Warn().Str("grandparent_id", grandparentNode.ID).Msg("grandparent widget not found")
			return fmt.Errorf("grandparent widget not found")
		}

		panedWidget, ok := grandparentWidget.(layout.PanedWidget)
		if !ok {
			log.Warn().Msg("grandparent widget is not a PanedWidget")
			return fmt.Errorf("grandparent is not a PanedWidget")
		}

		// Determine if the stack's parent (the new split node) is start or end child
		isStartChild := grandparentNode.Left() == output.ParentNode

		log.Debug().
			Bool("is_start_child", isStartChild).
			Msg("stack split: determined position in grandparent")

		// Clear the position first to unparent the stack widget
		if isStartChild {
			panedWidget.SetStartChild(nil)
		} else {
			panedWidget.SetEndChild(nil)
		}

		// Create the new split view
		var splitView *layout.SplitView
		if direction == usecase.SplitLeft {
			splitView = layout.NewSplitView(ctx, factory, layout.OrientationHorizontal,
				newStackedView.Widget(), existingStackWidget, output.SplitRatio)
		} else {
			splitView = layout.NewSplitView(ctx, factory, layout.OrientationHorizontal,
				existingStackWidget, newStackedView.Widget(), output.SplitRatio)
		}
		c.wireSplitRatioPersistence(ctx, splitView, output.ParentNode.ID)

		// Put the new split view in the grandparent's slot
		if isStartChild {
			panedWidget.SetStartChild(splitView.Widget())
		} else {
			panedWidget.SetEndChild(splitView.Widget())
		}

		tr.RegisterSplit(output.ParentNode.ID, splitView.Widget(), layout.OrientationHorizontal)

		log.Debug().
			Bool("was_start_child", isStartChild).
			Msg("stack split: replaced stack in grandparent with new split view")
	}

	// 6. Register the new pane in tracking maps and activate it
	wsView.RegisterPaneView(output.NewPaneNode.Pane.ID, newPaneView)
	newPaneView.SetActive(true)

	if tr != nil {
		tr.RegisterPaneInStack(string(output.NewPaneNode.Pane.ID), newStackedView)
	}

	// 7. Attach WebView only for the new pane
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
			return nilString
		}()).
		Msg("determined split type")

	// 4. Create new PaneView for the new pane
	newPaneView := component.NewPaneView(factory, output.NewPaneNode.Pane.ID, nil)
	setupPaneViewHover(ctx, newPaneView, wsView)

	// 5. Wrap the new PaneView in a StackedView
	newStackedView := layout.NewStackedView(factory)
	newStackedView.AddPane(ctx, string(output.NewPaneNode.Pane.ID), output.NewPaneNode.Pane.Title, "", newPaneView.Widget())

	orientation, existingFirst, ok := splitOrientation(direction)
	if !ok {
		return nil
	}

	// 7. Handle root vs non-root split differently
	if isRootSplit {
		if err := c.replaceRootSplit(
			ctx,
			wsView,
			tr,
			factory,
			existingRootWidget,
			newStackedView,
			output,
			orientation,
			existingFirst,
		); err != nil {
			return err
		}
	} else {
		if err := c.replaceNonRootSplit(
			ctx,
			tr,
			factory,
			grandparentNode,
			output,
			activePaneWidget,
			newStackedView,
			orientation,
			existingFirst,
		); err != nil {
			return err
		}
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

func splitOrientation(direction usecase.SplitDirection) (layout.Orientation, bool, bool) {
	switch direction {
	case usecase.SplitRight:
		return layout.OrientationHorizontal, true, true
	case usecase.SplitLeft:
		return layout.OrientationHorizontal, false, true
	case usecase.SplitDown:
		return layout.OrientationVertical, true, true
	case usecase.SplitUp:
		return layout.OrientationVertical, false, true
	default:
		return layout.OrientationHorizontal, false, false
	}
}

func (c *WorkspaceCoordinator) wireSplitRatioPersistence(ctx context.Context, splitView *layout.SplitView, splitNodeID string) {
	if c == nil || splitView == nil || splitNodeID == "" {
		return
	}

	splitView.SetOnRatioChanged(func(ratio float64) {
		if err := c.SetSplitRatio(ctx, splitNodeID, ratio); err != nil {
			logging.FromContext(ctx).Warn().Err(err).Str("split_node_id", splitNodeID).Msg("failed to persist split ratio")
		}
	})
}

func (c *WorkspaceCoordinator) replaceRootSplit(
	ctx context.Context,
	wsView *component.WorkspaceView,
	tr *layout.TreeRenderer,
	factory layout.WidgetFactory,
	existingRootWidget layout.Widget,
	newStackedView *layout.StackedView,
	output *usecase.SplitPaneOutput,
	orientation layout.Orientation,
	existingFirst bool,
) error {
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
	c.wireSplitRatioPersistence(ctx, splitView, output.ParentNode.ID)

	wsView.SetRootWidgetDirect(splitView.Widget())
	tr.RegisterSplit(output.ParentNode.ID, splitView.Widget(), orientation)

	logging.FromContext(ctx).Debug().Msg("root split: replaced root with new split view")
	return nil
}

func (c *WorkspaceCoordinator) replaceNonRootSplit(
	ctx context.Context,
	tr *layout.TreeRenderer,
	factory layout.WidgetFactory,
	grandparentNode *entity.PaneNode,
	output *usecase.SplitPaneOutput,
	activePaneWidget layout.Widget,
	newStackedView *layout.StackedView,
	orientation layout.Orientation,
	existingFirst bool,
) error {
	log := logging.FromContext(ctx)
	grandparentWidget := tr.Lookup(grandparentNode.ID)
	if grandparentWidget == nil {
		log.Warn().Str("grandparent_id", grandparentNode.ID).Msg("grandparent widget not found in TreeRenderer")
		return fmt.Errorf("grandparent widget not found")
	}

	panedWidget, ok := grandparentWidget.(layout.PanedWidget)
	if !ok {
		log.Warn().Msg("grandparent widget is not a PanedWidget, falling back")
		return fmt.Errorf("grandparent is not a PanedWidget")
	}

	isStartChild := grandparentNode.Left() == output.ParentNode

	log.Debug().
		Bool("is_start_child", isStartChild).
		Str("grandparent_left_id", func() string {
			if grandparentNode.Left() != nil {
				return grandparentNode.Left().ID
			}
			return nilString
		}()).
		Str("new_parent_id", output.ParentNode.ID).
		Msg("determined position in grandparent")

	if isStartChild {
		panedWidget.SetStartChild(nil)
	} else {
		panedWidget.SetEndChild(nil)
	}

	var splitView *layout.SplitView
	if existingFirst {
		splitView = layout.NewSplitView(ctx, factory, orientation,
			activePaneWidget, newStackedView.Widget(), output.SplitRatio)
	} else {
		splitView = layout.NewSplitView(ctx, factory, orientation,
			newStackedView.Widget(), activePaneWidget, output.SplitRatio)
	}
	c.wireSplitRatioPersistence(ctx, splitView, output.ParentNode.ID)

	if isStartChild {
		panedWidget.SetStartChild(splitView.Widget())
	} else {
		panedWidget.SetEndChild(splitView.Widget())
	}

	tr.RegisterSplit(output.ParentNode.ID, splitView.Widget(), orientation)

	log.Debug().
		Bool("was_start_child", isStartChild).
		Msg("non-root split: replaced pane in grandparent with new split view")
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

	// BEFORE domain changes: capture incremental close context.
	closeCtx := c.captureIncrementalCloseContext(wsView, activePane)

	// Now do domain changes
	_, err := c.panesUC.Close(ctx, ws, activePane)
	if err != nil {
		log.Error().Err(err).Msg("failed to close pane")
		return err
	}

	c.finalizePaneClose(
		ctx,
		wsView,
		ws,
		closingPaneID,
		closeCtx,
	)
	if c.onPaneClosed != nil {
		c.onPaneClosed(closingPaneID)
	}

	// Notify state change for session snapshots
	c.notifyStateChanged()

	log.Info().Msg("pane closed")
	return nil
}

// ClosePaneByID closes a specific pane by ID.
// This is used for closing popup panes when window.close() is called.
func (c *WorkspaceCoordinator) ClosePaneByID(ctx context.Context, paneID entity.PaneID) error {
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

	// Find the pane node
	paneNode := ws.FindPane(paneID)
	if paneNode == nil {
		log.Warn().Str("pane_id", string(paneID)).Msg("pane not found")
		return nil
	}

	log.Debug().Str("pane_id", string(paneID)).Msg("closing pane by ID")

	// Don't close the last pane - close the tab instead
	if ws.PaneCount() <= 1 {
		if c.onCloseLastPane != nil {
			return c.onCloseLastPane(ctx)
		}
		return nil
	}

	// BEFORE domain changes: capture incremental close context.
	closeCtx := c.captureIncrementalCloseContext(wsView, paneNode)

	// Now do domain changes
	_, err := c.panesUC.Close(ctx, ws, paneNode)
	if err != nil {
		log.Error().Err(err).Msg("failed to close pane")
		return err
	}

	c.finalizePaneClose(
		ctx,
		wsView,
		ws,
		paneID,
		closeCtx,
	)
	if c.onPaneClosed != nil {
		c.onPaneClosed(paneID)
	}

	// Notify state change for session snapshots
	c.notifyStateChanged()

	log.Info().Str("pane_id", string(paneID)).Msg("pane closed by ID")
	return nil
}

// doIncrementalClose performs incremental close by promoting sibling without rebuild.
func (c *WorkspaceCoordinator) doIncrementalClose(
	ctx context.Context,
	wsView *component.WorkspaceView,
	closingPaneID entity.PaneID,
	siblingNode *entity.PaneNode,
	parentNode *entity.PaneNode,
	grandparentNode *entity.PaneNode,
	parentWidget layout.Widget,
	siblingIsStartChild bool, // true if sibling is start/left child in parent
	parentIsStartInGrand bool, // true if parent is start/left child in grandparent
) error {
	log := logging.FromContext(ctx)
	if parentNode == nil {
		return fmt.Errorf("parent node missing")
	}
	if siblingNode == nil {
		return fmt.Errorf("sibling node missing")
	}
	if parentWidget == nil {
		return fmt.Errorf("parent widget missing")
	}

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

func (c *WorkspaceCoordinator) finalizePaneClose(
	ctx context.Context,
	wsView *component.WorkspaceView,
	ws *entity.Workspace,
	paneID entity.PaneID,
	closeCtx incrementalCloseContext,
) {
	if wsView == nil {
		return
	}

	log := logging.FromContext(ctx)

	if closeCtx.parentNode != nil {
		var closeErr error

		if closeCtx.parentNode.IsStacked {
			closeErr = c.doIncrementalStackClose(ctx, wsView, paneID, closeCtx.parentNode)
		} else if closeCtx.precheckReason != "" {
			closeErr = fmt.Errorf("incremental close precheck failed: %s", closeCtx.precheckReason)
		} else if closeCtx.siblingNode != nil && closeCtx.parentWidget != nil {
			closeErr = c.doIncrementalClose(
				ctx,
				wsView,
				paneID,
				closeCtx.siblingNode,
				closeCtx.parentNode,
				closeCtx.grandparentNode,
				closeCtx.parentWidget,
				closeCtx.siblingIsStartChild,
				closeCtx.parentIsStartInGrand,
			)
		} else {
			closeErr = fmt.Errorf("missing context for incremental close")
		}

		if closeErr != nil {
			fallbackLog := log.Warn().Err(closeErr).
				Str("pane_id", string(paneID)).
				Str("parent_id", paneNodeID(closeCtx.parentNode)).
				Str("sibling_id", paneNodeID(closeCtx.siblingNode)).
				Str("grandparent_id", paneNodeID(closeCtx.grandparentNode)).
				Bool("has_parent_widget", closeCtx.parentWidget != nil)
			if closeCtx.precheckReason != "" {
				fallbackLog = fallbackLog.Str("precheck_reason", closeCtx.precheckReason)
			}
			fallbackLog.Msg("incremental close failed, falling back to rebuild")
			if err := wsView.Rebuild(ctx); err != nil {
				log.Error().Err(err).Msg("failed to rebuild workspace view")
			}
			c.contentCoord.ReleaseWebView(ctx, paneID)
			c.contentCoord.AttachToWorkspace(ctx, ws, wsView)
			c.SetupStackedPaneCallbacks(ctx, ws, wsView)
		}
	} else {
		if err := wsView.Rebuild(ctx); err != nil {
			log.Error().Err(err).Msg("failed to rebuild workspace view")
		}
		c.contentCoord.ReleaseWebView(ctx, paneID)
		c.contentCoord.AttachToWorkspace(ctx, ws, wsView)
		c.SetupStackedPaneCallbacks(ctx, ws, wsView)
	}

	if err := wsView.SetActivePaneID(ws.ActivePaneID); err != nil {
		log.Warn().Err(err).Msg("failed to set active pane in workspace view")
	}
	wsView.FocusPane(ws.ActivePaneID)
}

func (c *WorkspaceCoordinator) captureIncrementalCloseContext(
	wsView *component.WorkspaceView,
	closingPane *entity.PaneNode,
) incrementalCloseContext {
	closeCtx, err := deriveIncrementalCloseTreeContext(closingPane)
	if err != nil {
		closeCtx.precheckReason = err.Error()
		return closeCtx
	}

	if closeCtx.parentNode == nil || closeCtx.parentNode.IsStacked {
		return closeCtx
	}

	if wsView == nil {
		closeCtx.precheckReason = "workspace view unavailable"
		return closeCtx
	}

	tr := wsView.TreeRenderer()
	if tr == nil {
		closeCtx.precheckReason = "tree renderer unavailable"
		return closeCtx
	}

	closeCtx.parentWidget = tr.Lookup(closeCtx.parentNode.ID)
	if closeCtx.parentWidget == nil {
		closeCtx.precheckReason = "parent widget missing"
	}

	return closeCtx
}

func deriveIncrementalCloseTreeContext(closingPane *entity.PaneNode) (incrementalCloseContext, error) {
	var closeCtx incrementalCloseContext
	if closingPane == nil {
		return closeCtx, errors.New("closing pane missing")
	}

	parentNode := closingPane.Parent
	if parentNode == nil {
		return closeCtx, errors.New("closing pane has no parent")
	}

	closeCtx.parentNode = parentNode
	closeCtx.grandparentNode = parentNode.Parent

	if parentNode.IsStacked {
		return closeCtx, nil
	}

	if !parentNode.IsSplit() {
		return closeCtx, fmt.Errorf("parent node is not split: %s", parentNode.ID)
	}

	if len(parentNode.Children) != 2 {
		return closeCtx, fmt.Errorf("split parent has invalid child count: %d", len(parentNode.Children))
	}

	leftChild := parentNode.Left()
	rightChild := parentNode.Right()
	if leftChild == closingPane && rightChild == nil {
		return closeCtx, errors.New("sibling missing")
	}
	if rightChild == closingPane && leftChild == nil {
		return closeCtx, errors.New("sibling missing")
	}
	if leftChild == nil || rightChild == nil {
		return closeCtx, errors.New("split parent has nil child")
	}

	switch {
	case leftChild == closingPane:
		closeCtx.siblingNode = rightChild
		closeCtx.siblingIsStartChild = false
	case rightChild == closingPane:
		closeCtx.siblingNode = leftChild
		closeCtx.siblingIsStartChild = true
	default:
		return closeCtx, fmt.Errorf("closing pane not found under parent: %s", parentNode.ID)
	}

	if closeCtx.siblingNode == nil {
		return closeCtx, errors.New("sibling missing")
	}

	if closeCtx.grandparentNode != nil {
		switch {
		case closeCtx.grandparentNode.Left() == parentNode:
			closeCtx.parentIsStartInGrand = true
		case closeCtx.grandparentNode.Right() == parentNode:
			closeCtx.parentIsStartInGrand = false
		default:
			return closeCtx, fmt.Errorf("parent not found under grandparent: %s", closeCtx.grandparentNode.ID)
		}
	}

	return closeCtx, nil
}

func paneNodeID(node *entity.PaneNode) string {
	if node == nil {
		return nilString
	}
	return node.ID
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

	// Find the index of the closing pane in the StackedView UI
	closingIndex := stackedView.FindPaneIndex(string(closingPaneID))
	if closingIndex < 0 {
		return fmt.Errorf("pane %s not found in stacked view", closingPaneID)
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

	// Sync remaining title bars with current titles from ContentCoordinator.
	// The domain model (stackNode.Children) has already been updated by the use case,
	// so we iterate the remaining children and update their titles in the UI.
	c.syncStackedPaneTitles(ctx, stackedView, stackNode)

	log.Debug().
		Int("removed_index", closingIndex).
		Msg("incremental stack close completed successfully")
	return nil
}

// syncStackedPaneTitles updates all title bars in a StackedView with current titles.
func (c *WorkspaceCoordinator) syncStackedPaneTitles(ctx context.Context, stackedView *layout.StackedView, stackNode *entity.PaneNode) {
	log := logging.FromContext(ctx)

	if stackedView == nil || stackNode == nil {
		return
	}

	// Get UI pane count to avoid index out of bounds when domain and UI are out of sync
	uiPaneCount := stackedView.Count()

	for i, child := range stackNode.Children {
		if child.Pane == nil {
			continue
		}

		// Skip if index exceeds UI pane count (domain/UI desync)
		if i >= uiPaneCount {
			log.Debug().
				Int("index", i).
				Int("ui_count", uiPaneCount).
				Str("pane_id", string(child.Pane.ID)).
				Msg("skipping title sync: index exceeds UI pane count")
			continue
		}

		title := c.contentCoord.GetTitle(child.Pane.ID)
		if title == "" {
			title = child.Pane.Title
		}
		if title == "" {
			title = defaultPaneTitle
		}

		if err := stackedView.UpdateTitle(i, title); err != nil {
			log.Warn().
				Err(err).
				Int("index", i).
				Str("pane_id", string(child.Pane.ID)).
				Msg("failed to sync title bar")
		}
	}
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

	// Save old active pane ID before navigation updates the domain model
	oldActivePaneID := ws.ActivePaneID

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

	// Cancel pending hover timers and suppress future hover focus (Issue #89)
	wsView.CancelAllPendingHovers()
	wsView.SuppressHover(component.KeyboardFocusSuppressDuration)

	// Deactivate old pane explicitly (domain model already updated by NavigateGeometric)
	if oldActivePaneID != "" && oldActivePaneID != newPane.Pane.ID {
		wsView.DeactivatePane(oldActivePaneID)
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
// Uses CreateStack use case for new stacks, AddToStack for existing stacks.
func (c *WorkspaceCoordinator) StackPane(ctx context.Context) error {
	log := logging.FromContext(ctx)

	stackCtx, ok := c.prepareStackPane(ctx)
	if !ok {
		return nil
	}

	// Create new pane entity
	newPaneID := entity.PaneID(c.generateID())
	newPane := entity.NewPane(newPaneID)
	newPane.URI = domainurl.Normalize(config.Get().Workspace.NewPaneURL)
	newPane.Title = defaultPaneTitle

	// Determine if we need to create a new stack or add to existing
	var stackNode *entity.PaneNode
	var needsFirstPaneTitleUpdate bool

	if stackCtx.activeNode.Parent != nil && stackCtx.activeNode.Parent.IsStacked {
		// Already in a stack - use AddToStack use case
		stackNode = stackCtx.activeNode.Parent
		output, err := c.panesUC.AddToStack(ctx, stackCtx.ws, stackNode, newPane)
		if err != nil {
			log.Error().Err(err).Msg("failed to add pane to stack via use case")
			return err
		}
		log.Debug().
			Int("stack_size", len(stackNode.Children)).
			Int("insert_index", output.StackIndex).
			Msg("added to existing stack via use case")
	} else if stackCtx.activeNode.IsStacked {
		// Active node is already a stack container - add to it
		stackNode = stackCtx.activeNode
		output, err := c.panesUC.AddToStack(ctx, stackCtx.ws, stackNode, newPane)
		if err != nil {
			log.Error().Err(err).Msg("failed to add pane to stack via use case")
			return err
		}
		log.Debug().
			Int("stack_size", len(stackNode.Children)).
			Int("insert_index", output.StackIndex).
			Msg("added to stack container via use case")
	} else {
		// Need to create a new stack - use CreateStack use case
		// But CreateStack creates its own pane, so we need a different approach
		// Convert to stack first, then the new pane is already created
		output, err := c.panesUC.CreateStack(ctx, stackCtx.ws, stackCtx.activeNode)
		if err != nil {
			log.Error().Err(err).Msg("failed to create stack via use case")
			return err
		}
		stackNode = output.StackNode
		// CreateStack creates its own new pane, use that instead
		newPane = output.NewPane
		newPaneID = newPane.ID
		needsFirstPaneTitleUpdate = true

		// Update the original pane's title in the domain
		if output.OriginalNode != nil && output.OriginalNode.Pane != nil {
			output.OriginalNode.Pane.Title = stackCtx.originalTitle
		}

		log.Debug().
			Int("stack_size", len(stackNode.Children)).
			Msg("created new stack via use case")
	}

	// Create PaneView for the new pane
	newPaneView := component.NewPaneView(c.widgetFactory, newPaneID, nil)
	setupPaneViewHover(ctx, newPaneView, stackCtx.wsView)
	stackCtx.wsView.RegisterPaneView(newPaneID, newPaneView)

	// Add to the UI StackedView
	if err := c.stackedPaneMgr.AddPaneToStack(ctx, stackCtx.wsView, stackCtx.activePaneID, newPaneView, defaultPaneTitle); err != nil {
		log.Error().Err(err).Msg("failed to add pane to stack")
		return err
	}

	// Update the first pane's title if we just converted from leaf to stacked
	if needsFirstPaneTitleUpdate {
		c.updateFirstStackTitle(ctx, stackCtx.wsView, stackCtx.activePaneID, stackCtx.originalTitle)
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
		// Load initial page for the new pane
		if err := wv.LoadURI(ctx, newPane.URI); err != nil {
			log.Warn().Err(err).Str("uri", newPane.URI).Msg("failed to load initial page")
		}
	}

	// Update workspace view
	if err := stackCtx.wsView.SetActivePaneID(newPaneID); err != nil {
		log.Warn().Err(err).Msg("failed to set active pane")
	}

	stackCtx.wsView.NotifyNewPaneCreated(ctx)

	// Set up title bar click and close callbacks
	tr := stackCtx.wsView.TreeRenderer()
	if tr != nil {
		stackedView := tr.GetStackedViewForPane(string(stackCtx.activePaneID))
		if stackedView != nil {
			capturedStackNode := stackNode
			stackedView.SetOnActivate(func(index int) {
				c.onTitleBarClick(ctx, capturedStackNode, stackedView, index)
			})
			stackedView.SetOnClosePane(func(paneID string) {
				c.onStackedPaneClose(ctx, entity.PaneID(paneID))
			})
		}
	}

	// Notify state change for session snapshots
	c.notifyStateChanged()

	log.Info().
		Str("original_pane", string(stackCtx.activePaneID)).
		Str("new_pane", string(newPaneID)).
		Int("stack_size", len(stackNode.Children)).
		Msg("stacked new pane")

	return nil
}

func (c *WorkspaceCoordinator) prepareStackPane(
	ctx context.Context,
) (*stackPaneContext, bool) {
	log := logging.FromContext(ctx)
	if c.stackedPaneMgr == nil {
		log.Warn().Msg("stacked pane manager not available")
		return nil, false
	}

	ws, wsView := c.getActiveWS()
	if ws == nil {
		log.Warn().Msg("no active workspace")
		return nil, false
	}

	activeNode := ws.ActivePane()
	if activeNode == nil || activeNode.Pane == nil {
		log.Warn().Msg("no active pane")
		return nil, false
	}

	if wsView == nil {
		log.Warn().Msg("no workspace view")
		return nil, false
	}

	activePaneID := activeNode.Pane.ID
	originalTitle := c.contentCoord.GetTitle(activePaneID)
	if originalTitle == "" {
		originalTitle = activeNode.Pane.Title
	}
	if originalTitle == "" {
		originalTitle = defaultPaneTitle
	}

	return &stackPaneContext{
		ws:            ws,
		wsView:        wsView,
		activeNode:    activeNode,
		activePaneID:  activePaneID,
		originalTitle: originalTitle,
	}, true
}

func (c *WorkspaceCoordinator) updateFirstStackTitle(
	ctx context.Context,
	wsView *component.WorkspaceView,
	activePaneID entity.PaneID,
	originalTitle string,
) {
	log := logging.FromContext(ctx)
	tr := wsView.TreeRenderer()
	if tr == nil {
		return
	}
	stackedView := tr.GetStackedViewForPane(string(activePaneID))
	if stackedView == nil {
		return
	}
	if err := stackedView.UpdateTitle(0, originalTitle); err != nil {
		log.Warn().Err(err).Str("title", originalTitle).Msg("failed to update first pane title")
		return
	}
	log.Debug().Str("title", originalTitle).Msg("updated first pane title in StackedView")
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

	// Sync the incoming pane's title bar with its current WebView title
	// to avoid showing a stale title after navigation occurred while hidden.
	incomingTitle := c.contentCoord.GetTitle(clickedPaneID)
	if incomingTitle == "" {
		incomingTitle = clickedChild.Pane.Title
	}
	if incomingTitle != "" {
		if err := sv.UpdateTitle(clickedIndex, incomingTitle); err != nil {
			log.Warn().Err(err).Msg("failed to update incoming pane title")
		}
	}

	// Update StackedView active index
	if err := sv.SetActive(ctx, clickedIndex); err != nil {
		log.Warn().Err(err).Int("index", clickedIndex).Msg("failed to set active pane in stack")
		return
	}

	// Update domain model
	stackNode.ActiveStackIndex = clickedIndex

	// Update workspace view (also updates domain model)
	_, wsView := c.getActiveWS()
	if wsView != nil {
		// Cancel pending hover timers and suppress future hover focus (Issue #89)
		wsView.CancelAllPendingHovers()
		wsView.SuppressHover(component.KeyboardFocusSuppressDuration)

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

// onStackedPaneClose handles close button clicks on stacked pane title bars.
func (c *WorkspaceCoordinator) onStackedPaneClose(ctx context.Context, paneID entity.PaneID) {
	log := logging.FromContext(ctx)

	log.Debug().Str("pane_id", string(paneID)).Msg("closing stacked pane via close button")

	if err := c.ClosePaneByID(ctx, paneID); err != nil {
		log.Error().Err(err).Str("pane_id", string(paneID)).Msg("failed to close stacked pane")
	}
}

// SetupStackedPaneCallbacks sets up title bar click and close callbacks for all stacked panes.
// This must be called after Rebuild() to restore callbacks on newly created StackedView instances.
func (c *WorkspaceCoordinator) SetupStackedPaneCallbacks(ctx context.Context, ws *entity.Workspace, wsView *component.WorkspaceView) {
	log := logging.FromContext(ctx)

	if ws == nil || ws.Root == nil || wsView == nil {
		return
	}

	tr := wsView.TreeRenderer()
	if tr == nil {
		return
	}

	// Walk the tree and set up callbacks for all stacked nodes
	ws.Root.Walk(func(node *entity.PaneNode) bool {
		if !node.IsStacked || len(node.Children) == 0 {
			return true
		}

		// Get the StackedView for any pane in this stack
		firstChild := node.Children[0]
		if firstChild.Pane == nil {
			return true
		}

		stackedView := tr.GetStackedViewForPane(string(firstChild.Pane.ID))
		if stackedView == nil {
			return true
		}

		capturedStackNode := node
		stackedView.SetOnActivate(func(index int) {
			c.onTitleBarClick(ctx, capturedStackNode, stackedView, index)
		})
		stackedView.SetOnClosePane(func(paneID string) {
			c.onStackedPaneClose(ctx, entity.PaneID(paneID))
		})

		log.Debug().
			Str("stack_id", node.ID).
			Int("stack_size", len(node.Children)).
			Msg("set up stacked pane callbacks")

		return true
	})
}

// --- Popup Insertion ---

// SetOnCreatePopupTab sets the callback for creating popup tabs.
// This is used when popup behavior is "tabbed".
func (c *WorkspaceCoordinator) SetOnCreatePopupTab(fn func(ctx context.Context, input InsertPopupInput) error) {
	c.onCreatePopupTab = fn
}

// InsertPopup inserts a popup pane into the workspace based on the specified behavior.
// Supports split, stacked, and tabbed behaviors.
func (c *WorkspaceCoordinator) InsertPopup(ctx context.Context, input InsertPopupInput) error {
	log := logging.FromContext(ctx)

	log.Debug().
		Str("parent_pane", string(input.ParentPaneID)).
		Str("popup_pane", string(input.PopupPane.ID)).
		Str("behavior", string(input.Behavior)).
		Str("placement", input.Placement).
		Str("popup_type", input.PopupType.String()).
		Msg("inserting popup into workspace")

	switch input.Behavior {
	case config.PopupBehaviorSplit:
		return c.insertPopupSplit(ctx, input)
	case config.PopupBehaviorStacked:
		return c.insertPopupStacked(ctx, input)
	case config.PopupBehaviorTabbed:
		return c.insertPopupTabbed(ctx, input)
	default:
		// Default to split right
		return c.insertPopupSplit(ctx, input)
	}
}

// insertPopupSplit inserts a popup as a split pane adjacent to the parent.
func (c *WorkspaceCoordinator) insertPopupSplit(ctx context.Context, input InsertPopupInput) error {
	log := logging.FromContext(ctx)

	ws, wsView := c.getActiveWS()
	if ws == nil {
		return fmt.Errorf("no active workspace")
	}

	// Find parent pane node
	parentNode := ws.FindPane(input.ParentPaneID)
	if parentNode == nil {
		return fmt.Errorf("parent pane not found: %s", input.ParentPaneID)
	}

	direction := resolvePopupSplitDirection(input.Placement)

	// Check if parent is in a stack - if so, split around the stack
	isStackSplit := (direction == usecase.SplitLeft || direction == usecase.SplitRight) &&
		parentNode.Parent != nil && parentNode.Parent.IsStacked

	// Get existing widget before domain changes
	existingWidget := c.resolvePopupSplitWidget(wsView, parentNode, isStackSplit)

	// Perform the split with pre-created pane
	output, err := c.panesUC.Split(ctx, usecase.SplitPaneInput{
		Workspace:  ws,
		TargetPane: parentNode,
		Direction:  direction,
		NewPane:    input.PopupPane,
	})
	if err != nil {
		return fmt.Errorf("split for popup: %w", err)
	}

	// Set popup as active
	ws.ActivePaneID = input.PopupPane.ID

	// Update UI
	if wsView != nil {
		c.applySplitToView(ctx, wsView, ws, output, direction, existingWidget, isStackSplit, input.ParentPaneID)
		c.attachPopupWebView(ctx, wsView, input)
	}

	log.Info().
		Str("popup_pane", string(input.PopupPane.ID)).
		Str("direction", string(direction)).
		Msg("popup inserted as split pane")

	return nil
}

func resolvePopupSplitDirection(placement string) usecase.SplitDirection {
	switch placement {
	case string(usecase.SplitLeft):
		return usecase.SplitLeft
	case string(usecase.SplitRight):
		return usecase.SplitRight
	case "top", string(usecase.SplitUp):
		return usecase.SplitUp
	case "bottom", string(usecase.SplitDown):
		return usecase.SplitDown
	default:
		return usecase.SplitRight
	}
}

func (c *WorkspaceCoordinator) resolvePopupSplitWidget(
	wsView *component.WorkspaceView,
	parentNode *entity.PaneNode,
	isStackSplit bool,
) layout.Widget {
	if wsView == nil {
		return nil
	}

	if !isStackSplit {
		return wsView.GetRootWidget()
	}

	tr := wsView.TreeRenderer()
	if tr == nil {
		return nil
	}

	stackedView := tr.GetStackedViewForPane(string(parentNode.Pane.ID))
	if stackedView == nil {
		return nil
	}

	return stackedView.Widget()
}

func (c *WorkspaceCoordinator) attachPopupWebView(
	ctx context.Context,
	wsView *component.WorkspaceView,
	input InsertPopupInput,
) {
	if wsView == nil || input.WebView == nil {
		return
	}

	widget := c.contentCoord.WrapWidget(ctx, input.WebView)
	if widget == nil {
		return
	}

	paneView := wsView.GetPaneView(input.PopupPane.ID)
	if paneView == nil {
		return
	}
	paneView.SetWebViewWidget(widget)
}

// insertPopupStacked inserts a popup as a stacked pane on top of the parent.
// Uses CreateStack and AddToStack use cases for proper domain model management.
func (c *WorkspaceCoordinator) insertPopupStacked(ctx context.Context, input InsertPopupInput) error {
	log := logging.FromContext(ctx)

	ws, wsView := c.getActiveWS()
	if ws == nil {
		return fmt.Errorf("no active workspace")
	}

	parentNode := ws.FindPane(input.ParentPaneID)
	if parentNode == nil {
		return fmt.Errorf("parent pane not found: %s", input.ParentPaneID)
	}

	// Resolve or create stack node (track conversion for potential rollback)
	stackNode, conversionInfo := c.resolveOrCreateStackNode(ctx, parentNode, input.ParentPaneID)

	// Add popup pane to stack using use case
	if _, err := c.panesUC.AddToStack(ctx, ws, stackNode, input.PopupPane); err != nil {
		// Rollback stack conversion if we created a new stack
		conversionInfo.revert()
		return fmt.Errorf("failed to add popup to stack: %w", err)
	}

	if wsView == nil {
		// No workspace view to update; domain model already updated by use case.
		return nil
	}

	if err := c.attachPopupPaneView(ctx, input, wsView, stackNode, log); err != nil {
		// UI update failed - rollback domain changes to maintain consistency
		log.Warn().Err(err).Msg("rolling back domain changes due to UI failure")

		// Remove popup pane from stack
		if removeErr := c.panesUC.RemoveFromStack(ctx, stackNode, input.PopupPane.ID); removeErr != nil {
			log.Error().Err(removeErr).Msg("failed to rollback popup pane addition")
		}

		// Revert stack conversion if we created a new stack
		conversionInfo.revert()

		return err
	}

	log.Info().
		Str("popup_pane", string(input.PopupPane.ID)).
		Int("stack_size", len(stackNode.Children)).
		Msg("popup inserted as stacked pane")

	return nil
}

// stackConversionInfo holds state needed to revert a leaf-to-stack conversion.
type stackConversionInfo struct {
	wasConverted bool             // true if resolveOrCreateStackNode converted a leaf to a stack
	originalPane *entity.Pane     // the pane that was in the node before conversion
	node         *entity.PaneNode // the node that was converted
}

// revert undoes a leaf-to-stack conversion.
func (info *stackConversionInfo) revert() {
	if !info.wasConverted || info.node == nil {
		return
	}

	// Restore node to leaf state
	info.node.Pane = info.originalPane
	info.node.IsStacked = false
	info.node.ActiveStackIndex = 0
	info.node.Children = nil
}

// resolveOrCreateStackNode finds an existing stack or creates one for popup insertion.
// Note: For popups, we can't use CreateStack use case since it creates a new pane,
// but we already have the popup pane. This conversion is a special case.
// Returns the stack node and conversion info for potential rollback.
func (c *WorkspaceCoordinator) resolveOrCreateStackNode(
	ctx context.Context,
	parentNode *entity.PaneNode,
	parentPaneID entity.PaneID,
) (*entity.PaneNode, stackConversionInfo) {
	log := logging.FromContext(ctx)

	// Check if parent is already in a stack
	if parentNode.Parent != nil && parentNode.Parent.IsStacked {
		log.Debug().
			Str("stack_id", parentNode.Parent.ID).
			Msg("using existing parent stack for popup")
		return parentNode.Parent, stackConversionInfo{}
	}

	// Check if parent is already a stack container
	if parentNode.IsStacked {
		log.Debug().
			Str("stack_id", parentNode.ID).
			Msg("parent is already a stack container")
		return parentNode, stackConversionInfo{}
	}

	// Need to convert leaf to stack for popup insertion.
	// We can't use CreateStack use case since it creates a new pane,
	// but we already have the popup pane to insert.
	originalPane := parentNode.Pane
	originalTitle := c.contentCoord.GetTitle(parentPaneID)
	if originalTitle == "" {
		originalTitle = originalPane.Title
	}
	if originalTitle == "" {
		originalTitle = defaultPaneTitle
	}
	originalPane.Title = originalTitle

	// Create child node for the original pane
	originalPaneChild := &entity.PaneNode{
		ID:     parentNode.ID + "_0",
		Pane:   originalPane,
		Parent: parentNode,
	}

	// Convert parentNode to stack container
	parentNode.Pane = nil
	parentNode.IsStacked = true
	parentNode.ActiveStackIndex = 0
	parentNode.Children = []*entity.PaneNode{originalPaneChild}

	log.Debug().
		Str("node_id", parentNode.ID).
		Str("original_pane", string(originalPane.ID)).
		Msg("converted leaf to stacked node for popup")

	return parentNode, stackConversionInfo{
		wasConverted: true,
		originalPane: originalPane,
		node:         parentNode,
	}
}

func (c *WorkspaceCoordinator) attachPopupPaneView(
	ctx context.Context,
	input InsertPopupInput,
	wsView *component.WorkspaceView,
	stackNode *entity.PaneNode,
	log *zerolog.Logger,
) error {
	if wsView == nil {
		return nil
	}
	newPaneView := component.NewPaneView(c.widgetFactory, input.PopupPane.ID, nil)
	setupPaneViewHover(ctx, newPaneView, wsView)
	wsView.RegisterPaneView(input.PopupPane.ID, newPaneView)

	if err := c.stackedPaneMgr.AddPaneToStack(ctx, wsView, input.ParentPaneID, newPaneView, input.PopupPane.Title); err != nil {
		log.Error().Err(err).Msg("failed to add popup pane to stack")
		return err
	}

	if input.WebView != nil {
		widget := c.contentCoord.WrapWidget(ctx, input.WebView)
		if widget != nil {
			newPaneView.SetWebViewWidget(widget)
		}
	}

	if err := wsView.SetActivePaneID(input.PopupPane.ID); err != nil {
		log.Warn().Err(err).Msg("failed to set active pane in workspace view")
	}

	tr := wsView.TreeRenderer()
	if tr != nil {
		stackedView := tr.GetStackedViewForPane(string(input.ParentPaneID))
		if stackedView != nil {
			capturedStackNode := stackNode
			stackedView.SetOnActivate(func(index int) {
				c.onTitleBarClick(ctx, capturedStackNode, stackedView, index)
			})
			stackedView.SetOnClosePane(func(paneID string) {
				c.onStackedPaneClose(ctx, entity.PaneID(paneID))
			})
		}
	}
	return nil
}

// insertPopupTabbed creates a new tab for the popup.
func (c *WorkspaceCoordinator) insertPopupTabbed(ctx context.Context, input InsertPopupInput) error {
	log := logging.FromContext(ctx)

	if c.onCreatePopupTab == nil {
		log.Warn().Msg("tabbed popup behavior requested but no tab handler configured, falling back to split")
		return c.insertPopupSplit(ctx, input)
	}

	// Delegate to tab coordinator via callback
	if err := c.onCreatePopupTab(ctx, input); err != nil {
		return fmt.Errorf("create popup tab: %w", err)
	}

	log.Info().
		Str("popup_pane", string(input.PopupPane.ID)).
		Msg("popup inserted as new tab")

	return nil
}

func (c *WorkspaceCoordinator) ConsumeOrExpelPane(ctx context.Context, direction usecase.ConsumeOrExpelDirection) error {
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

	activeNode := ws.ActivePane()
	if activeNode == nil {
		log.Warn().Msg("no active pane")
		return nil
	}

	result, err := c.panesUC.ConsumeOrExpel(ctx, ws, activeNode, direction)
	if err != nil {
		return err
	}
	if result != nil && result.ErrorMessage != "" {
		c.ShowToastOnActivePane(ctx, result.ErrorMessage, component.ToastWarning)
		return nil
	}

	if wsView != nil {
		if err := wsView.Rebuild(ctx); err != nil {
			log.Warn().Err(err).Msg("failed to rebuild workspace view")
		}
		c.contentCoord.AttachToWorkspace(ctx, ws, wsView)
		c.SetupStackedPaneCallbacks(ctx, ws, wsView)
		if err := wsView.SetActivePaneID(ws.ActivePaneID); err != nil {
			log.Warn().Err(err).Msg("failed to set active pane in workspace view")
		}
		wsView.FocusPane(ws.ActivePaneID)
	}

	c.notifyStateChanged()
	return nil
}

// ShowZoomToast displays a zoom level toast on the active pane.
func (c *WorkspaceCoordinator) ShowZoomToast(ctx context.Context, zoomPercent int) {
	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}

	paneView := wsView.GetActivePaneView()
	if paneView != nil {
		paneView.ShowZoomToast(ctx, zoomPercent)
	}
}

// ShowToastOnActivePane displays a toast notification on the active pane.
func (c *WorkspaceCoordinator) ShowToastOnActivePane(ctx context.Context, message string, level component.ToastLevel) {
	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}

	paneView := wsView.GetActivePaneView()
	if paneView != nil {
		paneView.ShowToast(ctx, message, level)
	}
}

// Resize updates the active split ratio and applies it to GTK widgets.
func (c *WorkspaceCoordinator) Resize(ctx context.Context, dir usecase.ResizeDirection) error {
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

	target := ws.ActivePane()
	if target == nil {
		return nil
	}

	cfg := config.Get()
	err := c.panesUC.Resize(ctx, ws, target, dir, cfg.Workspace.ResizeMode.StepPercent, cfg.Workspace.ResizeMode.MinPanePercent)
	if errors.Is(err, usecase.ErrNothingToResize) {
		c.ShowToastOnActivePane(ctx, "Nothing to resize", component.ToastInfo)
		return nil
	}
	if err != nil {
		return err
	}

	if wsView != nil {
		c.updateSplitPositions(wsView, ws)
	}

	c.notifyStateChanged()
	return nil
}

func (c *WorkspaceCoordinator) SetSplitRatio(ctx context.Context, splitNodeID string, ratio float64) error {
	log := logging.FromContext(ctx)

	if c.panesUC == nil {
		log.Warn().Msg("panes use case not available")
		return nil
	}

	ws, _ := c.getActiveWS()
	if ws == nil {
		log.Warn().Msg("no active workspace")
		return nil
	}

	cfg := config.Get()
	err := c.panesUC.SetSplitRatio(ctx, usecase.SetSplitRatioInput{
		Workspace:      ws,
		SplitNodeID:    splitNodeID,
		Ratio:          ratio,
		MinPanePercent: cfg.Workspace.ResizeMode.MinPanePercent,
	})
	if err != nil {
		return err
	}

	c.notifyStateChanged()
	return nil
}

func (c *WorkspaceCoordinator) updateSplitPositions(wsView *component.WorkspaceView, ws *entity.Workspace) {
	if wsView == nil || ws == nil || ws.Root == nil {
		return
	}

	tr := wsView.TreeRenderer()
	if tr == nil {
		return
	}

	ws.Root.Walk(func(node *entity.PaneNode) bool {
		if node.IsSplit() {
			_ = tr.UpdateSplitRatio(node.ID, node.SplitRatio)
		}
		return true
	})
}
