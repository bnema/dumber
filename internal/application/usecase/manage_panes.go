package usecase

import (
	"context"
	"fmt"
	"sort"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

// SplitDirection indicates the direction of a pane split.
type SplitDirection string

const (
	SplitLeft  SplitDirection = "left"
	SplitRight SplitDirection = "right"
	SplitUp    SplitDirection = "up"
	SplitDown  SplitDirection = "down"
)

// NavigateDirection indicates the direction for focus navigation.
type NavigateDirection string

const (
	NavLeft  NavigateDirection = "left"
	NavRight NavigateDirection = "right"
	NavUp    NavigateDirection = "up"
	NavDown  NavigateDirection = "down"
)

// ManagePanesUseCase handles pane tree operations.
type ManagePanesUseCase struct {
	idGenerator IDGenerator
}

// NewManagePanesUseCase creates a new pane management use case.
func NewManagePanesUseCase(idGenerator IDGenerator) *ManagePanesUseCase {
	return &ManagePanesUseCase{
		idGenerator: idGenerator,
	}
}

// SplitPaneInput contains parameters for splitting a pane.
type SplitPaneInput struct {
	Workspace  *entity.Workspace
	TargetPane *entity.PaneNode
	Direction  SplitDirection
	NewPane    *entity.Pane // Optional: existing pane to insert (for popups)
	InitialURL string       // URL for new pane (default: about:blank)
}

// SplitPaneOutput contains the result of a split operation.
type SplitPaneOutput struct {
	NewPaneNode *entity.PaneNode
	ParentNode  *entity.PaneNode // New parent container
	SplitRatio  float64          // Always 0.5 for new splits
}

// Split creates a new pane adjacent to the target pane.
func (uc *ManagePanesUseCase) Split(ctx context.Context, input SplitPaneInput) (*SplitPaneOutput, error) {
	log := logging.FromContext(ctx)
	log.Debug().
		Str("direction", string(input.Direction)).
		Str("target_id", input.TargetPane.ID).
		Msg("splitting pane")

	if input.Workspace == nil {
		return nil, fmt.Errorf("workspace is required")
	}
	if input.TargetPane == nil {
		return nil, fmt.Errorf("target pane is required")
	}

	// If target is inside a stack and splitting left/right, split around the entire stack
	// (i.e., create the new pane as a sibling to the stack, not inside it)
	targetNode := input.TargetPane
	if input.Direction == SplitLeft || input.Direction == SplitRight {
		if targetNode.Parent != nil && targetNode.Parent.IsStacked {
			// Promote to the stack container so split happens outside the stack
			targetNode = targetNode.Parent
			log.Debug().Msg("target is inside a stack, splitting around stack container")
		}
	}

	// Create new pane
	var newPane *entity.Pane
	if input.NewPane != nil {
		newPane = input.NewPane
	} else {
		paneID := entity.PaneID(uc.idGenerator())
		newPane = entity.NewPane(paneID)
		if input.InitialURL != "" {
			newPane.URI = input.InitialURL
		} else {
			newPane.URI = "about:blank"
		}
	}

	// Create new pane node
	newPaneNode := &entity.PaneNode{
		ID:   string(newPane.ID),
		Pane: newPane,
	}

	// Determine split orientation
	var splitDir entity.SplitDirection
	switch input.Direction {
	case SplitLeft, SplitRight:
		splitDir = entity.SplitHorizontal
	case SplitUp, SplitDown:
		splitDir = entity.SplitVertical
	}

	// Create parent container node
	parentID := uc.idGenerator()
	parentNode := &entity.PaneNode{
		ID:         parentID,
		SplitDir:   splitDir,
		SplitRatio: 0.5,
		Children:   make([]*entity.PaneNode, 2),
	}

	// Arrange children based on direction
	switch input.Direction {
	case SplitLeft, SplitUp:
		// New pane goes first (left/top)
		parentNode.Children[0] = newPaneNode
		parentNode.Children[1] = targetNode
	case SplitRight, SplitDown:
		// Existing pane stays first, new pane goes second (right/bottom)
		parentNode.Children[0] = targetNode
		parentNode.Children[1] = newPaneNode
	}

	// Update parent references
	newPaneNode.Parent = parentNode
	oldParent := targetNode.Parent
	targetNode.Parent = parentNode

	// Insert parent into tree
	if oldParent == nil {
		// Target was root
		input.Workspace.Root = parentNode
	} else {
		// Replace target with parent in old parent's children
		for i, child := range oldParent.Children {
			if child == targetNode {
				oldParent.Children[i] = parentNode
				break
			}
		}
		parentNode.Parent = oldParent
	}

	log.Info().
		Str("new_pane_id", string(newPane.ID)).
		Str("parent_id", parentID).
		Str("direction", string(input.Direction)).
		Msg("pane split completed")

	return &SplitPaneOutput{
		NewPaneNode: newPaneNode,
		ParentNode:  parentNode,
		SplitRatio:  0.5,
	}, nil
}

// Close removes a pane and promotes its sibling.
// Returns the sibling node that was promoted, or nil if this was the last pane.
func (uc *ManagePanesUseCase) Close(ctx context.Context, ws *entity.Workspace, paneNode *entity.PaneNode) (*entity.PaneNode, error) {
	log := logging.FromContext(ctx)
	if uc == nil {
		return nil, fmt.Errorf("manage panes use case is nil")
	}
	log.Debug().Str("pane_id", paneNode.ID).Msg("closing pane")

	if ws == nil {
		return nil, fmt.Errorf("workspace is required")
	}
	if paneNode == nil {
		return nil, fmt.Errorf("pane node is required")
	}
	if !paneNode.IsLeaf() {
		return nil, fmt.Errorf("cannot close non-leaf node")
	}

	parent := paneNode.Parent

	// If no parent, this is the root (last pane)
	if parent == nil {
		log.Info().Msg("closing last pane in workspace")
		ws.Root = nil
		ws.ActivePaneID = ""
		return nil, nil
	}

	// Find sibling
	var sibling *entity.PaneNode
	for _, child := range parent.Children {
		if child != paneNode {
			sibling = child
			break
		}
	}

	if sibling == nil {
		return nil, fmt.Errorf("no sibling found for pane")
	}

	grandparent := parent.Parent

	// Promote sibling to parent's position
	if grandparent == nil {
		// Parent was root, sibling becomes new root
		ws.Root = sibling
		sibling.Parent = nil
	} else {
		// Replace parent with sibling in grandparent
		for i, child := range grandparent.Children {
			if child == parent {
				grandparent.Children[i] = sibling
				break
			}
		}
		sibling.Parent = grandparent
	}

	// Update active pane if needed
	if ws.ActivePaneID == paneNode.Pane.ID {
		// Find a new active pane from the sibling subtree
		var newActive *entity.Pane
		sibling.Walk(func(node *entity.PaneNode) bool {
			if node.IsLeaf() && node.Pane != nil {
				newActive = node.Pane
				return false // Stop walking
			}
			return true
		})
		if newActive != nil {
			ws.ActivePaneID = newActive.ID
		} else {
			ws.ActivePaneID = ""
		}
	}

	log.Info().
		Str("closed_pane_id", paneNode.ID).
		Str("promoted_sibling_id", sibling.ID).
		Msg("pane closed, sibling promoted")

	return sibling, nil
}

// Focus sets the active pane in the workspace.
func (uc *ManagePanesUseCase) Focus(ctx context.Context, ws *entity.Workspace, paneID entity.PaneID) error {
	log := logging.FromContext(ctx)
	if uc == nil {
		return fmt.Errorf("manage panes use case is nil")
	}
	log.Debug().Str("pane_id", string(paneID)).Msg("focusing pane")

	if ws == nil {
		return fmt.Errorf("workspace is required")
	}

	node := ws.FindPane(paneID)
	if node == nil {
		return fmt.Errorf("pane not found: %s", paneID)
	}

	oldActive := ws.ActivePaneID
	ws.ActivePaneID = paneID

	log.Info().
		Str("from", string(oldActive)).
		Str("to", string(paneID)).
		Msg("focus changed")

	return nil
}

// NavigateFocus moves focus to an adjacent pane.
// Returns the newly focused pane, or nil if navigation not possible.
func (uc *ManagePanesUseCase) NavigateFocus(
	ctx context.Context,
	ws *entity.Workspace,
	direction NavigateDirection,
) (*entity.PaneNode, error) {
	log := logging.FromContext(ctx)
	log.Debug().Str("direction", string(direction)).Msg("navigating focus")

	if ws == nil {
		return nil, fmt.Errorf("workspace is required")
	}

	activeNode := ws.ActivePane()
	if activeNode == nil {
		return nil, fmt.Errorf("no active pane")
	}

	// Try structural navigation first
	targetNode := uc.findAdjacentPane(activeNode, direction)

	if targetNode == nil {
		log.Debug().Msg("no adjacent pane found")
		return nil, nil
	}

	ws.ActivePaneID = targetNode.Pane.ID

	log.Info().
		Str("from", activeNode.ID).
		Str("to", targetNode.ID).
		Str("direction", string(direction)).
		Msg("focus navigated")

	return targetNode, nil
}

// GeometricNavigationInput contains data for geometric focus navigation.
type GeometricNavigationInput struct {
	ActivePaneID entity.PaneID
	PaneRects    []entity.PaneRect // All visible panes with their positions
	Direction    NavigateDirection
}

// GeometricNavigationOutput contains the result.
type GeometricNavigationOutput struct {
	TargetPaneID entity.PaneID
	Found        bool
}

// NavigateFocusGeometric finds the nearest pane in the given direction using geometry.
// Algorithm:
//  1. Get center of active pane
//  2. Filter candidates that are in the direction (dx < 0 for Left, etc.)
//  3. Score by: primary_distance * 1000 + perpendicular_distance
//  4. Return lowest scoring candidate
func (uc *ManagePanesUseCase) NavigateFocusGeometric(
	ctx context.Context,
	input GeometricNavigationInput,
) (*GeometricNavigationOutput, error) {
	log := logging.FromContext(ctx)
	if uc == nil {
		return nil, fmt.Errorf("manage panes use case is nil")
	}
	log.Debug().
		Str("direction", string(input.Direction)).
		Str("active", string(input.ActivePaneID)).
		Int("candidates", len(input.PaneRects)).
		Msg("geometric navigation")

	// Find active pane rect
	var activeRect *entity.PaneRect
	for i := range input.PaneRects {
		if input.PaneRects[i].PaneID == input.ActivePaneID {
			activeRect = &input.PaneRects[i]
			break
		}
	}
	if activeRect == nil {
		log.Debug().Msg("active pane rect not found")
		return &GeometricNavigationOutput{Found: false}, nil
	}

	acx, acy := activeRect.Center()

	type candidate struct {
		paneID entity.PaneID
		score  int
	}
	var candidates []candidate

	for _, rect := range input.PaneRects {
		if rect.PaneID == input.ActivePaneID {
			continue
		}

		cx, cy := rect.Center()
		dx := cx - acx
		dy := cy - acy

		var inDirection bool
		var primaryDist, perpDist int

		switch input.Direction {
		case NavLeft:
			inDirection = dx < 0
			primaryDist = abs(dx)
			perpDist = abs(dy)
		case NavRight:
			inDirection = dx > 0
			primaryDist = abs(dx)
			perpDist = abs(dy)
		case NavUp:
			inDirection = dy < 0
			primaryDist = abs(dy)
			perpDist = abs(dx)
		case NavDown:
			inDirection = dy > 0
			primaryDist = abs(dy)
			perpDist = abs(dx)
		}

		if inDirection {
			score := primaryDist*1000 + perpDist
			candidates = append(candidates, candidate{rect.PaneID, score})
		}
	}

	if len(candidates) == 0 {
		log.Debug().Msg("no candidates in direction")
		return &GeometricNavigationOutput{Found: false}, nil
	}

	// Sort by score (lowest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score < candidates[j].score
	})

	log.Debug().
		Str("target", string(candidates[0].paneID)).
		Int("score", candidates[0].score).
		Msg("geometric navigation found target")

	return &GeometricNavigationOutput{
		TargetPaneID: candidates[0].paneID,
		Found:        true,
	}, nil
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// findAdjacentPane finds the adjacent pane in the given direction using tree structure.
func (uc *ManagePanesUseCase) findAdjacentPane(node *entity.PaneNode, direction NavigateDirection) *entity.PaneNode {
	if node == nil {
		return nil
	}

	// Determine if we need to go horizontal or vertical
	isHorizontal := direction == NavLeft || direction == NavRight
	isForward := direction == NavRight || direction == NavDown

	// Walk up the tree to find an ancestor with the right split type
	current := node
	for current.Parent != nil {
		parent := current.Parent

		// Check if this parent's split orientation matches our direction
		parentIsHorizontal := parent.SplitDir == entity.SplitHorizontal

		if parentIsHorizontal == isHorizontal {
			// Find which child we came from
			childIndex := -1
			for i, child := range parent.Children {
				if child == current {
					childIndex = i
					break
				}
			}

			// Check if we can move in the desired direction
			var targetIndex int
			if isForward {
				targetIndex = childIndex + 1
			} else {
				targetIndex = childIndex - 1
			}

			if targetIndex >= 0 && targetIndex < len(parent.Children) {
				// Found a valid sibling, descend into it to find a leaf
				return uc.findLeafInDirection(parent.Children[targetIndex], !isForward)
			}
		}

		current = parent
	}

	return nil
}

