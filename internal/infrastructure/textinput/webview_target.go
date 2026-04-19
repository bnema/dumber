package textinput

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

// deleteBeforeCursorJS is the JavaScript template to delete n characters before cursor.
// It handles both input/textarea elements and contenteditable elements.
const deleteBeforeCursorJS = `
(function() {
	const n = %d;
	const el = window.__dumber_lastEditableEl || document.activeElement;
	if (!el) return;
	
	// Handle input and textarea elements
	if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') {
		const start = el.selectionStart;
		const end = el.selectionEnd;
		if (start === end && start >= n) {
			el.value = el.value.slice(0, start - n) + el.value.slice(end);
			el.selectionStart = el.selectionEnd = start - n;
			// Trigger input event for frameworks like React
			el.dispatchEvent(new Event('input', { bubbles: true }));
		}
		return;
	}
	
	// Handle contenteditable elements
	if (el.isContentEditable) {
		const sel = window.getSelection();
		if (!sel.rangeCount) return;
		for (let i = 0; i < n; i++) {
			sel.modify('extend', 'backward', 'character');
		}
		sel.deleteFromDocument();
	}
})();
`

// insertTextJS inserts text at the cursor position of the last focused editable element.
// Uses the tracked element from the accent detection script so insertion still works
// after GTK focus moves to the accent picker.
const insertTextJS = `
(function() {
	const text = %q;
	const el = window.__dumber_lastEditableEl || document.activeElement;
	if (!el) return;

	el.focus();

	if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') {
		const start = el.selectionStart;
		const end = el.selectionEnd;
		el.value = el.value.slice(0, start) + text + el.value.slice(end);
		el.selectionStart = el.selectionEnd = start + text.length;
		el.dispatchEvent(new Event('input', { bubbles: true }));
		return;
	}

	// execCommand is deprecated but remains the most reliable way to insert
	// text into contenteditable elements across WebKit/GTK. The modern
	// InputEvent('insertText') alternative has inconsistent support.
	if (el.isContentEditable) {
		document.execCommand('insertText', false, text);
	}
})();
`

// WebViewTarget implements text input and focus restoration for WebView fields.
type WebViewTarget struct {
	webView *webkit.WebView
}

var _ port.TextInputTarget = (*WebViewTarget)(nil)
var _ port.Focusable = (*WebViewTarget)(nil)

// NewWebViewTarget creates a new WebView target.
func NewWebViewTarget(webView *webkit.WebView) *WebViewTarget {
	return &WebViewTarget{
		webView: webView,
	}
}

// InsertText inserts text at the current cursor position in the WebView.
func (t *WebViewTarget) InsertText(ctx context.Context, text string) error {
	log := logging.FromContext(ctx)

	if t.webView == nil {
		log.Warn().Msg("WebViewTarget: webView is nil")
		return nil
	}

	script := fmt.Sprintf(insertTextJS, text)
	t.webView.EvaluateJavascript(script, -1, nil, nil, nil, nil, 0)

	log.Debug().
		Int("len", len(text)).
		Msg("inserted text into WebView via JS")

	return nil
}

// DeleteBeforeCursor deletes n characters before the cursor in the WebView.
// Uses JavaScript to manipulate the focused input element.
func (t *WebViewTarget) DeleteBeforeCursor(ctx context.Context, n int) error {
	log := logging.FromContext(ctx)

	if t.webView == nil {
		log.Warn().Msg("WebViewTarget: webView is nil")
		return nil
	}

	if n <= 0 {
		return nil
	}

	// Execute JavaScript to delete characters
	script := fmt.Sprintf(deleteBeforeCursorJS, n)
	t.webView.EvaluateJavascript(script, -1, nil, nil, nil, nil, 0)

	log.Debug().Int("n", n).Msg("deleted characters before cursor in WebView")
	return nil
}

// Focus restores GTK focus to the WebView widget and refocuses the last
// editable element inside the page.
func (t *WebViewTarget) Focus(_ context.Context) {
	if t.webView == nil {
		return
	}

	t.webView.GrabFocus()
	t.webView.EvaluateJavascript(`
		(function() {
			var el = window.__dumber_lastEditableEl;
			if (el) el.focus();
		})();
	`, -1, nil, nil, nil, nil, 0)
}
