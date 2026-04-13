package usecase

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
)

// ExecuteContextMenuActionInput identifies the action and its context.
type ExecuteContextMenuActionInput struct {
	Action  port.MenuAction
	Context port.MenuContext
}

// ExecuteContextMenuActionUseCase handles shared context menu actions.
type ExecuteContextMenuActionUseCase struct {
	clipboard port.Clipboard
	resolver  port.ImageDataResolver
	saver     port.ResolvedImageSaver
	delegator port.MenuActionDelegator
}

// NewExecuteContextMenuActionUseCase creates a new ExecuteContextMenuActionUseCase.
func NewExecuteContextMenuActionUseCase(
	clipboard port.Clipboard,
	resolver port.ImageDataResolver,
	saver port.ResolvedImageSaver,
	delegator port.MenuActionDelegator,
) *ExecuteContextMenuActionUseCase {
	return &ExecuteContextMenuActionUseCase{
		clipboard: clipboard,
		resolver:  resolver,
		saver:     saver,
		delegator: delegator,
	}
}

// Execute handles the requested action.
func (uc *ExecuteContextMenuActionUseCase) Execute(ctx context.Context, input ExecuteContextMenuActionInput) error {
	switch input.Action {
	case port.MenuActionCopyLink:
		if input.Context.LinkURI == "" {
			return fmt.Errorf("copy link: link URI not available")
		}
		if uc.clipboard == nil {
			return fmt.Errorf("copy link: clipboard not available")
		}
		if err := uc.clipboard.WriteText(ctx, input.Context.LinkURI); err != nil {
			return fmt.Errorf("copy link: %w", err)
		}
		return nil
	case port.MenuActionCopyImage:
		image, err := uc.resolveImageData(ctx, input.Context.ImageURI)
		if err != nil {
			return fmt.Errorf("copy image: %w", err)
		}
		if uc.clipboard == nil {
			return fmt.Errorf("copy image: clipboard not available")
		}
		if err := uc.clipboard.WriteImage(ctx, image); err != nil {
			return fmt.Errorf("copy image: %w", err)
		}
		return nil
	case port.MenuActionSaveImage:
		image, err := uc.resolveImageData(ctx, input.Context.ImageURI)
		if err != nil {
			return fmt.Errorf("save image: %w", err)
		}
		if uc.saver == nil {
			return fmt.Errorf("save image: image saver not available")
		}
		if err := uc.saver.SaveResolvedImage(ctx, image); err != nil {
			return fmt.Errorf("save image: %w", err)
		}
		return nil
	default:
		// All other normalized actions are delegated to the engine/UI layer.
		if uc.delegator == nil {
			return fmt.Errorf("delegate action %s: menu action delegator not available", input.Action)
		}
		if err := uc.delegator.DelegateMenuAction(ctx, input.Action, input.Context); err != nil {
			return fmt.Errorf("delegate action %s: %w", input.Action, err)
		}
		return nil
	}
}

func (uc *ExecuteContextMenuActionUseCase) resolveImageData(ctx context.Context, uri string) (port.ImageData, error) {
	if uri == "" {
		return port.ImageData{}, fmt.Errorf("image URI not available")
	}
	if uc.resolver == nil {
		return port.ImageData{}, fmt.Errorf("image data resolver not available")
	}

	image, err := uc.resolver.ResolveImageData(ctx, uri)
	if err != nil {
		return port.ImageData{}, err
	}
	if len(image.Bytes) == 0 {
		return port.ImageData{}, fmt.Errorf("image data not available")
	}

	return image, nil
}