// findLeafInDirection descends into a node to find a leaf pane.
// If fromEnd is true, prefers the last child; otherwise prefers the first.
func (uc *ManagePanesUseCase) findLeafInDirection(node *entity.PaneNode, fromEnd bool) *entity.PaneNode {
	if node == nil {
		return nil
	}

	// If this is a leaf, return it
	if node.IsLeaf() {
		return node
	}

	// If stacked, return active pane
	if node.IsStacked && len(node.Children) > 0 {
		activeIdx := node.ActiveStackIndex
		if activeIdx >= 0 && activeIdx < len(node.Children) {
			return uc.findLeafInDirection(node.Children[activeIdx], fromEnd)
		}
		return uc.findLeafInDirection(node.Children[0], fromEnd)
	}

	// Descend into children
	if len(node.Children) > 0 {
		if fromEnd {
			return uc.findLeafInDirection(node.Children[len(node.Children)-1], fromEnd)
		}
		return uc.findLeafInDirection(node.Children[0], fromEnd)
	}

	return nil
}

// CreateStackOutput contains the result of creating a stack from a pane.
type CreateStackOutput struct {
	StackNode    *entity.PaneNode // The stack container node
	OriginalNode *entity.PaneNode // The original pane, now a child of the stack
	NewPaneNode  *entity.PaneNode // The new pane added to the stack
	NewPane      *entity.Pane     // The new pane entity
}

