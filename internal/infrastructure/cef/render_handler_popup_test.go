package cef

import (
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/assert"
)

type popupSurfaceRecorder struct {
	visible bool
	rect    purecef.Rect
	calls   []string
}

func (r *popupSurfaceRecorder) SetPopupVisible(visible bool) {
	r.visible = visible
	r.calls = append(r.calls, "visible")
}

func (r *popupSurfaceRecorder) SetPopupRect(rect purecef.Rect) {
	r.rect = rect
	r.calls = append(r.calls, "rect")
}

func TestDumberRenderHandler_RoutesPopupAcceleratedPaintToPopupSurface(t *testing.T) {
	mainHandler := cefmocks.NewMockRenderHandler(t)
	popupHandler := cefmocks.NewMockRenderHandler(t)
	h := &dumberRenderHandler{
		main:  mainHandler,
		popup: popupHandler,
	}

	info := &purecef.AcceleratedPaintInfo{}
	popupHandler.EXPECT().OnAcceleratedPaint(nil, purecef.PaintElementTypePetPopup, []purecef.Rect(nil), info).Return().Once()

	h.OnAcceleratedPaint(nil, purecef.PaintElementTypePetPopup, []purecef.Rect(nil), info)

	mainHandler.AssertNotCalled(t, "OnAcceleratedPaint", nil, purecef.PaintElementTypePetPopup, []purecef.Rect(nil), info)
}

func TestDumberRenderHandler_TracksPopupLifecycle(t *testing.T) {
	recorder := &popupSurfaceRecorder{}
	// With a nil engine, WebView.runOnGTK/webview.go executes inline so popup
	// lifecycle updates stay synchronous in this unit test.
	wv := &WebView{}
	wv.engine = nil
	h := &dumberRenderHandler{
		wv:           wv,
		popupSurface: recorder,
	}

	popupRect := &purecef.Rect{X: 12, Y: 34, Width: 180, Height: 240}
	h.OnPopupShow(nil, 1)
	h.OnPopupSize(nil, popupRect)
	h.OnPopupShow(nil, 0)

	assert.Equal(t, []string{"visible", "rect", "visible"}, recorder.calls)
	assert.Equal(t, purecef.Rect{X: 12, Y: 34, Width: 180, Height: 240}, recorder.rect)
	assert.False(t, recorder.visible)
}

func TestDeviceToLogicalSize_RoundsUp(t *testing.T) {
	assert.Equal(t, int32(4), deviceToLogicalSize(5, 1.25))
	assert.Equal(t, int32(1), deviceToLogicalSize(1, 1.5))
	assert.Equal(t, int32(0), deviceToLogicalSize(0, 2.0))
	assert.Equal(t, int32(0), deviceToLogicalSize(-1, 2.0))
	assert.Equal(t, int32(10), deviceToLogicalSize(10, 1.0))
	assert.Equal(t, int32(1), deviceToLogicalSize(1, 100.0))
}
