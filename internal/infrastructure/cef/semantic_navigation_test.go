package cef

import (
	"context"
	"testing"

	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/dto"
)

func TestWebViewNavigateSemantic_HeadingExecutesStrictNavigationScript(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	browser.EXPECT().GetMainFrame().Return(frame).Once()
	frame.EXPECT().ExecuteJavaScript(mock.MatchedBy(mockStringContaining(t,
		"document.querySelectorAll(\"h1,h2,h3,h4,h5,h6\")",
		"heading.closest(\"[hidden],[inert],[aria-hidden=\\\"true\\\"]\")",
		"window.getComputedStyle(heading)",
		"firstAtOrAfterViewportTop",
		"if (next < 0 || next >= headings.length) return;",
		"previous && previous !== target",
		"scrollIntoView",
		"window[stateKey] = target",
		"const highlightColor = \"#22c55e\"",
		"let style = document.getElementById(\"dumber-vim-heading-target-style\")",
		"style.textContent = \".\" + className",
		"1 * 3",
	)), "", int32(0)).Once()

	wv := &WebView{browser: browser}
	err := wv.NavigateSemantic(context.Background(), dto.SemanticNavigationRequest{
		Target:         dto.SemanticNavigationTargetHeading,
		Direction:      dto.SemanticNavigationForward,
		Count:          3,
		HighlightColor: "#22c55e",
	})
	require.NoError(t, err)
}

func TestWebViewNavigateSemantic_NormalizesCountAndRejectsUnsupportedRequests(t *testing.T) {
	forward := headingNavigationScript(1, 1, "")
	backward := headingNavigationScript(-1, 1, "#4ade80")
	assert.Contains(t, forward, "index = firstAtOrAfterViewportTop - 1")
	assert.Contains(t, backward, "index = firstAtOrAfterViewportTop >= 0 ? firstAtOrAfterViewportTop : headings.length")
	assert.Contains(t, backward, "-1 * 1")
	assert.NotContains(t, backward, "window.innerHeight")
	assert.Contains(t, forward, "const highlightColor = \"#fbbf24\"")
	assert.Contains(t, backward, "const highlightColor = \"#4ade80\"")

	wv := &WebView{}
	err := wv.NavigateSemantic(context.Background(), dto.SemanticNavigationRequest{
		Target:    dto.SemanticNavigationTargetHeading,
		Direction: dto.SemanticNavigationForward,
		Count:     0,
	})
	require.NoError(t, err)

	err = wv.NavigateSemantic(context.Background(), dto.SemanticNavigationRequest{
		Target:    dto.SemanticNavigationTargetHeading + 1,
		Direction: dto.SemanticNavigationForward,
	})
	require.Error(t, err)

	err = wv.NavigateSemantic(context.Background(), dto.SemanticNavigationRequest{
		Target:    dto.SemanticNavigationTargetHeading,
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
