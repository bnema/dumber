//go:build !webkit_cgo

package webkit

import (
	"fmt"
	"log"
	"strconv"
	"sync/atomic"
	"unsafe"
)

// Package-level ID counter for stub WebViews
var stubViewIDCounter uint64

// nextStubViewID generates a unique ID for stub WebViews
func nextStubViewID() uintptr {
	return uintptr(atomic.AddUint64(&stubViewIDCounter, 1))
}

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
	popupHandler func(string) *WebView
	closeHandler func()
	useDomZoom   bool
	domZoomSeed  float64
	container    uintptr
	id           uintptr // WebView unique identifier

	// Window type fields (no-op in stub)
	windowType         WindowType
	windowFeatures     *WindowFeatures
	windowTypeCallback func(WindowType, *WindowFeatures)
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
	wv.id = nextStubViewID() // Assign unique ID
	// Construction succeeds; create a logical window placeholder.
	wv.window = &Window{Title: "Dumber Browser"}
	wv.container = newWidgetHandle()
	return wv, nil
}

// NewWebViewWithRelated creates a new WebView related to an existing one (non-CGO stub)
func NewWebViewWithRelated(cfg *Config, relatedView *WebView) (*WebView, error) {
	// In non-CGO build, just create a regular WebView
	return NewWebView(cfg)
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

func (w *WebView) RegisterPopupHandler(cb func(string) *WebView) { w.popupHandler = cb }

func (w *WebView) RegisterCloseHandler(cb func()) { w.closeHandler = cb }

func (w *WebView) dispatchScriptMessage(payload string) { //nolint:unused // Called from CGO WebKit callbacks
	if w != nil && w.msgHandler != nil {
		w.msgHandler(payload)
	}
}

func (w *WebView) dispatchPopupRequest(uri string) *WebView { //nolint:unused // Called from CGO WebKit callbacks
	if w != nil && w.popupHandler != nil {
		return w.popupHandler(uri)
	}
	return nil
}

// GetNativePointer returns nil for non-CGO build (stub implementation)
func (w *WebView) GetNativePointer() unsafe.Pointer { //nolint:unused // Needed for interface compatibility
	return nil
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

// Widget returns an empty handle in stub builds.
func (w *WebView) Widget() uintptr { return 0 }

// RootWidget returns the container widget handle in CGO builds. Stub returns 0.
func (w *WebView) RootWidget() uintptr { return w.container }

// DestroyWindow is a no-op in stub builds.
func (w *WebView) DestroyWindow() {}

// RunOnMainThread executes fn immediately in non-CGO builds.
func (w *WebView) RunOnMainThread(fn func()) {
	if fn != nil {
		fn()
	}
}

// PrepareForReparenting is a no-op in stub builds.
func (w *WebView) PrepareForReparenting() {}

// RefreshAfterReparenting is a no-op in stub builds.
func (w *WebView) RefreshAfterReparenting() {}

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

// CreateRelatedView returns a new WebView (stub creates independent)
func (w *WebView) CreateRelatedView() *WebView {
	nw, _ := NewWebView(w.config)
	return nw
}

// OnWindowTypeDetected registers a callback (stored but never invoked in stub)
func (w *WebView) OnWindowTypeDetected(callback func(WindowType, *WindowFeatures)) {
	if w != nil {
		w.windowTypeCallback = callback
	}
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

// SetWindowFeatures sets the window features for this WebView (stub)
func (w *WebView) SetWindowFeatures(features *WindowFeatures) {
	w.windowFeatures = features
}

// SetActive sets whether this WebView is currently active/focused (stub)
func (w *WebView) SetActive(active bool) {
	// No-op in stub
}

// IsActive returns whether this WebView is currently active/focused (stub)
func (w *WebView) IsActive() bool {
	return false
}

// ID returns the unique identifier for this WebView as a string (stub)
func (w *WebView) ID() string {
	if w == nil {
		return ""
	}
	return strconv.FormatUint(uint64(w.id), 10)
}
