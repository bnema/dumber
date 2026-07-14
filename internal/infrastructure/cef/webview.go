package cef

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/shared/syncdispatch"
)

// Compile-time interface checks.
var (
	_ port.WebView               = (*WebView)(nil)
	_ port.NativeWidgetProvider  = (*WebView)(nil)
	_ port.DevToolsOpener        = (*WebView)(nil)
	_ port.PopupLifecycleCapable = (*WebView)(nil)
	_ port.PopupOpenerCapable    = (*WebView)(nil)
	_ port.ViewportSyncCapable   = (*WebView)(nil)
	_ port.OAuthCallbackCapable  = (*WebView)(nil)
)

// errDestroyed is returned when an operation is attempted on a destroyed WebView.
var errDestroyed = errors.New("cef: webview is destroyed")

// unrefTickCallback is the canonical puregotk/purego callback-slot release.
// It remains injectable so lifecycle tests can prove exactly-once ownership.
var unrefTickCallback = glib.UnrefCallback

// errNoBrowser is returned when the browser has not been created yet.
var errNoBrowser = errors.New("cef: browser not yet created")

const (
	clipboardSelectionDebounceDelay = 300 * time.Millisecond
	pendingNavigationRetryDelay     = 50 * time.Millisecond
	pendingNavigationMaxRetries     = 80
	nativePopupAttachFallbackDelay  = 250 * time.Millisecond
)

type stoppableTimer interface {
	Stop() bool
}

var cefLoadWatchdogDelays = []time.Duration{
	250 * time.Millisecond,
	1 * time.Second,
	2 * time.Second,
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
}

const (
	cefGTKSyncDispatchTimeout       = 2 * time.Second
	cefGTKSyncDispatchSlowThreshold = 250 * time.Millisecond
)

var (
	cefNewTask          = purecef.NewTask
	cefNewStringVisitor = purecef.NewStringVisitor
	cefPostTask         = purecef.PostTask
	cefPostDelayedTask  = purecef.PostDelayedTask
	cefScheduleAfter    = func(delay time.Duration, fn func()) { time.AfterFunc(delay, fn) }
)

// WebView implements port.WebView using a CEF off-screen browser rendered and
// driven through purego-cef2gtk. Dumber owns browser state and callbacks; the
// bridge owns GTK rendering and input forwarding.
type WebView struct {
	id                        port.WebViewID
	ctx                       context.Context
	engine                    *Engine
	factory                   *WebViewFactory
	browser                   purecef.Browser
	host                      purecef.BrowserHost
	client                    purecef.RawClient // prevent GC from collecting the client before CEF AddRef's it
	viewBridge                *Cef2gtkAdapter
	popupSurface              *popupBridgeSurface
	nativeWidget              *gtk.Widget
	handlers                  *handlerSet
	findCtrl                  *cefFindController
	profileCleanup            func()
	profileMu                 sync.Mutex
	renderStallMu             sync.Mutex
	renderStallStop           chan struct{}
	renderStallDone           chan struct{}
	renderStallRecoveryLastAt time.Time
	latestProfileSnapshot     cef2gtk.ProfileSnapshot
	previousProfileSnapshot   cef2gtk.ProfileSnapshot
	latestProfileSnapshotAt   time.Time

	removeSizeObserver      func()
	viewportSyncMu          sync.Mutex
	viewportSyncPending     bool
	viewportSyncReason      string
	viewportMapFunc         func(gtk.Widget)
	viewportShowFunc        func(gtk.Widget)
	viewportHideFunc        func(gtk.Widget)
	viewportUnmapFunc       func(gtk.Widget)
	viewportRealizeFunc     func(gtk.Widget)
	viewportMapSignalID     uint
	viewportShowSignalID    uint
	viewportHideSignalID    uint
	viewportUnmapSignalID   uint
	viewportRealizeSignalID uint
	viewportResizePulseSeq  atomic.Uint64
	// effectiveVisibility records the last CEF WasHidden state dispatched. It
	// is guarded by mu because GTK and CEF UI work cross threads.
	effectiveVisibilityKnown bool
	effectiveVisible         bool

	adaptiveWindowlessFrameRate bool
	windowlessFrameRateMax      int32
	adaptiveFrameRatePoll       *glib.SourceFunc
	adaptiveFrameRatePollID     uint
	lastAdaptiveFrameRate       int32

	// beginFrameTick drives CEF external BeginFrame requests while the GTK
	// widget is visible. Access is guarded by mu.
	beginFrameTick   *gtk.TickCallback
	beginFrameTickID uint

	// pendingCreate holds browser creation params until the GL area is realized.
	pendingCreate *pendingBrowserCreate
	// initialBrowserCreateResizeHandled gates the one-shot onFirstResize path so
	// later size observer events fall through to normal viewport sync even when
	// browser creation started from another GTK lifecycle path.
	initialBrowserCreateResizeHandled bool

	// pendingURI is set when LoadURI is called before the browser exists.
	pendingURI          string
	pendingURISetAt     time.Time
	pendingURIStartedAt time.Time

	// GTK sync dispatch hooks are injectable for tests. Production uses the GTK
	// default main context through runOnGTK and isOnGTKThread.
	gtkSyncDispatch func(func())
	gtkSyncIsOwner  func() bool
	gtkSyncTimeout  time.Duration

	// crashCount tracks consecutive renderer crashes to prevent infinite
	// crash → redirect → crash loops.
	crashCount atomic.Int32

	// Callbacks and browsing-context state set by the UI layer.
	mu                         sync.RWMutex
	callbacks                  *port.WebViewCallbacks
	browsingContextDecision    dto.HostDecision
	hasBrowsingContextDecision bool
	nativePopupHostAbort       func()

	// Synthetic popup proxies created by the renderer bridge's window.open shim.
	syntheticPopupMu sync.Mutex
	syntheticPopups  map[string]*syntheticPopupState

	// Programmatic popup lifecycle callbacks used for OAuth auto-close and
	// synthetic window.open() proxy support on CEF.
	closeCallbacks            []func()
	navigationCallbacks       []func(string)
	openerMessageCallbacks    []func()
	openerNavigationCallbacks []func(string)
	popupReadyToShow          func()
	popupReadyShown           bool

	// Native popup bookkeeping. CreateRelated() returns a popup shell with no
	// browser attached yet; OnBeforePopup may wire CEF's real popup browser into
	// that shell. If native attachment is unavailable or stalls, the shell can
	// still create its own browser directly while preserving the same popup pane
	// lifecycle from the coordinator's perspective.
	nativePopupCandidate        bool
	nativePopupParent           *WebView
	nativePopupID               int32
	nativePopupFallbackStarted  bool
	nativePopupFallbackTimer    stoppableTimer
	nativePopupFallbackSchedule func(time.Duration, func()) stoppableTimer
	pendingNativePopups         map[int32]*WebView
	popupNoJavaScriptAccess     bool
	popupOpenerBridgeParent     *WebView
	popupOpenerBridgeParentURI  string

	// State cache (mutex-protected).
	uri                       string
	title                     string
	progress                  float64
	canGoBack                 bool
	canGoFwd                  bool
	isLoading                 bool
	selectedText              string
	focusedEditable           bool
	inputAttached             bool
	bridgeNonce               string
	selectionDebounceTimer    stoppableTimer
	selectionDebounceSeq      uint64
	selectionDebounceDelay    *time.Duration
	selectionDebounceSchedule func(time.Duration, func()) stoppableTimer

	// Last known hover URI for middle-click → new tab.
	lastHoverURI string

	// Favicon source visitors are retained until CEF calls them back because
	// GetSource delivers asynchronously through a C callback pointer.
	faviconSourceVisitorsMu sync.Mutex
	faviconSourceVisitors   []faviconSourceVisitorRetention

	// Load diagnostics state (mutex-protected).
	loadDiagSeq             uint64
	loadDiagStartedAt       time.Time
	loadDiagLastProgressAt  time.Time
	loadDiagLastAddressAt   time.Time
	loadDiagLastLoadStateAt time.Time

	// Atomic state.
	destroyed                     atomic.Bool
	fullscreen                    atomic.Bool
	generation                    atomic.Uint64
	audioPlaying                  atomic.Bool
	zoomFactor                    atomic.Value // float64, initialized to 1.0
	lastAppliedZoomScaleRatioBits atomic.Uint64

	// Browser creation defaults copied from the factory so native popup shells
	// can apply the same settings in OnBeforePopup.
	windowlessFrameRate int32
	backgroundColor     uint32
	inputConfig         RuntimeInputConfig

	// Touchpad input diagnostics are debounced to avoid flooding logs while
	// debugging continuous scroll streams.
	touchpadNavigation *touchpadNavigationRecognizer
	inputDiagMu        sync.Mutex
	inputDiagLastLog   time.Time
	inputDiagEvents    int
	inputDiagDX        float64
	inputDiagDY        float64
	inputDiagDeltaXSum int64
	inputDiagDeltaYSum int64

	// Audio output factory and active stream.
	audioOutputFactory port.AudioOutputFactory
	audioStreamMu      sync.Mutex
	activeAudioStream  port.AudioOutputStream

	// Audio instrumentation counters (diagnostic only).
	audioPacketCount atomic.Uint64 // total OnAudioStreamPacket calls
	audioWriteCount  atomic.Uint64 // successful Write calls to stream
}

