package cef

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
)

var _ port.PageInputFocuser = (*WebView)(nil)

// FocusFirstInput focuses the first visible, editable page input when one is
// available. The operation is intentionally fire-and-forget because the DOM
// is owned by the renderer and may change after the Vim command is handled.
func (wv *WebView) FocusFirstInput() {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	wv.RunJavaScript(context.Background(), focusFirstInputScript())
}

func focusFirstInputScript() string {
	return `(() => {
  const candidates = Array.from(document.querySelectorAll("input,textarea,[contenteditable]"));
  const hiddenInputTypes = new Set([
    "button", "checkbox", "color", "file", "hidden", "image", "radio", "range", "reset", "submit"
  ]);
  const isVisible = (element) => {
    if (!(element instanceof HTMLElement) || !element.isConnected) return false;
    if (element.closest("[hidden],[inert],[aria-hidden=\"true\"]")) return false;
    if (element.getClientRects().length === 0) return false;
    const style = window.getComputedStyle(element);
    return style.display !== "none" && style.visibility !== "hidden" && style.visibility !== "collapse";
  };
  const isEligible = (element) => {
    if (!isVisible(element)) return false;
    if (element instanceof HTMLInputElement) {
      return !element.disabled && !element.readOnly && !hiddenInputTypes.has(element.type.toLowerCase());
    }
    if (element instanceof HTMLTextAreaElement) {
      return !element.disabled && !element.readOnly;
    }
    return element.isContentEditable && element.getAttribute("contenteditable") !== "false";
  };
  const target = candidates.find(isEligible);
  if (target) target.focus();
})();`
}
