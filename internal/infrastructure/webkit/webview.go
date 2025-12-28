package webkit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gobject"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/rs/zerolog"
)

// Compile-time interface check: WebView must implement port.WebView.
var _ port.WebView = (*WebView)(nil)

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
	isRelated bool // true if created via NewWebViewWithRelated (shares web process with parent)
	uri       string
	title     string
	progress  float64
	canGoBack bool
	canGoFwd  bool
	isLoading bool

	// Media/fullscreen state for idle inhibition cleanup
	isFullscreen   atomic.Bool
	isPlayingAudio atomic.Bool

	// Progress throttling (~60fps)
	lastProgressUpdate atomic.Int64 // Unix nanoseconds

	// Signal handler IDs for disconnection
	signalIDs []uint32

	// Callbacks (set by UI layer)
	OnLoadChanged       func(LoadEvent)
	OnTitleChanged      func(string)
	OnURIChanged        func(string)
	OnProgressChanged   func(float64)
	OnFaviconChanged    func(*gdk.Texture) // Called when page favicon changes
	OnClose             func()
	OnCreate            func(PopupRequest) *WebView // Return new WebView or nil to block popup
	OnReadyToShow       func()                      // Called when popup is ready to display
	OnLinkMiddleClick   func(uri string) bool       // Return true if handled (blocks navigation)
	OnEnterFullscreen   func() bool                 // Return true to prevent fullscreen
	OnLeaveFullscreen   func() bool                 // Return true to prevent leaving fullscreen
	OnAudioStateChanged func(playing bool)          // Called when audio playback starts/stops
	OnLinkHover         func(uri string)            // Called when hovering over a link/image/media (empty string when leaving)

	logger zerolog.Logger
	mu     sync.RWMutex

	frontendAttached atomic.Bool

	// asyncCallbacks keeps references to async JS callbacks to prevent GC
	asyncCallbacks []interface{}

	// findController is cached to prevent GC from collecting the Go wrapper
	findController     *findControllerAdapter
	findControllerOnce sync.Once
}

type findControllerAdapter struct {
	fc *webkit.FindController
}

func (a *findControllerAdapter) Search(text string, opts port.FindOptions, maxMatches uint) {
	if a == nil || a.fc == nil {
		return
	}
	var flags uint32
	if opts.WrapAround {
		flags |= uint32(webkit.FindOptionsWrapAroundValue)
	}
	if opts.CaseInsensitive {
		flags |= uint32(webkit.FindOptionsCaseInsensitiveValue)
	}
	if opts.AtWordStarts {
		flags |= uint32(webkit.FindOptionsAtWordStartsValue)
	}
	a.fc.Search(text, flags, maxMatches)
}

func (a *findControllerAdapter) CountMatches(text string, opts port.FindOptions, maxMatches uint) {
	if a == nil || a.fc == nil {
		return
	}
	var flags uint32
	if opts.WrapAround {
		flags |= uint32(webkit.FindOptionsWrapAroundValue)
	}
	if opts.CaseInsensitive {
		flags |= uint32(webkit.FindOptionsCaseInsensitiveValue)
	}
	if opts.AtWordStarts {
		flags |= uint32(webkit.FindOptionsAtWordStartsValue)
	}
	a.fc.CountMatches(text, flags, maxMatches)
}

func (a *findControllerAdapter) SearchNext() {
	if a == nil || a.fc == nil {
		return
	}
	a.fc.SearchNext()
}

func (a *findControllerAdapter) SearchPrevious() {
	if a == nil || a.fc == nil {
		return
	}
	a.fc.SearchPrevious()
}

func (a *findControllerAdapter) SearchFinish() {
	if a == nil || a.fc == nil {
		return
	}
	a.fc.SearchFinish()
}

func (a *findControllerAdapter) GetSearchText() string {
	if a == nil || a.fc == nil {
		return ""
	}
	return a.fc.GetSearchText()
}

func (a *findControllerAdapter) OnFoundText(callback func(matchCount uint)) uint32 {
	if a == nil || a.fc == nil || callback == nil {
		return 0
	}
	cb := func(_ webkit.FindController, matchCount uint) {
		callback(matchCount)
	}
	return a.fc.ConnectFoundText(&cb)
}

