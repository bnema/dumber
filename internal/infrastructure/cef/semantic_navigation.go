package cef

import (
	"context"
	"fmt"
	"strconv"

	"github.com/bnema/dumber/internal/application/port"
)

// NavigateSemantic performs the initial live Vim structural navigation path.
// It deliberately uses only stable HTML semantics and a static script: CEF's
// raw accessibility payload is not yet a documented application contract.
func (wv *WebView) NavigateSemantic(ctx context.Context, request port.SemanticNavigationRequest) error {
	if wv == nil || wv.destroyed.Load() {
		return errDestroyed
	}
	if request.Target != port.SemanticNavigationTargetHeading {
		return fmt.Errorf("unsupported semantic navigation target: %d", request.Target)
	}
	if request.Direction != port.SemanticNavigationForward && request.Direction != port.SemanticNavigationBackward {
		return fmt.Errorf("unsupported semantic navigation direction: %d", request.Direction)
	}

	count := request.Count
	if count < 1 {
		count = 1
	}
	wv.RunJavaScript(ctx, headingNavigationScript(int(request.Direction), count))
	return nil
}

func headingNavigationScript(direction, count int) string {
	return `(() => {
  const stateKey = "__dumberVimHeadingTarget";
  const className = "dumber-vim-heading-target";
  const headings = Array.from(document.querySelectorAll("h1,h2,h3,h4,h5,h6"))
    .filter((heading) => heading.getClientRects().length > 0);
  if (headings.length === 0) return;

  const prior = window[stateKey];
  let index = headings.indexOf(prior);
  if (index < 0) {
    const firstAtOrAfterViewportTop = headings.findIndex(
      (heading) => heading.getBoundingClientRect().top >= 0,
    );
    if (` + strconv.Itoa(direction) + ` > 0) {
      index = firstAtOrAfterViewportTop - 1;
    } else {
      index = firstAtOrAfterViewportTop >= 0 ? firstAtOrAfterViewportTop : headings.length;
    }
  }

  const next = index + (` + strconv.Itoa(direction) + ` * ` + strconv.Itoa(count) + `);
  if (next < 0 || next >= headings.length) return;
  const target = headings[next];

  const previous = document.querySelector("." + className);
  if (previous && previous !== target) previous.classList.remove(className);
  if (!document.getElementById("dumber-vim-heading-target-style")) {
    // This visual cue is best-effort: strict page CSP can reject a DOM style
    // element, but target selection and scrolling must stay independent of it.
    const style = document.createElement("style");
    style.id = "dumber-vim-heading-target-style";
    style.textContent = "." + className + " { outline: 3px solid #fbbf24 !important; outline-offset: 5px !important; border-radius: 3px !important; }";
    document.documentElement.appendChild(style);
  }
  window[stateKey] = target;
  target.classList.add(className);
  target.scrollIntoView({ block: "center", inline: "nearest", behavior: "smooth" });
})();`
}
