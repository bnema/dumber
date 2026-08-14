package cef

import (
	"testing"

	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestWebViewFocusFirstInputExecutesSafeFocusScript(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	browser.EXPECT().GetMainFrame().Return(frame).Once()
	frame.EXPECT().ExecuteJavaScript(mock.MatchedBy(mockStringContaining(t,
		`document.querySelectorAll("input,textarea,[contenteditable]")`,
		`element.closest("[hidden],[inert],[aria-hidden=\"true\"]")`,
		"getClientRects",
		"HTMLInputElement",
		"HTMLTextAreaElement",
		"target.focus()",
	)), "", int32(0)).Once()

	(&WebView{browser: browser}).FocusFirstInput()
}

func TestFocusFirstInputScriptNoOpsWithoutEligibleTarget(t *testing.T) {
	script := focusFirstInputScript()

	require.Contains(t, script, "const target = candidates.find(isEligible);")
	require.Contains(t, script, "if (target) target.focus();")
	require.Contains(t, script, "!element.disabled && !element.readOnly")
	require.Contains(t, script, `hiddenInputTypes.has(element.type.toLowerCase())`)
}
