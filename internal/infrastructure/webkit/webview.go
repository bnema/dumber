package webkit

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/rs/zerolog"
)

// WebViewID uniquely identifies a WebView instance.
type WebViewID uint64

// LoadEvent represents WebKit load events.
type LoadEvent int

const (
	LoadStarted    LoadEvent = LoadEvent(webkit.LoadStartedValue)
	LoadRedirected LoadEvent = LoadEvent(webkit.LoadRedirectedValue)
	LoadCommitted  LoadEvent = LoadEvent(webkit.LoadCommittedValue)
	LoadFinished   LoadEvent = LoadEvent(webkit.LoadFinishedValue)
)

// webViewRegistry tracks all active WebViews.
type webViewRegistry struct {
	views   map[WebViewID]*WebView
	counter atomic.Uint64
	mu      sync.RWMutex
}

var globalRegistry = &webViewRegistry{
	views: make(map[WebViewID]*WebView),
}

func (r *webViewRegistry) register(wv *WebView) WebViewID {
	id := WebViewID(r.counter.Add(1))
	r.mu.Lock()
	r.views[id] = wv
	r.mu.Unlock()
	return id
}

func (r *webViewRegistry) unregister(id WebViewID) {
	r.mu.Lock()
	delete(r.views, id)
	r.mu.Unlock()
}

// Lookup returns a WebView by ID, or nil if not found.
func (r *webViewRegistry) Lookup(id WebViewID) *WebView {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.views[id]
}

// LookupWebView returns a WebView by ID from the global registry.
func LookupWebView(id WebViewID) *WebView {
	return globalRegistry.Lookup(id)
}

// WebView wraps webkit.WebView with Go-level state tracking and callbacks.
type WebView struct {
	id    WebViewID
	inner *webkit.WebView

	// State (protected by mutex)
	destroyed atomic.Bool
	uri       string
	title     string
	progress  float64
	canGoBack bool
	canGoFwd  bool
	isLoading bool

	// Signal handler IDs for disconnection
	signalIDs []uint32

	// Callbacks (set by UI layer)
	OnLoadChanged     func(LoadEvent)
	OnTitleChanged    func(string)
	OnURIChanged      func(string)
	OnProgressChanged func(float64)
	OnClose           func()
	// TODO(puregotk-webkit): OnCreate for popup support
	// See: /home/brice/sync/alpha/obsidian/projects/puregotk-webkit/issues/related-webview.md

	logger zerolog.Logger
	mu     sync.RWMutex
}

// NewWebView creates a new WebView with the given context and settings.
func NewWebView(wkCtx *WebKitContext, settings *SettingsManager, logger zerolog.Logger) (*WebView, error) {
	if wkCtx == nil || !wkCtx.IsInitialized() {
		return nil, fmt.Errorf("webkit context not initialized")
	}

	inner := webkit.NewWebView()
	if inner == nil {
		return nil, fmt.Errorf("failed to create webkit webview")
	}

	wv := &WebView{
		inner:     inner,
		logger:    logger.With().Str("component", "webview").Logger(),
		signalIDs: make([]uint32, 0, 4),
	}

	// Register in global registry
	wv.id = globalRegistry.register(wv)

	// Apply settings if provided
	if settings != nil {
		settings.ApplyToWebView(inner)
	}

	// Connect signals
	wv.connectSignals()

	wv.logger.Debug().Uint64("id", uint64(wv.id)).Msg("webview created")

	return wv, nil
}

