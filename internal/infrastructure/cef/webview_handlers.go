package cef

import (
	"strings"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/transcoder"
	"github.com/bnema/dumber/internal/logging"
)

// Console message markers used by injected JavaScript for log filtering.
const (
	consoleMarkerVideoDiag        = "[VIDEO-DIAG]"
	consoleMarkerRedditVideoPatch = "[REDDIT-VIDEO-PATCH]"
)

// handlerSet implements all CEF handler interfaces and dispatches events to the
// owning WebView. A single struct is used so that the Client's Get*Handler
// methods can return the same receiver, avoiding extra allocations.
type handlerSet struct {
	wv                       *WebView
	enableContextMenuHandler bool
	transcodingHandler       purecef.ResourceRequestHandler
}

// Compile-time interface checks.
var (
	_ purecef.SafeClient          = (*handlerSet)(nil)
	_ purecef.RenderHandler       = (*handlerSet)(nil)
	_ purecef.DisplayHandler      = (*handlerSet)(nil)
	_ purecef.LoadHandler         = (*handlerSet)(nil)
	_ purecef.SafeLifeSpanHandler = (*handlerSet)(nil)
	_ purecef.RequestHandler      = (*handlerSet)(nil)
	_ purecef.AudioHandler        = (*handlerSet)(nil)
	_ purecef.ContextMenuHandler  = (*handlerSet)(nil)
	_ purecef.FindHandler         = (*handlerSet)(nil)
)

// ===========================================================================
// Client
// ===========================================================================

func (h *handlerSet) GetAudioHandler() purecef.AudioHandler {
	// Always return the audio handler — without it, CEF has no audio output
	// path and refuses to start media decoding, breaking all video playback.
	// The handler accepts audio but discards packets (no-op) until a real
	// audio backend (PipeWire/PulseAudio) is wired up.
	return h
}
func (h *handlerSet) GetCommandHandler() purecef.CommandHandler { return nil }
func (h *handlerSet) GetContextMenuHandler() purecef.ContextMenuHandler {
	if h.enableContextMenuHandler {
		return h
	}
	return nil
}
func (h *handlerSet) GetDialogHandler() purecef.DialogHandler         { return nil }
func (h *handlerSet) GetDisplayHandler() purecef.DisplayHandler       { return h }
func (h *handlerSet) GetDownloadHandler() purecef.DownloadHandler     { return nil }
func (h *handlerSet) GetDragHandler() purecef.DragHandler             { return nil }
func (h *handlerSet) GetFindHandler() purecef.FindHandler             { return h }
func (h *handlerSet) GetFocusHandler() purecef.FocusHandler           { return nil }
func (h *handlerSet) GetFrameHandler() purecef.FrameHandler           { return nil }
func (h *handlerSet) GetPermissionHandler() purecef.PermissionHandler { return nil }
func (h *handlerSet) GetJsdialogHandler() purecef.JsdialogHandler     { return nil }
func (h *handlerSet) GetKeyboardHandler() purecef.KeyboardHandler     { return nil }
func (h *handlerSet) GetLifeSpanHandler() purecef.SafeLifeSpanHandler { return h }
func (h *handlerSet) GetLoadHandler() purecef.LoadHandler             { return h }
func (h *handlerSet) GetPrintHandler() purecef.PrintHandler           { return nil }
func (h *handlerSet) GetRenderHandler() purecef.RenderHandler         { return h }
func (h *handlerSet) GetRequestHandler() purecef.RequestHandler       { return h }

func (h *handlerSet) OnProcessMessageReceived(_ purecef.Browser, _ purecef.Frame, _ purecef.ProcessID, _ purecef.ProcessMessage) int32 {
	return 0
}

// ===========================================================================
// RenderHandler (17 methods)
// ===========================================================================

func (h *handlerSet) GetAccessibilityHandler() purecef.AccessibilityHandler { return nil }

func (h *handlerSet) GetRootScreenRect(_ purecef.Browser, _ *purecef.Rect) int32 { return 0 }