func (a *findControllerAdapter) OnFailedToFindText(callback func()) uint32 {
	if a == nil || a.fc == nil || callback == nil {
		return 0
	}
	cb := func(_ webkit.FindController) {
		callback()
	}
	return a.fc.ConnectFailedToFindText(&cb)
}

func (a *findControllerAdapter) OnCountedMatches(callback func(matchCount uint)) uint32 {
	if a == nil || a.fc == nil || callback == nil {
		return 0
	}
	cb := func(_ webkit.FindController, matchCount uint) {
		callback(matchCount)
	}
	return a.fc.ConnectCountedMatches(&cb)
}

func (a *findControllerAdapter) DisconnectSignal(id uint32) {
	if a == nil || a.fc == nil || id == 0 {
		return
	}
	obj := gobject.ObjectNewFromInternalPtr(a.fc.GoPointer())
	gobject.SignalHandlerDisconnect(obj, id)
}

// NewWebView creates a new WebView with the given context and settings.
// Uses the persistent NetworkSession from wkCtx for cookie/data persistence.
// bgColor is optional - if provided, sets background immediately to prevent white flash.
func NewWebView(ctx context.Context, wkCtx *WebKitContext, settings *SettingsManager, bgColor *gdk.RGBA) (*WebView, error) {
	log := logging.FromContext(ctx)

	if wkCtx == nil || !wkCtx.IsInitialized() {
		return nil, fmt.Errorf("webkit context not initialized")
	}

	// Use NetworkSession-aware constructor for persistent cookie storage
	inner := webkit.NewWebViewWithNetworkSession(wkCtx.NetworkSession())
	if inner == nil {
		return nil, fmt.Errorf("failed to create webkit webview with network session")
	}

	// Set background color IMMEDIATELY after creation, before any other operations.
	// This prevents white flash by ensuring WebKit's renderer starts with correct color.
	if bgColor != nil {
		inner.SetBackgroundColor(bgColor)
	}

	wv := &WebView{
		inner:     inner,
		ucm:       inner.GetUserContentManager(),
		logger:    log.With().Str("component", "webview").Logger(),
		signalIDs: make([]uint32, 0, 4),
	}

	// Register in global registry
	wv.id = globalRegistry.register(wv)

	// Apply settings if provided
	if settings != nil {
		settings.ApplyToWebView(ctx, inner)
	}

	// Connect signals
	wv.connectSignals()

	wv.logger.Debug().Uint64("id", uint64(wv.id)).Msg("webview created")

	return wv, nil
}

// NewWebViewWithRelated creates a WebView that shares session/cookies with parent.
// This is required for popup windows to maintain authentication state.
func NewWebViewWithRelated(ctx context.Context, parent *WebView, settings *SettingsManager) (*WebView, error) {
	log := logging.FromContext(ctx)

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
		isRelated: true, // Shares web process with parent - must not terminate process on destroy
		ucm:       inner.GetUserContentManager(),
		logger:    log.With().Str("component", "webview-popup").Logger(),
		signalIDs: make([]uint32, 0, 6),
	}

	wv.id = globalRegistry.register(wv)

	if settings != nil {
		settings.ApplyToWebView(ctx, inner)
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
	wv.connectLoadChangedSignal()
	wv.connectCloseSignal()
	wv.connectCreateSignal()
	wv.connectReadyToShowSignal()
	wv.connectTitleSignal()
	wv.connectURISignal()
	wv.connectFaviconSignal()
	wv.connectProgressSignal()
	wv.connectDecidePolicySignal()
	wv.connectEnterFullscreenSignal()
	wv.connectLeaveFullscreenSignal()
	wv.connectAudioStateSignal()
	wv.connectMouseTargetChangedSignal()
}

func (wv *WebView) connectLoadChangedSignal() {
	loadChangedCb := func(inner webkit.WebView, event webkit.LoadEvent) {
		uri := inner.GetUri()
		title := inner.GetTitle()

		wv.mu.Lock()
		wv.uri = uri
		wv.title = title
		wv.canGoBack = inner.CanGoBack()
		wv.canGoFwd = inner.CanGoForward()
		wv.progress = inner.GetEstimatedLoadProgress()

		switch event {
		case webkit.LoadStartedValue:
			wv.isLoading = true
			wv.logger.Debug().Str("uri", uri).Msg("load started")
		case webkit.LoadRedirectedValue:
			wv.logger.Debug().Str("uri", uri).Msg("load redirected")
		case webkit.LoadCommittedValue:
			wv.logger.Debug().Str("uri", uri).Msg("load committed")
		case webkit.LoadFinishedValue:
			wv.isLoading = false
			wv.logger.Debug().Str("uri", uri).Str("title", title).Msg("load finished")
		}
		wv.mu.Unlock()

		if wv.OnLoadChanged != nil {
			wv.OnLoadChanged(LoadEvent(event))
		}
	}
	sigID := wv.inner.ConnectLoadChanged(&loadChangedCb)
	wv.signalIDs = append(wv.signalIDs, sigID)
}

