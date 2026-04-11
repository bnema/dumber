package cef

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time interface checks.
var (
	_ port.WebView              = (*WebView)(nil)
	_ port.NativeWidgetProvider = (*WebView)(nil)
	_ port.DevToolsOpener       = (*WebView)(nil)
)

// errDestroyed is returned when an operation is attempted on a destroyed WebView.
var errDestroyed = errors.New("cef: webview is destroyed")

// errNoBrowser is returned when the browser has not been created yet.
var errNoBrowser = errors.New("cef: browser not yet created")

// WebView implements port.WebView using a CEF off-screen browser rendered
// through a renderPipeline and driven by an inputBridge.
type WebView struct {
	id       port.WebViewID
	ctx      context.Context
	engine   *Engine
	browser  purecef.Browser
	host     purecef.BrowserHost
	client   purecef.Client // prevent GC from collecting the client before CEF AddRef's it
	pipeline *renderPipeline
	input    *inputBridge
	handlers *handlerSet
	findCtrl *cefFindController

	// beginFrameTick drives CEF external BeginFrame requests while the GTK
	// widget is visible. Access is guarded by mu.
	beginFrameTick   *gtk.TickCallback
	beginFrameTickID uint

	// pendingCreate holds browser creation params until the GL area is realized.
	pendingCreate *pendingBrowserCreate

	// pendingURI is set when LoadURI is called before the browser exists.
	pendingURI string

	// crashCount tracks consecutive renderer crashes to prevent infinite
	// crash → redirect → crash loops.
	crashCount atomic.Int32

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

	// Last known hover URI for middle-click → new tab.
	lastHoverURI string

	// Atomic state.
	destroyed    atomic.Bool
	fullscreen   atomic.Bool
	generation   atomic.Uint64
	audioPlaying atomic.Bool
	zoomFactor   atomic.Value // float64, initialized to 1.0

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
	windowInfo *purecef.WindowInfo
	client     purecef.Client
	settings   *purecef.BrowserSettings
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
	if browser == nil {
		// Browser not yet created — queue the URI for OnAfterCreated.
		wv.pendingURI = actualURI
		wv.mu.Unlock()
		return nil
	}
	wv.mu.Unlock()
	if frame := browser.GetMainFrame(); frame != nil {
		frame.LoadURL(actualURI)
	}
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
	cefLevel := cefZoomFromFactor(factor)
	logging.FromContext(wv.ctx).Debug().
		Float64("factor", factor).
		Float64("cef_level", cefLevel).
		Msg("cef: SetZoomLevel")
	host.SetZoomLevel(cefLevel)
	wv.zoomFactor.Store(factor)
	// Force CEF to produce a new frame at the new zoom level. In OSR mode,
	// SetZoomLevel changes the Blink layout zoom but doesn't guarantee a
	// repaint. WasResized is a no-op when view dimensions haven't changed.
	// NotifyScreenInfoChanged forces surface ID invalidation + a full
	// SynchronizeVisualProperties cycle, which makes the renderer produce
	// a new compositor frame at the new zoom level.
	host.NotifyScreenInfoChanged()
	// Zoom is applied asynchronously in the renderer process. Request a couple
	// of follow-up refreshes on the CEF UI thread so OSR captures the updated
	// compositor frame after the zoom IPC has been processed.
	wv.scheduleZoomRefresh()
	wv.scheduleZoomReadback(factor, cefLevel)
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

// ---------------------------------------------------------------------------
// JavaScript / Appearance
// ---------------------------------------------------------------------------

// RunJavaScript executes a script in the main world. Fire-and-forget.
func (wv *WebView) RunJavaScript(_ context.Context, script string) {
	if wv.destroyed.Load() {
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
	wv.closeAudioStream()
	wv.scheduleStopBeginFrameLoop()
	wv.mu.RLock()
	host := wv.host
	wv.mu.RUnlock()
	if host != nil {
		host.CloseBrowser(1)
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
	uri = toConceptualInternalURL(uri)
	wv.mu.Lock()
	wv.uri = uri
	cb := wv.callbacks
	wv.mu.Unlock()

	if cb != nil && cb.OnURIChanged != nil {
		wv.runOnGTK(func() {
			cb.OnURIChanged(uri)
		})
	}
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
	wv.mu.Lock()
	wv.progress = progress
	cb := wv.callbacks
	wv.mu.Unlock()

	if cb != nil && cb.OnProgressChanged != nil {
		wv.runOnGTK(func() {
			cb.OnProgressChanged(progress)
		})
	}
}

func (wv *WebView) updateLoadState(loading, back, fwd bool) {
	wv.mu.Lock()
	wv.isLoading = loading
	wv.canGoBack = back
	wv.canGoFwd = fwd
	wv.mu.Unlock()
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
		purecef.PostDelayedTask(purecef.ThreadIDTidUi, purecef.NewTask(cefTaskFunc(func() {
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
		})), delayMs)
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
		purecef.PostDelayedTask(purecef.ThreadIDTidUi, purecef.NewTask(cefTaskFunc(func() {
			if wv.destroyed.Load() {
				return
			}
			wv.mu.RLock()
			host := wv.host
			wv.mu.RUnlock()
			if host == nil {
				return
			}
			actualLevel := host.GetZoomLevel()
			logging.FromContext(wv.ctx).Debug().
				Int64("delay_ms", delayMs).
				Float64("expected_factor", expectedFactor).
				Float64("expected_cef_level", expectedLevel).
				Float64("actual_factor", factorFromCEFZoom(actualLevel)).
				Float64("actual_cef_level", actualLevel).
				Msg("cef: zoom level readback")
		})), delayMs)
	}
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
	if wv.destroyed.Load() || wv.pipeline == nil || wv.pipeline.glArea == nil {
		return
	}

	wv.mu.Lock()
	if wv.beginFrameTickID != 0 || wv.host == nil {
		wv.mu.Unlock()
		return
	}

	glArea := wv.pipeline.glArea
	cb := new(gtk.TickCallback)
	*cb = func(_, _, _ uintptr) bool {
		if wv.destroyed.Load() {
			return false
		}
		wv.mu.RLock()
		host := wv.host
		wv.mu.RUnlock()
		if host == nil {
			return false
		}
		host.SendExternalBeginFrame()
		return true
	}
	wv.beginFrameTick = cb
	wv.beginFrameTickID = glArea.AddTickCallback(cb, 0, nil)
	host := wv.host
	wv.mu.Unlock()

	if host != nil {
		host.SendExternalBeginFrame()
	}
}

func (wv *WebView) stopBeginFrameLoop() {
	if wv.pipeline == nil || wv.pipeline.glArea == nil {
		return
	}

	wv.mu.Lock()
	tickID := wv.beginFrameTickID
	wv.beginFrameTickID = 0
	wv.beginFrameTick = nil
	glArea := wv.pipeline.glArea
	wv.mu.Unlock()

	if tickID != 0 {
		glArea.RemoveTickCallback(tickID)
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

func (wv *WebView) takePendingCreate() *pendingBrowserCreate {
	wv.mu.Lock()
	defer wv.mu.Unlock()
	pc := wv.pendingCreate
	wv.pendingCreate = nil
	return pc
}

// closeAudioStream closes and clears the active audio output stream.
// This is safe to call even if no stream is active.
func (wv *WebView) closeAudioStream() {
	wv.audioStreamMu.Lock()
	defer wv.audioStreamMu.Unlock()

	if wv.activeAudioStream != nil {
		if err := wv.activeAudioStream.Close(); err != nil {
			if wv.ctx != nil {
				logging.FromContext(wv.ctx).Debug().
					Err(err).
					Msg("cef: error closing audio stream")
			}
		}
		wv.activeAudioStream = nil
	}
}
