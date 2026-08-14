package cef

import (
	"context"
	"testing"

	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
)

func TestWebViewNavigateSemantic_HeadingExecutesStrictNavigationScript(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	browser.EXPECT().GetMainFrame().Return(frame).Once()
	frame.EXPECT().ExecuteJavaScript(mock.MatchedBy(mockStringContaining(t,
		"document.querySelectorAll(\"h1,h2,h3,h4,h5,h6\")",
		"firstAtOrAfterViewportTop",
		"if (next < 0 || next >= headings.length) return;",
		"previous && previous !== target",
		"scrollIntoView",
		"window[stateKey] = target",
		"1 * 3",
	)), "", int32(0)).Once()

	wv := &WebView{browser: browser}
	err := wv.NavigateSemantic(context.Background(), port.SemanticNavigationRequest{
		Target:    port.SemanticNavigationTargetHeading,
		Direction: port.SemanticNavigationForward,
		Count:     3,
	})
	require.NoError(t, err)
}

func TestWebViewNavigateSemantic_NormalizesCountAndRejectsUnsupportedRequests(t *testing.T) {
	forward := headingNavigationScript(1, 1)
	backward := headingNavigationScript(-1, 1)
	assert.Contains(t, forward, "index = firstAtOrAfterViewportTop - 1")
	assert.Contains(t, backward, "index = firstAtOrAfterViewportTop >= 0 ? firstAtOrAfterViewportTop : headings.length")
	assert.Contains(t, backward, "-1 * 1")
	assert.NotContains(t, backward, "window.innerHeight")

	wv := &WebView{}
	err := wv.NavigateSemantic(context.Background(), port.SemanticNavigationRequest{
		Target:    port.SemanticNavigationTargetHeading,
		Direction: port.SemanticNavigationForward,
		Count:     0,
	})
	require.NoError(t, err)

	err = wv.NavigateSemantic(context.Background(), port.SemanticNavigationRequest{
		Target:    port.SemanticNavigationTargetHeading + 1,
		Direction: port.SemanticNavigationForward,
	})
	require.Error(t, err)

	err = wv.NavigateSemantic(context.Background(), port.SemanticNavigationRequest{
		Target:    port.SemanticNavigationTargetHeading,
		Direction: 0,
	})
	require.Error(t, err)
}

func mockStringContaining(t *testing.T, fragments ...string) func(string) bool {
	t.Helper()
	return func(script string) bool {
		for _, fragment := range fragments {
			if !assert.Contains(t, script, fragment) {
				return false
			}
		}
		return true
	}
}
