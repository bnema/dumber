package cef

import (
	"context"
	"encoding/base64"
	"errors"
	"sync"
	"sync/atomic"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
)

// Compile-time interface checks.
var (
	_ port.WebView              = (*WebView)(nil)
	_ port.NativeWidgetProvider = (*WebView)(nil)
)

// errDestroyed is returned when an operation is attempted on a destroyed WebView.
var errDestroyed = errors.New("cef: webview is destroyed")

// errNoBrowser is returned when the browser has not been created yet.
var errNoBrowser = errors.New("cef: browser not yet created")

// WebView implements port.WebView using a CEF off-screen browser rendered
// through a renderPipeline and driven by an inputBridge.
type WebView struct {
	id       port.WebViewID
	browser  purecef.Browser
	host     purecef.BrowserHost
	pipeline *renderPipeline
	input    *inputBridge
	handlers *handlerSet

	// Callbacks set by use case layer.
	mu        sync.RWMutex
	callbacks *port.WebViewCallbacks

	// State cache (mutex-protected).
	uri       string
	title     string
	progress  float64
	canGoBack bool
	canGoFwd  bool
	isLoading bool

	// Atomic state.
	destroyed  atomic.Bool
	fullscreen atomic.Bool
	generation atomic.Uint64
}

// ---------------------------------------------------------------------------
// Identity
// ---------------------------------------------------------------------------

// ID returns the unique identifier for this WebView.
func (wv *WebView) ID() port.WebViewID {
	return wv.id
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

// LoadURI navigates to the specified URI.
func (wv *WebView) LoadURI(_ context.Context, uri string) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	if wv.browser == nil {
		return errNoBrowser
	}
	wv.browser.GetMainFrame().LoadURL(uri)
	return nil
}

// LoadHTML loads HTML content with an optional base URI (ignored in Phase 1).
func (wv *WebView) LoadHTML(_ context.Context, content, _ string) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	if wv.browser == nil {
		return errNoBrowser
	}
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(content))
	wv.browser.GetMainFrame().LoadURL(dataURL)
	return nil
}

// Reload reloads the current page.
func (wv *WebView) Reload(_ context.Context) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	if wv.browser == nil {
		return errNoBrowser
	}
	wv.browser.Reload()
	return nil
}

// ReloadBypassCache reloads the current page, bypassing cache.
func (wv *WebView) ReloadBypassCache(_ context.Context) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	if wv.browser == nil {
		return errNoBrowser
	}
	wv.browser.ReloadIgnoreCache()
	return nil
}

// Stop stops the current page load.
func (wv *WebView) Stop(_ context.Context) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	if wv.browser == nil {
		return errNoBrowser
	}
	wv.browser.StopLoad()
	return nil
}

// GoBack navigates back in history.
func (wv *WebView) GoBack(_ context.Context) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	if wv.browser == nil {
		return errNoBrowser
	}
	wv.browser.GoBack()
	return nil
}

// GoForward navigates forward in history.
func (wv *WebView) GoForward(_ context.Context) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	if wv.browser == nil {
		return errNoBrowser
	}
	wv.browser.GoForward()
	return nil
}

// ---------------------------------------------------------------------------
// State queries (read from cache under RLock)
// ---------------------------------------------------------------------------

// URI returns the current URI.
func (wv *WebView) URI() string {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.uri
}

// Title returns the current page title.
func (wv *WebView) Title() string {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.title
}

// IsLoading returns true if a page is currently loading.
func (wv *WebView) IsLoading() bool {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.isLoading
}

// EstimatedProgress returns the load progress (0.0 to 1.0).
func (wv *WebView) EstimatedProgress() float64 {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.progress
}

// CanGoBack returns true if back navigation is available.
func (wv *WebView) CanGoBack() bool {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.canGoBack
}

// CanGoForward returns true if forward navigation is available.
func (wv *WebView) CanGoForward() bool {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.canGoFwd
}

// State returns the current WebView state as a snapshot.
func (wv *WebView) State() port.WebViewState {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return port.WebViewState{
		URI:       wv.uri,
		Title:     wv.title,
		IsLoading: wv.isLoading,
		Progress:  wv.progress,
		CanGoBack: wv.canGoBack,
		CanGoFwd:  wv.canGoFwd,
		ZoomLevel: 1.0,
	}
}

// IsFullscreen returns true if the WebView is currently in fullscreen mode.
func (wv *WebView) IsFullscreen() bool {
	return wv.fullscreen.Load()
}

