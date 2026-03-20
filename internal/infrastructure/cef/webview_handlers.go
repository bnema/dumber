package cef

import (
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// handlerSet implements all CEF handler interfaces and dispatches events to the
// owning WebView. A single struct is used so that the Client's Get*Handler
// methods can return the same receiver, avoiding extra allocations.
type handlerSet struct {
	wv *WebView
}

// Compile-time interface checks.
var (
	_ purecef.Client          = (*handlerSet)(nil)
	_ purecef.RenderHandler   = (*handlerSet)(nil)
	_ purecef.DisplayHandler  = (*handlerSet)(nil)
	_ purecef.LoadHandler     = (*handlerSet)(nil)
	_ purecef.LifeSpanHandler = (*handlerSet)(nil)
	_ purecef.RequestHandler  = (*handlerSet)(nil)
)

// ===========================================================================
// Client
// ===========================================================================

func (h *handlerSet) GetAudioHandler() purecef.AudioHandler             { return nil }
func (h *handlerSet) GetCommandHandler() purecef.CommandHandler         { return nil }
func (h *handlerSet) GetContextMenuHandler() purecef.ContextMenuHandler { return nil }
func (h *handlerSet) GetDialogHandler() purecef.DialogHandler           { return nil }
func (h *handlerSet) GetDisplayHandler() purecef.DisplayHandler         { return h }
func (h *handlerSet) GetDownloadHandler() purecef.DownloadHandler       { return nil }
func (h *handlerSet) GetDragHandler() purecef.DragHandler               { return nil }
func (h *handlerSet) GetFindHandler() purecef.FindHandler               { return nil }
func (h *handlerSet) GetFocusHandler() purecef.FocusHandler             { return nil }
func (h *handlerSet) GetFrameHandler() purecef.FrameHandler             { return nil }
func (h *handlerSet) GetPermissionHandler() purecef.PermissionHandler   { return nil }
func (h *handlerSet) GetJsdialogHandler() purecef.JsdialogHandler       { return nil }
func (h *handlerSet) GetKeyboardHandler() purecef.KeyboardHandler       { return nil }
func (h *handlerSet) GetLifeSpanHandler() purecef.LifeSpanHandler       { return h }
func (h *handlerSet) GetLoadHandler() purecef.LoadHandler               { return h }
func (h *handlerSet) GetPrintHandler() purecef.PrintHandler             { return nil }
func (h *handlerSet) GetRenderHandler() purecef.RenderHandler           { return h }
func (h *handlerSet) GetRequestHandler() purecef.RequestHandler         { return h }

func (h *handlerSet) OnProcessMessageReceived(_ purecef.Browser, _ purecef.Frame, _ purecef.ProcessID, _ purecef.ProcessMessage) int32 {
	return 0
}

// ===========================================================================
// RenderHandler (17 methods)
// ===========================================================================

func (h *handlerSet) GetAccessibilityHandler() purecef.AccessibilityHandler { return nil }

func (h *handlerSet) GetRootScreenRect(_ purecef.Browser, _ *purecef.Rect) int32 { return 0 }

// GetViewRect fills the rect struct with the pipeline dimensions.
// The rect pointer points to a cef_rect_t: {HostLayout padding, X, Y, Width, Height} all int32.
// The HostLayout field occupies 0 bytes on most platforms but we use the Rect type alias to be safe.
func (h *handlerSet) GetViewRect(_ purecef.Browser, rect *purecef.Rect) {
	if rect == nil {
		return
	}
	h.wv.pipeline.mu.Lock()
	w := h.wv.pipeline.width
	ht := h.wv.pipeline.height
	h.wv.pipeline.mu.Unlock()

	// CEF requires a non-empty rect. Return a 1x1 fallback if the GL area
	// has not been realized yet.
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
}

func (h *handlerSet) GetScreenPoint(_ purecef.Browser, _, _ int32, _, _ unsafe.Pointer) int32 {
	return 0
}

func (h *handlerSet) GetScreenInfo(_ purecef.Browser, _ *purecef.ScreenInfo) int32 { return 0 }

func (h *handlerSet) OnPopupShow(_ purecef.Browser, _ int32) {}

func (h *handlerSet) OnPopupSize(_ purecef.Browser, _ *purecef.Rect) {}

// OnPaint receives the BGRA pixel buffer from CEF and forwards dirty rects
// to the render pipeline for GPU upload.
func (h *handlerSet) OnPaint(
	_ purecef.Browser, _ purecef.PaintElementType,
	dirtyRects []purecef.Rect, buffer unsafe.Pointer, width, height int32,
) {
	rects := make([]rect, len(dirtyRects))
	for i, dr := range dirtyRects {
		rects[i] = rect{X: dr.X, Y: dr.Y, Width: dr.Width, Height: dr.Height}
	}
	h.wv.pipeline.handlePaint(buffer, width, height, rects)
}

func (h *handlerSet) OnAcceleratedPaint(_ purecef.Browser, _ purecef.PaintElementType, _ []purecef.Rect, _ *purecef.AcceleratedPaintInfo) {
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
		cb.OnEnterFullscreen()
	}
	if !entering && cb.OnLeaveFullscreen != nil {
		cb.OnLeaveFullscreen()
	}
}

