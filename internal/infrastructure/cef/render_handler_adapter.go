package cef

import (
	"sync"

	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"

	"github.com/bnema/dumber/internal/logging"
)

type popupSurface interface {
	SetPopupVisible(bool)
	SetPopupRect(purecef.Rect)
}

type dumberRenderHandler struct {
	wv           *WebView
	main         purecef.RenderHandler
	popup        purecef.RenderHandler
	popupSurface popupSurface
}

var _ purecef.RenderHandler = (*dumberRenderHandler)(nil)

// startupPresentationHooks consumes the one-shot facts emitted by
// purego-cef2gtk. The bridge owns the native DMABUF and frame-clock boundaries;
// Dumber only records their ordered application-level timeline.
func startupPresentationHooks(trace *startupTrace) cef2gtk.Hooks {
	return cef2gtk.Hooks{
		OnFirstAcceleratedPaint: func() {
			trace.Mark("first_accelerated_paint_received")
		},
		OnFirstDMABUFTextureSwap: func() {
			trace.Mark("first_dmabuf_texture_swap")
		},
		OnFirstPresentation: func() {
			trace.MarkGTKAfterPaint()
		},
		OnDMABUFUnsupported: func() {
			trace.SetIncompleteReason("dmabuf_texture_swap_unavailable")
		},
	}
}

func newDumberRenderHandler(wv *WebView) purecef.RenderHandler {
	if wv == nil || wv.viewBridge == nil {
		return nil
	}
	h := &dumberRenderHandler{wv: wv}

	var unsupportedPaintOnce sync.Once
	hooks := startupPresentationHooks(activeStartupTrace())
	// Tear down the theme background flash guard once the first frame is
	// presented, without dropping the startup trace mark the bridge already set.
	firstAcceleratedPaint := hooks.OnFirstAcceleratedPaint
	hooks.OnFirstAcceleratedPaint = func() {
		if firstAcceleratedPaint != nil {
			firstAcceleratedPaint()
		}
		wv.markFirstFramePainted()
	}
	hooks.OnUnsupportedPaint = func() {
		unsupportedPaintOnce.Do(func() {
			if wv.ctx != nil {
				logging.FromContext(wv.ctx).Warn().Msg("cef: unsupported CPU paint from accelerated bridge")
			}
		})
	}
	hooks.OnError = func(err error) {
		if wv.ctx != nil {
			logging.FromContext(wv.ctx).Warn().Err(err).Msg("cef: accelerated render bridge error")
		}
	}
	hooks.OnTextSelectionChanged = func(selectedText string, _ *purecef.Range) {
		handleRenderTextSelectionChanged(wv, selectedText)
	}
	h.main = wv.viewBridge.RenderHandler(hooks)

	if wv.popupSurface != nil {
		var popupUnsupportedPaintOnce sync.Once
		h.popupSurface = wv.popupSurface
		h.popup = wv.popupSurface.RenderHandler(cef2gtk.Hooks{
			OnUnsupportedPaint: func() {
				popupUnsupportedPaintOnce.Do(func() {
					if wv.ctx != nil {
						logging.FromContext(wv.ctx).Warn().Msg("cef: unsupported CPU paint from popup surface")
					}
				})
			},
			OnError: func(err error) {
				if wv.ctx != nil {
					logging.FromContext(wv.ctx).Warn().Err(err).Msg("cef: popup surface render error")
				}
			},
		})
	}

	return h
}

func (h *dumberRenderHandler) renderTargetForPaint(elementType purecef.PaintElementType) purecef.RenderHandler {
	if h == nil {
		return nil
	}
	if elementType == purecef.PaintElementTypePetPopup && h.popup != nil {
		return h.popup
	}
	return h.main
}

func (h *dumberRenderHandler) GetAccessibilityHandler() purecef.AccessibilityHandler {
	if h == nil || h.main == nil {
		return nil
	}
	return h.main.GetAccessibilityHandler()
}

func (h *dumberRenderHandler) GetRootScreenRect(browser purecef.Browser, rect *purecef.Rect) int32 {
	if h == nil || h.main == nil {
		return 0
	}
	return h.main.GetRootScreenRect(browser, rect)
}

func (h *dumberRenderHandler) GetViewRect(browser purecef.Browser, rect *purecef.Rect) {
	if h == nil || h.main == nil {
		if rect != nil {
			rect.X, rect.Y, rect.Width, rect.Height = 0, 0, 1, 1
		}
		return
	}
	h.main.GetViewRect(browser, rect)
}

func (h *dumberRenderHandler) GetScreenPoint(browser purecef.Browser, viewX, viewY int32, screenX, screenY *int32) int32 {
	if h == nil || h.main == nil {
		return 0
	}
	return h.main.GetScreenPoint(browser, viewX, viewY, screenX, screenY)
}

func (h *dumberRenderHandler) GetScreenInfo(browser purecef.Browser, info *purecef.ScreenInfo) int32 {
	if h == nil || h.main == nil {
		return 0
	}
	return h.main.GetScreenInfo(browser, info)
}

func (h *dumberRenderHandler) OnPopupShow(browser purecef.Browser, show int32) {
	if h != nil && h.popupSurface != nil {
		visible := show != 0
		if h.wv != nil {
			h.wv.runOnGTKSync(func() {
				h.popupSurface.SetPopupVisible(visible)
			})
		} else {
			h.popupSurface.SetPopupVisible(visible)
		}
	}
	if h != nil && h.popup != nil {
		h.popup.OnPopupShow(browser, show)
	}
}