func (wv *WebView) connectCloseSignal() {
	closeCb := func(_ webkit.WebView) {
		if wv.OnClose != nil {
			wv.OnClose()
		}
	}
	sigID := wv.inner.ConnectClose(&closeCb)
	wv.signalIDs = append(wv.signalIDs, sigID)
}

func (wv *WebView) connectCreateSignal() {
	createCb := func(_ webkit.WebView, navActionPtr uintptr) gtk.Widget {
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
	sigID := wv.inner.ConnectCreate(&createCb)
	wv.signalIDs = append(wv.signalIDs, sigID)
}

func (wv *WebView) connectReadyToShowSignal() {
	readyToShowCb := func(_ webkit.WebView) {
		if wv.OnReadyToShow != nil {
			wv.OnReadyToShow()
		}
	}
	sigID := wv.inner.ConnectReadyToShow(&readyToShowCb)
	wv.signalIDs = append(wv.signalIDs, sigID)
}

func (wv *WebView) connectTitleSignal() {
	titleCb := func() {
		title := wv.inner.GetTitle()
		wv.mu.Lock()
		wv.title = title
		wv.mu.Unlock()

		if wv.OnTitleChanged != nil {
			wv.OnTitleChanged(title)
		}
	}
	sigID := gobject.SignalConnect(wv.inner.GoPointer(), "notify::title", glib.NewCallback(&titleCb))
	wv.signalIDs = append(wv.signalIDs, sigID)
}

func (wv *WebView) connectURISignal() {
	uriCb := func() {
		uri := wv.inner.GetUri()
		wv.mu.Lock()
		oldUri := wv.uri
		wv.uri = uri
		wv.mu.Unlock()

		if wv.OnURIChanged != nil && uri != oldUri {
			wv.OnURIChanged(uri)
		}
	}
	sigID := gobject.SignalConnect(wv.inner.GoPointer(), "notify::uri", glib.NewCallback(&uriCb))
	wv.signalIDs = append(wv.signalIDs, sigID)
}

func (wv *WebView) connectFaviconSignal() {
	faviconCb := func() {
		favicon := wv.inner.GetFavicon()
		if wv.OnFaviconChanged != nil {
			wv.OnFaviconChanged(favicon)
		}
	}
	sigID := gobject.SignalConnect(wv.inner.GoPointer(), "notify::favicon", glib.NewCallback(&faviconCb))
	wv.signalIDs = append(wv.signalIDs, sigID)
}

// progressThrottleInterval limits progress callbacks to ~60fps to reduce UI overhead.
const progressThrottleInterval = 16 * time.Millisecond

func (wv *WebView) connectProgressSignal() {
	progressCb := func() {
		progress := wv.inner.GetEstimatedLoadProgress()

		// Throttle progress updates to ~60fps (16ms interval)
		// Always allow 0.0 (start) and 1.0 (complete) through
		now := time.Now().UnixNano()
		last := wv.lastProgressUpdate.Load()
		if progress > 0.0 && progress < 1.0 {
			if now-last < int64(progressThrottleInterval) {
				return // Skip this update
			}
		}
		wv.lastProgressUpdate.Store(now)

		wv.mu.Lock()
		wv.progress = progress
		wv.mu.Unlock()

		if wv.OnProgressChanged != nil {
			wv.OnProgressChanged(progress)
		}
	}
	sigID := gobject.SignalConnect(wv.inner.GoPointer(), "notify::estimated-load-progress", glib.NewCallback(&progressCb))
	wv.signalIDs = append(wv.signalIDs, sigID)
}

func (wv *WebView) connectDecidePolicySignal() {
	decidePolicyCb := func(_ webkit.WebView, decisionPtr uintptr, decisionType webkit.PolicyDecisionType) bool {
		if decisionType != webkit.PolicyDecisionTypeNavigationActionValue {
			return false // Let WebKit handle
		}

		navDecision := webkit.NavigationPolicyDecisionNewFromInternalPtr(decisionPtr)
		if navDecision == nil {
			return false
		}

		navAction := navDecision.GetNavigationAction()
		if navAction == nil {
			return false
		}

		if navAction.GetNavigationType() != webkit.NavigationTypeLinkClickedValue {
			return false
		}

		mouseButton := navAction.GetMouseButton()
		modifiers := navAction.GetModifiers()
		isMiddleClick := mouseButton == 2
		isCtrlClick := mouseButton == 1 && (gdk.ModifierType(modifiers)&gdk.ControlMaskValue) != 0

		if !isMiddleClick && !isCtrlClick {
			return false
		}

		request := navAction.GetRequest()
		if request == nil {
			return false
		}

		linkURI := request.GetUri()
		if linkURI == "" {
			return false
		}

		wv.logger.Debug().
			Str("uri", linkURI).
			Uint("button", mouseButton).
			Bool("ctrl", isCtrlClick).
			Msg("middle-click/ctrl+click on link detected")

		if wv.OnLinkMiddleClick != nil {
			if wv.OnLinkMiddleClick(linkURI) {
				navDecision.Ignore()
				return true
			}
		}

		return false
	}
	sigID := wv.inner.ConnectDecidePolicy(&decidePolicyCb)
	wv.signalIDs = append(wv.signalIDs, sigID)
}

func (wv *WebView) connectEnterFullscreenSignal() {
	enterFullscreenCb := func(_ webkit.WebView) bool {
		wv.isFullscreen.Store(true)
		wv.logger.Debug().Uint64("id", uint64(wv.id)).Msg("enter fullscreen")
		if wv.OnEnterFullscreen != nil {
			return wv.OnEnterFullscreen()
		}
		return false // Allow fullscreen
	}
	sigID := wv.inner.ConnectEnterFullscreen(&enterFullscreenCb)
	wv.signalIDs = append(wv.signalIDs, sigID)
}

func (wv *WebView) connectLeaveFullscreenSignal() {
	leaveFullscreenCb := func(_ webkit.WebView) bool {
		wv.isFullscreen.Store(false)
		wv.logger.Debug().Uint64("id", uint64(wv.id)).Msg("leave fullscreen")
		if wv.OnLeaveFullscreen != nil {
			return wv.OnLeaveFullscreen()
		}
		return false // Allow leaving fullscreen
	}
	sigID := wv.inner.ConnectLeaveFullscreen(&leaveFullscreenCb)
	wv.signalIDs = append(wv.signalIDs, sigID)
}

func (wv *WebView) connectAudioStateSignal() {
	audioCb := func() {
		playing := wv.inner.IsPlayingAudio()
		oldState := wv.isPlayingAudio.Swap(playing)
		if oldState != playing {
			wv.logger.Debug().
				Uint64("id", uint64(wv.id)).
				Bool("playing", playing).
				Msg("audio state changed")
			if wv.OnAudioStateChanged != nil {
				wv.OnAudioStateChanged(playing)
			}
		}
	}
	sigID := gobject.SignalConnect(wv.inner.GoPointer(), "notify::is-playing-audio", glib.NewCallback(&audioCb))
	wv.signalIDs = append(wv.signalIDs, sigID)
}

func (wv *WebView) connectMouseTargetChangedSignal() {
	mouseTargetCb := func(_ webkit.WebView, hitTestPtr uintptr, _ uint) {
		if wv.OnLinkHover == nil {
			return
		}

		hitResult := webkit.HitTestResultNewFromInternalPtr(hitTestPtr)
		if hitResult == nil {
			wv.OnLinkHover("")
			return
		}

		var uri string
		switch {
		case hitResult.ContextIsLink():
			uri = hitResult.GetLinkUri()
		case hitResult.ContextIsImage():
			uri = hitResult.GetImageUri()
		case hitResult.ContextIsMedia():
			uri = hitResult.GetMediaUri()
		}

		wv.OnLinkHover(uri)
	}
	sigID := wv.inner.ConnectMouseTargetChanged(&mouseTargetCb)
	wv.signalIDs = append(wv.signalIDs, sigID)
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

// IsFullscreen returns true if the WebView is currently in fullscreen mode.
func (wv *WebView) IsFullscreen() bool {
	return wv.isFullscreen.Load()
}

// IsPlayingAudio returns true if the WebView is currently playing audio.
func (wv *WebView) IsPlayingAudio() bool {
	return wv.isPlayingAudio.Load()
}

// GetFindController returns the WebKit FindController wrapped in the port interface.
// The adapter is cached to prevent the Go wrapper from being garbage collected.
func (wv *WebView) GetFindController() port.FindController {
	if wv == nil || wv.inner == nil {
		return nil
	}

	wv.findControllerOnce.Do(func() {
		fc := wv.inner.GetFindController()
		if fc != nil {
			wv.findController = &findControllerAdapter{fc: fc}
		}
	})

	return wv.findController
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
	logging.FromContext(ctx).Debug().Int("webview_id", int(wv.id)).Msg("loading HTML content")
	return nil
}

// Reload reloads the current page.
func (wv *WebView) Reload(ctx context.Context) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.inner.Reload()
	logging.FromContext(ctx).Debug().Int("webview_id", int(wv.id)).Msg("reloading webview")
	return nil
}

// ReloadBypassCache reloads the current page, bypassing the cache.
func (wv *WebView) ReloadBypassCache(ctx context.Context) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.inner.ReloadBypassCache()
	logging.FromContext(ctx).Debug().Int("webview_id", int(wv.id)).Msg("reloading webview bypassing cache")
	return nil
}

