package usecase

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
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

var _ port.ContextMenuActionExecutor = (*ExecuteContextMenuActionUseCase)(nil)

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

// ContextMenuActionExecutorFactory creates ExecuteContextMenuActionUseCases.
type ContextMenuActionExecutorFactory struct{}

var _ port.ContextMenuActionExecutorFactory = (*ContextMenuActionExecutorFactory)(nil)

// NewContextMenuActionExecutor creates a new shared context menu action executor.
func (*ContextMenuActionExecutorFactory) NewContextMenuActionExecutor(
	clipboard port.Clipboard,
	resolver port.ImageDataResolver,
	saver port.ResolvedImageSaver,
	delegator port.MenuActionDelegator,
) port.ContextMenuActionExecutor {
	return NewExecuteContextMenuActionUseCase(clipboard, resolver, saver, delegator)
}

// Execute handles the requested action.
func (uc *ExecuteContextMenuActionUseCase) Execute(ctx context.Context, input ExecuteContextMenuActionInput) error {
	return uc.ExecuteMenuAction(ctx, input.Action, input.Context)
}

// ExecuteMenuAction handles the requested action.
func (uc *ExecuteContextMenuActionUseCase) ExecuteMenuAction(
	ctx context.Context, action port.MenuAction, menuContext port.MenuContext,
) error {
	return uc.executeMenuAction(ctx, action, menuContext)
}

func (uc *ExecuteContextMenuActionUseCase) executeMenuAction(
	ctx context.Context, action port.MenuAction, menuContext port.MenuContext,
) error {
	switch action {
	case port.MenuActionCopyLink:
		if menuContext.LinkURI == "" {
			return fmt.Errorf("copy link: link URI not available")
		}
		if uc.clipboard == nil {
			return fmt.Errorf("copy link: clipboard not available")
		}
		if err := uc.clipboard.WriteText(ctx, menuContext.LinkURI); err != nil {
			return fmt.Errorf("copy link: %w", err)
		}
		return nil
	case port.MenuActionCopyImage:
		if uc.clipboard == nil {
			return fmt.Errorf("copy image: clipboard not available")
		}
		image, err := uc.resolveImageData(ctx, menuContext.ImageURI)
		if err != nil {
			return fmt.Errorf("copy image: %w", err)
		}
		if err := uc.clipboard.WriteImage(ctx, image); err != nil {
			return fmt.Errorf("copy image: %w", err)
		}
		return nil
	case port.MenuActionSaveImage:
		if uc.saver == nil {
			return fmt.Errorf("save image: image saver not available")
		}
		image, err := uc.resolveImageData(ctx, menuContext.ImageURI)
		if err != nil {
			return fmt.Errorf("save image: %w", err)
		}
		if err := uc.saver.SaveResolvedImage(ctx, image, menuContext); err != nil {
			return fmt.Errorf("save image: %w", err)
		}
		return nil
	case port.MenuActionCopySelection:
		if menuContext.SelectionText != "" {
			if uc.clipboard == nil {
				return fmt.Errorf("copy selection: clipboard not available")
			}
			if err := uc.clipboard.WriteText(ctx, menuContext.SelectionText); err != nil {
				return fmt.Errorf("copy selection: %w", err)
			}
			return nil
		}
	}

	// All other normalized actions are delegated to the engine/UI layer.
	if uc.delegator == nil {
		return fmt.Errorf("delegate action %s: menu action delegator not available", action)
	}
	if err := uc.delegator.DelegateMenuAction(ctx, action, menuContext); err != nil {
		return fmt.Errorf("delegate action %s: %w", action, err)
	}
	return nil
}

func (uc *ExecuteContextMenuActionUseCase) resolveImageData(ctx context.Context, uri string) (entity.ImageData, error) {
	if uri == "" {
		return entity.ImageData{}, fmt.Errorf("image URI not available")
	}
	if uc.resolver == nil {
		return entity.ImageData{}, fmt.Errorf("image data resolver not available")
	}

	image, err := uc.resolver.ResolveImageData(ctx, uri)
	if err != nil {
		return entity.ImageData{}, err
	}
	if len(image.Bytes) == 0 {
		return entity.ImageData{}, fmt.Errorf("image data not available")
	}

	return image, nil
}
