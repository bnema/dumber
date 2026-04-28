package cef

import (
	"context"
	"strings"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	downloadutil "github.com/bnema/dumber/internal/domain/download"
	"github.com/bnema/dumber/internal/logging"
)

// Console message markers used by injected JavaScript for log filtering.
const (
	consoleMarkerVideoDiag        = "[VIDEO-DIAG]"
	consoleMarkerRedditVideoPatch = "[REDDIT-VIDEO-PATCH]"
	consoleMarkerAutoCopy         = "[AUTO-COPY]"
)

// handlerSet implements all CEF handler interfaces and dispatches events to the
// owning WebView. A single struct is used so that the Client's Get*Handler
// methods can return the same receiver, avoiding extra allocations.
type handlerSet struct {
	wv *WebView
}

// Compile-time interface checks.
var (
	_ purecef.Client             = (*handlerSet)(nil)
	_ purecef.RenderHandler      = (*handlerSet)(nil)
	_ purecef.DisplayHandler     = (*handlerSet)(nil)
	_ purecef.LoadHandler        = (*handlerSet)(nil)
	_ purecef.LifeSpanHandler    = (*handlerSet)(nil)
	_ purecef.RequestHandler     = (*handlerSet)(nil)
	_ purecef.AudioHandler       = (*handlerSet)(nil)
	_ purecef.ContextMenuHandler = (*handlerSet)(nil)
	_ purecef.DownloadHandler    = (*handlerSet)(nil)
	_ purecef.FindHandler        = (*handlerSet)(nil)
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
func (h *handlerSet) GetCommandHandler() purecef.CommandHandler         { return nil }
func (h *handlerSet) GetContextMenuHandler() purecef.ContextMenuHandler { return h }
func (h *handlerSet) GetDialogHandler() purecef.DialogHandler           { return nil }
func (h *handlerSet) GetDisplayHandler() purecef.DisplayHandler         { return h }
func (h *handlerSet) GetDownloadHandler() purecef.DownloadHandler {
	if h.downloadHandler() != nil {
		return h
	}
	return nil
}
func (h *handlerSet) GetDragHandler() purecef.DragHandler             { return nil }
func (h *handlerSet) GetFindHandler() purecef.FindHandler             { return h }
func (h *handlerSet) GetFocusHandler() purecef.FocusHandler           { return nil }
func (h *handlerSet) GetFrameHandler() purecef.FrameHandler           { return nil }
func (h *handlerSet) GetPermissionHandler() purecef.PermissionHandler { return nil }
func (h *handlerSet) GetJsdialogHandler() purecef.JsdialogHandler     { return nil }
func (h *handlerSet) GetKeyboardHandler() purecef.KeyboardHandler     { return nil }
func (h *handlerSet) GetLifeSpanHandler() purecef.LifeSpanHandler     { return h }
func (h *handlerSet) GetLoadHandler() purecef.LoadHandler             { return h }
func (h *handlerSet) GetPrintHandler() purecef.PrintHandler           { return nil }
func (h *handlerSet) GetRenderHandler() purecef.RenderHandler         { return h }
func (h *handlerSet) GetRequestHandler() purecef.RequestHandler       { return h }

func (h *handlerSet) OnProcessMessageReceived(
	browser purecef.Browser,
	_ purecef.Frame,
	_ purecef.ProcessID,
	message purecef.ProcessMessage,
) int32 {
	if h == nil || h.wv == nil || h.wv.engine == nil {
		return 0
	}
	action, payload, ok := decodeRendererBridgeProcessMessage(message)
	if !ok {
		return 0
	}

	log := logging.FromContext(h.wv.ctx)
	log.Debug().
		Str("action", action).
		Int("payload_len", len(payload)).
		Msg("cef: renderer bridge message received")

	switch action {
	case rendererBridgeActionExplicitTextCopy:
		req, err := decodeRendererBridgeExplicitTextCopyPayload([]byte(payload))
		if err != nil {
			log.Debug().Str("action", action).Msg("cef: invalid explicit copy payload")
			return 1
		}
		h.wv.engine.handleExplicitClipboardBridgeText(h.wv.id, req.Action, req.Text)
	case rendererBridgeActionEditableFocusChanged:
		h.wv.setEditableFocus(payload == "1" || strings.EqualFold(payload, "true"))
	case rendererBridgeActionFocusSync:
		h.wv.engine.handleEditableFocusBridge(browser)
	case rendererBridgeActionPopupOpen:
		req, err := decodeRendererBridgePopupOpenPayload([]byte(payload))
		if err != nil {
			log.Debug().Str("action", action).Msg("cef: invalid popup_open payload")
			return 1
		}
		h.wv.handleSyntheticPopupOpen(req.URL, req.FrameName, req.ProxyID, req.UserGesture, req.NoJavaScriptAccess)
	case rendererBridgeActionPopupNavigate:
		req, err := decodeRendererBridgePopupNavigatePayload([]byte(payload))
		if err != nil {
			log.Debug().Str("action", action).Msg("cef: invalid popup_navigate payload")
			return 1
		}
		h.wv.handleSyntheticPopupNavigate(req.ProxyID, req.URL)
	case rendererBridgeActionPopupClose:
		req, err := decodeRendererBridgePopupClosePayload([]byte(payload))
		if err != nil {
			log.Debug().Str("action", action).Msg("cef: invalid popup_close payload")
			return 1
		}
		h.wv.handleSyntheticPopupClose(req.ProxyID)
	case rendererBridgeActionReady:
		log.Debug().
			Str("frame_url", logging.TruncateURL(payload, logging.PermissionLogURLMaxLen)).
			Msg("cef: renderer bridge ready")
	default:
		log.Debug().Str("action", action).Msg("cef: unknown renderer bridge action")
	}
	return 1
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
			Uint64("webview_id", uint64(h.wv.id)).
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
	si := purecef.NewScreenInfo()
	si.DeviceScaleFactor = float32(s)
	si.Depth = screenDepth
	si.DepthPerComponent = screenDepthPerComponent
	si.Rect = r
	si.AvailableRect = r
	*info = si
	if h.wv != nil && h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Debug().
			Uint64("webview_id", uint64(h.wv.id)).
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
// to the render pipeline for GPU upload. Main-view paints are copied directly
// into the persistent staging buffer on the CEF UI thread; only the GTK
// QueueRender hop remains. Popups still copy their transient buffer before
// crossing threads because they are small and infrequent.
func (h *handlerSet) OnPaint(
	_ purecef.Browser, elementType purecef.PaintElementType,
	dirtyRects []purecef.Rect, buffer []byte, width, height int32,
) {
	if len(buffer) == 0 || width <= 0 || height <= 0 {
		return
	}
	paintSeq := h.wv.pipeline.nextPaintSeq()
	resizeSeq, resizeAgeMs := uint64(0), int64(0)
	if elementType != purecef.PaintElementTypePetPopup {
		resizeSeq, resizeAgeMs = h.wv.pipeline.latestResizeDiagnostics()
	}
	if h.wv != nil && h.wv.ctx != nil {
		log := logging.FromContext(h.wv.ctx).Trace().
			Uint64("paint_seq", paintSeq).
			Int32("width", width).
			Int32("height", height).
			Int("dirty_rect_count", len(dirtyRects)).
			Int("buffer_len", len(buffer))
		if elementType != purecef.PaintElementTypePetPopup {
			log = log.
				Uint64("resize_seq", resizeSeq).
				Int64("time_since_resize_ms", resizeAgeMs)
		}
		log.Msg("cef: OnPaint begin")
	}
	if elementType != purecef.PaintElementTypePetPopup &&
		!h.shouldAcceptMainViewPaint(width, height, dirtyRects, paintSeq, resizeSeq, resizeAgeMs) {
		return
	}
	rects := make([]rect, len(dirtyRects))
	for i, dr := range dirtyRects {
		rects[i] = rect{X: dr.X, Y: dr.Y, Width: dr.Width, Height: dr.Height}
	}

	if elementType == purecef.PaintElementTypePetPopup {
		pixels := make([]byte, len(buffer))
		copy(pixels, buffer)
		h.wv.runOnGTK(func() {
			h.wv.pipeline.handlePopupPaint(pixels, width, height, paintSeq)
		})
		if h.wv != nil && h.wv.ctx != nil {
			logging.FromContext(h.wv.ctx).Trace().
				Uint64("paint_seq", paintSeq).
				Msg("cef: OnPaint queued popup to GTK")
		}
		return
	}

	h.wv.pipeline.handlePaint(buffer, width, height, rects, paintSeq)
	h.wv.runOnGTK(func() {
		h.wv.pipeline.queuePaintRender()
	})
	if h.wv != nil && h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Trace().
			Uint64("paint_seq", paintSeq).
			Msg("cef: OnPaint queued to GTK")
	}
}

func (h *handlerSet) shouldAcceptMainViewPaint(
	width, height int32,
	dirtyRects []purecef.Rect,
	paintSeq uint64,
	resizeSeq uint64,
	resizeAgeMs int64,
) bool {
	expectedWidth, expectedHeight, _ := h.wv.pipeline.viewRectSize()
	sizeMatchesCurrentView := width == expectedWidth && height == expectedHeight

	if h.wv.resizeReconciler != nil {
		h.wv.resizeReconciler.notePaint(resizeSeq, sizeMatchesCurrentView)
	}

	if h.wv.ctx != nil {
		if sizeMatchesCurrentView {
			_, _, shouldLog := h.wv.pipeline.markFirstPaintAfterResize()
			if shouldLog {
				logging.Trace().Mark("cef_first_content_paint")
				logging.FromContext(h.wv.ctx).Debug().
					Uint64("webview_id", uint64(h.wv.id)).
					Uint64("paint_seq", paintSeq).
					Uint64("resize_seq", resizeSeq).
					Int64("time_since_resize_ms", resizeAgeMs).
					Int32("width", width).
					Int32("height", height).
					Int("dirty_rect_count", len(dirtyRects)).
					Msg("cef: first paint after resize")
			}
		} else if h.wv.pipeline.markStalePaintAfterResize(resizeSeq) {
			logging.FromContext(h.wv.ctx).Debug().
				Uint64("webview_id", uint64(h.wv.id)).
				Uint64("paint_seq", paintSeq).
				Uint64("resize_seq", resizeSeq).
				Int64("time_since_resize_ms", resizeAgeMs).
				Int32("width", width).
				Int32("height", height).
				Int32("expected_width", expectedWidth).
				Int32("expected_height", expectedHeight).
				Int("dirty_rect_count", len(dirtyRects)).
				Msg("cef: stale-size paint after resize")
		}
	}

	return sizeMatchesCurrentView
}

func (h *handlerSet) OnAcceleratedPaint(
	_ purecef.Browser,
	_ purecef.PaintElementType,
	_ []purecef.Rect,
	_ *purecef.AcceleratedPaintInfo,
) {
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

func (h *handlerSet) StartDragging(
	_ purecef.Browser,
	_ purecef.DragData,
	_ purecef.DragOperationsMask,
	_, _ int32,
) int32 {
	return 0
}

func (h *handlerSet) UpdateDragCursor(_ purecef.Browser, _ purecef.DragOperationsMask) {}

func (h *handlerSet) OnScrollOffsetChanged(_ purecef.Browser, _, _ float64) {}

func (h *handlerSet) OnImeCompositionRangeChanged(_ purecef.Browser, _ *purecef.Range, _ []purecef.Rect) {
}

func (h *handlerSet) OnTextSelectionChanged(_ purecef.Browser, selectedText string, _ *purecef.Range) {
	if h == nil || h.wv == nil {
		return
	}
	previous, changed := h.wv.setSelectedText(selectedText)
	if !changed {
		return
	}
	if h.wv.ctx != nil && selectedText == "" {
		if previous != "" {
			logging.FromContext(h.wv.ctx).Debug().
				Int("prev_text_len", len(previous)).
				Msg("cef: text selection cleared")
		}
	} else if h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Debug().
			Int("text_len", len(selectedText)).
			Msg("cef: text selection changed")
	}
	if h.wv.engine != nil {
		h.wv.scheduleSelectionUpdate(selectedText)
	}
}

func (h *handlerSet) OnVirtualKeyboardRequested(_ purecef.Browser, _ purecef.TextInputMode) {}

// ===========================================================================
// DisplayHandler (13 methods)
// ===========================================================================

// OnAddressChange updates the cached URI when the main frame navigates.
func (h *handlerSet) OnAddressChange(_ purecef.Browser, frame purecef.Frame, url string) {
	if frame != nil && frame.IsMain() {
		if h.wv != nil && h.wv.ctx != nil {
			logging.FromContext(h.wv.ctx).Debug().
				Str("url", logging.TruncateURL(url, logging.PermissionLogURLMaxLen)).
				Msg("cef: OnAddressChange")
		}
		h.wv.updateURI(url)
	}
}

// OnTitleChange updates the cached title.
func (h *handlerSet) OnTitleChange(_ purecef.Browser, title string) {
	h.wv.updateTitle(title)
}

func (h *handlerSet) OnFaviconUrlchange(_ purecef.Browser, _ purecef.StringList) {}

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

func (h *handlerSet) OnConsoleMessage(
	_ purecef.Browser,
	level purecef.LogSeverity,
	message, source string,
	line int32,
) int32 {
	if h.wv != nil && h.wv.ctx != nil &&
		(strings.Contains(message, consoleMarkerVideoDiag) ||
			strings.Contains(message, consoleMarkerRedditVideoPatch) ||
			strings.Contains(message, consoleMarkerAutoCopy)) {
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
	if h.wv != nil && h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Trace().
			Float64("progress", progress).
			Msg("cef: OnLoadingProgressChange")
	}
	h.wv.updateProgress(progress)
}

func (h *handlerSet) OnCursorChange(
	_ purecef.Browser,
	_ uintptr,
	cursorType purecef.CursorType,
	_ *purecef.CursorInfo,
) int32 {
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
	if h.wv != nil && h.wv.ctx != nil {
		h.wv.mu.RLock()
		uri := h.wv.uri
		progress := h.wv.progress
		pendingURI := h.wv.pendingURI
		h.wv.mu.RUnlock()
		logging.FromContext(h.wv.ctx).Debug().
			Bool("loading", loading).
			Bool("can_go_back", cangoback != 0).
			Bool("can_go_forward", cangoforward != 0).
			Float64("progress", progress).
			Str("uri", logging.TruncateURL(uri, logging.PermissionLogURLMaxLen)).
			Str("pending_uri", logging.TruncateURL(pendingURI, logging.PermissionLogURLMaxLen)).
			Msg("cef: OnLoadingStateChange")
	}

	if !loading && h.wv.input != nil && h.wv.input.hasGTKFocus() {
		h.wv.mu.RLock()
		host := h.wv.host
		h.wv.mu.RUnlock()
		if host != nil {
			syncWindowlessBrowserFocus(host)
		}
	}

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
	bridgeNonce := h.wv.ensureBridgeNonce()
	if openerBridgeScript := h.wv.popupOpenerBridgeScript(bridgeNonce); openerBridgeScript != "" {
		if h.wv != nil && h.wv.ctx != nil {
			parentURI, active, blocked := h.wv.popupOpenerBridgeState()
			logging.FromContext(h.wv.ctx).Debug().
				Uint64("webview_id", uint64(h.wv.id)).
				Str("url", logging.TruncateURL(frame.GetURL(), logging.PermissionLogURLMaxLen)).
				Str("parent_uri", logging.TruncateURL(parentURI, logging.PermissionLogURLMaxLen)).
				Bool("bridge_active", active).
				Bool("bridge_blocked", blocked).
				Msg("cef: injecting popup opener bridge")
		}
		frame.ExecuteJavaScript(openerBridgeScript, frame.GetURL(), 0)
	}
	if h.wv != nil && h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Debug().
			Str("url", logging.TruncateURL(frame.GetURL(), logging.PermissionLogURLMaxLen)).
			Msg("cef: OnLoadStart")
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

// OnLoadEnd resets crash count and injects content scripts for the main frame.
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

	// If a queued startup navigation is still pending after about:blank finished,
	// replay it now that the initial main-frame load completed.
	frameURL := frame.GetURL()
	if pendingURI := h.wv.pendingNavigationURI(); pendingURI != "" && !pendingURIEquivalent(frameURL, pendingURI) {
		if strings.EqualFold(strings.TrimSpace(frameURL), "about:blank") {
			log.Debug().
				Str("pending_uri", logging.TruncateURL(pendingURI, logging.PermissionLogURLMaxLen)).
				Msg("cef: replaying pending navigation after about:blank load end")
			h.wv.schedulePendingNavigationReplay(0)
		}
	}

	// Inject scripts and styles after page load.
	// Must run on GTK thread — OnLoadEnd fires on the CEF IO thread,
	// and JavaScript injection requires the main thread.
	if h.wv.engine != nil && h.wv.engine.contentInj != nil {
		h.wv.runOnGTK(func() {
			h.wv.engine.contentInj.onLoadEnd(h.wv)
		})
	}

	// NOTE: We intentionally do NOT dispatch LoadFinished or ProgressChanged(1.0)
	// here. OnLoadEnd is a per-frame event, and during cross-site navigations
	// (process swap), CEF fires OnLoadEnd for the OLD main frame before the new
	// page finishes loading. Dispatching LoadFinished here caused the progress
	// bar to hide prematurely while the new page was still loading.
	//
	// OnLoadingStateChange(isLoading=false) is the correct browser-level signal
	// for load completion — CEF guarantees it fires AFTER all OnLoadEnd calls.
	// CEF's OnLoadingProgressChange also provides progress=1.0 at completion.
}

// OnLoadError is a no-op.
func (h *handlerSet) OnLoadError(_ purecef.Browser, _ purecef.Frame, _ purecef.Errorcode, _, _ string) {
}

// ===========================================================================
// LifeSpanHandler (6 methods)
// ===========================================================================

// OnBeforePopup intercepts popup requests (target="_blank", window.open).
// When the coordinator returns a related CEF popup shell we hand that shell's
// client back to CEF and allow native popup creation so opener semantics are
// preserved. Otherwise we keep blocking and let the coordinator's fallback
// pane handle the navigation.
func (h *handlerSet) OnBeforePopup(
	_ purecef.Browser, _ purecef.Frame, popupID int32, targetURL, targetFrameName string,
	_ purecef.WindowOpenDisposition, userGesture int32, _ *purecef.PopupFeatures,
	windowInfo *purecef.WindowInfo, clientSlot *purecef.RawClientWriteSlot, settings *purecef.BrowserSettings,
	_ *purecef.DictionaryValue, noJavaScriptAccess *bool,
) bool {
	if targetURL == "" {
		return true
	}

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb == nil || cb.OnCreate == nil {
		return true
	}

	requestNoJavaScriptAccess := false
	if noJavaScriptAccess != nil {
		requestNoJavaScriptAccess = *noJavaScriptAccess
	}
	var popup port.WebView
	h.wv.runOnGTKSync(func() {
		popup = cb.OnCreate(port.PopupRequest{
			TargetURI:          targetURL,
			FrameName:          targetFrameName,
			IsUserGesture:      userGesture != 0,
			NoJavaScriptAccess: requestNoJavaScriptAccess,
			ParentViewID:       h.wv.id,
		})
	})

	cefPopup, ok := popup.(*WebView)
	if !ok || cefPopup == nil {
		return true
	}
	cefPopup.setPopupNoJavaScriptAccess(requestNoJavaScriptAccess)
	if cefPopup.prepareNativePopup(popupID, targetURL, windowInfo, clientSlot, settings) {
		return false
	}

	cefPopup.discardNativePopupCandidate()
	logging.FromContext(h.currentContext()).Debug().
		Int32("popup_id", popupID).
		Str("target_url", logging.TruncateURL(targetURL, logging.PermissionLogURLMaxLen)).
		Msg("cef: native popup bridge unavailable, blocking CEF popup and using popup shell browser creation")
	return true
}

func (h *handlerSet) OnBeforePopupAborted(_ purecef.Browser, popupID int32) {
	if popup := h.wv.takePendingNativePopup(popupID); popup != nil {
		popup.handleNativePopupAborted()
	}
}

func (h *handlerSet) OnBeforeDevToolsPopup(
	_ purecef.Browser, _ *purecef.WindowInfo, _ *purecef.RawClientWriteSlot,
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
	log.Debug().Int32("browser_id", browserID).Msg("cef: OnAfterCreated")

	host := browser.GetHost()
	if host == nil {
		log.Warn().Msg("cef: OnAfterCreated returned nil host")
		return
	}

	state := h.attachAfterCreatedBrowser(browser, host, browserID)
	if state.closeDuplicate {
		log.Warn().
			Int32("browser_id", browserID).
			Int32("existing_browser_id", state.duplicateBrowserID).
			Msg("cef: duplicate popup browser attached after shell already had a browser; closing duplicate")
		host.CloseBrowser(1)
		return
	}

	h.finishAfterCreated(browser, host, state)
}

func (h *handlerSet) attachAfterCreatedBrowser(
	browser purecef.Browser,
	host purecef.BrowserHost,
	browserID int32,
) afterCreatedState {
	state := afterCreatedState{}
	h.wv.mu.Lock()
	defer h.wv.mu.Unlock()

	if existing := h.wv.browser; existing != nil {
		existingID := existing.GetIdentifier()
		if existingID != 0 && existingID != browserID {
			state.closeDuplicate = true
			state.duplicateBrowserID = existingID
			return state
		}
	}

	h.wv.browser = browser
	h.wv.host = host
	h.wv.pendingCreate = nil
	h.wv.input.setHost(host)
	if h.wv.findCtrl != nil {
		h.wv.findCtrl.setHost(host)
	}
	state.nativePopupParent = h.wv.nativePopupParent
	state.nativePopupID = h.wv.nativePopupID
	h.wv.nativePopupParent = nil
	h.wv.nativePopupID = 0
	h.wv.nativePopupFallbackStarted = false

	// Mark browser as visible — CEF OSR starts in hidden state and suppresses
	// painting/caret updates until explicitly told the browser is shown.
	host.WasHidden(0)
	state.shouldSyncFocus = h.afterCreatedShouldSyncFocus()
	state.hasPendingNavigation = strings.TrimSpace(h.wv.pendingURI) != ""
	return state
}

func (h *handlerSet) afterCreatedShouldSyncFocus() bool {
	if h.wv.input == nil {
		return false
	}
	if h.wv.input.hasGTKFocus() {
		return true
	}
	return h.wv.input.glArea != nil && h.wv.input.glArea.HasFocus()
}

func (h *handlerSet) finishAfterCreated(
	browser purecef.Browser,
	host purecef.BrowserHost,
	state afterCreatedState,
) {
	if h.wv.engine != nil {
		h.wv.engine.recordBrowserAfterCreated(browser)
		h.wv.engine.registerWebView(h.wv)
		h.wv.engine.bindBrowserWebView(browser, h.wv)
	}
	h.wv.stopNativePopupFallbackTimer()
	if state.shouldSyncFocus {
		syncWindowlessBrowserFocus(host)
	} else {
		// Request an initial OSR frame even before the first real navigation
		// commits. This restores the about:blank warm-up paint that the stable
		// startup path relied on and prevents the render pipeline from staying idle.
		host.Invalidate(purecef.PaintElementTypePetView)
	}
	if state.hasPendingNavigation {
		h.wv.schedulePendingNavigationReplay(0)
	}
	if state.nativePopupParent != nil && state.nativePopupID != 0 {
		state.nativePopupParent.clearPendingNativePopup(state.nativePopupID, h.wv)
	}
	h.wv.scheduleStartBeginFrameLoop()
	h.wv.fireReadyToShow()
}

type afterCreatedState struct {
	shouldSyncFocus      bool
	hasPendingNavigation bool
	nativePopupParent    *WebView
	nativePopupID        int32
	duplicateBrowserID   int32
	closeDuplicate       bool
}

// DoClose returns false to allow the default close behavior.
func (h *handlerSet) DoClose(_ purecef.Browser) bool {
	return false
}

// OnBeforeClose fires the OnClose callback.
func (h *handlerSet) OnBeforeClose(browser purecef.Browser) {
	if h.wv.engine != nil {
		browserID := int32(0)
		if browser != nil {
			browserID = browser.GetIdentifier()
		}
		h.wv.engine.unregisterWebView(h.wv, browserID)
	}
	h.wv.mu.Lock()
	h.wv.browser = nil
	h.wv.host = nil
	h.wv.input.setHost(nil)
	if h.wv.findCtrl != nil {
		h.wv.findCtrl.setHost(nil)
	}
	h.wv.mu.Unlock()
	h.wv.resizeReconciler.stop()
	h.wv.scheduleStopBeginFrameLoop()

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb != nil && cb.OnClose != nil {
		h.wv.runOnGTK(func() {
			cb.OnClose()
		})
	}
	h.wv.runCloseCallbacks()
}

// ===========================================================================
// RequestHandler (11 methods)
// ===========================================================================

func (h *handlerSet) OnBeforeBrowse(browser purecef.Browser, frame purecef.Frame, request purecef.Request, _, _ int32) bool {
	if frame == nil || !frame.IsMain() || request == nil {
		return false
	}

	handler := h.downloadHandler()
	if handler == nil || !strings.EqualFold(request.GetMethod(), "GET") {
		return false
	}

	url := request.GetURL()
	if !downloadutil.ShouldForceDownloadForURI(url) || browser == nil {
		return false
	}

	host := browser.GetHost()
	if host == nil {
		return false
	}

	logging.FromContext(h.currentContext()).Debug().
		Str("url", logging.TruncateURL(url, maxSchemeTruncatedURLLength)).
		Msg("cef: forcing download for navigation")

	host.StartDownload(url)
	return true
}

func (h *handlerSet) CanDownload(browser purecef.Browser, url, requestMethod string) bool {
	handler := h.downloadHandler()
	if handler == nil {
		return false
	}
	return handler.canDownload(browser, url, requestMethod)
}

func (h *handlerSet) OnBeforeDownload(
	browser purecef.Browser,
	downloadItem purecef.DownloadItem,
	suggestedName string,
	callback purecef.BeforeDownloadCallback,
) bool {
	handler := h.downloadHandler()
	if handler == nil {
		return false
	}
	return handler.onBeforeDownload(h.currentContext(), browser, downloadItem, suggestedName, callback)
}

func (h *handlerSet) OnDownloadUpdated(
	_ purecef.Browser,
	downloadItem purecef.DownloadItem,
	callback purecef.DownloadItemCallback,
) {
	handler := h.downloadHandler()
	if handler == nil {
		return
	}
	handler.onDownloadUpdated(h.currentContext(), downloadItem, callback)
}

func (h *handlerSet) downloadHandler() *downloadHandler {
	if h == nil || h.wv == nil || h.wv.engine == nil {
		return nil
	}
	return h.wv.engine.currentDownloadHandler()
}

func (h *handlerSet) currentContext() context.Context {
	if h == nil || h.wv == nil || h.wv.ctx == nil {
		return context.Background()
	}
	return h.wv.ctx
}

func (h *handlerSet) OnOpenUrlfromTab(
	_ purecef.Browser,
	_ purecef.Frame,
	_ string,
	_ purecef.WindowOpenDisposition,
	_ int32,
) int32 {
	return 0
}

func (h *handlerSet) GetResourceRequestHandler(
	_ purecef.Browser, _ purecef.Frame, _ purecef.Request,
	_, _ int32, _ string, _ *int32,
) purecef.ResourceRequestHandler {
	return nil
}

func (h *handlerSet) GetAuthCredentials(
	_ purecef.Browser, _ string, _ int32, _ string, _ int32,
	_, _ string, _ purecef.AuthCallback,
) int32 {
	return 0
}

func (h *handlerSet) OnCertificateError(
	_ purecef.Browser,
	_ purecef.Errorcode,
	_ string,
	_ purecef.Sslinfo,
	_ purecef.Callback,
) int32 {
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

// OnAudioStreamStarted handles the start of an audio stream.
// The third callback argument is the channel count (not frames-per-buffer);
// frames-per-buffer comes from params.FramesPerBuffer.
func (h *handlerSet) OnAudioStreamStarted(_ purecef.Browser, params *purecef.AudioParameters, channels int32) {
	// Validate params and factory first
	if params == nil || h.wv.audioOutputFactory == nil {
		return
	}

	// Build format from CEF parameters
	format := h.buildAudioStreamFormat(params, channels)

	if h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Info().
			Int("sample_rate", format.SampleRate).
			Int("channels", format.ChannelCount).
			Int("frames_per_buffer", format.FramesPerBuffer).
			Int32("channel_layout", params.ChannelLayout).
			Msg("cef: OnAudioStreamStarted")
	}

	// Reset packet counters for the new stream
	h.wv.audioPacketCount.Store(0)
	h.wv.audioWriteCount.Store(0)

	// Close any existing stream first
	h.wv.closeAudioStream()

	// Create new stream
	ctx := h.wv.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	stream, err := h.wv.audioOutputFactory.NewStream(ctx, format)
	if err != nil {
		// Log error but don't panic - audio is non-critical.
		// Do NOT set audioPlaying — the stream was never created.
		if h.wv.ctx != nil {
			logging.FromContext(h.wv.ctx).Warn().
				Err(err).
				Int("sample_rate", format.SampleRate).
				Int("channels", format.ChannelCount).
				Msg("cef: failed to create audio output stream")
		}
		return
	}

	// Set playing state only after stream creation succeeds
	h.wv.setAudioPlaying(true)

	if h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Info().
			Int("sample_rate", format.SampleRate).
			Int("channels", format.ChannelCount).
			Int("frames_per_buffer", format.FramesPerBuffer).
			Msg("cef: audio output stream created successfully")
	}

	// Store the new stream
	h.wv.audioStreamMu.Lock()
	h.wv.activeAudioStream = stream
	h.wv.audioStreamMu.Unlock()
}

