//go:build !webkit_cgo

package webkit

import (
	"fmt"
	"log"
)

// WebView represents a browser view powered by WebKit2GTK.
// Methods are currently stubs returning ErrNotImplemented to satisfy TDD ordering.
type WebView struct {
	config       *Config
	visible      bool
	zoom         float64
	url          string
	destroyed    bool
	window       *Window
	msgHandler   func(payload string)
	titleHandler func(title string)
	uriHandler   func(uri string)
	zoomHandler  func(level float64)
	useDomZoom   bool
	domZoomSeed  float64
}

// NewWebView constructs a new WebView instance.
func NewWebView(cfg *Config) (*WebView, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	log.Printf("[webkit] NewWebView (non-CGO stub) — UI will not be displayed. Build with -tags=webkit_cgo for native window.")
	wv := &WebView{config: cfg, useDomZoom: cfg.UseDomZoom}
	if cfg.ZoomDefault <= 0 {
		wv.zoom = 1.0
	} else {
		wv.zoom = cfg.ZoomDefault
	}
	wv.domZoomSeed = wv.zoom
	// Construction succeeds; create a logical window placeholder.
	wv.window = &Window{Title: "Dumber Browser"}
	return wv, nil
}

// LoadURL navigates the webview to the specified URL.
func (w *WebView) LoadURL(rawURL string) error {
	if w == nil || w.destroyed {
		return ErrNotImplemented
	}
	if rawURL == "" {
		return fmt.Errorf("url cannot be empty")
	}
	// Minimal validation; actual navigation handled by CGO bridge later.
	w.url = rawURL
	log.Printf("[webkit] LoadURL (non-CGO stub): %s", rawURL)
	return nil
}

// Show makes the WebView visible.
func (w *WebView) Show() error {
	if w == nil || w.destroyed {
		return ErrNotImplemented
	}
	w.visible = true
	log.Printf("[webkit] Show (non-CGO stub): no native window will appear")
	return nil
}

// Hide hides the WebView window.
func (w *WebView) Hide() error {
	if w == nil || w.destroyed {
		return ErrNotImplemented
	}
	w.visible = false
	return nil
}

// Destroy releases native resources.
func (w *WebView) Destroy() error {
	if w == nil {
		return ErrNotImplemented
	}
	w.destroyed = true
	return nil
}

// Window returns the associated native window wrapper (non-nil).
func (w *WebView) Window() *Window { return w.window }

// GetCurrentURL returns the last requested URL (non-CGO build approximation).
func (w *WebView) GetCurrentURL() string { return w.url }

// GoBack is not supported in the non-CGO stub.
func (w *WebView) GoBack() error { return ErrNotImplemented }

// GoForward is not supported in the non-CGO stub.
func (w *WebView) GoForward() error { return ErrNotImplemented }

// Reload is not supported in the non-CGO stub.
func (w *WebView) Reload() error { return ErrNotImplemented }

// ReloadBypassCache is not supported in the non-CGO stub.
func (w *WebView) ReloadBypassCache() error { return ErrNotImplemented }

// ShowDevTools is a no-op in the non-CGO build.
func (w *WebView) ShowDevTools() error { return nil }

// CloseDevTools is a no-op in the non-CGO build.
func (w *WebView) CloseDevTools() error { return nil }

// RegisterScriptMessageHandler registers a callback invoked when the content script posts a message.
func (w *WebView) RegisterScriptMessageHandler(cb func(payload string)) { w.msgHandler = cb }

func (w *WebView) dispatchScriptMessage(payload string) { //nolint:unused // Called from CGO WebKit callbacks
	if w != nil && w.msgHandler != nil {
		w.msgHandler(payload)
	}
}

// RegisterTitleChangedHandler registers a callback invoked when the page title changes.
func (w *WebView) RegisterTitleChangedHandler(cb func(title string)) { w.titleHandler = cb }

func (w *WebView) dispatchTitleChanged(title string) { //nolint:unused // Called from CGO WebKit callbacks
	if w != nil && w.titleHandler != nil {
		w.titleHandler(title)
	}
}

// RegisterURIChangedHandler registers a callback invoked when the current page URI changes.
func (w *WebView) RegisterURIChangedHandler(cb func(uri string)) { w.uriHandler = cb }

func (w *WebView) dispatchURIChanged(uri string) { //nolint:unused // Called from CGO WebKit callbacks
	if w != nil && w.uriHandler != nil {
		w.uriHandler(uri)
	}
}

// RegisterZoomChangedHandler registers a callback invoked when zoom level changes.
func (w *WebView) RegisterZoomChangedHandler(cb func(level float64)) { w.zoomHandler = cb }

func (w *WebView) dispatchZoomChanged(level float64) {
	if w != nil && w.zoomHandler != nil {
		w.zoomHandler(level)
	}
}

// RunOnMainThread executes fn immediately in non-CGO builds.
func (w *WebView) RunOnMainThread(fn func()) {
	if fn != nil {
		fn()
	}
}

// UsesDomZoom reports whether DOM-based zoom is enabled in this WebView.
func (w *WebView) UsesDomZoom() bool {
	return w != nil && w.useDomZoom
}

// SeedDomZoom stores the desired DOM zoom level for the next navigation (stub implementation).
func (w *WebView) SeedDomZoom(level float64) {
	if w == nil || !w.useDomZoom {
		return
	}
	if level < 0.25 {
		level = 0.25
	} else if level > 5.0 {
		level = 5.0
	}
	w.domZoomSeed = level
}

// InitializeContentBlocking initializes WebKit content blocking with filter manager (stub)
func (w *WebView) InitializeContentBlocking(filterManager interface{}) error {
	return ErrNotImplemented
}

// OnNavigate sets up domain-specific cosmetic filtering on navigation (stub)
func (w *WebView) OnNavigate(url string, filterManager interface{}) {
	// No-op in stub
}

// UpdateContentFilters updates the content filters dynamically (stub)
func (w *WebView) UpdateContentFilters(filterManager interface{}) error {
	return ErrNotImplemented
}
