package coordinator

import (
	"errors"
	"fmt"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/layout"
)

type incrementalCloseContext struct {
	parentNode           *entity.PaneNode
	siblingNode          *entity.PaneNode
	grandparentNode      *entity.PaneNode
	parentWidget         layout.Widget
	siblingIsStartChild  bool
	parentIsStartInGrand bool
	precheckReason       string
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
