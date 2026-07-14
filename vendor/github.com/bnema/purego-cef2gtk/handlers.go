package cef2gtk

import (
	"errors"
	"sync"
	"time"

	"github.com/bnema/purego-cef/cef"
	"github.com/bnema/purego-cef2gtk/internal/gtkgl"
)

type renderQueue interface {
	ImportAndQueueOnGTKThread(*cef.AcceleratedPaintInfo) (gtkgl.QueuedFrame, error)
	QueueRender()
}

type asyncRenderQueue interface {
	ImportAndQueueAsync(*cef.AcceleratedPaintInfo, func(error)) error
}

// deferredAcceleratedError prevents an async renderer that reports an error
// synchronously from invoking a user hook while OnAcceleratedPaint holds the
// lifecycle read lock. Errors reported after release are already outside it.
type deferredAcceleratedError struct {
	h        *renderHandler
	mu       sync.Mutex
	released bool
	pending  []error
}

func (d *deferredAcceleratedError) report(err error) {
	if d == nil || err == nil {
		return
	}
	d.mu.Lock()
	if !d.released {
		d.pending = append(d.pending, err)
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()
	d.h.handleAcceleratedError(err)
}

func (d *deferredAcceleratedError) release() {
	if d == nil {
		return
	}
	d.mu.Lock()
	if d.released {
		d.mu.Unlock()
		return
	}
	d.released = true
	pending := d.pending
	d.pending = nil
	d.mu.Unlock()
	for _, err := range pending {
		d.h.handleAcceleratedError(err)
	}
}

type renderHandler struct {
	view        *View
	renderer    renderQueue
	diag        *diagnosticsRecorder
	staticHooks Hooks
}

var _ cef.RenderHandler = (*renderHandler)(nil)

// RenderHandler returns a CEF render handler adapter for this view.
func (v *View) RenderHandler(hooks Hooks) cef.RenderHandler {
	if v == nil {
		return &renderHandler{staticHooks: hooks}
	}

	// Serialize handler replacement with Destroy, which clears both handler and
	// renderer. Hook readers use firstPresentationMu and always copy before
	// invoking user code.
	v.renderLifecycleMu.Lock()
	defer v.renderLifecycleMu.Unlock()
	v.setHooks(hooks)
	v.configureFirstPresentationHooks()
	if v.handler != nil {
		return v.handler
	}
	v.handler = &renderHandler{view: v, renderer: v.renderer, diag: v.diag}
	return v.handler
}

func (h *renderHandler) GetAccessibilityHandler() cef.AccessibilityHandler { return nil }
func (h *renderHandler) GetRootScreenRect(_ cef.Browser, rect *cef.Rect) int32 {
	if rect == nil || h == nil || h.view == nil {
		return 0
	}
	rect.X, rect.Y = 0, 0
	rect.Width, rect.Height = h.view.osrViewRectSize()
	h.view.traceScreenGeometryCallback("root-screen-rect", 0, 0, true)
	return 1
}
func (h *renderHandler) GetViewRect(_ cef.Browser, rect *cef.Rect) {
	if rect == nil {
		return
	}
	rect.X, rect.Y = 0, 0
	if h.view == nil {
		rect.Width, rect.Height = 1, 1
		return
	}
	rect.Width, rect.Height = h.view.osrViewRectSize()
	h.view.traceViewRect(rect.Width, rect.Height)
}
func (h *renderHandler) GetScreenPoint(_ cef.Browser, viewX, viewY int32, screenX, screenY *int32) int32 {
	if screenX == nil || screenY == nil || h == nil || h.view == nil {
		return 0
	}
	// Wayland does not expose stable global screen coordinates to clients. CEF's
	// OSR callbacks still need a consistent coordinate space for hit-testing and
	// popup placement. Use a view-local root screen with device-pixel output for
	// Linux as required by CefRenderHandler::GetScreenPoint.
	*screenX, *screenY = h.view.osrScreenPoint(viewX, viewY)
	h.view.traceScreenGeometryCallback("screen-point", viewX, viewY, true)
	return 1
}
func (h *renderHandler) GetScreenInfo(_ cef.Browser, info *cef.ScreenInfo) int32 {
	if info == nil || h.view == nil {
		return 0
	}
	width, height := h.view.osrViewRectSize()
	rect := cef.Rect{X: 0, Y: 0, Width: width, Height: height}
	si := cef.NewScreenInfo()
	si.DeviceScaleFactor = h.view.osrScreenInfoScale()
	h.view.traceScreenInfo(width, height, si.DeviceScaleFactor)
	si.Depth = 24
	si.DepthPerComponent = 8
	si.Rect = rect
	si.AvailableRect = rect
	*info = si
	return 1
}
func (h *renderHandler) OnPopupShow(cef.Browser, int32)     {}
func (h *renderHandler) OnPopupSize(cef.Browser, *cef.Rect) {}
func (h *renderHandler) OnPaint(cef.Browser, cef.PaintElementType, []cef.Rect, []byte, int32, int32) {
	if h.diag != nil {
		h.diag.RecordUnsupportedPaint()
	}
	if h.view != nil {
		h.view.recordProfileUnsupportedPaint()
		h.view.emitProfileIfDue(time.Now())
	}
	hooks := h.hooks()
	if hooks.OnUnsupportedPaint != nil {
		hooks.OnUnsupportedPaint()
	}
}
func (h *renderHandler) OnAcceleratedPaint(_ cef.Browser, _ cef.PaintElementType, _ []cef.Rect, info *cef.AcceleratedPaintInfo) {
	if h.diag != nil {
		h.diag.RecordAcceleratedPaint()
	}
	if h.view != nil {
		// Claim the one-shot state while teardown is excluded, but invoke its hooks
		// only after releasing the lifecycle lock. A hook is allowed to Destroy its
		// view, and must not wait on the read lock held by this CEF callback.
		h.view.renderLifecycleMu.RLock()
		if h.view.destroyed.Load() {
			h.view.renderLifecycleMu.RUnlock()
			if h.diag != nil {
				h.diag.RecordStaleAcceleratedPaint()
			}
			return
		}
		hooks, unsupported, first := h.view.claimFirstAcceleratedPaint()
		h.view.recordProfileFrameReceived()
		h.view.renderLifecycleMu.RUnlock()

		if first {
			invokeFirstAcceleratedPaintHooks(hooks, unsupported)
		}
	}

	errorDelivery := &deferredAcceleratedError{h: h}
	queued := false
	if h.view != nil {
		// Keep imports and queueing serialized with renderer teardown. An async
		// renderer may call its error function synchronously, so defer its hook
		// delivery until after this lock has been released.
		h.view.renderLifecycleMu.RLock()
		if h.view.destroyed.Load() {
			h.view.renderLifecycleMu.RUnlock()
			if h.diag != nil {
				h.diag.RecordStaleAcceleratedPaint()
			}
			return
		}
		queued = h.importAndQueueAcceleratedFrame(info, errorDelivery.report)
		h.view.renderLifecycleMu.RUnlock()
	} else {
		errorDelivery.release()
		queued = h.importAndQueueAcceleratedFrame(info, errorDelivery.report)
	}

	// No user callback (including ProfileOptions.OnSnapshot) may run while the
	// lifecycle lock above protects renderer teardown.
	errorDelivery.release()
	if queued && h.view != nil {
		h.view.emitProfileIfDue(time.Now())
	}
}

// importAndQueueAcceleratedFrame performs renderer work while its caller holds
// the lifecycle lock, if any. onError must not invoke user code synchronously.
func (h *renderHandler) importAndQueueAcceleratedFrame(info *cef.AcceleratedPaintInfo, onError func(error)) bool {
	if h.renderer == nil {
		onError(gtkgl.ErrNilAcceleratedRenderer)
		return false
	}
	if async, ok := h.renderer.(asyncRenderQueue); ok {
		if err := async.ImportAndQueueAsync(info, onError); err != nil {
			onError(err)
			return false
		}
		h.recordAcceleratedFrameQueued()
		return true
	}
	if _, err := h.renderer.ImportAndQueueOnGTKThread(info); err != nil {
		onError(err)
		return false
	}
	h.recordAcceleratedFrameQueued()
	return true
}

func (h *renderHandler) recordAcceleratedFrameQueued() {
	h.renderer.QueueRender()
	if h.view != nil {
		h.view.recordProfileFrameQueued()
	}
}
func (h *renderHandler) handleAcceleratedError(err error) {
	if h.diag != nil {
		h.diag.RecordImportFailure(err)
	}
	if h.view != nil {
		h.view.recordProfileImportFailure()
		h.view.emitProfileIfDue(time.Now())
	}
	if isTransientGLAreaLifecycleError(err) {
		return
	}
	hooks := h.hooks()
	if hooks.OnError != nil {
		hooks.OnError(err)
	}
}

func isTransientGLAreaLifecycleError(err error) bool {
	return errors.Is(err, gtkgl.ErrGLAreaNotRealized) || errors.Is(err, gtkgl.ErrMissingGLAreaContext)
}
func (h *renderHandler) hooks() Hooks {
	if h != nil && h.view != nil {
		return h.view.snapshotHooks()
	}
	if h != nil {
		return h.staticHooks
	}
	return Hooks{}
}

func (h *renderHandler) GetTouchHandleSize(cef.Browser, cef.HorizontalAlignment, *cef.Size) {}
func (h *renderHandler) OnTouchHandleStateChanged(cef.Browser, *cef.TouchHandleState)       {}
func (h *renderHandler) StartDragging(cef.Browser, cef.DragData, cef.DragOperationsMask, int32, int32) int32 {
	return 0
}
func (h *renderHandler) UpdateDragCursor(cef.Browser, cef.DragOperationsMask)             {}
func (h *renderHandler) OnScrollOffsetChanged(cef.Browser, float64, float64)              {}
func (h *renderHandler) OnImeCompositionRangeChanged(cef.Browser, *cef.Range, []cef.Rect) {}
func (h *renderHandler) OnTextSelectionChanged(_ cef.Browser, selectedText string, selectedRange *cef.Range) {
	hooks := h.hooks()
	if hooks.OnTextSelectionChanged != nil {
		hooks.OnTextSelectionChanged(selectedText, selectedRange)
	}
}
func (h *renderHandler) OnVirtualKeyboardRequested(cef.Browser, cef.TextInputMode) {}
