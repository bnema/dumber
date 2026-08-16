package cef

import (
	"testing"

	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestWebViewFocusNextInputExecutesSafeFocusScript(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	browser.EXPECT().GetMainFrame().Return(frame).Once()
	frame.EXPECT().ExecuteJavaScript(mock.MatchedBy(mockStringContaining(t,
		`document.querySelectorAll("input,textarea,[contenteditable]")`,
		`element.closest("[hidden],[inert],[aria-hidden=\"true\"]")`,
		"getClientRects",
		"HTMLInputElement",
		"HTMLTextAreaElement",
		"activeIndex",
		"target.focus()",
	)), "", int32(0)).Once()

	(&WebView{browser: browser}).FocusNextInput()
}

func TestWebViewNavigatePageFocusExecutesDirectionalScript(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	browser.EXPECT().GetMainFrame().Return(frame).Once()
	frame.EXPECT().ExecuteJavaScript(mock.MatchedBy(mockStringContaining(t,
		"const direction = -1;",
		"a[href]",
		"textarea:not(:disabled)",
		"document.activeElement",
		"targetIndex",
		"candidates[targetIndex].focus()",
	)), "", int32(0)).Once()

	(&WebView{browser: browser}).NavigatePageFocus(true)
}

func TestFocusNextInputScriptWrapsAndNoOpsWithoutEligibleTarget(t *testing.T) {
	script := focusNextInputScript()

	require.Contains(t, script, "const eligible = candidates.filter(isEligible);")
	require.Contains(t, script, "if (eligible.length === 0) return;")
	require.Contains(t, script, "const target = eligible[(activeIndex + 1 + eligible.length) % eligible.length];")
	require.Contains(t, script, "!element.matches(\":disabled\") && !element.readOnly")
	require.Contains(t, script, `hiddenInputTypes.has(element.type.toLowerCase())`)
}

func TestPageFocusNavigationScriptStopsAtPageBoundaries(t *testing.T) {
	script := pageFocusNavigationScript(true)

	require.Contains(t, script, "const direction = -1;")
	require.Contains(t, script, "if (targetIndex < 0 || targetIndex >= candidates.length) return;")
	require.Contains(t, script, "candidates[targetIndex].focus();")
}