// connectSignals sets up signal handlers for the WebView.
func (wv *WebView) connectSignals() {
	// load-changed signal
	loadChangedCb := func(inner webkit.WebView, event webkit.LoadEvent) {
		wv.mu.Lock()
		wv.uri = inner.GetUri()
		wv.title = inner.GetTitle()
		wv.canGoBack = inner.CanGoBack()
		wv.canGoFwd = inner.CanGoForward()
		wv.progress = inner.GetEstimatedLoadProgress()

		switch event {
		case webkit.LoadStartedValue:
			wv.isLoading = true
		case webkit.LoadFinishedValue:
			wv.isLoading = false
		}
		wv.mu.Unlock()

		if wv.OnLoadChanged != nil {
			wv.OnLoadChanged(LoadEvent(event))
		}
	}
	sigID := wv.inner.ConnectLoadChanged(&loadChangedCb)
	wv.signalIDs = append(wv.signalIDs, sigID)

	// close signal
	closeCb := func(inner webkit.WebView) {
		if wv.OnClose != nil {
			wv.OnClose()
		}
	}
	sigID = wv.inner.ConnectClose(&closeCb)
	wv.signalIDs = append(wv.signalIDs, sigID)

	// Note: notify::title, notify::uri, notify::estimated-load-progress
	// would need GObject property change notifications which may require
	// different API patterns in puregotk. For now, we update these in
	// load-changed which covers most cases.
}

// ID returns the unique identifier for this WebView.
func (wv *WebView) ID() WebViewID {
	return wv.id
}

// Widget returns the underlying webkit.WebView for GTK embedding.
func (wv *WebView) Widget() *webkit.WebView {
	return wv.inner
}

// LoadURI loads the given URI.
func (wv *WebView) LoadURI(uri string) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.inner.LoadUri(uri)
	wv.logger.Debug().Str("uri", uri).Msg("loading URI")
	return nil
}

// LoadHTML loads HTML content with an optional base URI.
func (wv *WebView) LoadHTML(content, baseURI string) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.inner.LoadHtml(content, baseURI)
	return nil
}

// Reload reloads the current page.
func (wv *WebView) Reload() error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.inner.Reload()
	return nil
}

// ReloadBypassCache reloads the current page, bypassing the cache.
func (wv *WebView) ReloadBypassCache() error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.inner.ReloadBypassCache()
	return nil
}

// Stop stops the current load operation.
func (wv *WebView) Stop() error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.inner.StopLoading()
	return nil
}

// GoBack navigates back in history.
func (wv *WebView) GoBack() error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	if !wv.inner.CanGoBack() {
		return fmt.Errorf("cannot go back")
	}
	wv.inner.GoBack()
	return nil
}

// GoForward navigates forward in history.
func (wv *WebView) GoForward() error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	if !wv.inner.CanGoForward() {
		return fmt.Errorf("cannot go forward")
	}
	wv.inner.GoForward()
	return nil
}

// CanGoBack returns true if back navigation is possible.
func (wv *WebView) CanGoBack() bool {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.canGoBack
}

// CanGoForward returns true if forward navigation is possible.
func (wv *WebView) CanGoForward() bool {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.canGoFwd
}

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

// EstimatedProgress returns the estimated load progress (0.0 to 1.0).
func (wv *WebView) EstimatedProgress() float64 {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.progress
}

// SetZoomLevel sets the zoom level (1.0 = 100%).
func (wv *WebView) SetZoomLevel(level float64) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.inner.SetZoomLevel(level)
	return nil
}

// GetZoomLevel returns the current zoom level.
func (wv *WebView) GetZoomLevel() float64 {
	if wv.destroyed.Load() {
		return 1.0
	}
	return wv.inner.GetZoomLevel()
}

// IsDestroyed returns true if the WebView has been destroyed.
func (wv *WebView) IsDestroyed() bool {
	return wv.destroyed.Load()
}

// Destroy cleans up the WebView resources.
func (wv *WebView) Destroy() {
	if wv.destroyed.Swap(true) {
		return // Already destroyed
	}

	// Unregister from global registry
	globalRegistry.unregister(wv.id)

	// Note: Signal disconnection and GTK widget destruction
	// would happen here in a full implementation.
	// For now, we rely on GTK's reference counting.

	wv.logger.Debug().Uint64("id", uint64(wv.id)).Msg("webview destroyed")
}