// CreateStack converts a pane into a stacked container and adds a new pane.
// The original pane becomes the first child, a new pane becomes the second child.
// Returns the stack node and the new pane node for UI operations.
//
// Domain tree transformation:
//
//	Before: parent -> targetNode (leaf with pane)
//	After:  parent -> targetNode (stack) -> [originalChild (leaf), newChild (leaf)]
//
// The targetNode keeps its ID but becomes a stack container.
// This allows the UI layer to update widget mappings incrementally.
func (uc *ManagePanesUseCase) CreateStack(
	ctx context.Context,
	ws *entity.Workspace,
	paneNode *entity.PaneNode,
) (*CreateStackOutput, error) {
	log := logging.FromContext(ctx)
	log.Debug().Str("pane_id", paneNode.ID).Msg("creating stack from pane")

	if ws == nil {
		return nil, fmt.Errorf("workspace is required")
	}
	if paneNode == nil {
		return nil, fmt.Errorf("pane node is required")
	}
	if !paneNode.IsLeaf() {
		return nil, fmt.Errorf("can only create stack from leaf pane")
	}
	if paneNode.IsStacked {
		return nil, fmt.Errorf("pane is already stacked")
	}

	// Save original pane reference
	originalPane := paneNode.Pane
	originalPaneID := paneNode.ID

	// Create child node for the original pane
	originalChildNode := &entity.PaneNode{
		ID:     originalPaneID + "_0",
		Pane:   originalPane,
		Parent: paneNode,
	}

	// Create new pane
	newPaneID := entity.PaneID(uc.idGenerator())
	newPane := entity.NewPane(newPaneID)

	// Create child node for the new pane
	newChildNode := &entity.PaneNode{
		ID:     string(newPaneID),
		Pane:   newPane,
		Parent: paneNode,
	}

	// Convert targetNode to stack container
	paneNode.Pane = nil
	paneNode.IsStacked = true
	paneNode.ActiveStackIndex = 1 // New pane is active
	paneNode.Children = []*entity.PaneNode{originalChildNode, newChildNode}

	// Update workspace active pane
	ws.ActivePaneID = newPaneID

	log.Info().
		Str("stack_id", paneNode.ID).
		Str("original_pane", string(originalPane.ID)).
		Str("new_pane", string(newPaneID)).
		Msg("stack created")

	return &CreateStackOutput{
		StackNode:    paneNode,
		OriginalNode: originalChildNode,
		NewPaneNode:  newChildNode,
		NewPane:      newPane,
	}, nil
}