// pendingBrowserCreate holds the parameters needed to create a CEF browser,
// deferred until the GL area has a non-zero size.
type pendingBrowserCreate struct {
	windowInfo                 *purecef.WindowInfo
	client                     purecef.RawClient
	settings                   *purecef.BrowserSettings
	extraInfo                  purecef.DictionaryValue
	postTaskRetries            int
	observedSizeRetries        int
	observedSizeRetryScheduled bool
}

type cefTaskFunc func()

func (fn cefTaskFunc) Execute() {
	if fn != nil {
		fn()
	}
}

// ---------------------------------------------------------------------------
// Identity
// ---------------------------------------------------------------------------

func (wv *WebView) ID() port.WebViewID {
	return wv.id
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

func (wv *WebView) LoadURI(_ context.Context, uri string) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	actualURI := toActualInternalURL(uri)
	wv.mu.Lock()
	browser := wv.browser
	// Always remember the latest requested URI so a browser/main-frame race
	// cannot strand startup on about:blank.
	wv.setPendingNavigationLocked(actualURI, time.Now())
	wv.mu.Unlock()
	if browser == nil {
		return nil
	}
	wv.schedulePendingNavigationReplay(0)
	return nil
}

// LoadHTML loads HTML content with an optional base URI (ignored in Phase 1).
func (wv *WebView) LoadHTML(ctx context.Context, content, _ string) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	const maxDataURLSize = 1 << 20 // 1MB
	if len(content) > maxDataURLSize {
		logging.FromContext(ctx).Warn().
			Int("content_len", len(content)).
			Msg("cef: LoadHTML content exceeds 1MB, data URL may fail")
	}
	wv.mu.RLock()
	browser := wv.browser
	wv.mu.RUnlock()
	if browser == nil {
		return errNoBrowser
	}
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(content))
	if frame := browser.GetMainFrame(); frame != nil {
		frame.LoadURL(dataURL)
	}
	return nil
}

func (wv *WebView) Reload(_ context.Context) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	wv.mu.RLock()
	browser := wv.browser
	wv.mu.RUnlock()
	if browser == nil {
		return errNoBrowser
	}
	browser.Reload()
	return nil
}

// ReloadBypassCache reloads the current page, bypassing cache.
func (wv *WebView) ReloadBypassCache(_ context.Context) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	wv.mu.RLock()
	browser := wv.browser
	wv.mu.RUnlock()
	if browser == nil {
		return errNoBrowser
	}
	browser.ReloadIgnoreCache()
	return nil
}

// Stop stops the current page load.
func (wv *WebView) Stop(_ context.Context) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	wv.mu.RLock()
	browser := wv.browser
	wv.mu.RUnlock()
	if browser == nil {
		return errNoBrowser
	}
	browser.StopLoad()
	return nil
}

// GoBack navigates back in history.
func (wv *WebView) GoBack(_ context.Context) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	wv.mu.RLock()
	browser := wv.browser
	wv.mu.RUnlock()
	if browser == nil {
		return errNoBrowser
	}
	browser.GoBack()
	return nil
}

// GoForward navigates forward in history.
func (wv *WebView) GoForward(_ context.Context) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	wv.mu.RLock()
	browser := wv.browser
	wv.mu.RUnlock()
	if browser == nil {
		return errNoBrowser
	}
	browser.GoForward()
	return nil
}

// ---------------------------------------------------------------------------
// State queries (read from cache under RLock)
// ---------------------------------------------------------------------------

func (wv *WebView) URI() string {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.uri
}

func (wv *WebView) Title() string {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.title
}

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
		ZoomLevel: wv.GetZoomLevel(),
	}
}

// IsFullscreen returns true if the WebView is currently in fullscreen mode.
func (wv *WebView) IsFullscreen() bool {
	return wv.fullscreen.Load()
}

// IsPlayingAudio returns true if any audio stream is active.
func (wv *WebView) IsPlayingAudio() bool {
	return wv.audioPlaying.Load()
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
// Zoom
// ---------------------------------------------------------------------------

// chromiumZoomBase is the base used by Chromium's logarithmic zoom level.
// CEF zoom level = log(factor) / log(1.2), where factor is the linear multiplier.
const chromiumZoomBase = 1.2

// cefZoomFromFactor converts a linear zoom factor (1.0 = 100%) to CEF's
// logarithmic zoom level.
func cefZoomFromFactor(factor float64) float64 {
	if factor <= 0 {
		return 0
	}
	return math.Log(factor) / math.Log(chromiumZoomBase)
}

func zoomScaleRatio(surfaceScale, backingScale float64) float64 {
	backingScale = normalizeScale(backingScale)
	if backingScale <= 1 {
		return 1
	}
	return normalizeScale(surfaceScale) / backingScale
}

func cefZoomFromPageAndScaleFactors(pageZoom, surfaceScale, backingScale float64) float64 {
	return cefZoomFromFactor(pageZoom * zoomScaleRatio(surfaceScale, backingScale))
}

func pageZoomFromCEFAndScaleLevel(level, surfaceScale, backingScale float64) float64 {
	return factorFromCEFZoom(level) / zoomScaleRatio(surfaceScale, backingScale)
}

func (wv *WebView) applyCEFZoomLevel(host purecef.BrowserHost, factor, cefLevel, surfaceScale, backingScale float64) {
	host.SetZoomLevel(cefLevel)
	// Force CEF to produce a new frame at the new zoom level. In OSR mode,
	// SetZoomLevel changes the Blink layout zoom but doesn't guarantee a
	// repaint. WasResized is a no-op when view dimensions haven't changed.
	// NotifyScreenInfoChanged forces surface ID invalidation + a full
	// SynchronizeVisualProperties cycle, which makes the renderer produce
	// a new compositor frame at the new zoom level.
	host.NotifyScreenInfoChanged()
	wv.recordAppliedZoomScaleRatio(surfaceScale, backingScale)
	// Zoom is applied asynchronously in the renderer process. Request a couple
	// of follow-up refreshes on the CEF UI thread so OSR captures the updated
	// compositor frame after the zoom IPC has been processed.
	wv.scheduleZoomRefresh()
	wv.scheduleZoomReadback(factor, cefLevel)
}

// factorFromCEFZoom converts a Chromium/CEF logarithmic zoom level back to a
// linear zoom factor.
func factorFromCEFZoom(level float64) float64 {
	return math.Pow(chromiumZoomBase, level)
}

// GetZoomLevel returns the current linear zoom factor.
func (wv *WebView) GetZoomLevel() float64 {
	if v := wv.zoomFactor.Load(); v != nil {
		return v.(float64)
	}
	return 1.0
}

// SetZoomLevel sets the zoom level using a linear factor (1.0 = 100%).
func (wv *WebView) SetZoomLevel(_ context.Context, factor float64) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	wv.mu.RLock()
	host := wv.host
	wv.mu.RUnlock()
	if host == nil {
		return errNoBrowser
	}
	surfaceScale := wv.viewBridgeScale()
	backingScale := wv.osrBackingScaleFactor()
	cefLevel := cefZoomFromPageAndScaleFactors(factor, surfaceScale, backingScale)
	logging.FromContext(wv.ctx).Debug().
		Float64("factor", factor).
		Float64("cef_level", cefLevel).
		Float64("osr_backing_scale", backingScale).
		Float64("surface_scale", surfaceScale).
		Msg("cef: SetZoomLevel")
	wv.applyCEFZoomLevel(host, factor, cefLevel, surfaceScale, backingScale)
	wv.zoomFactor.Store(factor)
	return nil
}

// ---------------------------------------------------------------------------
// DevTools
// ---------------------------------------------------------------------------

// OpenDevTools opens the Chromium DevTools in a separate window.
func (wv *WebView) OpenDevTools() {
	if wv.destroyed.Load() {
		return
	}
	wv.mu.RLock()
	host := wv.host
	wv.mu.RUnlock()
	if host == nil {
		return
	}
	windowInfo := purecef.NewWindowInfo()
	settings := purecef.NewBrowserSettings()
	host.ShowDevTools(&windowInfo, nil, &settings, nil)
}

// ---------------------------------------------------------------------------
// Find
// ---------------------------------------------------------------------------