func (h *handlerSet) OnTooltip(_ purecef.Browser, _ uintptr) int32 { return 0 }

// OnStatusMessage fires the OnLinkHover callback with the status text.
func (h *handlerSet) OnStatusMessage(_ purecef.Browser, value string) {
	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb != nil && cb.OnLinkHover != nil {
		cb.OnLinkHover(value)
	}
}

func (h *handlerSet) OnConsoleMessage(_ purecef.Browser, _ purecef.LogSeverity, _, _ string, _ int32) int32 {
	return 0
}

func (h *handlerSet) OnAutoResize(_ purecef.Browser, _ *purecef.Size) int32 { return 0 }

// OnLoadingProgressChange updates the cached progress value.
func (h *handlerSet) OnLoadingProgressChange(_ purecef.Browser, progress float64) {
	h.wv.updateProgress(progress)
}

func (h *handlerSet) OnCursorChange(_ purecef.Browser, _ uintptr, cursorType purecef.CursorType, _ *purecef.CursorInfo) int32 {
	name := cefCursorToGDKName(cursorType)
	h.wv.pipeline.glArea.SetCursorFromName(&name)
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
			cb.OnLoadChanged(port.LoadStarted)
		} else {
			cb.OnLoadChanged(port.LoadFinished)
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
		cb.OnLoadChanged(port.LoadCommitted)
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
	h.wv.crashCount = 0
	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb != nil {
		if cb.OnLoadChanged != nil {
			cb.OnLoadChanged(port.LoadFinished)
		}
		if cb.OnProgressChanged != nil {
			cb.OnProgressChanged(1.0)
		}
	}
}

// OnLoadError is a no-op in Phase 1.
func (h *handlerSet) OnLoadError(_ purecef.Browser, _ purecef.Frame, _ purecef.Errorcode, _, _ string) {
}

// ===========================================================================
// LifeSpanHandler (6 methods)
// ===========================================================================

// OnBeforePopup blocks all popups in Phase 1.
func (h *handlerSet) OnBeforePopup(
	_ purecef.Browser, _ purecef.Frame, _ int32, _, _ string,
	_ purecef.WindowOpenDisposition, _ int32, _ *purecef.PopupFeatures,
	_ *purecef.WindowInfo, _ unsafe.Pointer, _ *purecef.BrowserSettings,
	_, _ unsafe.Pointer,
) bool {
	return true // block
}

func (h *handlerSet) OnBeforePopupAborted(_ purecef.Browser, _ int32) {}

func (h *handlerSet) OnBeforeDevToolsPopup(
	_ purecef.Browser, _ *purecef.WindowInfo, _ unsafe.Pointer,
	_ *purecef.BrowserSettings, _, _ unsafe.Pointer,
) {
}