// AddToStackOutput contains the result of adding a pane to a stack.
type AddToStackOutput struct {
	NewPaneNode *entity.PaneNode // The new pane node added to the stack
	StackIndex  int              // Index of the new pane in the stack
}

// AddToStack adds a new pane to an existing stack.
// Optionally pass a pre-created pane, or nil to create a new one.
func (uc *ManagePanesUseCase) AddToStack(
	ctx context.Context,
	ws *entity.Workspace,
	stackNode *entity.PaneNode,
	pane *entity.Pane,
) (*AddToStackOutput, error) {
	log := logging.FromContext(ctx)

	if ws == nil {
		return nil, fmt.Errorf("workspace is required")
	}
	if stackNode == nil {
		return nil, fmt.Errorf("stack node is required")
	}
	if !stackNode.IsStacked {
		return nil, fmt.Errorf("node is not a stack")
	}

	// Create pane if not provided
	if pane == nil {
		paneID := entity.PaneID(uc.idGenerator())
		pane = entity.NewPane(paneID)
	}

	log.Debug().
		Str("stack_id", stackNode.ID).
		Str("pane_id", string(pane.ID)).
		Msg("adding pane to stack")

	// Create node for the new pane
	newNode := &entity.PaneNode{
		ID:     string(pane.ID),
		Pane:   pane,
		Parent: stackNode,
	}

	// Insert right after current active pane (not at end)
	currentActiveIndex := stackNode.ActiveStackIndex
	if currentActiveIndex < 0 || currentActiveIndex >= len(stackNode.Children) {
		// Clamp to valid range - default to end-1 if invalid
		currentActiveIndex = len(stackNode.Children) - 1
		if currentActiveIndex < 0 {
			currentActiveIndex = -1 // Will result in insertIndex = 0
		}
	}
	insertIndex := currentActiveIndex + 1

	// Slice insertion at correct position
	stackNode.Children = append(stackNode.Children, nil)
	copy(stackNode.Children[insertIndex+1:], stackNode.Children[insertIndex:])
	stackNode.Children[insertIndex] = newNode
	newIndex := insertIndex
	stackNode.ActiveStackIndex = newIndex

	// Update workspace active pane
	ws.ActivePaneID = pane.ID

	log.Info().
		Str("stack_id", stackNode.ID).
		Str("pane_id", string(pane.ID)).
		Int("stack_size", len(stackNode.Children)).
		Msg("pane added to stack")

	return &AddToStackOutput{
		NewPaneNode: newNode,
		StackIndex:  newIndex,
	}, nil
}

