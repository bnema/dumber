package textinput

import (
	"context"
	"fmt"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

// clipboardRestoreDelay is the delay before restoring the clipboard after paste.
const clipboardRestoreDelay = 50 * time.Millisecond

// deleteBeforeCursorJS is the JavaScript template to delete n characters before cursor.
// It handles both input/textarea elements and contenteditable elements.
const deleteBeforeCursorJS = `
(function() {
	const n = %d;
	const el = document.activeElement;
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
		const range = sel.getRangeAt(0);
		for (let i = 0; i < n; i++) {
			sel.modify('extend', 'backward', 'character');
		}
		sel.deleteFromDocument();
	}
})();
`

// WebViewTarget implements TextInputTarget for WebView text fields.
// It uses the clipboard-paste pattern to insert text:
// 1. Save current clipboard
// 2. Write text to clipboard
// 3. Execute paste command
// 4. Restore original clipboard after a short delay
type WebViewTarget struct {
	webView   *webkit.WebView
	clipboard port.Clipboard
}

// Compile-time interface check.
var _ port.TextInputTarget = (*WebViewTarget)(nil)

// NewWebViewTarget creates a new WebView target.
func NewWebViewTarget(webView *webkit.WebView, clipboard port.Clipboard) *WebViewTarget {
	return &WebViewTarget{
		webView:   webView,
		clipboard: clipboard,
	}
}

// InsertText inserts text at the current cursor position in the WebView.
// Uses clipboard-paste pattern since WebView doesn't expose direct text insertion.
func (t *WebViewTarget) InsertText(ctx context.Context, text string) error {
	log := logging.FromContext(ctx)

	if t.webView == nil {
		log.Warn().Msg("WebViewTarget: webView is nil")
		return nil
	}

	// Save current clipboard contents
	originalClipboard, _ := t.clipboard.ReadText(ctx)

	// Write the accent to clipboard
	if err := t.clipboard.WriteText(ctx, text); err != nil {
		log.Error().Err(err).Msg("failed to write text to clipboard")
		return err
	}

	// Execute paste command
	t.webView.ExecuteEditingCommand(webkit.EDITING_COMMAND_PASTE)

	log.Debug().
		Str("text", text).
		Msg("inserted text into WebView via clipboard paste")

	// Restore original clipboard after a short delay
	// This allows the paste operation to complete first
	go func() {
		time.Sleep(clipboardRestoreDelay)
		if originalClipboard != "" {
			if err := t.clipboard.WriteText(ctx, originalClipboard); err != nil {
				log.Debug().Err(err).Msg("failed to restore clipboard")
			}
		}
	}()

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
