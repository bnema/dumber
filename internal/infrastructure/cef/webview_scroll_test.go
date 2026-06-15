package cef

import (
	"context"
	"errors"
	"sync"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
)

// ---------------------------------------------------------------------------
// scrollCommandToNativeKey mapping tests (pure Go, no CEF host needed)
// ---------------------------------------------------------------------------

func TestScrollCommandToNativeKey_MapsAllKnownCommands(t *testing.T) {
	tests := []struct {
		command port.PageScrollCommand
		wantVK  int32
		wantOK  bool
		name    string
	}{
		{port.PageScrollCommandLeft, vkLeft, true, "PageScrollLeft → VK_LEFT"},
		{port.PageScrollCommandRight, vkRight, true, "PageScrollRight → VK_RIGHT"},
		{port.PageScrollCommandUp, vkUp, true, "PageScrollUp → VK_UP"},
		{port.PageScrollCommandDown, vkDown, true, "PageScrollDown → VK_DOWN"},
		{port.PageScrollCommandUpFast, vkPrior, true, "PageScrollUpFast → VK_PRIOR"},
		{port.PageScrollCommandDownFast, vkNext, true, "PageScrollDownFast → VK_NEXT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vk, ok := scrollCommandToNativeKey(tt.command)
			if ok != tt.wantOK {
				t.Fatalf("scrollCommandToNativeKey(%d) ok = %v, want %v", tt.command, ok, tt.wantOK)
			}
			if vk != tt.wantVK {
				t.Fatalf("scrollCommandToNativeKey(%d) vk = %d, want %d", tt.command, vk, tt.wantVK)
			}
		})
	}
}

func TestScrollCommandToNativeKey_UnknownCommandReturnsFalse(t *testing.T) {
	// Commands outside the known 0-5 range, including negative values.
	unknowns := []port.PageScrollCommand{-1, 6, 42, 999}
	for _, cmd := range unknowns {
		t.Run("", func(t *testing.T) {
			vk, ok := scrollCommandToNativeKey(cmd)
			if ok {
				t.Fatalf("scrollCommandToNativeKey(%d) ok = true, want false", cmd)
			}
			if vk != 0 {
				t.Fatalf("scrollCommandToNativeKey(%d) vk = %d, want 0", cmd, vk)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ScrollPage integration tests (use a stub host that records SendKeyEvent)
// ---------------------------------------------------------------------------

// scrollRecorderHost wraps purecef.BrowserHost and records every SendKeyEvent
// call, allowing tests to verify that native key taps were issued.
type scrollRecorderHost struct {
	purecef.BrowserHost // embedded nil — only SendKeyEvent is called in these tests

	mu     sync.Mutex
	events []purecef.KeyEvent
}

func (h *scrollRecorderHost) SendKeyEvent(event *purecef.KeyEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Record a copy so the test can inspect it after the call returns.
	h.events = append(h.events, *event)
}

func (h *scrollRecorderHost) recordedEvents() []purecef.KeyEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]purecef.KeyEvent, len(h.events))
	copy(out, h.events)
	return out
}

func (h *scrollRecorderHost) resetRecords() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = nil
}

func TestScrollPage_NativePath_SendsKeyDownAndKeyUp(t *testing.T) {
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
	if len(events) != 2 {
		t.Fatalf("expected 2 key events (RawKeyDown + KeyUp), got %d", len(events))
	}

	// First event: RawKeyDown
	if events[0].Type != purecef.KeyEventTypeKeyeventRawkeydown {
		t.Fatalf("event[0].Type = %d, want %d (KeyEventTypeKeyeventRawkeydown)",
			events[0].Type, purecef.KeyEventTypeKeyeventRawkeydown)
	}
	if events[0].WindowsKeyCode != vkDown {
		t.Fatalf("event[0].WindowsKeyCode = %d, want %d (VK_DOWN)", events[0].WindowsKeyCode, vkDown)
	}

	// Second event: KeyUp
	if events[1].Type != purecef.KeyEventTypeKeyeventKeyup {
		t.Fatalf("event[1].Type = %d, want %d (KeyEventTypeKeyeventKeyup)",
			events[1].Type, purecef.KeyEventTypeKeyeventKeyup)
	}
	if events[1].WindowsKeyCode != vkDown {
		t.Fatalf("event[1].WindowsKeyCode = %d, want %d (VK_DOWN)", events[1].WindowsKeyCode, vkDown)
	}
}

func TestScrollPage_NativePath_AllCommandsProduceCorrectKeyCode(t *testing.T) {
	host := &scrollRecorderHost{}
	wv := &WebView{host: host}

	commands := []struct {
		command port.PageScrollCommand
		wantVK  int32
		name    string
	}{
		{port.PageScrollCommandLeft, vkLeft, "PageScrollLeft"},
		{port.PageScrollCommandRight, vkRight, "PageScrollRight"},
		{port.PageScrollCommandUp, vkUp, "PageScrollUp"},
		{port.PageScrollCommandDown, vkDown, "PageScrollDown"},
		{port.PageScrollCommandUpFast, vkPrior, "PageScrollUpFast"},
		{port.PageScrollCommandDownFast, vkNext, "PageScrollDownFast"},
	}

	for _, tt := range commands {
		t.Run(tt.name, func(t *testing.T) {
			host.resetRecords()

			req := port.PageScrollRequest{
				Command: tt.command,
			}
			if err := wv.ScrollPage(context.Background(), req); err != nil {
				t.Fatalf("ScrollPage() error: %v", err)
			}

			events := host.recordedEvents()
			if len(events) != 2 {
				t.Fatalf("expected 2 key events, got %d", len(events))
			}
			if events[0].WindowsKeyCode != tt.wantVK {
				t.Fatalf("RawKeyDown WindowsKeyCode = %d, want %d", events[0].WindowsKeyCode, tt.wantVK)
			}
			if events[1].WindowsKeyCode != tt.wantVK {
				t.Fatalf("KeyUp WindowsKeyCode = %d, want %d", events[1].WindowsKeyCode, tt.wantVK)
			}
		})
	}
}

func TestScrollPage_NativePath_FallbackToJSWhenHostIsNil(t *testing.T) {
	// With no host set, ScrollPage should still succeed via the JS fallback
	// (the host == nil check before calling SendKeyEvent).
	wv := &WebView{}
	// No .host set — will be nil.

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

func TestScrollPage_UnmappedCommand_FallsBackToJS(t *testing.T) {
	host := &scrollRecorderHost{}
	wv := &WebView{host: host}

	// A command outside the 0-5 range has no native mapping and must not
	// attempt to send key events.
	req := port.PageScrollRequest{
		Command:    port.PageScrollCommand(42),
		FallbackDX: 0,
		FallbackDY: 80,
	}

	err := wv.ScrollPage(context.Background(), req)
	if err != nil {
		t.Fatalf("ScrollPage() with unmapped command should fall back: %v", err)
	}

	events := host.recordedEvents()
	if len(events) != 0 {
		t.Fatalf("expected 0 key events for unmapped command, got %d", len(events))
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
