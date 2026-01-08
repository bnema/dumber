package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
	domainurl "github.com/bnema/dumber/internal/domain/url"
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

// ResizeDirection indicates the direction for pane resizing.
type ResizeDirection string

const (
	ResizeIncreaseLeft  ResizeDirection = "increase_left"
	ResizeIncreaseRight ResizeDirection = "increase_right"
	ResizeIncreaseUp    ResizeDirection = "increase_up"
	ResizeIncreaseDown  ResizeDirection = "increase_down"

	ResizeDecreaseLeft  ResizeDirection = "decrease_left"
	ResizeDecreaseRight ResizeDirection = "decrease_right"
	ResizeDecreaseUp    ResizeDirection = "decrease_up"
	ResizeDecreaseDown  ResizeDirection = "decrease_down"

	ResizeIncrease ResizeDirection = "increase"
	ResizeDecrease ResizeDirection = "decrease"
)

var ErrNothingToResize = errors.New("nothing to resize")

type ConsumeOrExpelDirection string

const (
	ConsumeOrExpelLeft  ConsumeOrExpelDirection = "left"
	ConsumeOrExpelRight ConsumeOrExpelDirection = "right"
	ConsumeOrExpelUp    ConsumeOrExpelDirection = "up"
	ConsumeOrExpelDown  ConsumeOrExpelDirection = "down"
)

type ConsumeOrExpelResult struct {
	Action       string // "consumed", "expelled", or "none"
	ErrorMessage string
}

