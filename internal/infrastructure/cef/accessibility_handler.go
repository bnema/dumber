package cef

import (
	"time"

	purecef "github.com/bnema/purego-cef/cef"
)

// accessibilityPayload is the UI-thread serialization result handed to a
// non-blocking sink. Capture, stats, and I/O happen off the CEF UI thread.
type accessibilityPayload struct {
	Kind           string
	JSON           string
	Bytes          int
	SerializeNanos int64
}

// accessibilityPayloadSink is the optional capture worker seam. Production
// leaves WebView.a11yWorker nil unless DUMBER_A11Y_CAPTURE=1.
type accessibilityPayloadSink interface {
	Submit(accessibilityPayload) bool
}

type accessibilityHandler struct {
	sink      func(accessibilityPayload) bool
	writeJSON func(purecef.Value, purecef.JsonWriterOptions) string
}

var _ purecef.AccessibilityHandler = (*accessibilityHandler)(nil)

func newAccessibilityHandler(sink func(accessibilityPayload) bool) purecef.AccessibilityHandler {
	return &accessibilityHandler{
		sink:      sink,
		writeJSON: purecef.WriteJson,
	}
}

func (h *accessibilityHandler) OnAccessibilityTreeChange(value purecef.Value) {
	h.emit("tree", value)
}

func (h *accessibilityHandler) OnAccessibilityLocationChange(value purecef.Value) {
	h.emit("location", value)
}

func (h *accessibilityHandler) emit(kind string, value purecef.Value) {
	if h == nil || h.sink == nil {
		return
	}
	if value == nil {
		return
	}
	started := time.Now()
	jsonText := h.writeJSON(value, purecef.JsonWriterOptionsJsonWriterDefault)
	h.sink(accessibilityPayload{
		Kind:           kind,
		JSON:           jsonText,
		Bytes:          len(jsonText),
		SerializeNanos: time.Since(started).Nanoseconds(),
	})
}

func (wv *WebView) enqueueAccessibilityPayload(p accessibilityPayload) bool {
	if wv == nil || wv.a11yWorker == nil {
		return false
	}
	return wv.a11yWorker.Submit(p)
}