// Stop stops the current load operation.
func (wv *WebView) Stop(ctx context.Context) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.inner.StopLoading()
	logging.FromContext(ctx).Debug().Int("webview_id", int(wv.id)).Msg("stopping webview load")
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
	logging.FromContext(ctx).Debug().Int("webview_id", int(wv.id)).Msg("webview go back")
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
	logging.FromContext(ctx).Debug().Int("webview_id", int(wv.id)).Msg("webview go forward")
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

// Favicon returns the current page favicon as a GdkTexture.
// Returns nil if no favicon is available.
func (wv *WebView) Favicon() *gdk.Texture {
	if wv.destroyed.Load() {
		return nil
	}
	return wv.inner.GetFavicon()
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
	logging.FromContext(ctx).Debug().Float64("factor", level).Int("webview_id", int(wv.id)).Msg("set webview zoom level")
	return nil
}

// GetZoomLevel returns the current zoom level.
func (wv *WebView) GetZoomLevel() float64 {
	if wv.destroyed.Load() {
		return 1.0
	}
	return wv.inner.GetZoomLevel()
}

// SetBackgroundColor sets the WebView background color.
// This color is shown before content is painted, eliminating white flash.
// Values are in range 0.0-1.0 for red, green, blue, alpha.
func (wv *WebView) SetBackgroundColor(r, g, b, a float32) {
	if wv.destroyed.Load() {
		return
	}
	rgba := &gdk.RGBA{
		Red:   r,
		Green: g,
		Blue:  b,
		Alpha: a,
	}
	wv.inner.SetBackgroundColor(rgba)
}

