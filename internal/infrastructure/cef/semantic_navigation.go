package cef

import (
	"context"
	"fmt"
	"strconv"

	"github.com/bnema/dumber/internal/application/dto"
)

// NavigateSemantic performs the initial live Vim structural navigation path.
// It deliberately uses only stable HTML semantics and a static script: CEF's
// raw accessibility payload is not yet a documented application contract.
func (wv *WebView) NavigateSemantic(ctx context.Context, request dto.SemanticNavigationRequest) error {
	if wv == nil || wv.destroyed.Load() {
		return errDestroyed
	}
	if request.Target != dto.SemanticNavigationTargetHeading {
		return fmt.Errorf("unsupported semantic navigation target: %d", request.Target)
	}
	if request.Direction != dto.SemanticNavigationForward && request.Direction != dto.SemanticNavigationBackward {
		return fmt.Errorf("unsupported semantic navigation direction: %d", request.Direction)
	}

	count := request.Count
	if count < 1 {
		count = 1
	}
	wv.RunJavaScript(ctx, headingNavigationScript(int(request.Direction), count, request.HighlightColor))
	return nil
}

func headingNavigationScript(direction, count int, highlightColor string) string {
	if highlightColor == "" {
		highlightColor = "#fbbf24"
	}
	return `(() => {
  const stateKey = "__dumberVimHeadingTarget";
  const className = "dumber-vim-heading-target";
  const highlightColor = ` + strconv.Quote(highlightColor) + `;
  const headings = Array.from(document.querySelectorAll("h1,h2,h3,h4,h5,h6"))
    .filter((heading) => {
      if (heading.getClientRects().length === 0) return false;
      if (heading.closest("[hidden],[inert],[aria-hidden=\"true\"]")) return false;
      const style = window.getComputedStyle(heading);
      return style.visibility !== "hidden" && style.visibility !== "collapse";
    });
  if (headings.length === 0) return;

  const prior = window[stateKey];
  let index = headings.indexOf(prior);
  if (index < 0) {
    const firstAtOrAfterViewportTop = headings.findIndex(
      (heading) => heading.getBoundingClientRect().top >= 0,
    );
    if (` + strconv.Itoa(direction) + ` > 0) {
      index = firstAtOrAfterViewportTop - 1;
      if (index < -1) index = -1;
    } else {
      index = firstAtOrAfterViewportTop >= 0 ? firstAtOrAfterViewportTop : headings.length;
    }
  }

  const next = index + (` + strconv.Itoa(direction) + ` * ` + strconv.Itoa(count) + `);
  if (next < 0 || next >= headings.length) return;
  const target = headings[next];

  const previous = document.querySelector("." + className);
  if (previous && previous !== target) previous.classList.remove(className);
  let style = document.getElementById("dumber-vim-heading-target-style");
  if (!style) {
    // This visual cue is best-effort: strict page CSP can reject a DOM style
    // element, but target selection and scrolling must stay independent of it.
    style = document.createElement("style");
    style.id = "dumber-vim-heading-target-style";
    document.documentElement.appendChild(style);
  }
  // Refresh the outline on every navigation so an in-session accent change is
  // reflected even when the target style element already exists.
  style.textContent = "." + className +
    " { outline: 3px solid " + highlightColor +
    " !important; outline-offset: 5px !important; border-radius: 3px !important; }";
  window[stateKey] = target;
  target.classList.add(className);
  target.scrollIntoView({ block: "center", inline: "nearest", behavior: "smooth" });
})();`
}
