package cef

import (
	"context"
	"fmt"
	"strconv"

	purecef "github.com/bnema/purego-cef/cef"

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
	wv.RunJavaScript(ctx, headingNavigationScript(int(request.Direction), count, request.HighlightColor, wv.vimHeadingHighlightNamespace()))
	return nil
}

// ClearSemanticNavigationHighlight removes the visual target left by the most
// recent Vim semantic navigation in this WebView.
func (wv *WebView) ClearSemanticNavigationHighlight(_ context.Context) error {
	if wv == nil || wv.destroyed.Load() {
		return errDestroyed
	}
	return wv.scheduleJavaScript(clearHeadingNavigationHighlightScript(wv.vimHeadingHighlightNamespace()))
}

func (wv *WebView) vimHeadingHighlightNamespace() string {
	return fmt.Sprintf("dumber-vim-heading-%p", wv)
}

// scheduleJavaScript makes CEF UI-thread scheduling failures observable to
// callers that need to guarantee a transient UI state is cleared.
func (wv *WebView) scheduleJavaScript(script string) error {
	if wv == nil || wv.destroyed.Load() {
		return errDestroyed
	}
	wv.mu.RLock()
	browser := wv.browser
	wv.mu.RUnlock()
	if browser == nil {
		return fmt.Errorf("schedule JavaScript: browser unavailable")
	}
	if wv.engine == nil {
		frame := browser.GetMainFrame()
		if frame == nil {
			return fmt.Errorf("schedule JavaScript: main frame unavailable")
		}
		frame.ExecuteJavaScript(script, "", 0)
		return nil
	}

	task := cefNewTask(cefTaskFunc(func() {
		wv.executeJavaScriptNow(script)
	}))
	if task == nil {
		return fmt.Errorf("schedule JavaScript: create CEF task")
	}
	if result := cefPostTask(purecef.ThreadIDTidUi, task); result != 1 {
		return fmt.Errorf("schedule JavaScript: post CEF task: result %d", result)
	}
	return nil
}

func headingNavigationScript(direction, count int, highlightColor, namespace string) string {
	if highlightColor == "" {
		highlightColor = "#fbbf24"
	}
	return `(() => {
  const stateKey = ` + strconv.Quote("__"+namespace+"Target") + `;
  const targetAttribute = ` + strconv.Quote("data-"+namespace+"-target") + `;
  const styleAttribute = ` + strconv.Quote("data-"+namespace+"-style") + `;
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

  const previous = document.querySelector("[" + targetAttribute + "]");
  if (previous && previous !== target) previous.removeAttribute(targetAttribute);
  let style = document.querySelector("style[" + styleAttribute + "]");
  if (!style) {
    // This visual cue is best-effort: strict page CSP can reject a DOM style
    // element, but target selection and scrolling must stay independent of it.
    style = document.createElement("style");
    style.setAttribute(styleAttribute, "");
    document.documentElement.appendChild(style);
  }
  // Refresh the outline on every navigation so an in-session accent change is
  // reflected even when the target style element already exists.
  style.textContent = "[" + targetAttribute + "]" +
    " { outline: 3px solid " + highlightColor +
    " !important; outline-offset: 5px !important; border-radius: 3px !important; }";
  window[stateKey] = target;
  target.setAttribute(targetAttribute, "");
  target.scrollIntoView({ block: "center", inline: "nearest", behavior: "smooth" });
})();`
}

func clearHeadingNavigationHighlightScript(namespace string) string {
	return `(() => {
  const stateKey = ` + strconv.Quote("__"+namespace+"Target") + `;
  const targetAttribute = ` + strconv.Quote("data-"+namespace+"-target") + `;
  const styleAttribute = ` + strconv.Quote("data-"+namespace+"-style") + `;
  document.querySelectorAll("[" + targetAttribute + "]").forEach((target) => {
    target.removeAttribute(targetAttribute);
  });
  document.querySelectorAll("style[" + styleAttribute + "]").forEach((style) => {
    style.remove();
  });
  delete window[stateKey];
})();`
}
