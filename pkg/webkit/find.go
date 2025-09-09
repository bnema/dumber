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
    js := fmt.Sprintf("window.__dumber_find_open && window.__dumber_find_open(%s)", quoted)
    return w.InjectScript(js)
}

// FindQuery sets or updates the current find query and applies highlighting.
func (w *WebView) FindQuery(q string) error {
    if w == nil {
        return ErrNotImplemented
    }
    quoted := strconv.Quote(q)
    js := fmt.Sprintf("window.__dumber_find_query && window.__dumber_find_query(%s)", quoted)
    return w.InjectScript(js)
}

// CloseFind closes the find overlay and clears highlights.
func (w *WebView) CloseFind() error {
    if w == nil {
        return ErrNotImplemented
    }
    return w.InjectScript("window.__dumber_find_close && window.__dumber_find_close()")
}