// NavigateStack cycles through stacked panes.
// direction: NavUp for previous, NavDown for next.
//
//nolint:revive // receiver required for interface consistency
func (uc *ManagePanesUseCase) NavigateStack(
	ctx context.Context,
	stackNode *entity.PaneNode,
	direction NavigateDirection,
) (*entity.Pane, error) {
	log := logging.FromContext(ctx)

	if stackNode == nil {
		return nil, fmt.Errorf("stack node is required")
	}

	log.Debug().
		Str("stack_id", stackNode.ID).
		Str("direction", string(direction)).
		Msg("navigating stack")
	if !stackNode.IsStacked {
		return nil, fmt.Errorf("node is not a stack")
	}
	if len(stackNode.Children) == 0 {
		return nil, fmt.Errorf("stack is empty")
	}

	count := len(stackNode.Children)
	current := stackNode.ActiveStackIndex

	var newIndex int
	switch direction {
	case NavUp:
		newIndex = (current - 1 + count) % count
	case NavDown:
		newIndex = (current + 1) % count
	default:
		return nil, fmt.Errorf("invalid stack navigation direction: %s (use up/down)", direction)
	}

	stackNode.ActiveStackIndex = newIndex

	activeChild := stackNode.Children[newIndex]
	var activePaneNode *entity.PaneNode
	if activeChild.IsLeaf() {
		activePaneNode = activeChild
	} else {
		// Find first leaf in the child
		activeChild.Walk(func(n *entity.PaneNode) bool {
			if n.IsLeaf() {
				activePaneNode = n
				return false
			}
			return true
		})
	}

	if activePaneNode == nil || activePaneNode.Pane == nil {
		return nil, fmt.Errorf("no pane found in stack at index %d", newIndex)
	}

	log.Info().
		Str("stack_id", stackNode.ID).
		Int("from_index", current).
		Int("to_index", newIndex).
		Str("pane_id", string(activePaneNode.Pane.ID)).
		Msg("stack navigated")

	return activePaneNode.Pane, nil
}