const consumeOrExpelExpelCycleMarker = "_expel_cycle"

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

	// If target is inside a stack, split around the entire stack
	// (i.e., create the new pane as a sibling to the stack, not inside it)
	// This applies to all directions - we don't want to nest split containers inside stacks.
	targetNode := input.TargetPane
	if targetNode.Parent != nil && targetNode.Parent.IsStacked {
		// Promote to the stack container so split happens outside the stack
		targetNode = targetNode.Parent
		log.Debug().Msg("target is inside a stack, splitting around stack container")
	}

	// Create new pane
	var newPane *entity.Pane
	if input.NewPane != nil {
		newPane = input.NewPane
	} else {
		paneID := entity.PaneID(uc.idGenerator())
		newPane = entity.NewPane(paneID)
		if input.InitialURL != "" {
			newPane.URI = domainurl.Normalize(input.InitialURL)
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

// Resize adjusts the nearest applicable split ratio for the given direction.
// stepPercent is applied per keystroke (e.g. 5.0 means 5%).
// minPanePercent enforces a minimum size for each side of a split.
func (uc *ManagePanesUseCase) Resize(
	ctx context.Context,
	ws *entity.Workspace,
	paneNode *entity.PaneNode,
	dir ResizeDirection,
	stepPercent float64,
	minPanePercent float64,
) error {
	log := logging.FromContext(ctx)
	if uc == nil {
		return fmt.Errorf("manage panes use case is nil")
	}

	if ws == nil {
		return fmt.Errorf("workspace is required")
	}
	if ws.Root == nil {
		return ErrNothingToResize
	}
	if paneNode == nil {
		return fmt.Errorf("pane node is required")
	}

	target := paneNode
	if target.Parent != nil && target.Parent.IsStacked {
		target = target.Parent
	}

	actualDir := dir
	switch dir {
	case ResizeIncrease:
		actualDir = findSmartResizeDirection(target, true)
	case ResizeDecrease:
		actualDir = findSmartResizeDirection(target, false)
	}
	if actualDir == "" {
		return ErrNothingToResize
	}

	axis, ok := axisForResizeDirection(actualDir)
	if !ok {
		return ErrNothingToResize
	}

	splitNode := findNearestSplitForAxis(target, axis)
	if splitNode == nil {
		return ErrNothingToResize
	}

	// We treat resize directions as moving the split divider.
	// SplitRatio is the proportion allocated to the first child (left/top).
	// - Moving the divider right/down increases SplitRatio.
	// - Moving the divider left/up decreases SplitRatio.
	delta := deltaForDividerMove(actualDir, stepPercent)

	minRatio := minPanePercent / 100.0
	maxRatio := 1.0 - minRatio
	oldRatio := splitNode.SplitRatio
	splitNode.SplitRatio = clampFloat64(splitNode.SplitRatio+delta, minRatio, maxRatio)

	log.Debug().
		Str("direction", string(dir)).
		Str("actual_direction", string(actualDir)).
		Float64("old_ratio", oldRatio).
		Float64("new_ratio", splitNode.SplitRatio).
		Msg("pane resized")

	return nil
}

type SetSplitRatioInput struct {
	Workspace      *entity.Workspace
	SplitNodeID    string
	Ratio          float64
	MinPanePercent float64
}

func (uc *ManagePanesUseCase) SetSplitRatio(ctx context.Context, input SetSplitRatioInput) error {
	log := logging.FromContext(ctx)
	if uc == nil {
		return fmt.Errorf("manage panes use case is nil")
	}
	if input.Workspace == nil {
		return fmt.Errorf("workspace is required")
	}
	if input.Workspace.Root == nil {
		return ErrNothingToResize
	}
	if input.SplitNodeID == "" {
		return fmt.Errorf("split node id is required")
	}

	var splitNode *entity.PaneNode
	input.Workspace.Root.Walk(func(node *entity.PaneNode) bool {
		if node.ID == input.SplitNodeID {
			splitNode = node
			return false
		}
		return true
	})

	if splitNode == nil || !splitNode.IsSplit() {
		return fmt.Errorf("split node not found: %s", input.SplitNodeID)
	}

	minRatio := input.MinPanePercent / 100.0
	maxRatio := 1.0 - minRatio
	oldRatio := splitNode.SplitRatio
	clamped := clampFloat64(input.Ratio, minRatio, maxRatio)
	splitNode.SplitRatio = roundSplitRatio(clamped)

	log.Debug().
		Str("split_node_id", input.SplitNodeID).
		Float64("old_ratio", oldRatio).
		Float64("new_ratio", splitNode.SplitRatio).
		Msg("split ratio set")

	return nil
}

func findSmartResizeDirection(target *entity.PaneNode, growActive bool) ResizeDirection {
	splitNode, axis, isStartChild := findNearestSplitForResize(target)
	if splitNode == nil {
		return ""
	}

	// Smart resize means "grow/shrink the active pane".
	// SplitRatio is the proportion allocated to the first child (left/top).
	// - If active is first child: grow by increasing ratio, shrink by decreasing.
	// - If active is second child: grow by decreasing ratio, shrink by increasing.
	growMeansIncreaseRatio := isStartChild
	if !growActive {
		growMeansIncreaseRatio = !growMeansIncreaseRatio
	}

	switch axis {
	case resizeAxisHorizontal:
		if growMeansIncreaseRatio {
			return ResizeIncreaseRight
		}
		return ResizeIncreaseLeft
	case resizeAxisVertical:
		if growMeansIncreaseRatio {
			return ResizeIncreaseDown
		}
		return ResizeIncreaseUp
	default:
		return ""
	}
}

// findNearestSplitForResize returns the nearest split ancestor for the active pane.
// It prefers horizontal splits over vertical when both are available at the same depth.
func findNearestSplitForResize(node *entity.PaneNode) (splitNode *entity.PaneNode, axis resizeAxis, isStartChild bool) {
	current := node
	for current != nil && current.Parent != nil {
		parent := current.Parent
		if parent.IsSplit() {
			isStartChild = parent.Left() == current
			switch parent.SplitDir {
			case entity.SplitHorizontal:
				return parent, resizeAxisHorizontal, isStartChild
			case entity.SplitVertical:
				return parent, resizeAxisVertical, isStartChild
			}
		}
		current = parent
	}
	return nil, resizeAxisNone, false
}

type resizeAxis int

const (
	resizeAxisNone resizeAxis = iota
	resizeAxisHorizontal
	resizeAxisVertical
)

func axisForResizeDirection(dir ResizeDirection) (resizeAxis, bool) {
	switch dir {
	case ResizeIncreaseLeft, ResizeIncreaseRight, ResizeDecreaseLeft, ResizeDecreaseRight:
		return resizeAxisHorizontal, true
	case ResizeIncreaseUp, ResizeIncreaseDown, ResizeDecreaseUp, ResizeDecreaseDown:
		return resizeAxisVertical, true
	default:
		return resizeAxisNone, false
	}
}

func deltaForDividerMove(dir ResizeDirection, stepPercent float64) float64 {
	if stepPercent < 0 {
		stepPercent = -stepPercent
	}
	delta := stepPercent / 100.0

	switch dir {
	case ResizeIncreaseRight, ResizeIncreaseDown:
		return delta
	case ResizeIncreaseLeft, ResizeIncreaseUp:
		return -delta
	case ResizeDecreaseRight, ResizeDecreaseDown:
		return -delta
	case ResizeDecreaseLeft, ResizeDecreaseUp:
		return delta
	default:
		return 0
	}
}

// findNearestSplitForAxis walks up the tree to find the nearest split matching the axis.
func findNearestSplitForAxis(node *entity.PaneNode, axis resizeAxis) *entity.PaneNode {
	current := node
	for current != nil && current.Parent != nil {
		parent := current.Parent
		if parent.IsSplit() {
			if axis == resizeAxisHorizontal && parent.SplitDir == entity.SplitHorizontal {
				return parent
			}
			if axis == resizeAxisVertical && parent.SplitDir == entity.SplitVertical {
				return parent
			}
		}
		current = parent
	}
	return nil
}

const splitRatioRoundFactor = 100.0

func roundSplitRatio(ratio float64) float64 {
	// Keep snapshots stable and readable; avoids persisting noisy float values.
	return math.Round(ratio*splitRatioRoundFactor) / splitRatioRoundFactor
}

func clampFloat64(v, minVal, maxVal float64) float64 {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

// Close removes a pane and promotes its sibling.
// Returns the sibling node that was promoted, or nil if this was the last pane.
// For stacked panes, removes the pane from the stack via RemoveFromStack.
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

	// Handle stacked panes differently - they can have more than 2 children
	if parent.IsStacked {
		return uc.closeStackedPane(ctx, ws, paneNode, parent)
	}

	// Binary split: find sibling (the other child of parent)
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

// closeStackedPane handles closing a pane within a stack.
// If multiple panes remain, removes the pane from the stack.
// If only one pane would remain, dissolves the stack.
func (uc *ManagePanesUseCase) closeStackedPane(
	ctx context.Context,
	ws *entity.Workspace,
	paneNode *entity.PaneNode,
	stackNode *entity.PaneNode,
) (*entity.PaneNode, error) {
	log := logging.FromContext(ctx)

	paneID := paneNode.Pane.ID

	// If more than 2 panes in stack, just remove this one
	if len(stackNode.Children) > 2 {
		if err := uc.RemoveFromStack(ctx, stackNode, paneID); err != nil {
			return nil, fmt.Errorf("failed to remove pane from stack: %w", err)
		}

		// Update active pane if we closed the active one
		if ws.ActivePaneID == paneID {
			// Use the pane at the current active index
			if stackNode.ActiveStackIndex >= 0 && stackNode.ActiveStackIndex < len(stackNode.Children) {
				activeChild := stackNode.Children[stackNode.ActiveStackIndex]
				if activeChild.Pane != nil {
					ws.ActivePaneID = activeChild.Pane.ID
				}
			}
		}

		log.Info().
			Str("pane_id", string(paneID)).
			Int("remaining", len(stackNode.Children)).
			Msg("pane removed from stack")

		return stackNode, nil
	}

	// If exactly 2 panes, remove this one and dissolve the stack
	if len(stackNode.Children) == 2 {
		// Find the remaining pane
		var remaining *entity.PaneNode
		for _, child := range stackNode.Children {
			if child.Pane != nil && child.Pane.ID != paneID {
				remaining = child
				break
			}
		}

		if remaining == nil {
			return nil, fmt.Errorf("no remaining pane found in stack")
		}

		// Dissolve the stack: promote remaining pane to stack's position
		grandparent := stackNode.Parent

		if grandparent == nil {
			// Stack was root, remaining becomes new root
			ws.Root = remaining
			remaining.Parent = nil
		} else {
			// Replace stack with remaining in grandparent
			for i, child := range grandparent.Children {
				if child == stackNode {
					grandparent.Children[i] = remaining
					break
				}
			}
			remaining.Parent = grandparent
		}

		// Update active pane if we closed the active one
		if ws.ActivePaneID == paneID && remaining.Pane != nil {
			ws.ActivePaneID = remaining.Pane.ID
		}

		log.Info().
			Str("closed_pane_id", string(paneID)).
			Str("remaining_pane_id", remaining.ID).
			Msg("stack dissolved, remaining pane promoted")

		return remaining, nil
	}

	// Should not happen: stack with less than 2 children
	return nil, fmt.Errorf("invalid stack: has %d children", len(stackNode.Children))
}

// Focus sets the active pane in the workspace.
// Delegates to ApplyFocusChange to ensure stack index is updated when focusing a stacked pane.
func (uc *ManagePanesUseCase) Focus(ctx context.Context, ws *entity.Workspace, paneID entity.PaneID) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("pane_id", string(paneID)).Msg("focusing pane")

	_, err := uc.ApplyFocusChange(ctx, ws, paneID)
	return err
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

// ApplyFocusChange sets the active pane and updates the stack index if the target pane is in a stack.
// This is the canonical way to change focus to a pane, ensuring stack state is kept in sync.
func (uc *ManagePanesUseCase) ApplyFocusChange(
	ctx context.Context,
	ws *entity.Workspace,
	targetPaneID entity.PaneID,
) (*entity.PaneNode, error) {
	log := logging.FromContext(ctx)
	if uc == nil {
		return nil, fmt.Errorf("manage panes use case is nil")
	}
	if ws == nil {
		return nil, fmt.Errorf("workspace is required")
	}

	targetNode := ws.FindPane(targetPaneID)
	if targetNode == nil {
		return nil, fmt.Errorf("pane not found: %s", targetPaneID)
	}

	oldActive := ws.ActivePaneID
	ws.ActivePaneID = targetPaneID

	// Update stack index if target is in a stack
	if targetNode.Parent != nil && targetNode.Parent.IsStacked {
		oldIndex := targetNode.Parent.ActiveStackIndex
		for i, child := range targetNode.Parent.Children {
			if child == targetNode {
				targetNode.Parent.ActiveStackIndex = i
				log.Debug().
					Str("target_pane_id", string(targetPaneID)).
					Int("old_stack_index", oldIndex).
					Int("new_stack_index", i).
					Int("num_children", len(targetNode.Parent.Children)).
					Msg("updated stack index for focus change")
				break
			}
		}
	}

	log.Info().
		Str("from", string(oldActive)).
		Str("to", string(targetPaneID)).
		Msg("focus changed")

	return targetNode, nil
}

// NavigateFocusGeometric finds the nearest pane in the given direction using geometry.
// Algorithm:
//  1. Get active pane rectangle
//  2. Filter candidates that are in the direction (dx < 0 for Left, etc.)
//  3. Prioritize panes with perpendicular overlap (same row for left/right, same column for up/down)
//  4. Score by: overlap_penalty + primary_distance * 1000 + perpendicular_distance
//  5. Return lowest scoring candidate
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

	candidates := scoreNavigationCandidates(*activeRect, input.PaneRects, input.Direction)

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

// navCandidate represents a pane candidate for navigation with its score.
type navCandidate struct {
	paneID entity.PaneID
	score  int
}

// scoreNavigationCandidates scores all panes in the given direction from activeRect.
// Panes with perpendicular overlap are heavily preferred (same row for left/right, same column for up/down).
func scoreNavigationCandidates(
	activeRect entity.PaneRect,
	paneRects []entity.PaneRect,
	direction NavigateDirection,
) []navCandidate {
	// Large penalty for panes without perpendicular overlap.
	// This ensures panes at the same level are always preferred.
	const noOverlapPenalty = 10_000_000

	acx, acy := activeRect.Center()
	var candidates []navCandidate

	for _, rect := range paneRects {
		if rect.PaneID == activeRect.PaneID {
			continue
		}

		cx, cy := rect.Center()
		dx := cx - acx
		dy := cy - acy

		inDirection, primaryDist, perpDist, hasOverlap := evalDirection(activeRect, rect, dx, dy, direction)

		if inDirection {
			score := primaryDist*1000 + perpDist
			if !hasOverlap {
				score += noOverlapPenalty
			}
			candidates = append(candidates, navCandidate{rect.PaneID, score})
		}
	}

	return candidates
}

// evalDirection determines if a candidate rect is in the given direction from activeRect.
// Returns: inDirection, primaryDist, perpDist, hasOverlap
func evalDirection(
	activeRect, rect entity.PaneRect,
	dx, dy int,
	direction NavigateDirection,
) (inDirection bool, primaryDist, perpDist int, hasOverlap bool) {
	switch direction {
	case NavLeft:
		return dx < 0, abs(dx), abs(dy), activeRect.OverlapsVertically(rect)
	case NavRight:
		return dx > 0, abs(dx), abs(dy), activeRect.OverlapsVertically(rect)
	case NavUp:
		return dy < 0, abs(dy), abs(dx), activeRect.OverlapsHorizontally(rect)
	case NavDown:
		return dy > 0, abs(dy), abs(dx), activeRect.OverlapsHorizontally(rect)
	default:
		return false, 0, 0, false
	}
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

func (uc *ManagePanesUseCase) ConsumeOrExpel(
	ctx context.Context,
	ws *entity.Workspace,
	activeNode *entity.PaneNode,
	direction ConsumeOrExpelDirection,
) (*ConsumeOrExpelResult, error) {
	if uc == nil {
		return nil, fmt.Errorf("manage panes use case is nil")
	}
	if ws == nil {
		return nil, fmt.Errorf("workspace is required")
	}
	if ws.Root == nil {
		return &ConsumeOrExpelResult{Action: "none", ErrorMessage: "Only one pane"}, nil
	}
	if activeNode == nil {
		return nil, fmt.Errorf("active pane node is required")
	}
	if activeNode.Pane == nil {
		return nil, fmt.Errorf("active pane node has no pane")
	}

	if ws.PaneCount() <= 1 {
		return &ConsumeOrExpelResult{Action: "none", ErrorMessage: "Only one pane"}, nil
	}

	// Expel if active pane is inside a stack.
	if activeNode.Parent != nil && activeNode.Parent.IsStacked {
		return uc.expelFromStack(ctx, ws, activeNode, direction)
	}

	// Consume if active pane is not inside a stack.
	return uc.consumeIntoSiblingStack(ctx, ws, activeNode, direction)
}

func (*ManagePanesUseCase) consumeIntoSiblingStack(
	ctx context.Context,
	ws *entity.Workspace,
	activeNode *entity.PaneNode,
	direction ConsumeOrExpelDirection,
) (*ConsumeOrExpelResult, error) {
	log := logging.FromContext(ctx)

	sibling, _ := findAdjacentSiblingForConsumeOrExpel(activeNode, direction)
	if sibling == nil {
		sibling = fallbackSiblingForConsumeOrExpel(activeNode, direction)
	}
	if sibling == nil {
		return &ConsumeOrExpelResult{Action: "none", ErrorMessage: directionNoSiblingMessage(direction)}, nil
	}

	// Left/right consume cycles layout for immediate split siblings:
	//  1) horizontal split -> vertical split (target on top)
	//  2) vertical split -> stack (top then bottom)
	if direction == ConsumeOrExpelLeft || direction == ConsumeOrExpelRight {
		cycled, cycleAction, err := cycleHorizontalToVerticalThenStack(ws, activeNode, sibling, direction)
		if err != nil {
			return nil, err
		}
		if cycled {
			switch cycleAction {
			case "vertical_split":
				log.Info().
					Str("pane_id", activeNode.ID).
					Str("sibling_id", sibling.ID).
					Msg("pane moved below sibling")
			case "stack":
				log.Info().
					Str("pane_id", activeNode.ID).
					Str("sibling_id", sibling.ID).
					Msg("split converted to stack")
			}
			return &ConsumeOrExpelResult{Action: "consumed"}, nil
		}
	}

	moved, err := detachLeafByPromotingSibling(ws, activeNode)
	if err != nil {
		return nil, err
	}
	moved.Parent = nil

	stackNode := sibling
	if stackNode.IsLeaf() {
		stackNode = convertLeafToStackContainer(stackNode)
	}
	if !stackNode.IsStacked {
		return nil, fmt.Errorf("target sibling is not stackable")
	}

	// Insert consumed pane at the bottom/end.
	moved.Parent = stackNode
	stackNode.Children = append(stackNode.Children, moved)
	stackNode.ActiveStackIndex = len(stackNode.Children) - 1
	ws.ActivePaneID = moved.Pane.ID

	log.Info().
		Str("pane_id", moved.ID).
		Str("stack_id", stackNode.ID).
		Int("stack_size", len(stackNode.Children)).
		Msg("pane consumed into sibling stack")

	return &ConsumeOrExpelResult{Action: "consumed"}, nil
}

func (uc *ManagePanesUseCase) expelFromStack(
	ctx context.Context,
	ws *entity.Workspace,
	activeNode *entity.PaneNode,
	direction ConsumeOrExpelDirection,
) (*ConsumeOrExpelResult, error) {
	log := logging.FromContext(ctx)

	stackNode := activeNode.Parent
	if stackNode == nil || !stackNode.IsStacked {
		return &ConsumeOrExpelResult{Action: "none"}, nil
	}

	expelled, err := removeLeafFromStack(stackNode, activeNode)
	if err != nil {
		return nil, err
	}
	expelled.Parent = nil

	// If only one pane remains, dissolve stack into a leaf node.
	if len(stackNode.Children) == 1 {
		dissolveStackIntoLeaf(stackNode)
	}

	// Split around the remaining container (stackNode) with expelled pane.
	// For left/right, expel cycles stack -> vertical split first:
	// - left: expelled pane above
	// - right: expelled pane below
	splitDirection := direction
	switch direction {
	case ConsumeOrExpelLeft:
		splitDirection = ConsumeOrExpelUp
	case ConsumeOrExpelRight:
		splitDirection = ConsumeOrExpelDown
	}

	if direction == ConsumeOrExpelLeft || direction == ConsumeOrExpelRight {
		err := splitExistingNodeWithMarker(
			ws,
			stackNode,
			expelled,
			splitDirection,
			uc.idGenerator,
			consumeOrExpelExpelCycleMarker,
		)
		if err != nil {
			return nil, err
		}
	} else {
		if err := splitExistingNode(ws, stackNode, expelled, splitDirection, uc.idGenerator); err != nil {
			return nil, err
		}
	}
	ws.ActivePaneID = expelled.Pane.ID

	log.Info().
		Str("pane_id", expelled.ID).
		Str("direction", string(direction)).
		Msg("pane expelled from stack")

	return &ConsumeOrExpelResult{Action: "expelled"}, nil
}

func directionNoSiblingMessage(direction ConsumeOrExpelDirection) string {
	switch direction {
	case ConsumeOrExpelLeft:
		return "No pane to the left"
	case ConsumeOrExpelRight:
		return "No pane to the right"
	case ConsumeOrExpelUp:
		return "No pane above"
	case ConsumeOrExpelDown:
		return "No pane below"
	default:
		return "No pane in that direction"
	}
}

func findAdjacentSiblingForConsumeOrExpel(node *entity.PaneNode, direction ConsumeOrExpelDirection) (*entity.PaneNode, string) {
	if node == nil {
		return nil, ""
	}

	// Determine if we need to go horizontal or vertical.
	isHorizontal := direction == ConsumeOrExpelLeft || direction == ConsumeOrExpelRight
	isForward := direction == ConsumeOrExpelRight || direction == ConsumeOrExpelDown

	// Walk up the tree to find an ancestor split matching our direction axis.
	current := node
	for current.Parent != nil {
		parent := current.Parent
		if parent.IsSplit() {
			parentIsHorizontal := parent.SplitDir == entity.SplitHorizontal
			if parentIsHorizontal == isHorizontal {
				if isForward {
					if parent.Right() != nil && parent.Left() == current {
						return parent.Right(), ""
					}
				} else {
					if parent.Left() != nil && parent.Right() == current {
						return parent.Left(), ""
					}
				}
			}
		}
		current = parent
	}

	return nil, directionNoSiblingMessage(direction)
}

func fallbackSiblingForConsumeOrExpel(node *entity.PaneNode, direction ConsumeOrExpelDirection) *entity.PaneNode {
	if node == nil || node.Parent == nil {
		return nil
	}

	parent := node.Parent
	if !parent.IsSplit() {
		return nil
	}

	axisHorizontal := direction == ConsumeOrExpelLeft || direction == ConsumeOrExpelRight
	if axisHorizontal {
		if parent.SplitDir == entity.SplitHorizontal {
			return nil
		}
	} else {
		if parent.SplitDir == entity.SplitVertical {
			return nil
		}
	}

	// Cross-axis fallback is only used to enable the leaf cycling behavior
	// (H -> V -> stack, and after expel: V -> H). It should never cause a leaf to
	// consume into a stacked/split sibling when pressing an unrelated axis key.
	var sibling *entity.PaneNode
	if parent.Left() == node {
		sibling = parent.Right()
	} else if parent.Right() == node {
		sibling = parent.Left()
	}
	if sibling == nil {
		return nil
	}
	if !node.IsLeaf() || !sibling.IsLeaf() {
		return nil
	}
	return sibling
}

func cycleHorizontalToVerticalThenStack(
	ws *entity.Workspace,
	activeNode *entity.PaneNode,
	sibling *entity.PaneNode,
	direction ConsumeOrExpelDirection,
) (cycled bool, cycleAction string, err error) {
	if ws == nil {
		return false, "", fmt.Errorf("workspace is required")
	}
	if activeNode == nil || sibling == nil {
		return false, "", nil
	}
	if !activeNode.IsLeaf() || !sibling.IsLeaf() {
		return false, "", nil
	}

	parent := activeNode.Parent
	if parent == nil || sibling.Parent != parent || !parent.IsSplit() {
		return false, "", nil
	}

	switch parent.SplitDir {
	case entity.SplitHorizontal:
		// Horizontal -> Vertical: target sibling should be on top.
		parent.SplitDir = entity.SplitVertical
		if parent.Children[0] != sibling {
			parent.Children[0], parent.Children[1] = parent.Children[1], parent.Children[0]
		}
		ws.ActivePaneID = activeNode.Pane.ID
		return true, "vertical_split", nil
	case entity.SplitVertical:
		// Vertical -> Horizontal when pressing left/right (reverse of first step).
		// This is used after expel: stack -> vertical; then Ctrl+[ / Ctrl+] should turn it into left/right.
		if direction == ConsumeOrExpelRight && strings.Contains(parent.ID, consumeOrExpelExpelCycleMarker) {
			parent.SplitDir = entity.SplitHorizontal
			// Active should become the right child.
			if parent.Children[1] != activeNode {
				parent.Children[0], parent.Children[1] = parent.Children[1], parent.Children[0]
			}
			ws.ActivePaneID = activeNode.Pane.ID
			return true, "horizontal_split", nil
		}

		// Otherwise, vertical -> stack.
		parent.SplitDir = entity.SplitNone
		parent.SplitRatio = 0
		parent.IsStacked = true
		parent.Pane = nil
		if parent.Children[0] == activeNode {
			parent.ActiveStackIndex = 0
		} else if parent.Children[1] == activeNode {
			parent.ActiveStackIndex = 1
		} else {
			parent.ActiveStackIndex = 0
		}
		ws.ActivePaneID = activeNode.Pane.ID
		return true, "stack", nil
	default:
		return false, "", nil
	}
}

func detachLeafByPromotingSibling(ws *entity.Workspace, leaf *entity.PaneNode) (*entity.PaneNode, error) {
	if ws == nil {
		return nil, fmt.Errorf("workspace is required")
	}
	if leaf == nil {
		return nil, fmt.Errorf("leaf is required")
	}
	if !leaf.IsLeaf() {
		return nil, fmt.Errorf("can only detach leaf node")
	}

	parent := leaf.Parent
	if parent == nil {
		return nil, fmt.Errorf("cannot detach root leaf")
	}
	if !parent.IsSplit() {
		return nil, fmt.Errorf("leaf parent is not a split")
	}

	var sibling *entity.PaneNode
	for _, child := range parent.Children {
		if child != leaf {
			sibling = child
			break
		}
	}
	if sibling == nil {
		return nil, fmt.Errorf("no sibling found for leaf")
	}

	grandparent := parent.Parent
	if grandparent == nil {
		ws.Root = sibling
		sibling.Parent = nil
	} else {
		for i, child := range grandparent.Children {
			if child == parent {
				grandparent.Children[i] = sibling
				break
			}
		}
		sibling.Parent = grandparent
	}

	leaf.Parent = nil
	return leaf, nil
}

func convertLeafToStackContainer(leaf *entity.PaneNode) *entity.PaneNode {
	if leaf == nil || !leaf.IsLeaf() {
		return leaf
	}

	originalPane := leaf.Pane
	originalChildNode := &entity.PaneNode{
		ID:     leaf.ID + "_0",
		Pane:   originalPane,
		Parent: leaf,
	}

	leaf.Pane = nil
	leaf.IsStacked = true
	leaf.ActiveStackIndex = 0
	leaf.Children = []*entity.PaneNode{originalChildNode}

	return leaf
}

func removeLeafFromStack(stackNode, leaf *entity.PaneNode) (*entity.PaneNode, error) {
	if stackNode == nil {
		return nil, fmt.Errorf("stack node is required")
	}
	if !stackNode.IsStacked {
		return nil, fmt.Errorf("node is not a stack")
	}
	if leaf == nil || leaf.Pane == nil {
		return nil, fmt.Errorf("leaf is required")
	}

	idx := -1
	for i, child := range stackNode.Children {
		if child == leaf {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("pane not found in stack")
	}

	stackNode.Children = append(stackNode.Children[:idx], stackNode.Children[idx+1:]...)
	if stackNode.ActiveStackIndex >= len(stackNode.Children) {
		stackNode.ActiveStackIndex = len(stackNode.Children) - 1
	}
	if stackNode.ActiveStackIndex < 0 {
		stackNode.ActiveStackIndex = 0
	}

	leaf.Parent = nil
	return leaf, nil
}

func dissolveStackIntoLeaf(stackNode *entity.PaneNode) {
	if stackNode == nil || !stackNode.IsStacked || len(stackNode.Children) != 1 {
		return
	}

	remaining := stackNode.Children[0]
	stackNode.Pane = remaining.Pane
	stackNode.Children = nil
	stackNode.IsStacked = false
	stackNode.ActiveStackIndex = 0
}

func splitExistingNode(
	ws *entity.Workspace,
	targetNode *entity.PaneNode,
	newSibling *entity.PaneNode,
	direction ConsumeOrExpelDirection,
	idGen IDGenerator,
) error {
	if ws == nil {
		return fmt.Errorf("workspace is required")
	}
	if targetNode == nil {
		return fmt.Errorf("target node is required")
	}
	if newSibling == nil {
		return fmt.Errorf("new sibling node is required")
	}
	if idGen == nil {
		return fmt.Errorf("id generator is required")
	}

	var splitDir entity.SplitDirection
	switch direction {
	case ConsumeOrExpelLeft, ConsumeOrExpelRight:
		splitDir = entity.SplitHorizontal
	case ConsumeOrExpelUp, ConsumeOrExpelDown:
		splitDir = entity.SplitVertical
	default:
		return fmt.Errorf("invalid direction: %s", direction)
	}

	parentNode := &entity.PaneNode{
		ID:         idGen(),
		SplitDir:   splitDir,
		SplitRatio: 0.5,
		Children:   make([]*entity.PaneNode, 2),
	}

	switch direction {
	case ConsumeOrExpelLeft, ConsumeOrExpelUp:
		parentNode.Children[0] = newSibling
		parentNode.Children[1] = targetNode
	case ConsumeOrExpelRight, ConsumeOrExpelDown:
		parentNode.Children[0] = targetNode
		parentNode.Children[1] = newSibling
	}

	newSibling.Parent = parentNode
	oldParent := targetNode.Parent
	targetNode.Parent = parentNode

	if oldParent == nil {
		ws.Root = parentNode
	} else {
		for i, child := range oldParent.Children {
			if child == targetNode {
				oldParent.Children[i] = parentNode
				break
			}
		}
		parentNode.Parent = oldParent
	}

	return nil
}

func splitExistingNodeWithMarker(
	ws *entity.Workspace,
	targetNode *entity.PaneNode,
	newSibling *entity.PaneNode,
	direction ConsumeOrExpelDirection,
	idGen IDGenerator,
	marker string,
) error {
	if ws == nil {
		return fmt.Errorf("workspace is required")
	}
	if targetNode == nil {
		return fmt.Errorf("target node is required")
	}
	if newSibling == nil {
		return fmt.Errorf("new sibling node is required")
	}
	if idGen == nil {
		return fmt.Errorf("id generator is required")
	}

	var splitDir entity.SplitDirection
	switch direction {
	case ConsumeOrExpelLeft, ConsumeOrExpelRight:
		splitDir = entity.SplitHorizontal
	case ConsumeOrExpelUp, ConsumeOrExpelDown:
		splitDir = entity.SplitVertical
	default:
		return fmt.Errorf("invalid direction: %s", direction)
	}

	parentNode := &entity.PaneNode{
		ID:         idGen() + marker,
		SplitDir:   splitDir,
		SplitRatio: 0.5,
		Children:   make([]*entity.PaneNode, 2),
	}

	switch direction {
	case ConsumeOrExpelLeft, ConsumeOrExpelUp:
		parentNode.Children[0] = newSibling
		parentNode.Children[1] = targetNode
	case ConsumeOrExpelRight, ConsumeOrExpelDown:
		parentNode.Children[0] = targetNode
		parentNode.Children[1] = newSibling
	}

	newSibling.Parent = parentNode
	oldParent := targetNode.Parent
	targetNode.Parent = parentNode

	if oldParent == nil {
		ws.Root = parentNode
	} else {
		for i, child := range oldParent.Children {
			if child == targetNode {
				oldParent.Children[i] = parentNode
				break
			}
		}
		parentNode.Parent = oldParent
	}

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