// OnAfterCreated stores the browser and host references and enables input.
func (h *handlerSet) OnAfterCreated(browser purecef.Browser) {
	log := logging.FromContext(h.wv.ctx)
	log.Debug().
		Bool("browser_nil", browser == nil).
		Msg("cef: OnAfterCreated")
	h.wv.browser = browser
	h.wv.host = browser.GetHost()
	h.wv.input.setHost(h.wv.host)

	// Replay any navigation that was requested before the browser existed.
	if uri := h.wv.pendingURI; uri != "" {
		h.wv.pendingURI = ""
		browser.GetMainFrame().LoadURL(uri)
	}
}

// DoClose returns false to allow the default close behavior.
func (h *handlerSet) DoClose(_ purecef.Browser) bool {
	return false
}

// OnBeforeClose fires the OnClose callback.
func (h *handlerSet) OnBeforeClose(_ purecef.Browser) {
	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb != nil && cb.OnClose != nil {
		cb.OnClose()
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
	_ purecef.Browser, _ purecef.Frame, _ purecef.Request,
	_, _ int32, _ string, _ unsafe.Pointer,
) purecef.ResourceRequestHandler {
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
	_ purecef.Browser, _ int32, _ string, _ int32, _ int,
	_ unsafe.Pointer, _ purecef.SelectClientCertificateCallback,
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
	h.wv.crashCount++
	if h.wv.crashCount > maxConsecutiveCrashes {
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
		cb.OnWebProcessTerminated(reason, label, uri)
	}
}

func (h *handlerSet) OnDocumentAvailableInMainFrame(_ purecef.Browser) {}

// cefCursorToGDKName maps CEF cursor types to GDK/CSS cursor names.
//
//nolint:mnd,cyclop // cursor type lookup table
func cefCursorToGDKName(ct purecef.CursorType) string {
	switch ct {
	case purecef.CursorTypeCtPointer:
		return "default"
	case purecef.CursorTypeCtCross:
		return "crosshair"
	case purecef.CursorTypeCtHand:
		return "pointer"
	case purecef.CursorTypeCtIbeam:
		return "text"
	case purecef.CursorTypeCtWait:
		return "wait"
	case purecef.CursorTypeCtHelp:
		return "help"
	case purecef.CursorTypeCtEastresize:
		return "e-resize"
	case purecef.CursorTypeCtNorthresize:
		return "n-resize"
	case purecef.CursorTypeCtNortheastresize:
		return "ne-resize"
	case purecef.CursorTypeCtNorthwestresize:
		return "nw-resize"
	case purecef.CursorTypeCtSouthresize:
		return "s-resize"
	case purecef.CursorTypeCtSoutheastresize:
		return "se-resize"
	case purecef.CursorTypeCtSouthwestresize:
		return "sw-resize"
	case purecef.CursorTypeCtWestresize:
		return "w-resize"
	case purecef.CursorTypeCtNorthsouthresize:
		return "ns-resize"
	case purecef.CursorTypeCtEastwestresize:
		return "ew-resize"
	case purecef.CursorTypeCtNortheastsouthwestresize:
		return "nesw-resize"
	case purecef.CursorTypeCtNorthwestsoutheastresize:
		return "nwse-resize"
	case purecef.CursorTypeCtColumnresize:
		return "col-resize"
	case purecef.CursorTypeCtRowresize:
		return "row-resize"
	case purecef.CursorTypeCtMove:
		return "move"
	case purecef.CursorTypeCtProgress:
		return "progress"
	case purecef.CursorTypeCtNodrop:
		return "no-drop"
	case purecef.CursorTypeCtNotallowed:
		return "not-allowed"
	case purecef.CursorTypeCtGrab:
		return "grab"
	case purecef.CursorTypeCtGrabbing:
		return "grabbing"
	case purecef.CursorTypeCtZoomin:
		return "zoom-in"
	case purecef.CursorTypeCtZoomout:
		return "zoom-out"
	default:
		return "default"
	}
}