// GetViewRect fills the rect struct with the pipeline dimensions.
func (h *handlerSet) GetViewRect(_ purecef.Browser, rect *purecef.Rect) {
	if rect == nil {
		return
	}
	callSeq := h.wv.pipeline.nextViewRectSeq()
	w, ht, s := h.wv.pipeline.viewRectSize()

	// CEF expects view geometry in DIP coordinates while OnPaint dimensions are
	// in device pixels. The render pipeline tracks device pixels, so convert
	// back to DIP before answering GetViewRect/GetScreenInfo.
	w /= s
	ht /= s

	// CEF requires a non-empty rect. Return a 1x1 fallback if the GL area has
	// not been realized yet.
	if w <= 0 {
		w = 1
	}
	if ht <= 0 {
		ht = 1
	}

	rect.X = 0
	rect.Y = 0
	rect.Width = w
	rect.Height = ht
	if h.wv != nil && h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Debug().
			Uint64("view_rect_seq", callSeq).
			Int32("width", rect.Width).
			Int32("height", rect.Height).
			Int32("scale", s).
			Msg("cef: GetViewRect")
	}
}

func (h *handlerSet) GetScreenPoint(_ purecef.Browser, _, _ int32, _, _ *int32) int32 {
	return 0
}

func (h *handlerSet) GetScreenInfo(_ purecef.Browser, info *purecef.ScreenInfo) int32 {
	if info == nil {
		return 0
	}
	callSeq := h.wv.pipeline.nextScreenInfoSeq()
	w, ht, s := h.wv.pipeline.viewRectSize()
	w /= s
	ht /= s
	if w <= 0 {
		w = 1
	}
	if ht <= 0 {
		ht = 1
	}

	const (
		screenDepth             = 24
		screenDepthPerComponent = 8
	)
	r := purecef.Rect{X: 0, Y: 0, Width: w, Height: ht}
	*info = purecef.ScreenInfo{
		DeviceScaleFactor: float32(s),
		Depth:             screenDepth,
		DepthPerComponent: screenDepthPerComponent,
		Rect:              r,
		AvailableRect:     r,
	}
	if h.wv != nil && h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Debug().
			Uint64("screen_info_seq", callSeq).
			Int32("width", w).
			Int32("height", ht).
			Int32("scale", s).
			Msg("cef: GetScreenInfo")
	}
	return 1
}

func (h *handlerSet) OnPopupShow(_ purecef.Browser, show int32) {
	if h.wv == nil || h.wv.pipeline == nil {
		return
	}
	h.wv.runOnGTK(func() {
		h.wv.pipeline.setPopupVisible(show != 0)
	})
}

func (h *handlerSet) OnPopupSize(_ purecef.Browser, popupRect *purecef.Rect) {
	if h.wv == nil || h.wv.pipeline == nil || popupRect == nil {
		return
	}
	popup := rect{
		X:      popupRect.X,
		Y:      popupRect.Y,
		Width:  popupRect.Width,
		Height: popupRect.Height,
	}
	h.wv.runOnGTK(func() {
		h.wv.pipeline.setPopupRect(popup)
	})
}

// OnPaint receives the BGRA pixel buffer from CEF and forwards dirty rects
// to the render pipeline for GPU upload. With a multi-threaded CEF UI loop we
// must first copy the transient CEF buffer before hopping back to GTK.
func (h *handlerSet) OnPaint(
	_ purecef.Browser, elementType purecef.PaintElementType,
	dirtyRects []purecef.Rect, buffer []byte, width, height int32,
) {
	if len(buffer) == 0 || width <= 0 || height <= 0 {
		return
	}
	paintSeq := h.wv.pipeline.nextPaintSeq()
	if h.wv != nil && h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Trace().
			Uint64("paint_seq", paintSeq).
			Int32("width", width).
			Int32("height", height).
			Int("dirty_rect_count", len(dirtyRects)).
			Int("buffer_len", len(buffer)).
			Msg("cef: OnPaint begin")
	}
	rects := make([]rect, len(dirtyRects))
	for i, dr := range dirtyRects {
		rects[i] = rect{X: dr.X, Y: dr.Y, Width: dr.Width, Height: dr.Height}
	}
	if h.wv.engine != nil && h.wv.engine.multiThreadedMessageLoop {
		pixels := make([]byte, len(buffer))
		copy(pixels, buffer)
		h.wv.runOnGTK(func() {
			if elementType == purecef.PaintElementTypePetPopup {
				h.wv.pipeline.handlePopupPaint(pixels, width, height, paintSeq)
				return
			}
			h.wv.pipeline.handlePaint(pixels, width, height, rects, paintSeq)
		})
		if h.wv != nil && h.wv.ctx != nil {
			logging.FromContext(h.wv.ctx).Trace().
				Uint64("paint_seq", paintSeq).
				Msg("cef: OnPaint queued to GTK")
		}
		return
	}
	if elementType == purecef.PaintElementTypePetPopup {
		h.wv.pipeline.handlePopupPaint(buffer, width, height, paintSeq)
		return
	}
	h.wv.pipeline.handlePaint(buffer, width, height, rects, paintSeq)
	if h.wv != nil && h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Trace().
			Uint64("paint_seq", paintSeq).
			Msg("cef: OnPaint handled inline")
	}
}

