package cef

import (
	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"

	"github.com/bnema/dumber/internal/logging"
)

type dumberRenderHandler struct {
	wv       *WebView
	delegate purecef.RenderHandler
}

var _ purecef.RenderHandler = (*dumberRenderHandler)(nil)

func newDumberRenderHandler(wv *WebView) purecef.RenderHandler {
	h := &dumberRenderHandler{wv: wv}
	if wv == nil || wv.viewBridge == nil {
		return h
	}
	h.delegate = wv.viewBridge.RenderHandler(cef2gtk.Hooks{
		OnUnsupportedPaint: func() {
			if wv.ctx != nil {
				logging.FromContext(wv.ctx).Warn().Msg("cef: unsupported CPU paint from accelerated bridge")
			}
		},
		OnError: func(err error) {
			if wv.ctx != nil {
				logging.FromContext(wv.ctx).Warn().Err(err).Msg("cef: accelerated render bridge error")
			}
		},
		OnTextSelectionChanged: func(selectedText string, selectedRange *purecef.Range) {
			h.OnTextSelectionChanged(nil, selectedText, selectedRange)
		},
	})
	return h
}

func (h *dumberRenderHandler) GetAccessibilityHandler() purecef.AccessibilityHandler {
	if h.delegate != nil {
		return h.delegate.GetAccessibilityHandler()
	}
	return nil
}
func (h *dumberRenderHandler) GetRootScreenRect(browser purecef.Browser, rect *purecef.Rect) int32 {
	if h.delegate != nil {
		return h.delegate.GetRootScreenRect(browser, rect)
	}
	return 0
}
func (h *dumberRenderHandler) GetViewRect(browser purecef.Browser, rect *purecef.Rect) {
	if h.delegate != nil {
		h.delegate.GetViewRect(browser, rect)
		return
	}
	if rect != nil {
		rect.X, rect.Y, rect.Width, rect.Height = 0, 0, 1, 1
	}
}
func (h *dumberRenderHandler) GetScreenPoint(browser purecef.Browser, viewX, viewY int32, screenX, screenY *int32) int32 {
	if h.delegate != nil {
		return h.delegate.GetScreenPoint(browser, viewX, viewY, screenX, screenY)
	}
	return 0
}
func (h *dumberRenderHandler) GetScreenInfo(browser purecef.Browser, info *purecef.ScreenInfo) int32 {
	if h.delegate != nil {
		return h.delegate.GetScreenInfo(browser, info)
	}
	return 0
}
func (h *dumberRenderHandler) OnPopupShow(browser purecef.Browser, show int32) {
	if h.delegate != nil {
		h.delegate.OnPopupShow(browser, show)
	}
}
func (h *dumberRenderHandler) OnPopupSize(browser purecef.Browser, rect *purecef.Rect) {
	if h.delegate != nil {
		h.delegate.OnPopupSize(browser, rect)
	}
}
func (h *dumberRenderHandler) OnPaint(browser purecef.Browser, elementType purecef.PaintElementType, dirtyRects []purecef.Rect, buffer []byte, width, height int32) {
	if h.delegate != nil {
		h.delegate.OnPaint(browser, elementType, dirtyRects, buffer, width, height)
	}
}
func (h *dumberRenderHandler) OnAcceleratedPaint(browser purecef.Browser, elementType purecef.PaintElementType, dirtyRects []purecef.Rect, info *purecef.AcceleratedPaintInfo) {
	if h.delegate != nil {
		h.delegate.OnAcceleratedPaint(browser, elementType, dirtyRects, info)
	}
}
func (h *dumberRenderHandler) GetTouchHandleSize(browser purecef.Browser, orientation purecef.HorizontalAlignment, size *purecef.Size) {
	if h.delegate != nil {
		h.delegate.GetTouchHandleSize(browser, orientation, size)
	}
}
func (h *dumberRenderHandler) OnTouchHandleStateChanged(browser purecef.Browser, state *purecef.TouchHandleState) {
	if h.delegate != nil {
		h.delegate.OnTouchHandleStateChanged(browser, state)
	}
}
func (h *dumberRenderHandler) StartDragging(browser purecef.Browser, dragData purecef.DragData, allowedOps purecef.DragOperationsMask, x, y int32) int32 {
	if h.delegate != nil {
		return h.delegate.StartDragging(browser, dragData, allowedOps, x, y)
	}
	return 0
}
func (h *dumberRenderHandler) UpdateDragCursor(browser purecef.Browser, operation purecef.DragOperationsMask) {
	if h.delegate != nil {
		h.delegate.UpdateDragCursor(browser, operation)
	}
}
func (h *dumberRenderHandler) OnScrollOffsetChanged(browser purecef.Browser, x, y float64) {
	if h.delegate != nil {
		h.delegate.OnScrollOffsetChanged(browser, x, y)
	}
}
func (h *dumberRenderHandler) OnImeCompositionRangeChanged(browser purecef.Browser, selectedRange *purecef.Range, characterBounds []purecef.Rect) {
	if h.delegate != nil {
		h.delegate.OnImeCompositionRangeChanged(browser, selectedRange, characterBounds)
	}
}
func (h *dumberRenderHandler) OnTextSelectionChanged(_ purecef.Browser, selectedText string, _ *purecef.Range) {
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
func (h *dumberRenderHandler) OnVirtualKeyboardRequested(browser purecef.Browser, mode purecef.TextInputMode) {
	if h.delegate != nil {
		h.delegate.OnVirtualKeyboardRequested(browser, mode)
	}
}
