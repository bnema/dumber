package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestExecuteContextMenuActionUseCase_CopyImageFailsWithoutResolvedData(t *testing.T) {
	clipboard := portmocks.NewMockClipboard(t)
	resolver := portmocks.NewMockImageDataResolver(t)
	saver := portmocks.NewMockResolvedImageSaver(t)
	delegator := portmocks.NewMockMenuActionDelegator(t)
	resolver.EXPECT().
		ResolveImageData(mock.Anything, "https://example.com/image.png").
		Return(entity.ImageData{}, nil).
		Once()
	uc := NewExecuteContextMenuActionUseCase(clipboard, resolver, saver, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action:  port.MenuActionCopyImage,
		Context: port.MenuContext{ImageURI: "https://example.com/image.png"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "copy image:")
	require.Contains(t, err.Error(), "image data not available")
}

func TestExecuteContextMenuActionUseCase_CopyImageWritesResolvedImage(t *testing.T) {
	clipboard := portmocks.NewMockClipboard(t)
	resolver := portmocks.NewMockImageDataResolver(t)
	saver := portmocks.NewMockResolvedImageSaver(t)
	delegator := portmocks.NewMockMenuActionDelegator(t)
	image := entity.ImageData{Bytes: []byte{1, 2, 3}, MimeType: "image/png"}
	resolver.EXPECT().
		ResolveImageData(mock.Anything, "https://example.com/image.png").
		Return(image, nil).
		Once()
	clipboard.EXPECT().
		WriteImage(mock.Anything, image).
		Return(nil).
		Once()
	uc := NewExecuteContextMenuActionUseCase(clipboard, resolver, saver, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action:  port.MenuActionCopyImage,
		Context: port.MenuContext{ImageURI: "https://example.com/image.png"},
	})

	require.NoError(t, err)
}

func TestExecuteContextMenuActionUseCase_CopyImageFailsFastWithoutClipboard(t *testing.T) {
	resolver := portmocks.NewMockImageDataResolver(t)
	saver := portmocks.NewMockResolvedImageSaver(t)
	delegator := portmocks.NewMockMenuActionDelegator(t)
	uc := NewExecuteContextMenuActionUseCase(nil, resolver, saver, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action:  port.MenuActionCopyImage,
		Context: port.MenuContext{ImageURI: "https://example.com/image.png"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "clipboard not available")
}

func TestExecuteContextMenuActionUseCase_SaveImageDelegatesResolvedImage(t *testing.T) {
	clipboard := portmocks.NewMockClipboard(t)
	resolver := portmocks.NewMockImageDataResolver(t)
	saver := portmocks.NewMockResolvedImageSaver(t)
	delegator := portmocks.NewMockMenuActionDelegator(t)
	image := entity.ImageData{Bytes: []byte{1, 2, 3}, MimeType: "image/png"}
	menuContext := port.MenuContext{ImageURI: "https://example.com/image.png"}
	resolver.EXPECT().
		ResolveImageData(mock.Anything, menuContext.ImageURI).
		Return(image, nil).
		Once()
	saver.EXPECT().
		SaveResolvedImage(mock.Anything, image, menuContext).
		Return(nil).
		Once()
	uc := NewExecuteContextMenuActionUseCase(clipboard, resolver, saver, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action:  port.MenuActionSaveImage,
		Context: menuContext,
	})

	require.NoError(t, err)
}