// GetFindController returns the find-in-page controller.
func (wv *WebView) GetFindController() port.FindController {
	if wv.findCtrl == nil {
		return nil
	}
	return wv.findCtrl
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

// SetOnReadyToShow implements port.PopupLifecycleCapable.
func (wv *WebView) SetOnReadyToShow(fn func()) {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	wv.popupReadyToShow = fn
	shouldTryReady := wv.browser != nil && fn != nil
	wv.mu.Unlock()
	if shouldTryReady {
		wv.fireReadyToShow()
	}
}

// SetOnClose implements port.PopupLifecycleCapable.
func (wv *WebView) SetOnClose(fn func()) {
	wv.AddCloseCallback(fn)
}

// Show implements port.PopupLifecycleCapable.
func (wv *WebView) Show() {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	wv.SyncViewport(wv.ctx, "popup-show")
}

// PrimePopupNavigation implements port.PopupLifecycleCapable.
func (wv *WebView) PrimePopupNavigation(uri string) {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	actualURI := toActualInternalURL(strings.TrimSpace(uri))
	if actualURI == "" {
		return
	}
	wv.mu.Lock()
	browser := wv.browser
	wv.setPendingNavigationLocked(actualURI, time.Now())
	wv.mu.Unlock()
	if browser != nil {
		wv.schedulePendingNavigationReplay(0)
	}
}

// AddCloseCallback implements port.OAuthCallbackCapable.
func (wv *WebView) AddCloseCallback(fn func()) {
	if wv == nil || fn == nil {
		return
	}
	wv.mu.Lock()
	wv.closeCallbacks = append(wv.closeCallbacks, fn)
	wv.mu.Unlock()
}

// AddNavigationCallback implements port.OAuthCallbackCapable.
func (wv *WebView) AddNavigationCallback(fn func(uri string)) {
	if wv == nil || fn == nil {
		return
	}
	wv.mu.Lock()
	wv.navigationCallbacks = append(wv.navigationCallbacks, fn)
	wv.mu.Unlock()
}

// Close implements port.OAuthCallbackCapable.
func (wv *WebView) Close() {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	wv.mu.RLock()
	host := wv.host
	wv.mu.RUnlock()
	if host != nil {
		host.CloseBrowser(1)
		return
	}
	wv.runCloseCallbacks()
}

func (wv *WebView) fireReadyToShow() {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	if wv.popupReadyShown || wv.popupReadyToShow == nil || !wv.inputAttached {
		wv.mu.Unlock()
		return
	}
	fn := wv.popupReadyToShow
	wv.popupReadyShown = true
	wv.mu.Unlock()
	wv.runOnGTK(fn)
}

func (wv *WebView) markInputAttached() {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	wv.mu.Lock()
	wv.inputAttached = true
	wv.mu.Unlock()
	wv.fireReadyToShow()
}

func (wv *WebView) markNativePopupCandidate(parent *WebView) {
	if wv == nil {
		return
	}
	wv.stopNativePopupFallbackTimer()
	wv.mu.Lock()
	wv.nativePopupCandidate = true
	wv.nativePopupParent = parent
	wv.nativePopupID = 0
	wv.nativePopupFallbackStarted = false
	wv.mu.Unlock()
}

func (wv *WebView) isNativePopupCandidate() bool {
	if wv == nil {
		return false
	}
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.nativePopupCandidate
}

func (wv *WebView) discardNativePopupCandidate() {
	if wv == nil {
		return
	}
	var timer stoppableTimer
	wv.mu.Lock()
	timer = wv.nativePopupFallbackTimer
	wv.nativePopupFallbackTimer = nil
	wv.nativePopupCandidate = false
	wv.nativePopupParent = nil
	wv.nativePopupID = 0
	wv.nativePopupFallbackStarted = false
	wv.popupOpenerBridgeParent = nil
	wv.popupOpenerBridgeParentURI = ""
	wv.syncPopupOpenerBridgeExtraInfoLocked()
	wv.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
}

func (wv *WebView) PreparePaneHostedBrowsingContext() {
	if wv == nil {
		return
	}
	wv.discardNativePopupCandidate()
}

func (wv *WebView) SetBrowsingContextHostDecision(decision dto.HostDecision) {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	wv.browsingContextDecision = decision
	wv.hasBrowsingContextDecision = true
	wv.mu.Unlock()
}

func (wv *WebView) BrowsingContextHostDecision() (dto.HostDecision, bool) {
	if wv == nil {
		return dto.HostDecision{}, false
	}
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.browsingContextDecision, wv.hasBrowsingContextDecision
}

func (wv *WebView) SetNativePopupHostAbort(fn func()) {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	wv.nativePopupHostAbort = fn
	wv.mu.Unlock()
}

func (wv *WebView) AbortNativePopupHost() {
	if wv == nil {
		return
	}
	wv.mu.RLock()
	fn := wv.nativePopupHostAbort
	wv.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func (wv *WebView) awaitsNativePopupAttachment() bool {
	if wv == nil {
		return false
	}
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.browser == nil && !wv.nativePopupFallbackStarted && (wv.nativePopupCandidate || wv.nativePopupID != 0)
}

func (wv *WebView) awaitsBrowserCreateFromNativePopupFallback() bool {
	if wv == nil {
		return false
	}
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.browser == nil && wv.nativePopupFallbackStarted && wv.pendingCreate != nil
}

func (wv *WebView) nativePopupFallbackScheduler() func(time.Duration, func()) stoppableTimer {
	if wv != nil && wv.nativePopupFallbackSchedule != nil {
		return wv.nativePopupFallbackSchedule
	}
	return func(delay time.Duration, fn func()) stoppableTimer {
		return time.AfterFunc(delay, fn)
	}
}

func (wv *WebView) stopNativePopupFallbackTimer() {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	timer := wv.nativePopupFallbackTimer
	wv.nativePopupFallbackTimer = nil
	wv.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
}

func (wv *WebView) scheduleNativePopupFallback(delay time.Duration, fn func()) {
	if wv == nil || fn == nil || wv.destroyed.Load() {
		return
	}
	wv.stopNativePopupFallbackTimer()
	var timer stoppableTimer
	timer = wv.nativePopupFallbackScheduler()(delay, func() {
		wv.mu.Lock()
		if wv.destroyed.Load() || wv.nativePopupFallbackTimer != timer {
			wv.mu.Unlock()
			return
		}
		wv.nativePopupFallbackTimer = nil
		wv.mu.Unlock()
		fn()
	})
	wv.mu.Lock()
	if wv.destroyed.Load() {
		wv.mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		return
	}
	wv.nativePopupFallbackTimer = timer
	wv.mu.Unlock()
}

func (wv *WebView) preparePopupShellDirectBrowserCreation() bool {
	if wv == nil {
		return false
	}
	wv.stopNativePopupFallbackTimer()
	wv.mu.Lock()
	if wv.destroyed.Load() || wv.browser != nil || wv.nativePopupFallbackStarted || wv.pendingCreate == nil {
		wv.mu.Unlock()
		return false
	}
	parent := wv.nativePopupParent
	popupID := wv.nativePopupID
	wv.nativePopupFallbackStarted = true
	wv.nativePopupCandidate = false
	wv.nativePopupParent = nil
	wv.nativePopupID = 0
	if !wv.popupNoJavaScriptAccess && parent != nil && !parent.destroyed.Load() {
		wv.popupOpenerBridgeParent = parent
		wv.popupOpenerBridgeParentURI = parent.URI()
	} else {
		wv.popupOpenerBridgeParent = nil
		wv.popupOpenerBridgeParentURI = ""
	}
	wv.syncPopupOpenerBridgeExtraInfoLocked()
	wv.mu.Unlock()
	if parent != nil && popupID != 0 {
		parent.clearPendingNativePopup(popupID, wv)
	}
	return true
}

func (wv *WebView) startNativePopupFallback() bool {
	if wv == nil {
		return false
	}
	wv.mu.RLock()
	destroyed := wv.destroyed.Load()
	alreadyStarted := wv.nativePopupFallbackStarted
	wv.mu.RUnlock()
	if destroyed || alreadyStarted {
		return false
	}
	return wv.preparePopupShellDirectBrowserCreation()
}

func (wv *WebView) trackPendingNativePopup(popupID int32, popup *WebView) {
	if wv == nil || popupID == 0 || popup == nil {
		return
	}
	wv.mu.Lock()
	if wv.pendingNativePopups == nil {
		wv.pendingNativePopups = make(map[int32]*WebView)
	}
	wv.pendingNativePopups[popupID] = popup
	wv.mu.Unlock()
}

func (wv *WebView) takePendingNativePopup(popupID int32) *WebView {
	if wv == nil || popupID == 0 {
		return nil
	}
	wv.mu.Lock()
	defer wv.mu.Unlock()
	popup := wv.pendingNativePopups[popupID]
	delete(wv.pendingNativePopups, popupID)
	return popup
}

func (wv *WebView) clearPendingNativePopup(popupID int32, popup *WebView) {
	if wv == nil || popupID == 0 || popup == nil {
		return
	}
	wv.mu.Lock()
	defer wv.mu.Unlock()
	if current := wv.pendingNativePopups[popupID]; current == popup {
		delete(wv.pendingNativePopups, popupID)
	}
}

func (wv *WebView) prepareNativePopup(
	popupID int32,
	targetURL string,
	windowInfo *purecef.WindowInfo,
	clientSlot *purecef.RawClientWriteSlot,
	settings *purecef.BrowserSettings,
) bool {
	if wv == nil || wv.destroyed.Load() || popupID == 0 || windowInfo == nil || clientSlot == nil {
		return false
	}

	prep, ok := wv.nativePopupPreparationSnapshot()
	if !ok {
		return false
	}
	client, ok := wv.activateNativePopup(popupID, targetURL)
	if !ok {
		return false
	}
	if prep.parent != nil {
		prep.parent.trackPendingNativePopup(popupID, wv)
	}

	configureNativePopupWindow(windowInfo, settings, prep.frameRate, prep.backgroundColor)
	clientSlot.Set(client)
	return true
}

func (wv *WebView) nativePopupPreparationSnapshot() (nativePopupPreparation, bool) {
	if wv == nil {
		return nativePopupPreparation{}, false
	}
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	if !wv.canPrepareNativePopupLocked() {
		return nativePopupPreparation{}, false
	}
	return nativePopupPreparation{
		parent:          wv.nativePopupParent,
		frameRate:       wv.windowlessFrameRate,
		backgroundColor: wv.backgroundColor,
	}, true
}

func (wv *WebView) activateNativePopup(popupID int32, targetURL string) (purecef.RawClient, bool) {
	if wv == nil {
		return nil, false
	}
	wv.mu.Lock()
	defer wv.mu.Unlock()
	if !wv.canPrepareNativePopupLocked() {
		return nil, false
	}
	wv.nativePopupCandidate = false
	wv.nativePopupID = popupID
	wv.nativePopupFallbackStarted = false
	if strings.TrimSpace(wv.pendingURI) == "" {
		wv.setPendingNavigationLocked(toActualInternalURL(targetURL), time.Now())
	}
	if strings.TrimSpace(wv.pendingURI) != "" {
		wv.uri = toConceptualInternalURL(wv.pendingURI)
	} else {
		wv.uri = toConceptualInternalURL(targetURL)
	}
	wv.isLoading = true
	return wv.client, true
}

func (wv *WebView) canPrepareNativePopupLocked() bool {
	return wv.nativePopupCandidate && wv.client != nil && !wv.nativePopupFallbackStarted && wv.browser == nil
}

func configureNativePopupWindow(
	windowInfo *purecef.WindowInfo,
	settings *purecef.BrowserSettings,
	frameRate int32,
	backgroundColor uint32,
) {
	cef2gtk.ConfigureWindowInfo(windowInfo, cef2gtk.WindowInfoOptions{})
	if externalBeginFrameEnabled() {
		windowInfo.ExternalBeginFrameEnabled = 1
	}
	if settings == nil {
		return
	}
	cef2gtk.ConfigureBrowserSettings(settings, cef2gtk.BrowserSettingsOptions{WindowlessFrameRate: frameRate})
	settings.LocalStorage = 1
	if backgroundColor != 0 {
		settings.BackgroundColor = backgroundColor
	}
}

type nativePopupPreparation struct {
	parent          *WebView
	frameRate       int32
	backgroundColor uint32
}

func (wv *WebView) handleNativePopupAborted() {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	wv.nativePopupCandidate = false
	wv.nativePopupID = 0
	wv.mu.Unlock()
}

// ---------------------------------------------------------------------------
// JavaScript / Appearance
// ---------------------------------------------------------------------------

// RunJavaScript executes a script in the main world. Fire-and-forget.
func (wv *WebView) RunJavaScript(_ context.Context, script string) {
	if wv.destroyed.Load() {
		return
	}
	if wv.engine == nil {
		wv.executeJavaScriptNow(script)
		return
	}
	task := cefNewTask(cefTaskFunc(func() {
		wv.executeJavaScriptNow(script)
	}))
	if task == nil {
		return
	}
	cefPostTask(purecef.ThreadIDTidUi, task)
}

func (wv *WebView) executeJavaScriptNow(script string) {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	wv.mu.RLock()
	browser := wv.browser
	wv.mu.RUnlock()
	if browser == nil {
		return
	}
	if frame := browser.GetMainFrame(); frame != nil {
		frame.ExecuteJavaScript(script, "", 0)
	}
}

// colorScale converts a 0.0–1.0 color component to an 8-bit integer.
const colorScale = 255

var bridgeNonceRandom = rand.Read

func newBridgeNonce() string {
	buf := make([]byte, 16)
	if _, err := bridgeNonceRandom(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}

func (wv *WebView) rotateBridgeNonce() string {
	nonce := newBridgeNonce()
	if nonce == "" {
		return ""
	}
	wv.mu.Lock()
	wv.bridgeNonce = nonce
	wv.mu.Unlock()
	return nonce
}

func (wv *WebView) ensureBridgeNonce() string {
	if wv == nil {
		return ""
	}
	wv.mu.RLock()
	nonce := wv.bridgeNonce
	wv.mu.RUnlock()
	if nonce != "" {
		return nonce
	}
	return wv.rotateBridgeNonce()
}

// SetBackgroundColor sets the background via JS injection (CEF has no runtime API).
func (wv *WebView) SetBackgroundColor(r, g, b, a float64) {
	script := fmt.Sprintf(
		`(function(){ if (document.documentElement) { document.documentElement.style.backgroundColor='rgba(%d,%d,%d,%.2f)'; } })()`,
		int(r*colorScale), int(g*colorScale), int(b*colorScale), a,
	)
	wv.RunJavaScript(context.Background(), script)
}

// ResetBackgroundToDefault clears the injected background color.
func (wv *WebView) ResetBackgroundToDefault() {
	wv.RunJavaScript(context.Background(),
		`(function(){ if (document.documentElement) { document.documentElement.style.backgroundColor=''; } })()`)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func (wv *WebView) IsDestroyed() bool {
	return wv.destroyed.Load()
}

func (wv *WebView) Destroy() {
	if !wv.destroyed.CompareAndSwap(false, true) {
		return
	}
	wv.syntheticPopupMu.Lock()
	wv.syntheticPopups = nil
	wv.syntheticPopupMu.Unlock()
	wv.mu.Lock()
	wv.navigationCallbacks = nil
	wv.openerMessageCallbacks = nil
	wv.openerNavigationCallbacks = nil
	wv.popupReadyToShow = nil
	wv.touchpadNavigation = nil
	wv.inputAttached = false
	wv.pendingNativePopups = nil
	wv.nativePopupParent = nil
	wv.nativePopupCandidate = false
	wv.nativePopupID = 0
	wv.nativePopupFallbackStarted = false
	wv.popupOpenerBridgeParent = nil
	wv.popupOpenerBridgeParentURI = ""
	wv.mu.Unlock()
	wv.stopNativePopupFallbackTimer()
	wv.stopRenderStallWatchdog()
	wv.cancelSelectionDebounce()
	wv.closeAudioStream()
	wv.scheduleStopAdaptiveFrameRatePolling()
	wv.scheduleStopBeginFrameLoop()
	wv.mu.RLock()
	host := wv.host
	wv.mu.RUnlock()
	if host != nil {
		host.CloseBrowser(1)
	} else {
		wv.runCloseCallbacks()
		wv.destroyViewBridgeOnGTKSync()
	}
	if wv.profileCleanup != nil {
		wv.profileCleanup()
		wv.profileCleanup = nil
	}
}

// destroyViewBridgeOnGTKSync, destroyViewBridgeOnGTKAsync, and
// destroyViewBridgeOnGTKThread may race with one another during teardown. The
// early viewBridge nil checks avoid scheduling unnecessary GTK work; the final
// nil guard on the GTK thread is authoritative and makes duplicate scheduling
// harmless.
func (wv *WebView) destroyViewBridgeOnGTKSync() {
	if wv == nil || wv.viewBridge == nil {
		return
	}
	wv.runOnGTKSyncLabelAllowLateStart("cef.destroy_view_bridge", true, func() {
		wv.destroyViewBridgeOnGTKThread()
	})
}

func (wv *WebView) destroyViewBridgeOnGTKAsync() {
	if wv == nil || wv.viewBridge == nil {
		return
	}
	wv.runOnGTK(func() {
		wv.destroyViewBridgeOnGTKThread()
	})
}

func (wv *WebView) destroyViewBridgeOnGTKThread() {
	if wv == nil || wv.viewBridge == nil {
		return
	}
	if wv.removeSizeObserver != nil {
		wv.removeSizeObserver()
		wv.removeSizeObserver = nil
	}
	wv.disconnectViewportSyncHooksOnGTKThread()
	if wv.popupSurface != nil {
		wv.popupSurface.DestroyOnGTKThread()
		wv.popupSurface = nil
	}
	_ = wv.viewBridge.DetachInput()
	_ = wv.viewBridge.Destroy()
	wv.viewBridge = nil
	wv.nativeWidget = nil
}

// ---------------------------------------------------------------------------
// NativeWidgetProvider
// ---------------------------------------------------------------------------

// NativeWidget returns the uintptr for embedding the bridge GTK widget.
func (wv *WebView) NativeWidget() uintptr {
	if wv == nil {
		return 0
	}
	var ptr uintptr
	wv.runOnGTKSync(func() {
		switch {
		case wv.nativeWidget != nil:
			ptr = wv.nativeWidget.GoPointer()
		case wv.viewBridge != nil:
			ptr = wv.viewBridge.NativeWidget()
		}
	})
	return ptr
}

// ---------------------------------------------------------------------------
// State update helpers (called from handlers)
// ---------------------------------------------------------------------------

func (wv *WebView) updateURI(uri string) {
	uri = toConceptualInternalURL(uri)
	now := time.Now()
	wv.mu.Lock()
	wv.uri = uri
	wv.loadDiagLastAddressAt = now
	cb := wv.callbacks
	pendingMatched := wv.clearPendingNavigationIfEquivalentLocked(uri)
	wv.mu.Unlock()

	if pendingMatched && wv.ctx != nil {
		logging.FromContext(wv.ctx).Debug().
			Str("uri", logging.TruncateURL(uri, logging.PermissionLogURLMaxLen)).
			Msg("cef: pending navigation acknowledged by address change")
	}

	if cb != nil && cb.OnURIChanged != nil {
		wv.runOnGTK(func() {
			cb.OnURIChanged(uri)
		})
	}
	wv.runNavigationCallbacks(uri)
}

func (wv *WebView) updateTitle(title string) {
	wv.mu.Lock()
	wv.title = title
	cb := wv.callbacks
	wv.mu.Unlock()

	if cb != nil && cb.OnTitleChanged != nil {
		wv.runOnGTK(func() {
			cb.OnTitleChanged(title)
		})
	}
}

func (wv *WebView) updateProgress(progress float64) {
	now := time.Now()
	wv.mu.Lock()
	wv.progress = progress
	wv.loadDiagLastProgressAt = now
	cb := wv.callbacks
	wv.mu.Unlock()

	if cb != nil && cb.OnProgressChanged != nil {
		wv.runOnGTK(func() {
			cb.OnProgressChanged(progress)
		})
	}
}

func (wv *WebView) updateLoadState(loading, back, fwd bool) {
	now := time.Now()
	wv.mu.Lock()
	wasLoading := wv.isLoading
	wv.isLoading = loading
	wv.canGoBack = back
	wv.canGoFwd = fwd
	wv.loadDiagLastLoadStateAt = now
	var loadDiagSeq uint64
	if loading && !wasLoading {
		wv.loadDiagSeq++
		wv.loadDiagStartedAt = now
		wv.loadDiagLastProgressAt = time.Time{}
		wv.loadDiagLastAddressAt = time.Time{}
		loadDiagSeq = wv.loadDiagSeq
	}
	clearedPendingURI := ""
	currentURI := ""
	if !loading && wv.hasObservedAddressForPendingNavigationLocked() {
		clearedPendingURI = wv.clearPendingNavigationLocked()
		currentURI = wv.uri
	}
	wv.mu.Unlock()

	if clearedPendingURI != "" && wv.ctx != nil {
		logging.FromContext(wv.ctx).Debug().
			Str("pending_uri", logging.TruncateURL(clearedPendingURI, logging.PermissionLogURLMaxLen)).
			Str("uri", logging.TruncateURL(currentURI, logging.PermissionLogURLMaxLen)).
			Msg("cef: cleared pending navigation after load finished at final address")
	}
	if loadDiagSeq != 0 {
		wv.scheduleLoadWatchdogs(loadDiagSeq)
	}
}

func (wv *WebView) setAudioPlaying(playing bool) {
	wv.audioPlaying.Store(playing)
	wv.mu.RLock()
	cb := wv.callbacks
	wv.mu.RUnlock()
	if cb != nil && cb.OnAudioStateChanged != nil {
		wv.runOnGTK(func() {
			cb.OnAudioStateChanged(playing)
		})
	}
}

func (wv *WebView) setSelectedText(text string) (string, bool) {
	wv.mu.Lock()
	previous := wv.selectedText
	changed := previous != text
	wv.selectedText = text
	wv.mu.Unlock()
	return previous, changed
}

func (wv *WebView) setEditableFocus(editable bool) {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	previous := wv.focusedEditable
	wv.focusedEditable = editable
	if editable {
		wv.selectionDebounceSeq++
		timer := wv.selectionDebounceTimer
		wv.selectionDebounceTimer = nil
		wv.mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		if previous != editable && wv.ctx != nil {
			logging.FromContext(wv.ctx).Debug().Bool("editable", editable).Msg("cef: editable focus changed")
		}
		return
	}
	wv.mu.Unlock()
	if previous != editable && wv.ctx != nil {
		logging.FromContext(wv.ctx).Debug().Bool("editable", editable).Msg("cef: editable focus changed")
	}
}

func (wv *WebView) cancelSelectionDebounce() {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	wv.selectionDebounceSeq++
	timer := wv.selectionDebounceTimer
	wv.selectionDebounceTimer = nil
	wv.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
}

func (wv *WebView) selectionDebounceInterval() time.Duration {
	if wv == nil || wv.selectionDebounceDelay == nil {
		return clipboardSelectionDebounceDelay
	}
	return *wv.selectionDebounceDelay
}

func (wv *WebView) selectionDebounceScheduler() func(time.Duration, func()) stoppableTimer {
	if wv != nil && wv.selectionDebounceSchedule != nil {
		return wv.selectionDebounceSchedule
	}
	return func(delay time.Duration, fn func()) stoppableTimer {
		return time.AfterFunc(delay, fn)
	}
}

func (wv *WebView) scheduleSelectionUpdate(text string) {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	wv.mu.Lock()
	if wv.destroyed.Load() || wv.engine == nil {
		wv.mu.Unlock()
		return
	}
	if wv.focusedEditable {
		wv.mu.Unlock()
		if wv.ctx != nil {
			logging.FromContext(wv.ctx).Debug().
				Int("text_len", len(text)).
				Msg("cef: selection auto-copy skipped while editable focused")
		}
		return
	}
	wv.selectionDebounceSeq++
	seq := wv.selectionDebounceSeq
	if timer := wv.selectionDebounceTimer; timer != nil {
		timer.Stop()
	}
	delay := wv.selectionDebounceInterval()
	if delay <= 0 {
		wv.selectionDebounceTimer = nil
		wv.mu.Unlock()
		wv.flushSelectionUpdate(seq, text)
		return
	}
	scheduler := wv.selectionDebounceScheduler()
	wv.selectionDebounceTimer = scheduler(delay, func() {
		wv.flushSelectionUpdate(seq, text)
	})
	wv.mu.Unlock()
}

func (wv *WebView) flushSelectionUpdate(seq uint64, text string) {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	wv.mu.Lock()
	if wv.destroyed.Load() || wv.focusedEditable || seq != wv.selectionDebounceSeq {
		wv.mu.Unlock()
		return
	}
	engine := wv.engine
	viewID := wv.id
	wv.mu.Unlock()
	if engine != nil {
		if wv.ctx != nil {
			logging.FromContext(wv.ctx).Debug().
				Int("text_len", len(text)).
				Msg("cef: debounced selection update flushed")
		}
		engine.handleClipboardSelectionUpdate(viewID, text)
	}
}

func (wv *WebView) selectedTextSnapshot() string {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.selectedText
}

func (wv *WebView) bridgeInputOptions() cef2gtk.InputOptions {
	if wv == nil {
		return cef2gtk.InputOptions{}
	}
	return cef2gtk.InputOptions{
		Scale: 0,
		Scroll: cef2gtk.ScrollOptions{
			WheelMultiplier:      wv.inputConfig.ScrollWheelMultiplier,
			PreciseMultiplier:    wv.inputConfig.ScrollPreciseMultiplier,
			HorizontalMultiplier: wv.inputConfig.ScrollHorizontalMultiplier,
			VerticalMultiplier:   wv.inputConfig.ScrollVerticalMultiplier,
			MaxDelta:             wv.inputConfig.ScrollMaxDelta,
		},
		OnMiddleClick: func(_, _ float64) bool {
			return wv.handleMiddleClickFromBridge()
		},
		OnScroll: wv.handleScrollInput,
		NavigationSwipe: cef2gtk.NavigationSwipeOptions{
			// Dumber handles thresholding and progress UI locally from OnScroll so
			// horizontal scrolling continues to reach CEF while history navigation
			// waits for a deliberate release past the visual threshold.
			Enabled: false,
		},
		SelectionText: wv.selectedTextSnapshot,
		OnClipboardShortcut: func(action, text string) {
			if wv.engine != nil {
				wv.engine.handleExplicitClipboardBridgeText(wv.id, action, text)
			}
		},
	}
}

// handleScrollInput is invoked by cef2gtk from GTK scroll event callbacks on
// the GTK main thread, which keeps lazy touchpad recognizer ownership
// single-threaded while every scroll event is still forwarded to CEF.
func (wv *WebView) handleScrollInput(event cef2gtk.ScrollEvent) cef2gtk.ScrollDecision {
	wv.handleTouchpadNavigationScroll(event)
	return wv.handleScrollInputDiagnostic(event)
}

func (wv *WebView) handleTouchpadNavigationScroll(event cef2gtk.ScrollEvent) {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	if !wv.inputConfig.TouchpadNavigationEnabled {
		return
	}
	wv.mu.Lock()
	if wv.destroyed.Load() {
		wv.mu.Unlock()
		return
	}
	if wv.touchpadNavigation == nil {
		wv.touchpadNavigation = newTouchpadNavigationRecognizer()
	}
	result := wv.touchpadNavigation.Handle(touchpadNavigationInput{
		Event:        event,
		Config:       wv.inputConfig,
		CanGoBack:    wv.canGoBack,
		CanGoForward: wv.canGoFwd,
		ViewWidth:    wv.touchpadNavigationViewWidth(),
	})
	wv.mu.Unlock()
	if result.HasIndicator {
		wv.emitTouchpadNavigationGesture(result.Indicator)
	}
	if result.HasAction {
		wv.handleNavigationSwipeAction(result.Action)
	}
}

func (wv *WebView) touchpadNavigationViewWidth() float64 {
	if wv == nil || wv.nativeWidget == nil {
		return 0
	}
	if width := wv.nativeWidget.GetAllocatedWidth(); width > 0 {
		return float64(width)
	}
	return float64(wv.nativeWidget.GetWidth())
}

func (wv *WebView) emitTouchpadNavigationGesture(gesture entity.TouchpadNavigationGesture) {
	if wv == nil {
		return
	}
	wv.mu.RLock()
	cb := wv.callbacks
	wv.mu.RUnlock()
	if cb != nil && cb.OnTouchpadNavigationGesture != nil {
		// Scroll input is delivered from GTK callbacks, so invoking directly keeps
		// progress and final indicators ordered before any navigation action.
		cb.OnTouchpadNavigationGesture(gesture)
	}
}

func (wv *WebView) handleScrollInputDiagnostic(event cef2gtk.ScrollEvent) cef2gtk.ScrollDecision {
	if event.Phase != cef2gtk.ScrollPhaseUpdate || (math.Abs(event.DX) == 0 && math.Abs(event.DY) == 0) {
		return cef2gtk.ScrollForwardToCEF
	}

	wv.inputDiagMu.Lock()
	wv.inputDiagEvents++
	wv.inputDiagDX += event.DX
	wv.inputDiagDY += event.DY
	wv.inputDiagDeltaXSum += int64(event.DeltaX)
	wv.inputDiagDeltaYSum += int64(event.DeltaY)
	now := time.Now()
	if !wv.inputDiagLastLog.IsZero() && now.Sub(wv.inputDiagLastLog) < time.Second {
		wv.inputDiagMu.Unlock()
		return cef2gtk.ScrollForwardToCEF
	}
	count := wv.inputDiagEvents
	dx, dy := wv.inputDiagDX, wv.inputDiagDY
	deltaX, deltaY := wv.inputDiagDeltaXSum, wv.inputDiagDeltaYSum
	wv.inputDiagEvents = 0
	wv.inputDiagDX = 0
	wv.inputDiagDY = 0
	wv.inputDiagDeltaXSum = 0
	wv.inputDiagDeltaYSum = 0
	wv.inputDiagLastLog = now
	wv.inputDiagMu.Unlock()

	logging.FromContext(wv.ctx).Trace().
		Uint64("webview_id", uint64(wv.id)).
		Int("events", count).
		Str("phase", fmt.Sprint(event.Phase)).
		Str("unit", fmt.Sprint(event.Unit)).
		Bool("unit_known", event.UnitKnown).
		Float64("dx_sum", dx).
		Float64("dy_sum", dy).
		Int64("cef_delta_x_sum", deltaX).
		Int64("cef_delta_y_sum", deltaY).
		Float64("scroll_wheel_multiplier", wv.inputConfig.ScrollWheelMultiplier).
		Float64("scroll_precise_multiplier", wv.inputConfig.ScrollPreciseMultiplier).
		Float64("scroll_horizontal_multiplier", wv.inputConfig.ScrollHorizontalMultiplier).
		Float64("scroll_vertical_multiplier", wv.inputConfig.ScrollVerticalMultiplier).
		Int32("scroll_max_delta", wv.inputConfig.ScrollMaxDelta).
		Bool("nav_enabled", wv.inputConfig.TouchpadNavigationEnabled).
		Float64("nav_min_delta", wv.inputConfig.TouchpadNavigationMinDelta).
		Float64("nav_max_vertical_ratio", wv.inputConfig.TouchpadNavigationMaxVerticalRatio).
		Bool("can_go_back", wv.CanGoBack()).
		Bool("can_go_forward", wv.CanGoForward()).
		Msg("cef: touchpad scroll diagnostic")

	return cef2gtk.ScrollForwardToCEF
}

func (wv *WebView) handleNavigationSwipeAction(action cef2gtk.NavigationSwipeAction) {
	if wv.ctx != nil {
		logging.FromContext(wv.ctx).Debug().
			Uint64("webview_id", uint64(wv.id)).
			Str("action", fmt.Sprint(action)).
			Bool("can_go_back", wv.CanGoBack()).
			Bool("can_go_forward", wv.CanGoForward()).
			Msg("cef: touchpad navigation swipe recognized")
	}

	switch action {
	case cef2gtk.NavigationSwipeBack:
		if err := wv.GoBack(wv.ctx); err != nil && wv.ctx != nil {
			logging.FromContext(wv.ctx).Warn().Err(err).Msg("touchpad back navigation failed")
		}
	case cef2gtk.NavigationSwipeForward:
		if err := wv.GoForward(wv.ctx); err != nil && wv.ctx != nil {
			logging.FromContext(wv.ctx).Warn().Err(err).Msg("touchpad forward navigation failed")
		}
	}
}

func (wv *WebView) viewBridgeScale() float64 {
	if wv == nil || wv.viewBridge == nil {
		return 1
	}
	return normalizeScale(float64(wv.viewBridge.DeviceScaleFactor()))
}

func (wv *WebView) osrBackingScaleFactor() float64 {
	if wv == nil || wv.viewBridge == nil {
		return 1
	}
	return normalizeScale(wv.viewBridge.OSRBackingScaleFactor())
}

func (wv *WebView) recordAppliedZoomScaleRatio(surfaceScale, backingScale float64) {
	if wv == nil {
		return
	}
	wv.lastAppliedZoomScaleRatioBits.Store(math.Float64bits(zoomScaleRatio(surfaceScale, backingScale)))
}

func (wv *WebView) shouldReapplyZoomForScaleRatio(surfaceScale, backingScale float64) bool {
	if wv == nil {
		return false
	}
	ratio := zoomScaleRatio(surfaceScale, backingScale)
	bits := wv.lastAppliedZoomScaleRatioBits.Load()
	if bits == 0 {
		return true
	}
	return math.Float64frombits(bits) != ratio
}

func (wv *WebView) handleMiddleClickFromBridge() bool {
	if wv == nil {
		return false
	}
	wv.mu.RLock()
	hoverURI := wv.lastHoverURI
	cb := wv.callbacks
	wv.mu.RUnlock()
	if hoverURI == "" || cb == nil || cb.OnLinkMiddleClick == nil {
		return false
	}
	uri := hoverURI
	wv.runOnGTK(func() {
		cb.OnLinkMiddleClick(uri)
	})
	return true
}

func (wv *WebView) scheduleLoadWatchdogs(loadSeq uint64) {
	for _, delay := range cefLoadWatchdogDelays {
		d := delay
		time.AfterFunc(d, func() {
			wv.logLoadWatchdog(loadSeq, d)
		})
	}
}

func (wv *WebView) logLoadWatchdog(loadSeq uint64, delay time.Duration) {
	if wv == nil || wv.destroyed.Load() || wv.ctx == nil {
		return
	}

	wv.mu.RLock()
	currentSeq := wv.loadDiagSeq
	loading := wv.isLoading
	uri := wv.uri
	title := wv.title
	progress := wv.progress
	pendingURI := wv.pendingURI
	startedAt := wv.loadDiagStartedAt
	lastProgressAt := wv.loadDiagLastProgressAt
	lastAddressAt := wv.loadDiagLastAddressAt
	lastLoadStateAt := wv.loadDiagLastLoadStateAt
	wv.mu.RUnlock()

	if loadSeq != currentSeq || !loading {
		return
	}

	var acceleratedPaints, unsupportedPaints, acceleratedPaintErrors, importFailures, renderFailures int
	var surfaceWidth, surfaceHeight int32 = 1, 1
	var surfaceScale float32 = 1
	if wv.viewBridge != nil {
		diag := wv.viewBridge.Diagnostics()
		acceleratedPaints = diag.AcceleratedPaints
		unsupportedPaints = diag.UnsupportedPaints
		acceleratedPaintErrors = diag.AcceleratedPaintErrors
		importFailures = diag.ImportFailures
		renderFailures = diag.RenderFailures
		surfaceWidth, surfaceHeight = wv.viewBridge.Size()
		surfaceScale = wv.viewBridge.DeviceScaleFactor()
	}

	now := time.Now()
	logging.FromContext(wv.ctx).Debug().
		Uint64("load_seq", loadSeq).
		Int64("watch_delay_ms", delay.Milliseconds()).
		Int64("since_start_ms", sinceTimestampMs(startedAt, now)).
		Int64("since_progress_ms", sinceTimestampMs(lastProgressAt, now)).
		Int64("since_address_ms", sinceTimestampMs(lastAddressAt, now)).
		Int64("since_load_state_ms", sinceTimestampMs(lastLoadStateAt, now)).
		Str("uri", logging.TruncateURL(uri, logging.PermissionLogURLMaxLen)).
		Str("title", title).
		Float64("progress", progress).
		Str("pending_uri", logging.TruncateURL(pendingURI, logging.PermissionLogURLMaxLen)).
		Int32("surface_width", surfaceWidth).
		Int32("surface_height", surfaceHeight).
		Float32("surface_scale", surfaceScale).
		Int("accelerated_paints", acceleratedPaints).
		Int("unsupported_paints", unsupportedPaints).
		Int("accelerated_paint_errors", acceleratedPaintErrors).
		Int("import_failures", importFailures).
		Int("render_failures", renderFailures).
		Msg("cef: loading watchdog")
}

func (wv *WebView) pendingNavigationURI() string {
	if wv == nil {
		return ""
	}
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.pendingURI
}

func (wv *WebView) setPendingNavigationLocked(uri string, at time.Time) {
	wv.pendingURI = uri
	wv.pendingURIStartedAt = time.Time{}
	if uri == "" {
		wv.pendingURISetAt = time.Time{}
		return
	}
	wv.pendingURISetAt = at
}

func (wv *WebView) markPendingNavigationStartedLocked(uri string, at time.Time) {
	if !pendingURIEquivalent(wv.pendingURI, uri) {
		return
	}
	wv.pendingURIStartedAt = at
}

func (wv *WebView) clearPendingNavigationLocked() string {
	cleared := wv.pendingURI
	wv.pendingURI = ""
	wv.pendingURISetAt = time.Time{}
	wv.pendingURIStartedAt = time.Time{}
	return cleared
}

func (wv *WebView) clearPendingNavigationIfEquivalentLocked(uri string) bool {
	if !pendingURIEquivalent(wv.pendingURI, uri) {
		return false
	}
	wv.clearPendingNavigationLocked()
	return true
}

func (wv *WebView) hasObservedAddressForPendingNavigationLocked() bool {
	if strings.TrimSpace(wv.pendingURI) == "" || strings.TrimSpace(wv.uri) == "" {
		return false
	}
	if wv.loadDiagLastAddressAt.IsZero() || wv.pendingURIStartedAt.IsZero() {
		return false
	}
	return !wv.loadDiagLastAddressAt.Before(wv.pendingURIStartedAt)
}

func (wv *WebView) schedulePendingNavigationReplay(attempt int) {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	task := cefNewTask(cefTaskFunc(func() {
		wv.replayPendingNavigation(attempt)
	}))
	if task == nil {
		return
	}
	var result int32
	if attempt <= 0 {
		result = cefPostTask(purecef.ThreadIDTidUi, task)
	} else {
		result = cefPostDelayedTask(purecef.ThreadIDTidUi, task, int64(pendingNavigationRetryDelay/time.Millisecond))
	}
	if result == 1 {
		return
	}
	if wv.ctx != nil {
		log := logging.FromContext(wv.ctx).Warn().
			Int("attempt", attempt).
			Int32("result", result)
		if attempt >= pendingNavigationMaxRetries {
			log.Msg("cef: failed to schedule pending navigation replay; retries exhausted")
		} else {
			log.Msg("cef: failed to schedule pending navigation replay; retrying")
		}
	}
	if attempt >= pendingNavigationMaxRetries {
		return
	}
	cefScheduleAfter(pendingNavigationRetryDelay, func() {
		wv.schedulePendingNavigationReplay(attempt + 1)
	})
}

func (wv *WebView) replayPendingNavigation(attempt int) {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	wv.mu.RLock()
	uri := wv.pendingURI
	browser := wv.browser
	wv.mu.RUnlock()
	if uri == "" || browser == nil {
		return
	}
	frame := browser.GetMainFrame()
	if frame == nil {
		if attempt >= pendingNavigationMaxRetries {
			if wv.ctx != nil {
				logging.FromContext(wv.ctx).Warn().
					Int("attempt", attempt).
					Str("uri", logging.TruncateURL(uri, logging.PermissionLogURLMaxLen)).
					Msg("cef: pending navigation replay exhausted without main frame")
			}
			return
		}
		if wv.ctx != nil {
			logging.FromContext(wv.ctx).Debug().
				Int("attempt", attempt).
				Str("uri", logging.TruncateURL(uri, logging.PermissionLogURLMaxLen)).
				Msg("cef: pending navigation replay waiting for main frame")
		}
		wv.schedulePendingNavigationReplay(attempt + 1)
		return
	}
	currentURL := frame.GetURL()
	if pendingURIEquivalent(currentURL, uri) {
		wv.mu.Lock()
		wv.clearPendingNavigationIfEquivalentLocked(uri)
		wv.mu.Unlock()
		if wv.ctx != nil {
			logging.FromContext(wv.ctx).Debug().
				Int("attempt", attempt).
				Str("uri", logging.TruncateURL(uri, logging.PermissionLogURLMaxLen)).
				Msg("cef: pending navigation already active")
		}
		return
	}
	wv.mu.Lock()
	wv.markPendingNavigationStartedLocked(uri, time.Now())
	wv.mu.Unlock()
	frame.LoadURL(uri)
	if wv.ctx != nil {
		logging.FromContext(wv.ctx).Debug().
			Int("attempt", attempt).
			Str("uri", logging.TruncateURL(uri, logging.PermissionLogURLMaxLen)).
			Msg("cef: replayed pending navigation")
	}
}

func pendingURIEquivalent(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return toConceptualInternalURL(strings.TrimSpace(a)) == toConceptualInternalURL(strings.TrimSpace(b))
}

func sinceTimestampMs(ts, now time.Time) int64 {
	if ts.IsZero() {
		return -1
	}
	return now.Sub(ts).Milliseconds()
}

func (wv *WebView) updateHoverURI(uri string) {
	wv.mu.Lock()
	wv.lastHoverURI = uri
	wv.mu.Unlock()
}

// scheduleZoomRefresh posts two invalidation requests to the CEF UI thread:
//   - 16ms delay: one frame at 60fps, gives the renderer time to process
//     the zoom IPC before we request the first repaint.
//   - 48ms delay: three frames at 60fps, a second invalidation to catch
//     any compositor frame that was still in-flight during the first.
//
// These values are empirically tuned for CEF's async zoom application.
func (wv *WebView) scheduleZoomRefresh() {
	for _, delayMs := range [...]int64{16, 48} {
		task := cefNewTask(cefTaskFunc(func() {
			if wv.destroyed.Load() {
				return
			}
			wv.mu.RLock()
			host := wv.host
			wv.mu.RUnlock()
			if host == nil {
				return
			}
			host.Invalidate(purecef.PaintElementTypePetView)
		}))
		if task == nil {
			continue
		}
		cefPostDelayedTask(purecef.ThreadIDTidUi, task, delayMs)
	}
}

// scheduleZoomReadback posts two diagnostic readbacks to verify zoom applied:
//   - 0ms delay: immediate check on the CEF UI thread to log the zoom level
//     right after SetZoomLevel returns (usually still the old value).
//   - 64ms delay: four frames at 60fps, enough time for the renderer to
//     process the zoom IPC and reflect the new level back to the browser.
//
// These values are empirically tuned for CEF's async zoom application.
func (wv *WebView) scheduleZoomReadback(expectedFactor, expectedLevel float64) {
	for _, delayMs := range [...]int64{0, 64} {
		task := cefNewTask(cefTaskFunc(func() {
			if wv.destroyed.Load() {
				return
			}
			wv.mu.RLock()
			host := wv.host
			wv.mu.RUnlock()
			if host == nil {
				return
			}
			surfaceScale := wv.viewBridgeScale()
			backingScale := wv.osrBackingScaleFactor()
			actualLevel := host.GetZoomLevel()
			logging.FromContext(wv.ctx).Debug().
				Int64("delay_ms", delayMs).
				Float64("expected_factor", expectedFactor).
				Float64("expected_cef_level", expectedLevel).
				Float64("actual_factor", pageZoomFromCEFAndScaleLevel(actualLevel, surfaceScale, backingScale)).
				Float64("actual_cef_factor", factorFromCEFZoom(actualLevel)).
				Float64("actual_cef_level", actualLevel).
				Float64("osr_backing_scale", backingScale).
				Float64("surface_scale", surfaceScale).
				Msg("cef: zoom level readback")
		}))
		if task == nil {
			continue
		}
		cefPostDelayedTask(purecef.ThreadIDTidUi, task, delayMs)
	}
}

func (wv *WebView) reapplyCurrentZoomForBackingScale(reason string) {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	wv.mu.RLock()
	host := wv.host
	wv.mu.RUnlock()
	if host == nil {
		return
	}
	factor := wv.GetZoomLevel()
	surfaceScale := wv.viewBridgeScale()
	backingScale := wv.osrBackingScaleFactor()
	cefLevel := cefZoomFromPageAndScaleFactors(factor, surfaceScale, backingScale)
	wv.applyCEFZoomLevel(host, factor, cefLevel, surfaceScale, backingScale)
	logging.FromContext(wv.ctx).Debug().
		Str("reason", reason).
		Float64("factor", factor).
		Float64("cef_level", cefLevel).
		Float64("osr_backing_scale", backingScale).
		Float64("surface_scale", surfaceScale).
		Msg("cef: reapplied zoom after OSR backing scale change")
}

func (wv *WebView) scheduleStartBeginFrameLoop() {
	if !externalBeginFrameEnabled() {
		return
	}
	wv.runOnGTK(func() {
		wv.startBeginFrameLoop()
	})
}

func (wv *WebView) scheduleStopBeginFrameLoop() {
	wv.runOnGTK(func() {
		wv.stopBeginFrameLoop()
	})
}

func (wv *WebView) startBeginFrameLoop() {
	if wv.destroyed.Load() || wv.viewBridge == nil || wv.viewBridge.Widget() == nil {
		return
	}

	wv.mu.Lock()
	if wv.beginFrameTickID != 0 || wv.host == nil {
		wv.mu.Unlock()
		return
	}

	widget := wv.viewBridge.Widget()
	cb := new(gtk.TickCallback)
	*cb = func(_, _, _ uintptr) bool {
		if wv.destroyed.Load() {
			wv.releaseBeginFrameTickCallback()
			return false
		}
		wv.mu.RLock()
		host := wv.host
		wv.mu.RUnlock()
		if host == nil {
			wv.releaseBeginFrameTickCallback()
			return false
		}
		host.SendExternalBeginFrame()
		if wv.viewBridge != nil {
			wv.viewBridge.RecordExternalBeginFrameSent()
		}
		return true
	}
	wv.beginFrameTick = cb
	wv.beginFrameTickID = widget.AddTickCallback(cb, 0, nil)
	host := wv.host
	wv.mu.Unlock()

	if host != nil {
		host.SendExternalBeginFrame()
		if wv.viewBridge != nil {
			wv.viewBridge.RecordExternalBeginFrameSent()
		}
	}
}

func (wv *WebView) stopBeginFrameLoop() {
	wv.mu.Lock()
	tickID := wv.beginFrameTickID
	callback := wv.beginFrameTick
	wv.beginFrameTickID = 0
	wv.beginFrameTick = nil
	bridge := wv.viewBridge
	wv.mu.Unlock()

	if tickID != 0 && bridge != nil {
		if widget := bridge.Widget(); widget != nil {
			widget.RemoveTickCallback(tickID)
		}
	}
	if callback != nil {
		_ = unrefTickCallback(callback)
	}
}

// releaseBeginFrameTickCallback releases the purego callback slot after GTK
// removes a source because its callback returned false.
func (wv *WebView) releaseBeginFrameTickCallback() {
	wv.mu.Lock()
	callback := wv.beginFrameTick
	wv.beginFrameTickID = 0
	wv.beginFrameTick = nil
	wv.mu.Unlock()
	if callback != nil {
		_ = unrefTickCallback(callback)
	}
}

func (wv *WebView) runOnGTK(fn func()) {
	if fn == nil {
		return
	}
	// When engine is nil (test/bootstrap), call directly.
	if wv.engine == nil {
		fn()
		return
	}

	// Heap-allocate the callback so it survives until glib invokes it.
	cb := new(glib.SourceOnceFunc)
	*cb = func(_ uintptr) {
		fn()
	}
	glib.IdleAddOnce(cb, 0)
}

func (wv *WebView) isOnGTKThread() bool {
	mainContext := glib.MainContextDefault()
	return mainContext != nil && mainContext.IsOwner()
}

// runOnGTKSync executes fn on the GTK main context and waits for completion.
// Callers already running on the GTK thread execute inline to avoid
// self-deadlocking while waiting for an IdleAddOnce callback. Calls are bounded
// so a wedged GTK loop cannot block CEF callbacks forever.
func (wv *WebView) runOnGTKSync(fn func()) syncdispatch.SyncDispatchResult {
	return wv.runOnGTKSyncLabel("cef.gtk_sync", fn)
}

func (wv *WebView) runOnGTKSyncLabel(label string, fn func()) syncdispatch.SyncDispatchResult {
	return wv.runOnGTKSyncLabelAllowLateStart(label, false, fn)
}

func (wv *WebView) runOnGTKSyncLabelAllowLateStart(
	label string,
	allowLateStartAfterTimeout bool,
	fn func(),
) syncdispatch.SyncDispatchResult {
	if wv == nil {
		if fn != nil {
			fn()
		}
		return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchInline}
	}
	if wv.engine == nil && wv.gtkSyncDispatch == nil && wv.gtkSyncIsOwner == nil {
		if fn != nil {
			fn()
		}
		return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchInline}
	}

	isOwner := wv.gtkSyncIsOwner
	if isOwner == nil {
		isOwner = wv.isOnGTKThread
	}
	dispatch := wv.gtkSyncDispatch
	if dispatch == nil {
		dispatch = wv.runOnGTK
	}
	timeout := wv.gtkSyncTimeout
	if timeout <= 0 {
		timeout = cefGTKSyncDispatchTimeout
	}

	result := syncdispatch.RunSynchronousDispatch(syncdispatch.SyncDispatchOptions{
		Label:                      label,
		Timeout:                    timeout,
		IsOwner:                    isOwner,
		Dispatch:                   dispatch,
		AllowLateStartAfterTimeout: allowLateStartAfterTimeout,
	}, fn)
	wv.logGTKSyncDispatchResult(result)
	return result
}

func (wv *WebView) logGTKSyncDispatchResult(result syncdispatch.SyncDispatchResult) {
	if wv == nil {
		return
	}
	logger := logging.FromContext(wv.ctx)
	switch result.Status {
	case syncdispatch.SyncDispatchTimedOut:
		logger.Warn().
			Str("dispatch_label", result.Label).
			Dur("elapsed", result.Elapsed).
			Dur("timeout", wv.effectiveGTKSyncTimeout()).
			Uint64("webview_id", uint64(wv.id)).
			Msg("cef: GTK synchronous dispatch timed out before callback started")
	case syncdispatch.SyncDispatchCompletedAfterTimeout:
		logger.Warn().
			Str("dispatch_label", result.Label).
			Dur("elapsed", result.Elapsed).
			Dur("timeout", wv.effectiveGTKSyncTimeout()).
			Uint64("webview_id", uint64(wv.id)).
			Msg("cef: GTK synchronous dispatch completed after timeout")
	case syncdispatch.SyncDispatchQueuedAfterTimeout:
		logger.Warn().
			Str("dispatch_label", result.Label).
			Dur("elapsed", result.Elapsed).
			Dur("timeout", wv.effectiveGTKSyncTimeout()).
			Uint64("webview_id", uint64(wv.id)).
			Msg("cef: GTK synchronous dispatch left queued after timeout")
	case syncdispatch.SyncDispatchCompleted:
		if result.Elapsed >= cefGTKSyncDispatchSlowThreshold {
			logger.Debug().
				Str("dispatch_label", result.Label).
				Dur("elapsed", result.Elapsed).
				Uint64("webview_id", uint64(wv.id)).
				Msg("cef: GTK synchronous dispatch completed slowly")
		}
	}
}

func (wv *WebView) effectiveGTKSyncTimeout() time.Duration {
	if wv != nil && wv.gtkSyncTimeout > 0 {
		return wv.gtkSyncTimeout
	}
	return cefGTKSyncDispatchTimeout
}

func errGTKSyncDispatchIncomplete(operation string, result syncdispatch.SyncDispatchResult) error {
	return fmt.Errorf("%s: GTK dispatch did not complete: %s", operation, result.Status)
}

func (wv *WebView) runCloseCallbacks() {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	callbacks := append([]func(){}, wv.closeCallbacks...)
	wv.closeCallbacks = nil
	wv.mu.Unlock()
	if len(callbacks) == 0 {
		return
	}
	wv.runOnGTK(func() {
		for _, fn := range callbacks {
			if fn != nil {
				fn()
			}
		}
	})
}

func (wv *WebView) runNavigationCallbacks(uri string) {
	if wv == nil {
		return
	}
	wv.mu.RLock()
	callbacks := append([]func(string){}, wv.navigationCallbacks...)
	wv.mu.RUnlock()
	if len(callbacks) == 0 {
		return
	}
	wv.runOnGTK(func() {
		for _, fn := range callbacks {
			if fn != nil {
				fn(uri)
			}
		}
	})
}

func (wv *WebView) shouldStartBrowserCreateFromSizeObserver() bool {
	if wv == nil {
		return false
	}
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.pendingCreate != nil && !wv.initialBrowserCreateResizeHandled
}

func (wv *WebView) markInitialBrowserCreateResizeHandled() {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	defer wv.mu.Unlock()
	wv.initialBrowserCreateResizeHandled = true
}

func (wv *WebView) takePendingCreate() *pendingBrowserCreate {
	wv.mu.Lock()
	defer wv.mu.Unlock()
	pc := wv.pendingCreate
	wv.pendingCreate = nil
	return pc
}

// closeAudioStream detaches then closes the active output stream. Detaching
// under the lock keeps packet snapshots consistent without holding the lock
// while PipeWire shutdown runs; the port guarantees Close/Write safety.
func (wv *WebView) closeAudioStream() {
	wv.audioStreamMu.Lock()
	stream := wv.activeAudioStream
	wv.activeAudioStream = nil
	wv.audioStreamMu.Unlock()

	if stream != nil {
		if err := stream.Close(); err != nil && wv.ctx != nil {
			logging.FromContext(wv.ctx).Debug().Err(err).Msg("cef: error closing audio stream")
		}
	}
}
