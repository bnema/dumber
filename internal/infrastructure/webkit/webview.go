package webkit

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	urlutil "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
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
	byUCM   map[uintptr]WebViewID
	counter atomic.Uint64
	mu      sync.RWMutex
}

var globalRegistry = &webViewRegistry{
	views: make(map[WebViewID]*WebView),
	byUCM: make(map[uintptr]WebViewID),
}

func (r *webViewRegistry) register(wv *WebView) WebViewID {
	id := WebViewID(r.counter.Add(1))
	r.mu.Lock()
	r.views[id] = wv
	if wv != nil && wv.ucm != nil {
		r.byUCM[wv.ucm.GoPointer()] = id
	}
	r.mu.Unlock()
	return id
}

func (r *webViewRegistry) unregister(id WebViewID) {
	r.mu.Lock()
	wv := r.views[id]
	delete(r.views, id)
	if wv != nil && wv.ucm != nil {
		delete(r.byUCM, wv.ucm.GoPointer())
	}
	r.mu.Unlock()
}

// Lookup returns a WebView by ID, or nil if not found.
func (r *webViewRegistry) Lookup(id WebViewID) *WebView {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.views[id]
}

func (r *webViewRegistry) LookupByUCMPointer(ptr uintptr) *WebView {
	if ptr == 0 {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byUCM[ptr]
	if !ok {
		return nil
	}
	return r.views[id]
}

// LookupWebView returns a WebView by ID from the global registry.
func LookupWebView(id WebViewID) *WebView {
	return globalRegistry.Lookup(id)
}

// LookupWebViewByUCMPointer returns a WebView by its UserContentManager pointer.
func LookupWebViewByUCMPointer(ptr uintptr) *WebView {
	return globalRegistry.LookupByUCMPointer(ptr)
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
	signalIDs []uintptr

	// Callbacks (set by UI layer)
	OnLoadChanged          func(LoadEvent)
	OnTitleChanged         func(string)
	OnURIChanged           func(string)
	OnProgressChanged      func(float64)
	OnFaviconChanged       func(*gdk.Texture) // Called when page favicon changes
	OnClose                func()
	OnCreate               func(PopupRequest) *WebView // Return new WebView or nil to block popup
	OnReadyToShow          func()                      // Called when popup is ready to display
	OnLinkMiddleClick      func(uri string) bool       // Return true if handled (blocks navigation)
	OnEnterFullscreen      func() bool                 // Return true to prevent fullscreen
	OnLeaveFullscreen      func() bool                 // Return true to prevent leaving fullscreen
	OnAudioStateChanged    func(playing bool)          // Called when audio playback starts/stops
	OnLinkHover            func(uri string)            // Called when hovering over a link/image/media (empty string when leaving)
	OnWebProcessTerminated func(reason webkit.WebProcessTerminationReason, reasonLabel string, uri string)

	// PermissionRequest is called when a site requests permission (mic, camera, screen sharing).
	// Return true to indicate the request was handled. Call allow()/deny() to respond.
	// The permission types are determined from the request object.
	OnPermissionRequest func(origin string, permTypes []string, allow, deny func()) bool

	logger zerolog.Logger
	mu     sync.RWMutex

	frontendAttached atomic.Bool
	navigationActive atomic.Bool

	// asyncCallbacks keeps references to async JS callbacks to prevent GC
	asyncCallbacks []interface{}

	// runJSErrorStats aggregates repeated non-fatal RunJavaScript errors by domain+signature.
	runJSErrorStats map[string]runJSErrorStat

	// findController is cached to prevent GC from collecting the Go wrapper
	findController     *findControllerAdapter
	findControllerOnce sync.Once

	backForwardList         *webkit.BackForwardList
	backForwardListSignalID uintptr
}

type runJSErrorStat struct {
	count   uint64
	lastLog time.Time
}

const (
	terminatePolicyAuto   = "auto"
	terminatePolicyAlways = "always"
	terminatePolicyNever  = "never"

	runJSNonFatalLogInterval = 30 * time.Second
	runJSAggregateLogEvery   = 20
	runJSUnknown             = "unknown"
)

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

func (a *findControllerAdapter) OnFoundText(callback func(matchCount uint)) uint {
	if a == nil || a.fc == nil || callback == nil {
		return 0
	}
	cb := func(_ webkit.FindController, matchCount uint) {
		callback(matchCount)
	}
	return a.fc.ConnectFoundText(&cb)
}

func (a *findControllerAdapter) OnFailedToFindText(callback func()) uint {
	if a == nil || a.fc == nil || callback == nil {
		return 0
	}
	cb := func(_ webkit.FindController) {
		callback()
	}
	return a.fc.ConnectFailedToFindText(&cb)
}

func (a *findControllerAdapter) OnCountedMatches(callback func(matchCount uint)) uint {
	if a == nil || a.fc == nil || callback == nil {
		return 0
	}
	cb := func(_ webkit.FindController, matchCount uint) {
		callback(matchCount)
	}
	return a.fc.ConnectCountedMatches(&cb)
}

func (a *findControllerAdapter) DisconnectSignal(id uint) {
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

	// Use options constructor to pass both WebContext and NetworkSession.
	// This ensures WebViews use our custom WebContext (with memory pressure settings)
	// and the persistent NetworkSession for cookie/storage.
	inner := webkit.NewWebViewWithOptions(&webkit.WebViewOptions{
		WebContext:     wkCtx.Context(),
		NetworkSession: wkCtx.NetworkSession(),
	})
	if inner == nil {
		return nil, fmt.Errorf("failed to create webkit webview with options")
	}

	// Set background color IMMEDIATELY after creation, before any other operations.
	// This prevents white flash by ensuring WebKit's renderer starts with correct color.
	if bgColor != nil {
		inner.SetBackgroundColor(bgColor)
	}

	wv := &WebView{
		inner:           inner,
		ucm:             inner.GetUserContentManager(),
		logger:          log.With().Str("component", "webview").Logger(),
		signalIDs:       make([]uintptr, 0, 4),
		runJSErrorStats: make(map[string]runJSErrorStat),
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

	log.Debug().
		Uint64("parent_ptr", uint64(parent.inner.GoPointer())).
		Msg("creating related webview with parent pointer")

	inner := webkit.NewWebViewWithRelatedView(parent.inner)
	if inner == nil {
		return nil, fmt.Errorf("failed to create related webkit webview")
	}

	log.Debug().
		Uint64("new_ptr", uint64(inner.GoPointer())).
		Uint64("new_widget_ptr", uint64(inner.Widget.GoPointer())).
		Msg("related webview created, checking pointers")

	wv := &WebView{
		inner:           inner,
		isRelated:       true, // Shares web process with parent - must not terminate process on destroy
		ucm:             inner.GetUserContentManager(),
		logger:          log.With().Str("component", "webview-popup").Logger(),
		signalIDs:       make([]uintptr, 0, 6),
		runJSErrorStats: make(map[string]runJSErrorStat),
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
	wv.connectBackForwardListChangedSignal()
	wv.connectWebProcessTerminatedSignal()
	wv.connectPermissionRequestSignal()
}

func (wv *WebView) connectLoadChangedSignal() {
	loadChangedCb := func(inner webkit.WebView, event webkit.LoadEvent) {
		uri := inner.GetUri()
		title := inner.GetTitle()

		wv.mu.Lock()
		wv.uri = uri
		wv.title = title
		// Note: canGoBack/canGoFwd are updated via back-forward-list::changed signal
		// which fires for both traditional navigation and SPA history.pushState()
		wv.progress = inner.GetEstimatedLoadProgress()

		switch event {
		case webkit.LoadStartedValue:
			wv.navigationActive.Store(true)
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
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
}

func (wv *WebView) connectCloseSignal() {
	closeCb := func(_ webkit.WebView) {
		if wv.OnClose != nil {
			wv.OnClose()
		}
	}
	sigID := wv.inner.ConnectClose(&closeCb)
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
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

		wv.logger.Debug().Msg("create signal: invoking OnCreate handler")
		newWV := wv.OnCreate(popupReq)
		wv.logger.Debug().Bool("nil", newWV == nil).Msg("create signal: OnCreate returned")
		if newWV == nil {
			return gtk.Widget{} // Block popup
		}

		wv.logger.Debug().
			Uint64("parent_id", uint64(wv.id)).
			Uint64("popup_id", uint64(newWV.id)).
			Msg("create signal: returning webview widget to WebKit")

		return newWV.inner.Widget
	}
	sigID := wv.inner.ConnectCreate(&createCb)
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
}

func (wv *WebView) connectReadyToShowSignal() {
	readyToShowCb := func(_ webkit.WebView) {
		if wv.OnReadyToShow != nil {
			wv.OnReadyToShow()
		}
	}
	sigID := wv.inner.ConnectReadyToShow(&readyToShowCb)
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
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
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
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
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
}

func (wv *WebView) connectFaviconSignal() {
	faviconCb := func() {
		favicon := wv.inner.GetFavicon()
		if wv.OnFaviconChanged != nil {
			wv.OnFaviconChanged(favicon)
		}
	}
	sigID := gobject.SignalConnect(wv.inner.GoPointer(), "notify::favicon", glib.NewCallback(&faviconCb))
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
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
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
}

func (wv *WebView) connectDecidePolicySignal() {
	decidePolicyCb := func(_ webkit.WebView, decisionPtr uintptr, decisionType webkit.PolicyDecisionType) bool {
		switch decisionType {
		case webkit.PolicyDecisionTypeResponseValue:
			return wv.handleResponsePolicyDecision(decisionPtr)
		case webkit.PolicyDecisionTypeNavigationActionValue, webkit.PolicyDecisionTypeNewWindowActionValue:
			// Both navigation and new window actions use NavigationPolicyDecision
			return wv.handleNavigationPolicyDecision(decisionPtr)
		default:
			return false
		}
	}
	sigID := wv.inner.ConnectDecidePolicy(&decidePolicyCb)
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
}

// handleResponsePolicyDecision handles response policy decisions (e.g., forcing downloads).
func (wv *WebView) handleResponsePolicyDecision(decisionPtr uintptr) bool {
	responseDecision := webkit.ResponsePolicyDecisionNewFromInternalPtr(decisionPtr)
	if responseDecision == nil {
		return false
	}

	if !responseDecision.IsMainFrameMainResource() {
		return false
	}

	if !shouldForceDownload(responseDecision) {
		return false
	}

	policyDecision := webkit.PolicyDecisionNewFromInternalPtr(decisionPtr)
	if policyDecision == nil {
		return false
	}

	response := responseDecision.GetResponse()
	mimeType := ""
	uri := ""
	if response != nil {
		mimeType = response.GetMimeType()
		uri = response.GetUri()
	}

	wv.logger.Debug().
		Str("mime_type", mimeType).
		Str("uri", uri).
		Msg("forcing download for response")

	// Start a fresh download instead of converting the in-progress response.
	// Using policyDecision.Download() can fail due to race conditions where
	// the network connection is released before the download can use it.
	// By ignoring the policy and starting a new download, we avoid this issue.
	// We defer the download to the next idle cycle to let WebKit clean up
	// the ignored navigation first.
	if uri != "" {
		policyDecision.Ignore()
		downloadURI := uri
		inner := wv.inner
		cb := glib.SourceFunc(func(_ uintptr) bool {
			inner.DownloadUri(downloadURI)
			return false // Don't repeat
		})
		// Store callback reference to prevent GC before GTK calls it
		wv.mu.Lock()
		wv.asyncCallbacks = append(wv.asyncCallbacks, &cb)
		wv.mu.Unlock()
		glib.IdleAdd(&cb, 0)
		return true
	}

	// Fallback to old behavior if URI is not available
	policyDecision.Download()
	return true
}

// handleNavigationPolicyDecision handles navigation policy decisions (e.g., middle-click, external schemes).
func (wv *WebView) handleNavigationPolicyDecision(decisionPtr uintptr) bool {
	navDecision := webkit.NavigationPolicyDecisionNewFromInternalPtr(decisionPtr)
	if navDecision == nil {
		return false
	}

	navAction := navDecision.GetNavigationAction()
	if navAction == nil {
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

	// Debug logging to trace navigation decisions
	wv.logger.Debug().
		Str("uri", linkURI).
		Int("nav_type", int(navAction.GetNavigationType())).
		Bool("user_gesture", navAction.IsUserGesture()).
		Msg("navigation policy decision")

	// Check for external URL schemes (e.g., vscode://, vscode-insiders://, spotify://)
	// These need to be launched via xdg-open rather than handled by WebKit
	// Only launch for user-initiated actions to prevent automatic redirects
	// from silently opening external applications
	if urlutil.IsExternalScheme(linkURI) {
		if !navAction.IsUserGesture() {
			// For non-user-initiated redirects, ignore silently
			navDecision.Ignore()
			return true
		}

		wv.logger.Info().
			Str("uri", linkURI).
			Msg("launching external URL scheme via xdg-open")

		// Launch asynchronously to avoid blocking WebKit
		go desktop.LaunchExternalURL(linkURI)

		// Ignore the navigation to prevent WebKit from showing an error
		navDecision.Ignore()

		// Go back to the previous page to avoid showing WebKit's error page.
		// This handles OAuth flows where after successful auth, the callback
		// redirects to a custom scheme (vscode://, vscode-insiders://, etc.)
		// Schedule on idle to let WebKit process the ignored decision first
		cb := glib.SourceFunc(func(_ uintptr) bool {
			if wv.inner != nil && !wv.destroyed.Load() && wv.inner.CanGoBack() {
				wv.inner.GoBack()
			}
			return false // Don't repeat
		})
		wv.mu.Lock()
		wv.asyncCallbacks = append(wv.asyncCallbacks, &cb)
		wv.mu.Unlock()
		glib.IdleAdd(&cb, 0)

		return true
	}

	// Only handle link clicks for middle-click/ctrl-click (open in new tab)
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

func shouldForceDownload(responseDecision *webkit.ResponsePolicyDecision) bool {
	if responseDecision == nil {
		return false
	}

	response := responseDecision.GetResponse()
	if response == nil {
		return false
	}

	mimeType := strings.ToLower(response.GetMimeType())
	if strings.HasPrefix(mimeType, "application/pdf") {
		return true
	}

	uri := response.GetUri()
	if uri == "" {
		return false
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		return strings.Contains(strings.ToLower(uri), ".pdf")
	}

	return strings.HasSuffix(strings.ToLower(parsed.Path), ".pdf")
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
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
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
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
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
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
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
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
}

func (wv *WebView) connectBackForwardListChangedSignal() {
	backForwardList := wv.inner.GetBackForwardList()
	if backForwardList == nil {
		wv.logger.Warn().Msg("failed to get back-forward-list")
		return
	}

	// The "changed" signal is emitted when the back-forward list changes,
	// including for SPA navigation via history.pushState/replaceState.
	// Parameters: (item_added, items_removed) - we ignore them and just refresh state.
	changedCb := func(_ webkit.BackForwardList, _ uintptr, _ uintptr) {
		canBack := wv.inner.CanGoBack()
		canFwd := wv.inner.CanGoForward()
		wv.logger.Debug().
			Bool("can_go_back", canBack).
			Bool("can_go_forward", canFwd).
			Uint("list_length", backForwardList.GetLength()).
			Msg("back-forward-list changed")
		wv.mu.Lock()
		wv.canGoBack = canBack
		wv.canGoFwd = canFwd
		wv.mu.Unlock()
	}
	sigID := backForwardList.ConnectChanged(&changedCb)
	wv.backForwardList = backForwardList
	wv.backForwardListSignalID = uintptr(sigID)
}

func webProcessTerminationReasonString(reason webkit.WebProcessTerminationReason) string {
	switch reason {
	case webkit.WebProcessCrashedValue:
		return "crashed"
	case webkit.WebProcessExceededMemoryLimitValue:
		return "exceeded_memory"
	case webkit.WebProcessTerminatedByApiValue:
		return "terminated_by_api"
	default:
		return "unknown"
	}
}

func mapWebProcessTerminationReason(reason webkit.WebProcessTerminationReason) port.WebProcessTerminationReason {
	switch reason {
	case webkit.WebProcessCrashedValue:
		return port.WebProcessTerminationCrashed
	case webkit.WebProcessExceededMemoryLimitValue:
		return port.WebProcessTerminationExceededMemory
	case webkit.WebProcessTerminatedByApiValue:
		return port.WebProcessTerminationByAPI
	default:
		return port.WebProcessTerminationUnknown
	}
}

func (wv *WebView) connectWebProcessTerminatedSignal() {
	terminatedCb := func(_ webkit.WebView, reason webkit.WebProcessTerminationReason) {
		uri := wv.URI()
		reasonLabel := webProcessTerminationReasonString(reason)
		wv.logger.Warn().
			Uint64("id", uint64(wv.id)).
			Str("reason", reasonLabel).
			Int("reason_code", int(reason)).
			Str("uri", uri).
			Msg("web process terminated")

		if wv.OnWebProcessTerminated != nil {
			wv.OnWebProcessTerminated(reason, reasonLabel, uri)
		}
	}
	sigID := wv.inner.ConnectWebProcessTerminated(&terminatedCb)
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
}

// connectPermissionRequestSignal sets up the permission-request signal handler.
// This is emitted when a site calls getUserMedia() or getDisplayMedia().
func (wv *WebView) connectPermissionRequestSignal() {
	permissionCb := func(_ webkit.WebView, requestPtr uintptr) bool {
		if wv.OnPermissionRequest == nil {
			return false // Not handled, WebKit will deny by default
		}

		// Extract and normalize origin from current URI
		uri := wv.URI()
		if uri == "" {
			wv.logger.Debug().Msg("permission request with empty origin, denying")
			return false
		}
		origin, err := urlutil.ExtractOrigin(uri)
		if err != nil {
			wv.logger.Debug().Str("uri", uri).Err(err).Msg("permission request: failed to extract origin, denying")
			return false
		}

		// Determine permission types from the request
		permTypes := wv.determinePermissionTypes(requestPtr)
		if len(permTypes) == 0 {
			wv.logger.Warn().Msg("permission request with unknown type, denying")
			return false
		}

		// Ref the request object to prevent use-after-free
		// The request may outlive the signal handler if we show a dialog
		requestObj := gobject.ObjectNewFromInternalPtr(requestPtr)
		if requestObj == nil {
			wv.logger.Warn().Msg("permission request: failed to wrap request object")
			return false
		}
		requestObj.Ref()

		// Create allow/deny callbacks that wrap the WebKit permission request
		allowCalled := false
		denyCalled := false

		allow := func() {
			if allowCalled || denyCalled {
				return
			}
			allowCalled = true
			wv.allowPermissionRequest(requestPtr)
			requestObj.Unref()
		}

		deny := func() {
			if allowCalled || denyCalled {
				return
			}
			denyCalled = true
			wv.denyPermissionRequest(requestPtr)
			requestObj.Unref()
		}

		// Call the handler
		return wv.OnPermissionRequest(origin, permTypes, allow, deny)
	}

	sigID := wv.inner.ConnectPermissionRequest(&permissionCb)
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
}

// determinePermissionTypes extracts permission types from a WebKit permission request.
// This uses type checking to identify UserMediaPermissionRequest and its specific types.
// We use GObject property accessors as the primary method because the C function wrappers
// can hit a purego bug where bool return values are misread.
func (wv *WebView) determinePermissionTypes(requestPtr uintptr) []string {
	var types []string

	// Try to cast to UserMediaPermissionRequest
	userMediaReq := webkit.UserMediaPermissionRequestNewFromInternalPtr(requestPtr)
	if userMediaReq != nil {
		// Use GObject property accessors — more reliable than the C function wrappers
		// which can hit the purego bool return value bug.
		isAudio := userMediaReq.GetPropertyIsForAudioDevice()
		isVideo := userMediaReq.GetPropertyIsForVideoDevice()

		wv.logger.Debug().
			Bool("is_audio", isAudio).
			Bool("is_video", isVideo).
			Msg("permission request type detection")

		if isAudio {
			types = append(types, "microphone")
		}
		if isVideo {
			// Check if this is display capture or camera
			if webkit.UserMediaPermissionIsForDisplayDevice(userMediaReq) {
				types = append(types, "display")
			} else {
				types = append(types, "camera")
			}
		}
		return types
	}

	// Check for device enumeration request
	deviceInfoReq := webkit.DeviceInfoPermissionRequestNewFromInternalPtr(requestPtr)
	if deviceInfoReq != nil {
		return []string{"device_info"}
	}

	// Unknown permission type - could be clipboard, notifications, geolocation, etc.
	// For now, return empty to trigger denial. Future phases will add these types.
	return nil
}

// allowPermissionRequest calls Allow() on the WebKit permission request.
func (wv *WebView) allowPermissionRequest(requestPtr uintptr) {
	// Try UserMediaPermissionRequest first
	userMediaReq := webkit.UserMediaPermissionRequestNewFromInternalPtr(requestPtr)
	if userMediaReq != nil {
		userMediaReq.Allow()
		wv.logger.Debug().Msg("permission request allowed")
		return
	}

	// Try DeviceInfoPermissionRequest
	deviceInfoReq := webkit.DeviceInfoPermissionRequestNewFromInternalPtr(requestPtr)
	if deviceInfoReq != nil {
		deviceInfoReq.Allow()
		wv.logger.Debug().Msg("permission request allowed")
		return
	}
}

// denyPermissionRequest calls Deny() on the WebKit permission request.
func (wv *WebView) denyPermissionRequest(requestPtr uintptr) {
	// Try UserMediaPermissionRequest first
	userMediaReq := webkit.UserMediaPermissionRequestNewFromInternalPtr(requestPtr)
	if userMediaReq != nil {
		userMediaReq.Deny()
		wv.logger.Debug().Msg("permission request denied")
		return
	}

	// Try DeviceInfoPermissionRequest
	deviceInfoReq := webkit.DeviceInfoPermissionRequestNewFromInternalPtr(requestPtr)
	if deviceInfoReq != nil {
		deviceInfoReq.Deny()
		wv.logger.Debug().Msg("permission request denied")
		return
	}
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
	wv.navigationActive.Store(true)
	wv.inner.LoadUri(uri)
	logging.FromContext(ctx).Debug().Str("uri", uri).Msg("loading URI")
	return nil
}

// LoadHTML loads HTML content with an optional base URI.
func (wv *WebView) LoadHTML(ctx context.Context, content, baseURI string) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	wv.navigationActive.Store(true)
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
// Uses WebKit's native history navigation.
func (wv *WebView) GoBack(ctx context.Context) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}

	logging.FromContext(ctx).Debug().
		Int("webview_id", int(wv.id)).
		Bool("can_go_back", wv.inner.CanGoBack()).
		Str("current_uri", wv.inner.GetUri()).
		Msg("webview go back")

	wv.inner.GoBack()

	// Note: We don't dispatch popstate manually anymore.
	// WebKit's go_back() should handle SPA navigation correctly.
	// If issues persist, the problem is likely elsewhere.

	return nil
}

// GoForward navigates forward in history.
func (wv *WebView) GoForward(ctx context.Context) error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}

	logging.FromContext(ctx).Debug().
		Int("webview_id", int(wv.id)).
		Bool("can_go_forward", wv.inner.CanGoForward()).
		Str("current_uri", wv.inner.GetUri()).
		Msg("webview go forward")

	wv.inner.GoForward()

	return nil
}

// GoBackDirect calls WebKit's go_back directly without any wrappers.
// This is intended for use from gesture handlers where preserving user
// gesture context is critical for SPA popstate handling.
// Matches Epiphany: webkit_web_view_go_back(web_view) directly in gesture callback.
func (wv *WebView) GoBackDirect() {
	if wv.destroyed.Load() {
		return
	}
	wv.inner.GoBack()
}

// GoForwardDirect calls WebKit's go_forward directly without any wrappers.
// This is intended for use from gesture handlers where preserving user
// gesture context is critical for SPA popstate handling.
func (wv *WebView) GoForwardDirect() {
	if wv.destroyed.Load() {
		return
	}
	wv.inner.GoForward()
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

// ResetBackgroundToDefault sets WebView background to white (browser default).
// Used for external pages to prevent dark background from bleeding through.
func (wv *WebView) ResetBackgroundToDefault() {
	wv.SetBackgroundColor(1.0, 1.0, 1.0, 1.0)
}

// Show makes the WebView widget visible.
// This should be called after the WebView is ready to be displayed.
func (wv *WebView) Show() {
	if wv.destroyed.Load() {
		return
	}
	wv.inner.SetVisible(true)
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
		wv.OnLinkHover = nil
		wv.OnWebProcessTerminated = nil
		wv.OnPermissionRequest = nil
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
	if callbacks.OnWebProcessTerminated != nil {
		wv.OnWebProcessTerminated = func(reason webkit.WebProcessTerminationReason, reasonLabel string, uri string) {
			callbacks.OnWebProcessTerminated(mapWebProcessTerminationReason(reason), reasonLabel, uri)
		}
	} else {
		wv.OnWebProcessTerminated = nil
	}
	wv.OnPermissionRequest = callbacks.OnPermissionRequest
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

// Print opens the print dialog for the current page.
func (wv *WebView) Print() error {
	if wv.destroyed.Load() {
		return fmt.Errorf("webview %d is destroyed", wv.id)
	}
	printOp := webkit.NewPrintOperation(wv.inner)
	if printOp == nil {
		return fmt.Errorf("failed to create print operation for webview %d", wv.id)
	}
	// RunDialog with nil parent window - GTK will use the widget's toplevel
	printOp.RunDialog(nil)
	wv.logger.Debug().Uint64("id", uint64(wv.id)).Msg("print dialog opened")
	return nil
}

// IsDestroyed returns true if the WebView has been destroyed.
func (wv *WebView) IsDestroyed() bool {
	return wv.destroyed.Load()
}

// IsRelated returns true when this WebView shares process/session with a parent popup.
func (wv *WebView) IsRelated() bool {
	return wv.isRelated
}

// HasNavigationActivity reports whether this WebView has been used for content navigation.
func (wv *WebView) HasNavigationActivity() bool {
	return wv.navigationActive.Load()
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
		gobject.SignalHandlerDisconnect(obj, uint(sigID))
	}
	wv.signalIDs = wv.signalIDs[:0] // Clear the slice

	if wv.backForwardList != nil && wv.backForwardListSignalID != 0 {
		bfObj := gobject.ObjectNewFromInternalPtr(wv.backForwardList.GoPointer())
		gobject.SignalHandlerDisconnect(bfObj, uint(wv.backForwardListSignalID))
		wv.backForwardListSignalID = 0
		wv.backForwardList = nil
	}

	wv.logger.Debug().Uint64("id", uint64(wv.id)).Msg("signals disconnected")
}

// Destroy cleans up the WebView resources and terminates the web process.
// This must be called when a WebView is no longer needed to free GPU resources,
// VA-API decoder contexts, and DMA-BUF buffers held by the web process.
func (wv *WebView) Destroy() {
	wv.DestroyWithPolicy("")
}

// DestroyWithPolicy cleans up the WebView resources with explicit process policy.
// Valid policies: auto, always, never.
func (wv *WebView) DestroyWithPolicy(policy string) {
	if wv.destroyed.Swap(true) {
		return // Already destroyed
	}
	policy = resolveTerminatePolicy(policy)

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
	wv.OnWebProcessTerminated = nil
	wv.OnPermissionRequest = nil

	// 3. Clear async callback references
	wv.mu.Lock()
	wv.asyncCallbacks = nil
	wv.mu.Unlock()

	// 4. Unparent from GTK hierarchy (must happen before process termination)
	// This releases the widget from any parent container.
	if wv.inner != nil {
		wv.inner.Unparent()
	}

	// 5. Optionally terminate web process (skip for related popup views sharing parent process).
	terminate := shouldTerminateWebProcess(policy, wv.isRelated, wv.navigationActive.Load())
	if terminate && wv.inner != nil {
		wv.inner.TerminateWebProcess()
	}

	// 6. Unregister from global registry
	globalRegistry.unregister(wv.id)

	// 7. Clear internal references to allow GC
	wv.inner = nil
	wv.ucm = nil
	wv.findController = nil

	wv.logger.Debug().
		Uint64("id", uint64(wv.id)).
		Bool("related", wv.isRelated).
		Bool("navigation_active", wv.navigationActive.Load()).
		Bool("terminated_process", terminate).
		Str("terminate_policy", policy).
		Msg("webview destroyed")
}

// ResetForPoolReuse sanitizes callbacks/state so the WebView can be safely reused.
func (wv *WebView) ResetForPoolReuse() {
	if wv == nil || wv.destroyed.Load() {
		return
	}

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
	wv.OnWebProcessTerminated = nil

	wv.mu.Lock()
	wv.uri = ""
	wv.title = ""
	wv.progress = 0
	wv.canGoBack = false
	wv.canGoFwd = false
	wv.isLoading = false
	wv.asyncCallbacks = nil
	wv.runJSErrorStats = make(map[string]runJSErrorStat)
	wv.lastProgressUpdate.Store(0)
	wv.mu.Unlock()

	wv.isFullscreen.Store(false)
	wv.isPlayingAudio.Store(false)
	wv.navigationActive.Store(false)

	if wv.inner != nil {
		wv.inner.StopLoading()
		wv.inner.SetVisible(false)
	}
}

func resolveTerminatePolicy(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(os.Getenv("DUMBER_WEBVIEW_TERMINATE_POLICY")))
	}
	switch value {
	case terminatePolicyAlways:
		return terminatePolicyAlways
	case terminatePolicyNever:
		return terminatePolicyNever
	case terminatePolicyAuto, "":
		return terminatePolicyAuto
	default:
		return terminatePolicyAuto
	}
}

func shouldTerminateWebProcess(policy string, isRelated, navigationActive bool) bool {
	if isRelated {
		return false
	}
	switch resolveTerminatePolicy(policy) {
	case terminatePolicyNever:
		return false
	case terminatePolicyAlways:
		return true
	case terminatePolicyAuto:
		return navigationActive
	default:
		return navigationActive
	}
}

func classifyRunJSEvaluateError(err error) (nonFatal bool, signature string) {
	// Defensive guard: callers should only pass non-nil errors, but tolerate nil safely.
	if err == nil {
		return true, ""
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	signature = "evaluate_error:" + normalizeRunJSErrorSignature(msg)
	switch {
	case strings.Contains(msg, "canceled"),
		strings.Contains(msg, "cancel"),
		strings.Contains(msg, "javascript execution context"),
		strings.Contains(msg, "execution context was destroyed"),
		strings.Contains(msg, "document unloaded"):
		return true, signature
	default:
		return false, signature
	}
}

func normalizeRunJSErrorSignature(msg string) string {
	msg = strings.ToLower(strings.TrimSpace(msg))
	if msg == "" {
		return "empty"
	}
	fields := strings.Fields(msg)
	if len(fields) == 0 {
		return "empty"
	}
	return strings.Join(fields, " ")
}

func (wv *WebView) runJSDomain() string {
	wv.mu.RLock()
	currentURI := strings.TrimSpace(wv.uri)
	wv.mu.RUnlock()
	if currentURI == "" {
		return runJSUnknown
	}
	parsed, err := url.Parse(currentURI)
	if err != nil {
		return runJSUnknown
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return runJSUnknown
	}
	return host
}

func (wv *WebView) shouldLogRunJSError(domain, signature string, nonFatal bool, now time.Time) (bool, uint64) {
	if wv == nil {
		return true, 1
	}
	if now.IsZero() {
		now = time.Now()
	}
	if domain == "" {
		domain = runJSUnknown
	}
	if signature == "" {
		signature = runJSUnknown
	}
	key := domain + "|" + signature

	wv.mu.Lock()
	if wv.runJSErrorStats == nil {
		wv.runJSErrorStats = make(map[string]runJSErrorStat)
	}
	stat := wv.runJSErrorStats[key]
	stat.count++
	count := stat.count

	shouldLog := true
	if nonFatal {
		shouldLog = count == 1 || count%runJSAggregateLogEvery == 0 || now.Sub(stat.lastLog) >= runJSNonFatalLogInterval
	}
	if shouldLog {
		stat.lastLog = now
	}
	wv.runJSErrorStats[key] = stat
	wv.mu.Unlock()

	return shouldLog, count
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
		if wv.destroyed.Load() {
			return
		}
		wv.mu.RLock()
		inner := wv.inner
		wv.mu.RUnlock()
		if inner == nil {
			return
		}

		domain := wv.runJSDomain()
		if resPtr == 0 {
			signature := "nil_async_result"
			shouldLog, count := wv.shouldLogRunJSError(domain, signature, true, time.Now())
			if shouldLog {
				log.Debug().
					Uint64("webview_id", uint64(wv.id)).
					Str("domain", domain).
					Str("signature", signature).
					Uint64("repeat_count", count).
					Msg("RunJavaScript: non-fatal nil async result")
			}
			return
		}

		res := &gio.AsyncResultBase{Ptr: resPtr}
		value, err := inner.EvaluateJavascriptFinish(res)
		if err != nil {
			nonFatal, signature := classifyRunJSEvaluateError(err)
			shouldLog, count := wv.shouldLogRunJSError(domain, signature, nonFatal, time.Now())
			if shouldLog {
				ev := log.Warn()
				msg := "RunJavaScript: failed"
				if nonFatal {
					ev = log.Debug()
					msg = "RunJavaScript: non-fatal failure"
				}
				ev.Err(err).
					Uint64("webview_id", uint64(wv.id)).
					Str("domain", domain).
					Str("signature", signature).
					Uint64("repeat_count", count).
					Msg(msg)
			}
			return
		}

		if value != nil {
			if jscCtx := value.GetContext(); jscCtx != nil {
				if exc := jscCtx.GetException(); exc != nil {
					exceptionMessage := strings.TrimSpace(exc.GetMessage())
					signature := "js_exception:" + normalizeRunJSErrorSignature(exceptionMessage)
					shouldLog, count := wv.shouldLogRunJSError(domain, signature, true, time.Now())
					if shouldLog {
						log.Debug().
							Str("exception", exceptionMessage).
							Uint64("webview_id", uint64(wv.id)).
							Str("domain", domain).
							Str("signature", signature).
							Uint64("repeat_count", count).
							Msg("RunJavaScript: JS exception")
					}
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
	wv.mu.RLock()
	inner := wv.inner
	wv.mu.RUnlock()
	if inner == nil || wv.destroyed.Load() {
		return
	}
	inner.EvaluateJavascript(script, -1, worldNamePtr, nil, nil, &cb, 0)
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