func (h *handlerSet) OnAcceleratedPaint(_ purecef.Browser, _ purecef.PaintElementType, _ []purecef.Rect, _ *purecef.AcceleratedPaintInfo) {
	count := h.wv.pipeline.recordAcceleratedPaint()
	if count <= 5 || count%100 == 0 {
		logging.FromContext(h.wv.ctx).Info().
			Uint64("count", count).
			Msg("cef: OnAcceleratedPaint")
	}
}

func (h *handlerSet) GetTouchHandleSize(_ purecef.Browser, _ purecef.HorizontalAlignment, _ *purecef.Size) {
}

func (h *handlerSet) OnTouchHandleStateChanged(_ purecef.Browser, _ *purecef.TouchHandleState) {}

func (h *handlerSet) StartDragging(_ purecef.Browser, _ purecef.DragData, _ purecef.DragOperationsMask, _, _ int32) int32 {
	return 0
}

func (h *handlerSet) UpdateDragCursor(_ purecef.Browser, _ purecef.DragOperationsMask) {}

func (h *handlerSet) OnScrollOffsetChanged(_ purecef.Browser, _, _ float64) {}

func (h *handlerSet) OnImeCompositionRangeChanged(_ purecef.Browser, _ *purecef.Range, _ []purecef.Rect) {
}

func (h *handlerSet) OnTextSelectionChanged(_ purecef.Browser, _ string, _ *purecef.Range) {}

func (h *handlerSet) OnVirtualKeyboardRequested(_ purecef.Browser, _ purecef.TextInputMode) {}

// ===========================================================================
// DisplayHandler (13 methods)
// ===========================================================================

// OnAddressChange updates the cached URI when the main frame navigates.
func (h *handlerSet) OnAddressChange(_ purecef.Browser, frame purecef.Frame, url string) {
	if frame != nil && frame.IsMain() {
		h.wv.updateURI(url)
	}
}

// OnTitleChange updates the cached title.
func (h *handlerSet) OnTitleChange(_ purecef.Browser, title string) {
	h.wv.updateTitle(title)
}

func (h *handlerSet) OnFaviconUrlchange(_ purecef.Browser, _ uintptr) {}

// OnFullscreenModeChange toggles the fullscreen atomic and fires callbacks.
func (h *handlerSet) OnFullscreenModeChange(_ purecef.Browser, fullscreen int32) {
	entering := fullscreen != 0
	h.wv.fullscreen.Store(entering)

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb == nil {
		return
	}
	if entering && cb.OnEnterFullscreen != nil {
		h.wv.runOnGTK(func() {
			cb.OnEnterFullscreen()
		})
	}
	if !entering && cb.OnLeaveFullscreen != nil {
		h.wv.runOnGTK(func() {
			cb.OnLeaveFullscreen()
		})
	}
}

func (h *handlerSet) OnTooltip(_ purecef.Browser, _ uintptr) int32 { return 0 }

// OnStatusMessage fires the OnLinkHover callback with the status text
// and caches the hover URI for middle-click interception.
func (h *handlerSet) OnStatusMessage(_ purecef.Browser, value string) {
	h.wv.updateHoverURI(value)

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb != nil && cb.OnLinkHover != nil {
		h.wv.runOnGTK(func() {
			cb.OnLinkHover(value)
		})
	}
}

