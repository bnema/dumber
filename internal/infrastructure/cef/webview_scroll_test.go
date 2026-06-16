package cef

import (
	"context"
	"errors"
	"sync"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
)

func TestPageScrollWheelDeltas_UseFallbackDeltasWithCEFSign(t *testing.T) {
	tests := []struct {
		name       string
		req        port.PageScrollRequest
		wantDeltaX int32
		wantDeltaY int32
	}{
		{"left", port.PageScrollRequest{FallbackDX: -80}, -80, 0},
		{"right", port.PageScrollRequest{FallbackDX: 80}, 80, 0},
		{"up", port.PageScrollRequest{FallbackDY: -80}, 0, 80},
		{"down", port.PageScrollRequest{FallbackDY: 80}, 0, -80},
		{"up fast", port.PageScrollRequest{FallbackDY: -320}, 0, 320},
		{"down fast", port.PageScrollRequest{FallbackDY: 320}, 0, -320},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDeltaX, gotDeltaY := pageScrollWheelDeltas(tt.req)
			if gotDeltaX != tt.wantDeltaX || gotDeltaY != tt.wantDeltaY {
				t.Fatalf("pageScrollWheelDeltas() = (%d, %d), want (%d, %d)", gotDeltaX, gotDeltaY, tt.wantDeltaX, tt.wantDeltaY)
			}
		})
	}
}

// scrollRecorderHost wraps purecef.BrowserHost and records mouse wheel events,
// allowing tests to verify that Page Mode uses Chromium's native scroll path.
type scrollRecorderHost struct {
	purecef.BrowserHost // embedded nil — only SendMouseWheelEvent is called in these tests

	mu     sync.Mutex
	events []recordedWheelEvent
}

type recordedWheelEvent struct {
	event          purecef.MouseEvent
	deltaX, deltaY int32
}

func (h *scrollRecorderHost) SendMouseWheelEvent(event *purecef.MouseEvent, deltaX int32, deltaY int32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Record a copy so the test can inspect it after the call returns.
	h.events = append(h.events, recordedWheelEvent{event: *event, deltaX: deltaX, deltaY: deltaY})
}

func (h *scrollRecorderHost) recordedEvents() []recordedWheelEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]recordedWheelEvent, len(h.events))
	copy(out, h.events)
	return out
}

func (h *scrollRecorderHost) resetRecords() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = nil
}

func TestScrollPage_NativeWheelPath_SendsPrecisionWheelEvent(t *testing.T) {
	host := &scrollRecorderHost{}
	wv := &WebView{host: host}

	req := port.PageScrollRequest{
		Command:    port.PageScrollCommandDown,
		FallbackDX: 0,
		FallbackDY: 80,
	}

	err := wv.ScrollPage(context.Background(), req)
	if err != nil {
		t.Fatalf("ScrollPage() unexpected error: %v", err)
	}

	events := host.recordedEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 wheel event, got %d", len(events))
	}
	if events[0].deltaX != 0 || events[0].deltaY != -80 {
		t.Fatalf("wheel delta = (%d, %d), want (0, -80)", events[0].deltaX, events[0].deltaY)
	}
	if events[0].event.Modifiers&uint32(purecef.EventFlagsEventflagPrecisionScrollingDelta) == 0 {
		t.Fatal("wheel event should set EVENTFLAG_PRECISION_SCROLLING_DELTA")
	}
}

func TestScrollPage_NativeWheelPath_AllCommandsUseFallbackDeltas(t *testing.T) {
	host := &scrollRecorderHost{}
	wv := &WebView{host: host}

	commands := []struct {
		name       string
		req        port.PageScrollRequest
		wantDeltaX int32
		wantDeltaY int32
	}{
		{"PageScrollLeft", port.PageScrollRequest{Command: port.PageScrollCommandLeft, FallbackDX: -80}, -80, 0},
		{"PageScrollRight", port.PageScrollRequest{Command: port.PageScrollCommandRight, FallbackDX: 80}, 80, 0},
		{"PageScrollUp", port.PageScrollRequest{Command: port.PageScrollCommandUp, FallbackDY: -80}, 0, 80},
		{"PageScrollDown", port.PageScrollRequest{Command: port.PageScrollCommandDown, FallbackDY: 80}, 0, -80},
		{"PageScrollUpFast", port.PageScrollRequest{Command: port.PageScrollCommandUpFast, FallbackDY: -320}, 0, 320},
		{"PageScrollDownFast", port.PageScrollRequest{Command: port.PageScrollCommandDownFast, FallbackDY: 320}, 0, -320},
	}

	for _, tt := range commands {
		t.Run(tt.name, func(t *testing.T) {
			host.resetRecords()

			if err := wv.ScrollPage(context.Background(), tt.req); err != nil {
				t.Fatalf("ScrollPage() error: %v", err)
			}

			events := host.recordedEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 wheel event, got %d", len(events))
			}
			if events[0].deltaX != tt.wantDeltaX || events[0].deltaY != tt.wantDeltaY {
				t.Fatalf("wheel delta = (%d, %d), want (%d, %d)", events[0].deltaX, events[0].deltaY, tt.wantDeltaX, tt.wantDeltaY)
			}
		})
	}
}

func TestScrollPage_NativeWheelPath_ZeroDeltaDoesNotSendEvent(t *testing.T) {
	host := &scrollRecorderHost{}
	wv := &WebView{host: host}

	if err := wv.ScrollPage(context.Background(), port.PageScrollRequest{}); err != nil {
		t.Fatalf("ScrollPage() error: %v", err)
	}
	if events := host.recordedEvents(); len(events) != 0 {
		t.Fatalf("expected no wheel events for zero delta, got %d", len(events))
	}
}

func TestScrollPage_NativePath_FallbackToJSWhenHostIsNil(t *testing.T) {
	// With no host set, ScrollPage should still succeed via the JS fallback.
	wv := &WebView{}

	req := port.PageScrollRequest{
		Command:    port.PageScrollCommandDown,
		FallbackDX: 0,
		FallbackDY: 80,
	}

	// Should not panic and should succeed (JS fallback is fire-and-forget
	// via RunJavaScript, which is safe with a nil browser).
	err := wv.ScrollPage(context.Background(), req)
	if err != nil {
		t.Fatalf("ScrollPage() with nil host should not return error: %v", err)
	}
}

func TestScrollPage_Destroyed_ReturnsError(t *testing.T) {
	wv := &WebView{}
	wv.destroyed.Store(true)

	err := wv.ScrollPage(context.Background(), port.PageScrollRequest{})
	if !errors.Is(err, errDestroyed) {
		t.Fatalf("ScrollPage() on destroyed webview: got %v, want %v", err, errDestroyed)
	}
}