// IsPlayingAudio returns false (Phase 1 stub).
func (wv *WebView) IsPlayingAudio() bool {
	return false
}

// Generation returns a monotonic counter incremented on pool reuse.
func (wv *WebView) Generation() uint64 {
	return wv.generation.Load()
}

// Favicon returns nil (Phase 1 stub).
func (wv *WebView) Favicon() port.Texture {
	return nil
}

// ---------------------------------------------------------------------------
// Zoom (Phase 1 stubs)
// ---------------------------------------------------------------------------

// GetZoomLevel returns 1.0 (Phase 1 stub).
func (wv *WebView) GetZoomLevel() float64 {
	return 1.0
}

// SetZoomLevel is a no-op (Phase 1 stub).
func (wv *WebView) SetZoomLevel(_ context.Context, _ float64) error {
	return nil
}

// ---------------------------------------------------------------------------
// Find
// ---------------------------------------------------------------------------

// GetFindController returns nil (Phase 1 stub).
func (wv *WebView) GetFindController() port.FindController {
	return nil
}

// ---------------------------------------------------------------------------
// Callbacks
// ---------------------------------------------------------------------------

// SetCallbacks registers callback handlers for WebView events.
func (wv *WebView) SetCallbacks(cb *port.WebViewCallbacks) {
	wv.mu.Lock()
	defer wv.mu.Unlock()
	wv.callbacks = cb
}

// ---------------------------------------------------------------------------
// JavaScript / Appearance
// ---------------------------------------------------------------------------

// RunJavaScript executes a script in the main world. Fire-and-forget.
func (wv *WebView) RunJavaScript(_ context.Context, script string) {
	if wv.destroyed.Load() || wv.browser == nil {
		return
	}
	wv.browser.GetMainFrame().ExecuteJavaScript(script, "", 0)
}

// SetBackgroundColor is a no-op in Phase 1.
func (wv *WebView) SetBackgroundColor(_, _, _, _ float64) {}

// ResetBackgroundToDefault is a no-op in Phase 1.
func (wv *WebView) ResetBackgroundToDefault() {}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// IsDestroyed returns true if the WebView has been destroyed.
func (wv *WebView) IsDestroyed() bool {
	return wv.destroyed.Load()
}

// Destroy releases all resources associated with this WebView.
func (wv *WebView) Destroy() {
	if !wv.destroyed.CompareAndSwap(false, true) {
		return
	}
	if wv.host != nil {
		wv.host.CloseBrowser(1)
	}
	if wv.pipeline != nil {
		wv.pipeline.destroy()
	}
}

// ---------------------------------------------------------------------------
// NativeWidgetProvider
// ---------------------------------------------------------------------------

// NativeWidget returns the uintptr for embedding the GLArea into GTK.
func (wv *WebView) NativeWidget() uintptr {
	if wv.pipeline == nil || wv.pipeline.glArea == nil {
		return 0
	}
	return wv.pipeline.glArea.GoPointer()
}

// ---------------------------------------------------------------------------
// State update helpers (called from handlers)
// ---------------------------------------------------------------------------

func (wv *WebView) updateURI(uri string) {
	wv.mu.Lock()
	wv.uri = uri
	wv.mu.Unlock()

	wv.mu.RLock()
	cb := wv.callbacks
	wv.mu.RUnlock()
	if cb != nil && cb.OnURIChanged != nil {
		cb.OnURIChanged(uri)
	}
}

func (wv *WebView) updateTitle(title string) {
	wv.mu.Lock()
	wv.title = title
	wv.mu.Unlock()

	wv.mu.RLock()
	cb := wv.callbacks
	wv.mu.RUnlock()
	if cb != nil && cb.OnTitleChanged != nil {
		cb.OnTitleChanged(title)
	}
}

func (wv *WebView) updateProgress(progress float64) {
	wv.mu.Lock()
	wv.progress = progress
	wv.mu.Unlock()

	wv.mu.RLock()
	cb := wv.callbacks
	wv.mu.RUnlock()
	if cb != nil && cb.OnProgressChanged != nil {
		cb.OnProgressChanged(progress)
	}
}

func (wv *WebView) updateLoadState(loading, back, fwd bool) {
	wv.mu.Lock()
	wv.isLoading = loading
	wv.canGoBack = back
	wv.canGoFwd = fwd
	wv.mu.Unlock()
}

// suppress unused import for unsafe (used by NativeWidget via GoPointer).
var _ = unsafe.Pointer(nil)