func (h *handlerSet) OnConsoleMessage(_ purecef.Browser, level purecef.LogSeverity, message, source string, line int32) int32 {
	if h.wv != nil && h.wv.ctx != nil && (strings.Contains(message, consoleMarkerVideoDiag) || strings.Contains(message, consoleMarkerRedditVideoPatch)) {
		log := logging.FromContext(h.wv.ctx).With().
			Str("component", "cef-console").
			Str("source", source).
			Int32("line", line).
			Logger()
		switch level {
		case purecef.LogSeverityLogseverityError, purecef.LogSeverityLogseverityFatal:
			log.Error().Str("message", message).Msg("cef: console message")
		case purecef.LogSeverityLogseverityWarning:
			log.Warn().Str("message", message).Msg("cef: console message")
		default:
			log.Info().Str("message", message).Msg("cef: console message")
		}
	}
	return 0
}

func (h *handlerSet) OnAutoResize(_ purecef.Browser, _ *purecef.Size) int32 { return 0 }

// OnLoadingProgressChange updates the cached progress value.
func (h *handlerSet) OnLoadingProgressChange(_ purecef.Browser, progress float64) {
	h.wv.updateProgress(progress)
}

func (h *handlerSet) OnCursorChange(_ purecef.Browser, _ uintptr, cursorType purecef.CursorType, _ *purecef.CursorInfo) int32 {
	name := cefCursorToGDKName(cursorType)
	h.wv.runOnGTK(func() {
		h.wv.pipeline.glArea.SetCursorFromName(&name)
	})
	return 1 // handled
}

func (h *handlerSet) OnMediaAccessChange(_ purecef.Browser, _, _ int32) {}

func (h *handlerSet) OnContentsBoundsChange(_ purecef.Browser, _ *purecef.Rect) int32 { return 0 }

func (h *handlerSet) GetRootWindowScreenRect(_ purecef.Browser, _ *purecef.Rect) int32 { return 0 }

// ===========================================================================
// LoadHandler (4 methods)
// ===========================================================================

// OnLoadingStateChange updates the loading/navigation state cache and fires callbacks.
func (h *handlerSet) OnLoadingStateChange(_ purecef.Browser, isloading, cangoback, cangoforward int32) {
	loading := isloading != 0
	h.wv.updateLoadState(loading, cangoback != 0, cangoforward != 0)

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb != nil && cb.OnLoadChanged != nil {
		if loading {
			h.wv.runOnGTK(func() {
				cb.OnLoadChanged(port.LoadStarted)
			})
		} else {
			h.wv.runOnGTK(func() {
				cb.OnLoadChanged(port.LoadFinished)
			})
		}
	}
}

// OnLoadStart fires LoadCommitted for main frame navigations.
// In CEF this callback runs after navigation commit, so it is the closest
// equivalent to WebKit's LoadCommitted event.
func (h *handlerSet) OnLoadStart(_ purecef.Browser, frame purecef.Frame, _ purecef.TransitionType) {
	if frame == nil || !frame.IsMain() {
		return
	}
	h.wv.updateURI(frame.GetURL())
	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb != nil && cb.OnLoadChanged != nil {
		h.wv.runOnGTK(func() {
			cb.OnLoadChanged(port.LoadCommitted)
		})
	}
}

// OnLoadEnd fires LoadFinished and sets progress to 1.0 for the main frame.
func (h *handlerSet) OnLoadEnd(_ purecef.Browser, frame purecef.Frame, httpStatusCode int32) {
	log := logging.FromContext(h.wv.ctx)
	log.Debug().
		Bool("frame_nil", frame == nil).
		Bool("is_main", frame != nil && frame.IsMain()).
		Int32("http_status", httpStatusCode).
		Msg("cef: OnLoadEnd")
	if frame == nil || !frame.IsMain() {
		return
	}
	// Successful load — reset the consecutive crash counter.
	h.wv.crashCount.Store(0)

	// Inject scripts and styles after page load.
	// Must run on GTK thread — OnLoadEnd fires on the CEF IO thread,
	// and JavaScript injection requires the main thread.
	if h.wv.engine != nil && h.wv.engine.contentInj != nil {
		h.wv.runOnGTK(func() {
			h.wv.engine.contentInj.onLoadEnd(h.wv)
		})
	}

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb != nil {
		if cb.OnLoadChanged != nil {
			h.wv.runOnGTK(func() {
				cb.OnLoadChanged(port.LoadFinished)
			})
		}
		if cb.OnProgressChanged != nil {
			h.wv.runOnGTK(func() {
				cb.OnProgressChanged(1.0)
			})
		}
	}
}