func TestExecuteContextMenuActionUseCase_SaveImageFailsFastWithoutSaver(t *testing.T) {
	clipboard := portmocks.NewMockClipboard(t)
	resolver := portmocks.NewMockImageDataResolver(t)
	delegator := portmocks.NewMockMenuActionDelegator(t)
	uc := NewExecuteContextMenuActionUseCase(clipboard, resolver, nil, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action:  port.MenuActionSaveImage,
		Context: port.MenuContext{ImageURI: "https://example.com/image.png"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "image saver not available")
}

func TestExecuteContextMenuActionUseCase_DelegatesInspect(t *testing.T) {
	menuContext := port.MenuContext{
		PageURI: "https://example.com",
		X:       17,
		Y:       42,
	}
	clipboard := portmocks.NewMockClipboard(t)
	resolver := portmocks.NewMockImageDataResolver(t)
	saver := portmocks.NewMockResolvedImageSaver(t)
	delegator := portmocks.NewMockMenuActionDelegator(t)
	delegator.EXPECT().
		DelegateMenuAction(mock.Anything, port.MenuActionInspectElement, menuContext).
		Return(nil).
		Once()
	uc := NewExecuteContextMenuActionUseCase(clipboard, resolver, saver, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action:  port.MenuActionInspectElement,
		Context: menuContext,
	})

	require.NoError(t, err)
}

func TestExecuteContextMenuActionUseCase_WrapsDelegateErrors(t *testing.T) {
	clipboard := portmocks.NewMockClipboard(t)
	resolver := portmocks.NewMockImageDataResolver(t)
	saver := portmocks.NewMockResolvedImageSaver(t)
	delegator := portmocks.NewMockMenuActionDelegator(t)
	delegator.EXPECT().
		DelegateMenuAction(mock.Anything, port.MenuActionCopySelection, port.MenuContext{HasSelection: true}).
		Return(errors.New("boom")).
		Once()
	uc := NewExecuteContextMenuActionUseCase(clipboard, resolver, saver, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action:  port.MenuActionCopySelection,
		Context: port.MenuContext{HasSelection: true},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "delegate action copy_selection:")
	require.Contains(t, err.Error(), "boom")
}

func TestExecuteContextMenuActionUseCase_CopySelectionWritesSelectedText(t *testing.T) {
	clipboard := portmocks.NewMockClipboard(t)
	resolver := portmocks.NewMockImageDataResolver(t)
	saver := portmocks.NewMockResolvedImageSaver(t)
	delegator := portmocks.NewMockMenuActionDelegator(t)
	clipboard.EXPECT().
		WriteText(mock.Anything, "selected text").
		Return(nil).
		Once()
	uc := NewExecuteContextMenuActionUseCase(clipboard, resolver, saver, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action: port.MenuActionCopySelection,
		Context: port.MenuContext{
			HasSelection:  true,
			SelectionText: "selected text",
		},
	})

	require.NoError(t, err)
}

func TestExecuteContextMenuActionUseCase_CopySelectionFallsBackToDelegatorWhenClipboardMissing(t *testing.T) {
	resolver := portmocks.NewMockImageDataResolver(t)
	saver := portmocks.NewMockResolvedImageSaver(t)
	delegator := portmocks.NewMockMenuActionDelegator(t)
	delegator.EXPECT().
		DelegateMenuAction(mock.Anything, port.MenuActionCopySelection, port.MenuContext{
			HasSelection:  true,
			SelectionText: "selected text",
		}).
		Return(nil).
		Once()
	uc := NewExecuteContextMenuActionUseCase(nil, resolver, saver, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action: port.MenuActionCopySelection,
		Context: port.MenuContext{
			HasSelection:  true,
			SelectionText: "selected text",
		},
	})

	require.NoError(t, err)
}

func TestExecuteContextMenuActionUseCase_CopyLinkWritesText(t *testing.T) {
	clipboard := portmocks.NewMockClipboard(t)
	resolver := portmocks.NewMockImageDataResolver(t)
	saver := portmocks.NewMockResolvedImageSaver(t)
	delegator := portmocks.NewMockMenuActionDelegator(t)
	clipboard.EXPECT().
		WriteText(mock.Anything, "https://example.com/link").
		Return(nil).
		Once()
	uc := NewExecuteContextMenuActionUseCase(clipboard, resolver, saver, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action:  port.MenuActionCopyLink,
		Context: port.MenuContext{LinkURI: "https://example.com/link"},
	})

	require.NoError(t, err)
}