// State returns the current WebView state as a snapshot.
func (wv *WebView) State() port.WebViewState {
	return port.WebViewState{
		URI:       wv.uri,
		Title:     wv.title,
		IsLoading: wv.isLoading,
		Progress:  wv.progress,
		CanGoBack: wv.canGoBack,
		CanGoFwd:  wv.canGoFwd,
		ZoomLevel: wv.GetZoomLevel(),
	}
}

// SetCallbacks registers callback handlers for WebView events.
// Pass nil to clear all callbacks.
func (wv *WebView) SetCallbacks(callbacks *port.WebViewCallbacks) {
	if callbacks == nil {
		wv.OnLoadChanged = nil
		wv.OnTitleChanged = nil
		wv.OnURIChanged = nil
		wv.OnProgressChanged = nil
		wv.OnFaviconChanged = nil
		wv.OnClose = nil
		wv.OnCreate = nil
		return
	}

	// Map port callbacks to webkit callbacks
	if callbacks.OnLoadChanged != nil {
		wv.OnLoadChanged = func(e LoadEvent) {
			callbacks.OnLoadChanged(port.LoadEvent(e))
		}
	}
	wv.OnTitleChanged = callbacks.OnTitleChanged
	wv.OnURIChanged = callbacks.OnURIChanged
	wv.OnProgressChanged = callbacks.OnProgressChanged
	if callbacks.OnFaviconChanged != nil {
		wv.OnFaviconChanged = func(texture *gdk.Texture) {
			callbacks.OnFaviconChanged(texture)
		}
	}
	wv.OnClose = callbacks.OnClose
	if callbacks.OnCreate != nil {
		wv.OnCreate = func(req PopupRequest) *WebView {
			portReq := port.PopupRequest{
				TargetURI:     req.TargetURI,
				FrameName:     req.FrameName,
				IsUserGesture: req.IsUserGesture,
				ParentViewID:  req.ParentID,
			}
			result := callbacks.OnCreate(portReq)
			if result == nil {
				return nil
			}
			// The callback returns port.WebView, we need to convert back
			// This assumes the returned WebView is actually a *webkit.WebView
			if wkView, ok := result.(*WebView); ok {
				return wkView
			}
			return nil
		}
	}
	wv.OnLinkHover = callbacks.OnLinkHover
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

// Close triggers the close callback as if window.close() was called.
// This is used for programmatic popup closure (e.g., OAuth auto-close).
func (wv *WebView) Close() {
	if wv.destroyed.Load() {
		return
	}
	if wv.OnClose != nil {
		wv.OnClose()
	}
}

// DisconnectSignals disconnects all GLib signal handlers from the WebView.
// This must be called before releasing the WebView to the pool or destroying it
// to prevent callbacks from firing on freed/pooled WebViews.
func (wv *WebView) DisconnectSignals() {
	if wv.inner == nil {
		return
	}

	obj := gobject.ObjectNewFromInternalPtr(wv.inner.GoPointer())
	for _, sigID := range wv.signalIDs {
		gobject.SignalHandlerDisconnect(obj, sigID)
	}
	wv.signalIDs = wv.signalIDs[:0] // Clear the slice

	wv.logger.Debug().Uint64("id", uint64(wv.id)).Msg("signals disconnected")
}

// Destroy cleans up the WebView resources and terminates the web process.
// This must be called when a WebView is no longer needed to free GPU resources,
// VA-API decoder contexts, and DMA-BUF buffers held by the web process.
func (wv *WebView) Destroy() {
	if wv.destroyed.Swap(true) {
		return // Already destroyed
	}

	// 1. Disconnect all signal handlers first to prevent callbacks during cleanup
	wv.DisconnectSignals()

	// 2. Clear all callbacks to prevent use-after-free
	wv.OnLoadChanged = nil
	wv.OnTitleChanged = nil
	wv.OnURIChanged = nil
	wv.OnProgressChanged = nil
	wv.OnFaviconChanged = nil
	wv.OnClose = nil
	wv.OnCreate = nil
	wv.OnReadyToShow = nil
	wv.OnLinkMiddleClick = nil
	wv.OnEnterFullscreen = nil
	wv.OnLeaveFullscreen = nil
	wv.OnAudioStateChanged = nil
	wv.OnLinkHover = nil

	// 3. Clear async callback references
	wv.mu.Lock()
	wv.asyncCallbacks = nil
	wv.mu.Unlock()

	// 4. Unparent from GTK hierarchy (must happen before process termination)
	// This releases the widget from any parent container.
	if wv.inner != nil {
		wv.inner.Unparent()
	}

	// 5. Terminate the web process to free GPU resources (VA-API, DMA-BUF, GL contexts)
	// This is critical to prevent zombie processes that hold video decoder resources.
	// IMPORTANT: Skip for related views (popups) - they share the web process with their
	// parent. Terminating the shared process would kill the parent WebView too!
	if !wv.isRelated && wv.inner != nil {
		wv.inner.TerminateWebProcess()
	}

	// 6. Unregister from global registry
	globalRegistry.unregister(wv.id)

	// 7. Clear internal references to allow GC
	wv.inner = nil
	wv.ucm = nil
	wv.findController = nil

	if wv.isRelated {
		wv.logger.Debug().Uint64("id", uint64(wv.id)).Msg("related webview destroyed (process shared with parent)")
	} else {
		wv.logger.Debug().Uint64("id", uint64(wv.id)).Msg("webview destroyed and web process terminated")
	}
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
		log.Debug().Msg("AttachFrontend: injecting scripts")
		injector.InjectScripts(ctx, wv.ucm, wv.id)
	}

	log.Debug().Msg("frontend assets attached to webview")
	return nil
}
