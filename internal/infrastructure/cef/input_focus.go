package cef

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
)

var (
	_ port.PageInputFocuser   = (*WebView)(nil)
	_ port.PageFocusNavigator = (*WebView)(nil)
)

// FocusNextInput moves focus to the next visible, editable page input. When
// no eligible input is active it focuses the first one; movement wraps at the
// end so repeated gi commands continue to cycle through the page inputs.
func (wv *WebView) FocusNextInput() {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	wv.RunJavaScript(context.Background(), focusNextInputScript())
}

// NavigatePageFocus keeps reverse and forward focus traversal inside the live
// page when GTK would otherwise move focus out of the WebView.
func (wv *WebView) NavigatePageFocus(backward bool) {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	wv.RunJavaScript(context.Background(), pageFocusNavigationScript(backward))
}

func focusNextInputScript() string {
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
      return !element.matches(":disabled") && !element.readOnly && !hiddenInputTypes.has(element.type.toLowerCase());
    }
    if (element instanceof HTMLTextAreaElement) {
      return !element.matches(":disabled") && !element.readOnly;
    }
    return element.isContentEditable && element.getAttribute("contenteditable") !== "false";
  };
  const eligible = candidates.filter(isEligible);
  if (eligible.length === 0) return;
  const active = document.activeElement;
  const activeIndex = eligible.findIndex((element) => element === active || element.contains(active));
  const target = eligible[(activeIndex + 1 + eligible.length) % eligible.length];
  target.focus();
})();`
}

func pageFocusNavigationScript(backward bool) string {
	direction := "1"
	if backward {
		direction = "-1"
	}
	return `(() => {
  const direction = ` + direction + `;
  const selector = [
    "a[href]", "area[href]", "button:not(:disabled)",
    "input:not(:disabled):not([type=\"hidden\"])", "select:not(:disabled)",
    "textarea:not(:disabled)", "[contenteditable]:not([contenteditable=\"false\"])",
    "[tabindex]:not([tabindex=\"-1\"])"
  ].join(",");
  const isVisible = (element) => {
    if (!(element instanceof HTMLElement) || !element.isConnected) return false;
    if (element.closest("[hidden],[inert],[aria-hidden=\"true\"]")) return false;
    if (element.getClientRects().length === 0) return false;
    const style = window.getComputedStyle(element);
    return style.display !== "none" && style.visibility !== "hidden" && style.visibility !== "collapse";
  };
  const candidates = Array.from(document.querySelectorAll(selector)).filter((element) => {
    return isVisible(element) && !element.matches(":disabled") && element.tabIndex >= 0;
  });
  if (candidates.length === 0) return;
  const active = document.activeElement;
  const activeIndex = candidates.findIndex((element) => element === active || element.contains(active));
  const targetIndex = activeIndex < 0
    ? (direction < 0 ? candidates.length - 1 : 0)
    : activeIndex + direction;
  if (targetIndex < 0 || targetIndex >= candidates.length) return;
  candidates[targetIndex].focus();
})();`
}
