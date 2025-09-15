package webkit

import (
	"fmt"
	"strconv"
)

// OpenFind opens the in-page find overlay (reusing omnibox) with an optional initial query.
func (w *WebView) OpenFind(initial string) error {
	if w == nil {
		return ErrNotImplemented
	}
	quoted := strconv.Quote(initial)
	// Prefer unified omnibox open via keyboard shortcut; fallback to legacy helper
	js := fmt.Sprintf("(window.__dumber?.omnibox?.open ? window.__dumber.omnibox.open('find', %s) : (window.__dumber_find_open && window.__dumber_find_open(%s)))", quoted, quoted)
	return w.InjectScript(js)
}

// FindQuery sets or updates the current find query and applies highlighting.
func (w *WebView) FindQuery(q string) error {
	if w == nil {
		return ErrNotImplemented
	}
	quoted := strconv.Quote(q)
	js := fmt.Sprintf("(window.__dumber?.omnibox?.findQuery ? window.__dumber.omnibox.findQuery(%s) : (window.__dumber_find_query && window.__dumber_find_query(%s)))", quoted, quoted)
	return w.InjectScript(js)
}

// CloseFind closes the find overlay and clears highlights.
func (w *WebView) CloseFind() error {
	if w == nil {
		return ErrNotImplemented
	}
	return w.InjectScript("(window.__dumber?.omnibox?.close ? window.__dumber.omnibox.close() : (window.__dumber_find_close && window.__dumber_find_close()))")
}