// OnLoadError is a no-op.
func (h *handlerSet) OnLoadError(_ purecef.Browser, _ purecef.Frame, _ purecef.Errorcode, _, _ string) {
}

// ===========================================================================
// LifeSpanHandler (6 methods)
// ===========================================================================

// OnBeforePopup intercepts popup requests (target="_blank", window.open).
// CEF OSR cannot create popup windows, so we fire the OnCreate callback
// to let the coordinator open the link in a new stacked pane.
//
//nolint:gocritic // signature imposed by purecef.LifeSpanHandler interface
func (h *handlerSet) OnBeforePopup(
	_ purecef.Browser, _ purecef.Frame, _ int32, targetURL, targetFrameName string,
	_ purecef.WindowOpenDisposition, userGesture int32, _ *purecef.PopupFeatures,
	_ *purecef.WindowInfo, _ *purecef.Client, _ *purecef.BrowserSettings,
	_ *purecef.DictionaryValue, _ *bool,
) bool {
	if targetURL == "" {
		return true
	}

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()

	if cb != nil && cb.OnCreate != nil {
		req := port.PopupRequest{
			TargetURI:     targetURL,
			FrameName:     targetFrameName,
			IsUserGesture: userGesture != 0,
			ParentViewID:  h.wv.id,
		}
		h.wv.runOnGTK(func() {
			cb.OnCreate(req)
		})
	}

	return true // always block CEF popup; the coordinator handles the new pane
}

func (h *handlerSet) OnBeforePopupAborted(_ purecef.Browser, _ int32) {}

//nolint:gocritic // signature imposed by purecef.LifeSpanHandler interface
func (h *handlerSet) OnBeforeDevToolsPopup(
	_ purecef.Browser, _ *purecef.WindowInfo, _ *purecef.Client,
	_ *purecef.BrowserSettings, _ *purecef.DictionaryValue, _ *bool,
) {
}

// OnAfterCreated stores the browser and host references and enables input.
func (h *handlerSet) OnAfterCreated(browser purecef.Browser) {
	log := logging.FromContext(h.wv.ctx)
	if browser == nil {
		log.Warn().Msg("cef: OnAfterCreated called with nil browser")
		return
	}
	browserID := browser.GetIdentifier()
	log.Debug().
		Int32("browser_id", browserID).
		Bool("context_menu_handler_enabled", h.enableContextMenuHandler).
		Msg("cef: OnAfterCreated")
	if h.wv.engine != nil {
		h.wv.engine.recordBrowserAfterCreated(browser)
		h.wv.engine.registerWebView(h.wv)
	}
	host := browser.GetHost()

	h.wv.mu.Lock()
	h.wv.browser = browser
	h.wv.host = host
	uri := h.wv.pendingURI
	h.wv.pendingURI = ""
	h.wv.input.setHost(host)
	if h.wv.findCtrl != nil {
		h.wv.findCtrl.setHost(host)
	}

	// Mark browser as visible — CEF OSR starts in hidden state and suppresses
	// UI elements like the text caret until explicitly told the browser is shown.
	host.WasHidden(0)

	// If the GLArea already has GTK focus (cold start race), tell CEF now.
	// The focus-enter event fired before the browser existed, so SetFocus
	// was never called and the caret won't blink until the user clicks.
	if h.wv.input != nil && h.wv.input.glArea != nil && h.wv.input.glArea.HasFocus() {
		host.SetFocus(1)
	}

	// Replay any navigation that was requested before the browser existed.
	if uri != "" {
		if frame := browser.GetMainFrame(); frame != nil {
			frame.LoadURL(uri)
		}
	}
	h.wv.mu.Unlock()

	h.wv.scheduleStartBeginFrameLoop()
}