// buildAudioStreamFormat converts CEF AudioParameters to port.AudioStreamFormat.
// channels is the authoritative channel count from the callback; params.FramesPerBuffer
// supplies the buffer size.
func (h *handlerSet) buildAudioStreamFormat(params *purecef.AudioParameters, channels int32) port.AudioStreamFormat {
	return port.AudioStreamFormat{
		SampleRate:      int(params.SampleRate),
		ChannelCount:    int(channels),
		FramesPerBuffer: int(params.FramesPerBuffer),
	}
}

// OnAudioStreamPacket receives audio packets and forwards them to the output stream.
// The mutex is held across both the stream snapshot and Write to prevent
// closeAudioStream from closing the stream mid-write.
func (h *handlerSet) OnAudioStreamPacket(_ purecef.Browser, data [][]float32, frames int32, pts int64) {
	if len(data) == 0 || frames <= 0 {
		return
	}

	// Copy the data before acquiring the stream lock because CEF can reuse the
	// buffer as soon as this callback returns.
	copiedData := make([][]float32, len(data))
	for i, channel := range data {
		if len(channel) < int(frames) {
			continue
		}
		copiedData[i] = make([]float32, frames)
		copy(copiedData[i], channel[:frames])
	}

	// Hold the lock across the stream read and Write so closeAudioStream
	// cannot close the stream between snapshot and write.
	h.wv.audioStreamMu.Lock()
	stream := h.wv.activeAudioStream
	if stream == nil {
		h.wv.audioStreamMu.Unlock()
		return
	}

	pktNum := h.wv.audioPacketCount.Add(1)
	if pktNum == 1 && h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Info().
			Int("channels", len(data)).
			Int32("frames", frames).
			Int64("pts", pts).
			Msg("cef: first audio packet received")
	}

	err := stream.Write(copiedData)
	h.wv.audioStreamMu.Unlock()

	if err != nil {
		if h.wv.ctx != nil {
			logging.FromContext(h.wv.ctx).Debug().
				Err(err).
				Uint64("packets_received", pktNum).
				Msg("cef: audio stream write failed")
		}
		return
	}
	h.wv.audioWriteCount.Add(1)
}

// OnAudioStreamStopped handles stream stop.
func (h *handlerSet) OnAudioStreamStopped(_ purecef.Browser) {
	if h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Info().
			Uint64("packets_received", h.wv.audioPacketCount.Load()).
			Uint64("writes_succeeded", h.wv.audioWriteCount.Load()).
			Msg("cef: OnAudioStreamStopped")
	}
	h.wv.setAudioPlaying(false)
	h.wv.closeAudioStream()
}

// OnAudioStreamError handles stream errors.
func (h *handlerSet) OnAudioStreamError(_ purecef.Browser, message string) {
	if h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Warn().
			Str("error", message).
			Uint64("packets_received", h.wv.audioPacketCount.Load()).
			Uint64("writes_succeeded", h.wv.audioWriteCount.Load()).
			Msg("cef: audio stream error")
	}
	h.wv.setAudioPlaying(false)
	h.wv.closeAudioStream()
}

// ===========================================================================
// FindHandler (1 method)
// ===========================================================================

// OnFindResult dispatches CEF find results to the WebView's FindController.
func (h *handlerSet) OnFindResult(
	_ purecef.Browser,
	identifier, count int32,
	_ *purecef.Rect,
	activematchordinal, finalupdate int32,
) {
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
