package webkit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/rs/zerolog"
)

// WebViewID is an alias to port.WebViewID for clean architecture compliance.
// Infrastructure layer uses the type defined in the application port.
type WebViewID = port.WebViewID

// LoadEvent represents WebKit load events.
type LoadEvent int

const (
	LoadStarted    LoadEvent = LoadEvent(webkit.LoadStartedValue)
	LoadRedirected LoadEvent = LoadEvent(webkit.LoadRedirectedValue)
	LoadCommitted  LoadEvent = LoadEvent(webkit.LoadCommittedValue)
	LoadFinished   LoadEvent = LoadEvent(webkit.LoadFinishedValue)
)

// PopupRequest contains information about a popup window request from the create signal.
type PopupRequest struct {
	TargetURI     string
	FrameName     string // e.g., "_blank", custom name, or empty
	IsUserGesture bool
	ParentID      WebViewID
}

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
	ucm   *webkit.UserContentManager

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
	OnCreate          func(PopupRequest) *WebView // Return new WebView or nil to block popup
	OnReadyToShow     func()                      // Called when popup is ready to display

	logger zerolog.Logger
	mu     sync.RWMutex

	frontendAttached atomic.Bool

	// asyncCallbacks keeps references to async JS callbacks to prevent GC
	asyncCallbacks []interface{}
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
		ucm:       inner.GetUserContentManager(),
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

// NewWebViewWithRelated creates a WebView that shares session/cookies with parent.
// This is required for popup windows to maintain authentication state.
func NewWebViewWithRelated(parent *WebView, settings *SettingsManager, logger zerolog.Logger) (*WebView, error) {
	if parent == nil {
		return nil, fmt.Errorf("parent webview is nil")
	}
	if parent.IsDestroyed() {
		return nil, fmt.Errorf("parent webview %d is destroyed", parent.id)
	}

	inner := webkit.NewWebViewWithRelatedView(parent.inner)
	if inner == nil {
		return nil, fmt.Errorf("failed to create related webkit webview")
	}

	wv := &WebView{
		inner:     inner,
		ucm:       inner.GetUserContentManager(),
		logger:    logger.With().Str("component", "webview-popup").Logger(),
		signalIDs: make([]uint32, 0, 6),
	}

	wv.id = globalRegistry.register(wv)

	if settings != nil {
		settings.ApplyToWebView(inner)
	}

	wv.connectSignals()

	wv.logger.Debug().
		Uint64("id", uint64(wv.id)).
		Uint64("parent_id", uint64(parent.id)).
		Msg("related webview created for popup")

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

	// create signal for popup window handling
	createCb := func(inner webkit.WebView, navActionPtr uintptr) gtk.Widget {
		if wv.OnCreate == nil {
			return gtk.Widget{} // Block popup
		}

		navAction := webkit.NavigationActionFromPointer(navActionPtr)
		if navAction == nil {
			return gtk.Widget{} // Block popup
		}
		defer navAction.Free()

		var targetURI string
		if req := navAction.GetRequest(); req != nil {
			targetURI = req.GetUri()
		}

		popupReq := PopupRequest{
			TargetURI:     targetURI,
			FrameName:     navAction.GetFrameName(),
			IsUserGesture: navAction.IsUserGesture(),
			ParentID:      wv.id,
		}

		newWV := wv.OnCreate(popupReq)
		if newWV == nil {
			return gtk.Widget{} // Block popup
		}

		return newWV.inner.Widget
	}
	sigID = wv.inner.ConnectCreate(&createCb)
	wv.signalIDs = append(wv.signalIDs, sigID)

	// ready-to-show signal for popup display
	readyToShowCb := func(inner webkit.WebView) {
		if wv.OnReadyToShow != nil {
			wv.OnReadyToShow()
		}
	}
	sigID = wv.inner.ConnectReadyToShow(&readyToShowCb)
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

// UserContentManager returns the content manager associated with this WebView.
func (wv *WebView) UserContentManager() *webkit.UserContentManager {
	return wv.ucm
}

// Widget returns the underlying webkit.WebView for GTK embedding.
func (wv *WebView) Widget() *webkit.WebView {
	return wv.inner
}

// LoadURI loads the given URI.
func (wv *WebView) LoadURI(ctx context.Context, uri string) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.inner.LoadUri(uri)
	logging.FromContext(ctx).Debug().Str("uri", uri).Msg("loading URI")
	return nil
}

// LoadHTML loads HTML content with an optional base URI.
func (wv *WebView) LoadHTML(ctx context.Context, content, baseURI string) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	var baseURIPtr *string
	if baseURI != "" {
		baseURIPtr = &baseURI
	}
	wv.inner.LoadHtml(content, baseURIPtr)
	return nil
}