// DoClose returns false to allow the default close behavior.
func (h *handlerSet) DoClose(_ purecef.Browser) bool {
	return false
}

// OnBeforeClose fires the OnClose callback.
func (h *handlerSet) OnBeforeClose(_ purecef.Browser) {
	if h.wv.engine != nil {
		h.wv.engine.unregisterWebView(h.wv)
	}
	h.wv.mu.Lock()
	h.wv.browser = nil
	h.wv.host = nil
	h.wv.input.setHost(nil)
	if h.wv.findCtrl != nil {
		h.wv.findCtrl.setHost(nil)
	}
	h.wv.mu.Unlock()
	h.wv.scheduleStopBeginFrameLoop()

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb != nil && cb.OnClose != nil {
		h.wv.runOnGTK(func() {
			cb.OnClose()
		})
	}
}

// ===========================================================================
// RequestHandler (11 methods)
// ===========================================================================

func (h *handlerSet) OnBeforeBrowse(_ purecef.Browser, _ purecef.Frame, _ purecef.Request, _, _ int32) bool {
	return false
}

func (h *handlerSet) OnOpenUrlfromTab(_ purecef.Browser, _ purecef.Frame, _ string, _ purecef.WindowOpenDisposition, _ int32) int32 {
	return 0
}

func (h *handlerSet) GetResourceRequestHandler(
	_ purecef.Browser, _ purecef.Frame, request purecef.Request,
	_, _ int32, _ string, disableDefaultHandling *int32,
) purecef.ResourceRequestHandler {
	if h.transcodingHandler != nil && request != nil && disableDefaultHandling != nil && transcoder.IsEagerTranscodeURL(request.GetURL()) {
		*disableDefaultHandling = 1
		if h.wv != nil && h.wv.ctx != nil {
			logging.FromContext(h.wv.ctx).Info().
				Str("url", logging.TruncateURL(request.GetURL(), 240)).
				Msg("cef: disabled default handling for eager transcode candidate")
		}
	}
	if h.transcodingHandler != nil {
		return h.transcodingHandler
	}
	return nil
}

func (h *handlerSet) GetAuthCredentials(
	_ purecef.Browser, _ string, _ int32, _ string, _ int32,
	_, _ string, _ purecef.AuthCallback,
) int32 {
	return 0
}

func (h *handlerSet) OnCertificateError(_ purecef.Browser, _ purecef.Errorcode, _ string, _ purecef.Sslinfo, _ purecef.Callback) int32 {
	return 0
}

func (h *handlerSet) OnSelectClientCertificate(
	_ purecef.Browser, _ int32, _ string, _ int32,
	_ []purecef.X509Certificate, _ purecef.SelectClientCertificateCallback,
) int32 {
	return 0
}

func (h *handlerSet) OnRenderViewReady(_ purecef.Browser) {}

func (h *handlerSet) OnRenderProcessUnresponsive(_ purecef.Browser, _ purecef.UnresponsiveProcessCallback) int32 {
	return 0
}

func (h *handlerSet) OnRenderProcessResponsive(_ purecef.Browser) {}

// maxConsecutiveCrashes caps how many times OnRenderProcessTerminated will
// forward the event before suppressing further notifications. This prevents
// infinite crash → redirect → crash loops.
const maxConsecutiveCrashes = 3

// OnRenderProcessTerminated fires the OnWebProcessTerminated callback with a mapped reason.
func (h *handlerSet) OnRenderProcessTerminated(_ purecef.Browser, status purecef.TerminationStatus, _ int32, _ string) {
	if h.wv.crashCount.Add(1) > maxConsecutiveCrashes {
		return // suppress to break the loop
	}

	var reason port.WebProcessTerminationReason
	var label string
	switch status {
	case purecef.TerminationStatusTsAbnormalTermination:
		reason = port.WebProcessTerminationCrashed
		label = "abnormal termination"
	case purecef.TerminationStatusTsProcessWasKilled:
		reason = port.WebProcessTerminationByAPI
		label = "killed"
	case purecef.TerminationStatusTsProcessCrashed:
		reason = port.WebProcessTerminationCrashed
		label = "crashed"
	case purecef.TerminationStatusTsProcessOom:
		reason = port.WebProcessTerminationExceededMemory
		label = "out of memory"
	default:
		reason = port.WebProcessTerminationUnknown
		label = "unknown"
	}

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	uri := h.wv.uri
	h.wv.mu.RUnlock()
	if cb != nil && cb.OnWebProcessTerminated != nil {
		h.wv.runOnGTK(func() {
			cb.OnWebProcessTerminated(reason, label, uri)
		})
	}
}

