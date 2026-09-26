package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/dto"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
)

// interactiveWebView composes generated mocks so a WebView also exposes the
// optional VimPageInteractor capability.
type interactiveWebView struct {
	*portmocks.MockWebView
	*portmocks.MockVimPageInteractor
}

func newInteractiveWebView(t *testing.T) (*interactiveWebView, *portmocks.MockVimPageInteractor) {
	t.Helper()
	interactor := portmocks.NewMockVimPageInteractor(t)
	return &interactiveWebView{MockWebView: portmocks.NewMockWebView(t), MockVimPageInteractor: interactor}, interactor
}

func TestVimNavigationUseCaseStartsPageInteractions(t *testing.T) {
	tests := []struct {
		action      string
		want        dto.VimPageInteractionRequest
		wantCapture bool
	}{
		{"hint-follow", dto.VimPageInteractionRequest{Kind: dto.VimPageHintFollow, HighlightColor: "#abc"}, true},
		{"hint-follow-new", dto.VimPageInteractionRequest{Kind: dto.VimPageHintFollowNew, HighlightColor: "#abc"}, true},
		{"hint-yank-url", dto.VimPageInteractionRequest{Kind: dto.VimPageHintYankURL, HighlightColor: "#abc"}, true},
		{"visual", dto.VimPageInteractionRequest{Kind: dto.VimPageVisual, HighlightColor: "#abc"}, true},
		{"yank-section", dto.VimPageInteractionRequest{
			Kind: dto.VimPageYankObject, Object: dto.VimPageTextObjectSection, HighlightColor: "#abc",
		}, false},
		{"yank-paragraph", dto.VimPageInteractionRequest{
			Kind: dto.VimPageYankObject, Object: dto.VimPageTextObjectParagraph, HighlightColor: "#abc",
		}, false},
		{"yank-code", dto.VimPageInteractionRequest{
			Kind: dto.VimPageYankObject, Object: dto.VimPageTextObjectCode, HighlightColor: "#abc",
		}, false},
		{"yank-table", dto.VimPageInteractionRequest{
			Kind: dto.VimPageYankObject, Object: dto.VimPageTextObjectTable, HighlightColor: "#abc",
		}, false},
		{"yank-list", dto.VimPageInteractionRequest{
			Kind: dto.VimPageYankObject, Object: dto.VimPageTextObjectList, HighlightColor: "#abc",
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			ctx := context.Background()
			wv, interactor := newInteractiveWebView(t)
			interactor.EXPECT().StartVimPageInteraction(ctx, tt.want).Return(nil).Once()

			outcome, err := NewVimNavigationUseCase().Execute(ctx, wv, tt.action, 1, "#abc")

			require.NoError(t, err)
			assert.Equal(t, tt.wantCapture, outcome.CapturePageKeys)
		})
	}
}

func TestVimNavigationUseCasePageInteractionErrorDoesNotCapture(t *testing.T) {
	ctx := context.Background()
	wv, interactor := newInteractiveWebView(t)
	boom := errors.New("boom")
	interactor.EXPECT().StartVimPageInteraction(ctx, dto.VimPageInteractionRequest{Kind: dto.VimPageVisual}).Return(boom).Once()

	outcome, err := NewVimNavigationUseCase().Execute(ctx, wv, "visual", 1, "")

	require.ErrorIs(t, err, boom)
	assert.False(t, outcome.CapturePageKeys)
}

func TestVimNavigationUseCasePageInteractionUnsupportedEngine(t *testing.T) {
	uc := NewVimNavigationUseCase()
	wv := portmocks.NewMockWebView(t)

	outcome, err := uc.Execute(context.Background(), wv, "hint-follow", 1, "")
	require.ErrorIs(t, err, errUnsupportedVimNavigationEngine)
	assert.False(t, outcome.CapturePageKeys)

	require.ErrorIs(t, uc.SendPageKey(context.Background(), wv, "a"), errUnsupportedVimNavigationEngine)
	require.NoError(t, uc.CancelPageInteraction(context.Background(), wv))
}

func TestVimNavigationUseCaseForwardsPageKeysAndCancel(t *testing.T) {
	ctx := context.Background()
	wv, interactor := newInteractiveWebView(t)
	interactor.EXPECT().SendVimPageKey(ctx, "J").Return(nil).Once()
	interactor.EXPECT().CancelVimPageInteraction(ctx).Return(nil).Once()
	uc := NewVimNavigationUseCase()

	require.NoError(t, uc.SendPageKey(ctx, wv, "J"))
	require.NoError(t, uc.CancelPageInteraction(ctx, wv))
}
