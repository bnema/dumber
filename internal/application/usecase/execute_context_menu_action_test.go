package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

type fakeClipboard struct {
	writeTextCalls  int
	writeImageCalls int
	text            string
	image           port.ImageData
}

func (f *fakeClipboard) WriteText(_ context.Context, text string) error {
	f.writeTextCalls++
	f.text = text
	return nil
}

func (f *fakeClipboard) WriteImage(_ context.Context, image port.ImageData) error {
	f.writeImageCalls++
	f.image = image
	return nil
}

func (*fakeClipboard) ReadText(context.Context) (string, error) { return "", nil }
func (*fakeClipboard) Clear(context.Context) error              { return nil }
func (*fakeClipboard) HasText(context.Context) (bool, error)    { return false, nil }

type fakeImageResolver struct {
	resolveCalls int
	uri          string
	image        port.ImageData
	err          error
}

func (f *fakeImageResolver) ResolveImageData(_ context.Context, uri string) (port.ImageData, error) {
	f.resolveCalls++
	f.uri = uri
	return f.image, f.err
}

type fakeResolvedImageSaver struct {
	saveCalls   int
	image       port.ImageData
	menuContext port.MenuContext
	err         error
}

func (f *fakeResolvedImageSaver) SaveResolvedImage(_ context.Context, image port.ImageData, menuContext port.MenuContext) error {
	f.saveCalls++
	f.image = image
	f.menuContext = menuContext
	return f.err
}

type fakeMenuActionDelegator struct {
	delegateCalls int
	action        port.MenuAction
	menuContext   port.MenuContext
	err           error
}

func (f *fakeMenuActionDelegator) DelegateMenuAction(_ context.Context, action port.MenuAction, menuContext port.MenuContext) error {
	f.delegateCalls++
	f.action = action
	f.menuContext = menuContext
	return f.err
}

func TestExecuteContextMenuActionUseCase_CopyImageFailsWithoutResolvedData(t *testing.T) {
	clipboard := &fakeClipboard{}
	resolver := &fakeImageResolver{}
	saver := &fakeResolvedImageSaver{}
	delegator := &fakeMenuActionDelegator{}
	uc := NewExecuteContextMenuActionUseCase(clipboard, resolver, saver, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action:  port.MenuActionCopyImage,
		Context: port.MenuContext{ImageURI: "https://example.com/image.png"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "copy image:")
	require.Contains(t, err.Error(), "image data not available")
	require.Zero(t, clipboard.writeImageCalls)
	require.Zero(t, saver.saveCalls)
	require.Zero(t, delegator.delegateCalls)
}

func TestExecuteContextMenuActionUseCase_SaveImageDelegatesResolvedImage(t *testing.T) {
	clipboard := &fakeClipboard{}
	resolver := &fakeImageResolver{image: port.ImageData{Bytes: []byte{1, 2, 3}, MimeType: "image/png"}}
	saver := &fakeResolvedImageSaver{}
	delegator := &fakeMenuActionDelegator{}
	uc := NewExecuteContextMenuActionUseCase(clipboard, resolver, saver, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action:  port.MenuActionSaveImage,
		Context: port.MenuContext{ImageURI: "https://example.com/image.png"},
	})

	require.NoError(t, err)
	require.Equal(t, 1, resolver.resolveCalls)
	require.Equal(t, "https://example.com/image.png", resolver.uri)
	require.Equal(t, 1, saver.saveCalls)
	require.Equal(t, port.ImageData{Bytes: []byte{1, 2, 3}, MimeType: "image/png"}, saver.image)
	require.Equal(t, port.MenuContext{ImageURI: "https://example.com/image.png"}, saver.menuContext)
	require.Zero(t, clipboard.writeTextCalls)
	require.Zero(t, clipboard.writeImageCalls)
	require.Zero(t, delegator.delegateCalls)
}

func TestExecuteContextMenuActionUseCase_DelegatesInspect(t *testing.T) {
	clipboard := &fakeClipboard{}
	resolver := &fakeImageResolver{}
	saver := &fakeResolvedImageSaver{}
	delegator := &fakeMenuActionDelegator{}
	uc := NewExecuteContextMenuActionUseCase(clipboard, resolver, saver, delegator)

	menuContext := port.MenuContext{
		PageURI: "https://example.com",
		X:       17,
		Y:       42,
	}
	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action:  port.MenuActionInspectElement,
		Context: menuContext,
	})

	require.NoError(t, err)
	require.Equal(t, 1, delegator.delegateCalls)
	require.Equal(t, port.MenuActionInspectElement, delegator.action)
	require.Equal(t, menuContext, delegator.menuContext)
	require.Zero(t, clipboard.writeTextCalls)
	require.Zero(t, clipboard.writeImageCalls)
	require.Zero(t, resolver.resolveCalls)
	require.Zero(t, saver.saveCalls)
}

func TestExecuteContextMenuActionUseCase_WrapsDelegateErrors(t *testing.T) {
	clipboard := &fakeClipboard{}
	resolver := &fakeImageResolver{}
	saver := &fakeResolvedImageSaver{}
	delegator := &fakeMenuActionDelegator{err: errors.New("boom")}
	uc := NewExecuteContextMenuActionUseCase(clipboard, resolver, saver, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action:  port.MenuActionCopySelection,
		Context: port.MenuContext{HasSelection: true},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "delegate action copy_selection:")
	require.Contains(t, err.Error(), "boom")
}

func TestExecuteContextMenuActionUseCase_CopyLinkWritesText(t *testing.T) {
	clipboard := &fakeClipboard{}
	resolver := &fakeImageResolver{}
	saver := &fakeResolvedImageSaver{}
	delegator := &fakeMenuActionDelegator{}
	uc := NewExecuteContextMenuActionUseCase(clipboard, resolver, saver, delegator)

	err := uc.Execute(context.Background(), ExecuteContextMenuActionInput{
		Action:  port.MenuActionCopyLink,
		Context: port.MenuContext{LinkURI: "https://example.com/link"},
	})

	require.NoError(t, err)
	require.Equal(t, 1, clipboard.writeTextCalls)
	require.Equal(t, "https://example.com/link", clipboard.text)
	require.Zero(t, clipboard.writeImageCalls)
	require.Zero(t, resolver.resolveCalls)
	require.Zero(t, saver.saveCalls)
	require.Zero(t, delegator.delegateCalls)
}