// RemoveFromStack removes a pane from a stack.
// If only one pane remains, the stack is dissolved.
//
//nolint:revive // receiver required for interface consistency
func (uc *ManagePanesUseCase) RemoveFromStack(ctx context.Context, stackNode *entity.PaneNode, paneID entity.PaneID) error {
	log := logging.FromContext(ctx)

	if stackNode == nil {
		return fmt.Errorf("stack node is required")
	}

	log.Debug().
		Str("stack_id", stackNode.ID).
		Str("pane_id", string(paneID)).
		Msg("removing pane from stack")
	if !stackNode.IsStacked {
		return fmt.Errorf("node is not a stack")
	}

	// Find and remove the pane
	var removed bool
	newChildren := make([]*entity.PaneNode, 0, len(stackNode.Children))
	for _, child := range stackNode.Children {
		if child.Pane != nil && child.Pane.ID == paneID {
			removed = true
			continue
		}
		newChildren = append(newChildren, child)
	}

	if !removed {
		return fmt.Errorf("pane not found in stack: %s", paneID)
	}

	stackNode.Children = newChildren

	// Adjust active index
	if stackNode.ActiveStackIndex >= len(stackNode.Children) {
		stackNode.ActiveStackIndex = len(stackNode.Children) - 1
	}
	if stackNode.ActiveStackIndex < 0 {
		stackNode.ActiveStackIndex = 0
	}

	log.Info().
		Str("stack_id", stackNode.ID).
		Str("pane_id", string(paneID)).
		Int("remaining", len(stackNode.Children)).
		Msg("pane removed from stack")

	return nil
}

// GetAllPanes returns all leaf panes in a workspace.
//
//nolint:revive // receiver required for interface consistency
func (uc *ManagePanesUseCase) GetAllPanes(ws *entity.Workspace) []*entity.Pane {
	if ws == nil || ws.Root == nil {
		return nil
	}
	return ws.AllPanes()
}

// CountPanes returns the number of panes in a workspace.
//
//nolint:revive // receiver required for interface consistency
func (uc *ManagePanesUseCase) CountPanes(ws *entity.Workspace) int {
	if ws == nil {
		return 0
	}
	return ws.PaneCount()
}
