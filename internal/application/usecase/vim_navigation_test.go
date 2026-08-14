package usecase

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type vimNavigationWebView struct {
	*portmocks.MockWebView
	focusNextInputCalls int
	pageFocusBackward   []bool
	semanticRequests    []port.SemanticNavigationRequest
}

func (wv *vimNavigationWebView) FocusNextInput() {
	wv.focusNextInputCalls++
}

func (wv *vimNavigationWebView) NavigatePageFocus(backward bool) {
	wv.pageFocusBackward = append(wv.pageFocusBackward, backward)
}

func (wv *vimNavigationWebView) NavigateSemantic(_ context.Context, request port.SemanticNavigationRequest) error {
	wv.semanticRequests = append(wv.semanticRequests, request)
	return nil
}

func TestVimNavigationUseCaseExecuteDispatchesConfiguredActions(t *testing.T) {
	wv := &vimNavigationWebView{MockWebView: portmocks.NewMockWebView(t)}
	uc := NewVimNavigationUseCase()

	require.NoError(t, uc.Execute(context.Background(), wv, "focus-input", 1, ""))
	require.NoError(t, uc.Execute(context.Background(), wv, "heading-prev", 3, "#aabbcc"))

	assert.Equal(t, 1, wv.focusNextInputCalls)
	require.Len(t, wv.semanticRequests, 1)
	assert.Equal(t, port.SemanticNavigationRequest{
		Target:         port.SemanticNavigationTargetHeading,
		Direction:      port.SemanticNavigationBackward,
		Count:          3,
		HighlightColor: "#aabbcc",
	}, wv.semanticRequests[0])
}

func TestVimNavigationUseCaseNavigatePageFocus(t *testing.T) {
	wv := &vimNavigationWebView{MockWebView: portmocks.NewMockWebView(t)}

	handled := NewVimNavigationUseCase().NavigatePageFocus(wv, true)

	assert.True(t, handled)
	assert.Equal(t, []bool{true}, wv.pageFocusBackward)
}

func TestVimNavigationUseCaseIgnoresUnsupportedCapability(t *testing.T) {
	wv := portmocks.NewMockWebView(t)

	require.NoError(t, NewVimNavigationUseCase().Execute(context.Background(), wv, "focus-input", 1, ""))
	assert.False(t, NewVimNavigationUseCase().NavigatePageFocus(wv, true))
}