// Reload reloads the current page.
func (wv *WebView) Reload(ctx context.Context) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.inner.Reload()
	return nil
}

// ReloadBypassCache reloads the current page, bypassing the cache.
func (wv *WebView) ReloadBypassCache(ctx context.Context) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.inner.ReloadBypassCache()
	return nil
}

// Stop stops the current load operation.
func (wv *WebView) Stop(ctx context.Context) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.inner.StopLoading()
	return nil
}

// GoBack navigates back in history.
func (wv *WebView) GoBack(ctx context.Context) error {
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
func (wv *WebView) GoForward(ctx context.Context) error {
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
func (wv *WebView) SetZoomLevel(ctx context.Context, level float64) error {
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

// ShowDevTools opens the WebKit inspector/developer tools.
func (wv *WebView) ShowDevTools() error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	inspector := wv.inner.GetInspector()
	if inspector == nil {
		return fmt.Errorf("failed to get inspector for webview %d", wv.id)
	}
	inspector.Show()
	wv.logger.Debug().Uint64("id", uint64(wv.id)).Msg("devtools shown")
	return nil
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

// RunJavaScript executes script in the specified world (empty for main world).
// This is fire-and-forget: it does not block and errors are logged asynchronously.
// Safe to call from any context including GTK signal handlers.
func (wv *WebView) RunJavaScript(ctx context.Context, script, worldName string) {
	if wv.destroyed.Load() {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	log := logging.FromContext(ctx)

	cb := gio.AsyncReadyCallback(func(_ uintptr, resPtr uintptr, _ uintptr) {
		if resPtr == 0 {
			log.Warn().Uint64("webview_id", uint64(wv.id)).Msg("RunJavaScript: nil async result")
			return
		}

		res := &gio.AsyncResultBase{Ptr: resPtr}
		value, err := wv.inner.EvaluateJavascriptFinish(res)
		if err != nil {
			log.Warn().Err(err).Uint64("webview_id", uint64(wv.id)).Msg("RunJavaScript: failed")
			return
		}

		if value != nil {
			if jscCtx := value.GetContext(); jscCtx != nil {
				if exc := jscCtx.GetException(); exc != nil {
					log.Warn().
						Str("exception", exc.GetMessage()).
						Uint64("webview_id", uint64(wv.id)).
						Msg("RunJavaScript: JS exception")
				}
			}
		}
	})

	// prevent callback from being GC'd before it's called
	wv.mu.Lock()
	wv.asyncCallbacks = append(wv.asyncCallbacks, cb)
	wv.mu.Unlock()

	// worldName: nil for main world, &worldName for specific world
	// sourceUri: nil (not used)
	var worldNamePtr *string
	if worldName != "" {
		worldNamePtr = &worldName
	}
	wv.inner.EvaluateJavascript(script, -1, worldNamePtr, nil, nil, &cb, 0)
}

// AttachFrontend injects scripts/styles and wires the message router once per WebView.
func (wv *WebView) AttachFrontend(ctx context.Context, injector *ContentInjector, router *MessageRouter) error {
	if ctx == nil {
		ctx = context.Background()
	}

	log := logging.FromContext(ctx).With().
		Str("component", "webview").
		Uint64("webview_id", uint64(wv.id)).
		Logger()

	log.Debug().
		Bool("injector_nil", injector == nil).
		Bool("router_nil", router == nil).
		Msg("AttachFrontend called")

	if wv.destroyed.Load() {
		log.Debug().Msg("AttachFrontend: webview is destroyed")
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	if injector == nil && router == nil {
		log.Debug().Msg("AttachFrontend: both injector and router are nil, skipping")
		return nil
	}

	if !wv.frontendAttached.CompareAndSwap(false, true) {
		log.Debug().Msg("AttachFrontend: already attached, skipping")
		return nil // already attached
	}

	var attachErr error

	defer func() {
		if attachErr != nil {
			// allow retry on next call
			wv.frontendAttached.Store(false)
		}
	}()

	if router != nil {
		log.Debug().Msg("AttachFrontend: setting up message router")
		if _, err := router.SetupMessageHandler(wv.ucm, ScriptWorldName); err != nil {
			attachErr = fmt.Errorf("setup message router: %w", err)
			log.Warn().Err(err).Msg("failed to attach message router")
			return attachErr
		}
		log.Debug().Msg("AttachFrontend: message router setup complete")
	}

	if injector != nil {
		log.Debug().Msg("AttachFrontend: injecting scripts and styles")
		injector.InjectScripts(ctx, wv.ucm, wv.id)
		injector.InjectStyles(ctx, wv.ucm)
	}

	log.Debug().Msg("frontend assets attached to webview")
	return nil
}