func (h *dumberRenderHandler) OnPopupSize(browser purecef.Browser, rect *purecef.Rect) {
	if h != nil && h.popupSurface != nil && rect != nil {
		popupRect := *rect
		if h.wv != nil {
			popupRect = popupRectForWidget(h.wv, popupRect)
			h.wv.runOnGTKSync(func() {
				h.popupSurface.SetPopupRect(popupRect)
			})
		} else {
			h.popupSurface.SetPopupRect(popupRect)
		}
	}
	if h != nil && h.popup != nil {
		h.popup.OnPopupSize(browser, rect)
	}
}

func popupRectForWidget(wv *WebView, rect purecef.Rect) purecef.Rect {
	if wv == nil {
		return rect
	}
	// Only convert when the OSR backing-scale compatibility path is active.
	// A monitor scale above 1 is not enough by itself because CEF already reports
	// logical popup coordinates when backing-scale mode is off.
	if wv.osrBackingScaleFactor() <= 1 {
		return rect
	}
	scale := wv.viewBridgeScale()
	return purecef.Rect{
		X:      deviceToLogicalCoord(rect.X, scale),
		Y:      deviceToLogicalCoord(rect.Y, scale),
		Width:  deviceToLogicalSize(rect.Width, scale),
		Height: deviceToLogicalSize(rect.Height, scale),
	}
}

func (h *dumberRenderHandler) OnPaint(
	browser purecef.Browser,
	elementType purecef.PaintElementType,
	dirtyRects []purecef.Rect,
	buffer []byte,
	width, height int32,
) {
	if delegate := h.renderTargetForPaint(elementType); delegate != nil {
		delegate.OnPaint(browser, elementType, dirtyRects, buffer, width, height)
	}
}

func (h *dumberRenderHandler) OnAcceleratedPaint(
	browser purecef.Browser,
	elementType purecef.PaintElementType,
	dirtyRects []purecef.Rect,
	info *purecef.AcceleratedPaintInfo,
) {
	if delegate := h.renderTargetForPaint(elementType); delegate != nil {
		delegate.OnAcceleratedPaint(browser, elementType, dirtyRects, info)
	}
}

func (h *dumberRenderHandler) GetTouchHandleSize(browser purecef.Browser, orientation purecef.HorizontalAlignment, size *purecef.Size) {
	if h == nil || h.main == nil {
		return
	}
	h.main.GetTouchHandleSize(browser, orientation, size)
}

func (h *dumberRenderHandler) OnTouchHandleStateChanged(browser purecef.Browser, state *purecef.TouchHandleState) {
	if h == nil || h.main == nil {
		return
	}
	h.main.OnTouchHandleStateChanged(browser, state)
}

func (h *dumberRenderHandler) StartDragging(
	browser purecef.Browser,
	dragData purecef.DragData,
	allowedOps purecef.DragOperationsMask,
	x, y int32,
) int32 {
	if h == nil || h.main == nil {
		return 0
	}
	return h.main.StartDragging(browser, dragData, allowedOps, x, y)
}

func (h *dumberRenderHandler) UpdateDragCursor(browser purecef.Browser, operation purecef.DragOperationsMask) {
	if h == nil || h.main == nil {
		return
	}
	h.main.UpdateDragCursor(browser, operation)
}

func (h *dumberRenderHandler) OnScrollOffsetChanged(browser purecef.Browser, x, y float64) {
	if h == nil || h.main == nil {
		return
	}
	h.main.OnScrollOffsetChanged(browser, x, y)
}

func (h *dumberRenderHandler) OnImeCompositionRangeChanged(
	browser purecef.Browser,
	selectedRange *purecef.Range,
	characterBounds []purecef.Rect,
) {
	if h == nil || h.main == nil {
		return
	}
	h.main.OnImeCompositionRangeChanged(browser, selectedRange, characterBounds)
}

func (h *dumberRenderHandler) OnTextSelectionChanged(browser purecef.Browser, selectedText string, selectedRange *purecef.Range) {
	if h == nil || h.main == nil {
		return
	}
	h.main.OnTextSelectionChanged(browser, selectedText, selectedRange)
}

func (h *dumberRenderHandler) OnVirtualKeyboardRequested(browser purecef.Browser, mode purecef.TextInputMode) {
	if h == nil || h.main == nil {
		return
	}
	h.main.OnVirtualKeyboardRequested(browser, mode)
}

func handleRenderTextSelectionChanged(wv *WebView, selectedText string) {
	if wv == nil {
		return
	}
	previous, changed := wv.setSelectedText(selectedText)
	if !changed {
		return
	}
	if wv.ctx != nil && selectedText == "" {
		if previous != "" {
			logging.FromContext(wv.ctx).Debug().
				Int("prev_text_len", len(previous)).
				Msg("cef: text selection cleared")
		}
	} else if wv.ctx != nil {
		logging.FromContext(wv.ctx).Debug().
			Int("text_len", len(selectedText)).
			Msg("cef: text selection changed")
	}
	if wv.engine != nil {
		wv.scheduleSelectionUpdate(selectedText)
	}
}