func (h *handlerSet) OnDocumentAvailableInMainFrame(_ purecef.Browser) {}

// ===========================================================================
// AudioHandler (5 methods)
// ===========================================================================

func (h *handlerSet) GetAudioParameters(_ purecef.Browser, _ *purecef.AudioParameters) int32 {
	return 1 // proceed with defaults
}

func (h *handlerSet) OnAudioStreamStarted(_ purecef.Browser, _ *purecef.AudioParameters, _ int32) {
	h.wv.setAudioPlaying(true)
}

func (h *handlerSet) OnAudioStreamPacket(_ purecef.Browser, _ [][]float32, _ int32, _ int64) {}

func (h *handlerSet) OnAudioStreamStopped(_ purecef.Browser) {
	h.wv.setAudioPlaying(false)
}

func (h *handlerSet) OnAudioStreamError(_ purecef.Browser, _ string) {
	h.wv.setAudioPlaying(false)
}

// ===========================================================================
// FindHandler (1 method)
// ===========================================================================

// OnFindResult dispatches CEF find results to the WebView's FindController.
func (h *handlerSet) OnFindResult(_ purecef.Browser, identifier, count int32, _ *purecef.Rect, activematchordinal, finalupdate int32) {
	if h.wv.findCtrl != nil {
		h.wv.findCtrl.handleFindResult(identifier, count, activematchordinal, finalupdate)
	}
}

// cefCursorNames maps CEF cursor types to GDK/CSS cursor names.
var cefCursorNames = map[purecef.CursorType]string{
	purecef.CursorTypeCtPointer:                  "default",
	purecef.CursorTypeCtCross:                    "crosshair",
	purecef.CursorTypeCtHand:                     "pointer",
	purecef.CursorTypeCtIbeam:                    "text",
	purecef.CursorTypeCtWait:                     "wait",
	purecef.CursorTypeCtHelp:                     "help",
	purecef.CursorTypeCtEastresize:               "e-resize",
	purecef.CursorTypeCtNorthresize:              "n-resize",
	purecef.CursorTypeCtNortheastresize:          "ne-resize",
	purecef.CursorTypeCtNorthwestresize:          "nw-resize",
	purecef.CursorTypeCtSouthresize:              "s-resize",
	purecef.CursorTypeCtSoutheastresize:          "se-resize",
	purecef.CursorTypeCtSouthwestresize:          "sw-resize",
	purecef.CursorTypeCtWestresize:               "w-resize",
	purecef.CursorTypeCtNorthsouthresize:         "ns-resize",
	purecef.CursorTypeCtEastwestresize:           "ew-resize",
	purecef.CursorTypeCtNortheastsouthwestresize: "nesw-resize",
	purecef.CursorTypeCtNorthwestsoutheastresize: "nwse-resize",
	purecef.CursorTypeCtColumnresize:             "col-resize",
	purecef.CursorTypeCtRowresize:                "row-resize",
	purecef.CursorTypeCtMove:                     "move",
	purecef.CursorTypeCtProgress:                 "progress",
	purecef.CursorTypeCtNodrop:                   "no-drop",
	purecef.CursorTypeCtNotallowed:               "not-allowed",
	purecef.CursorTypeCtGrab:                     "grab",
	purecef.CursorTypeCtGrabbing:                 "grabbing",
	purecef.CursorTypeCtZoomin:                   "zoom-in",
	purecef.CursorTypeCtZoomout:                  "zoom-out",
}

// cefCursorToGDKName maps a CEF cursor type to a GDK/CSS cursor name.
func cefCursorToGDKName(ct purecef.CursorType) string {
	if name, ok := cefCursorNames[ct]; ok {
		return name
	}
	return "default"
}
