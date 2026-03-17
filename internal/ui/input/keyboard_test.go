package input

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/stretchr/testify/assert"
)

// newTestWorkspace creates a minimal WorkspaceConfig with activation shortcuts set.
// Used by tests that construct a KeyboardHandler.
func newTestWorkspace() *entity.WorkspaceConfig {
	return &entity.WorkspaceConfig{
		TabMode:    entity.TabModeConfig{ActivationShortcut: "ctrl+t"},
		PaneMode:   entity.PaneModeConfig{ActivationShortcut: "ctrl+p"},
		ResizeMode: entity.ResizeModeConfig{ActivationShortcut: "ctrl+alt+r"},
	}
}

// newTestSession creates a minimal SessionConfig with activation shortcuts set.
func newTestSession() *entity.SessionConfig {
	return &entity.SessionConfig{
		SessionMode: entity.SessionModeConfig{ActivationShortcut: "ctrl+s"},
	}
}

// stubAccentHandler is a test stub for the AccentHandler interface.
type stubAccentHandler struct {
	onKeyPressedResult bool
}

func (s *stubAccentHandler) OnKeyPressed(_ context.Context, _ rune, _ bool) bool {
	return s.onKeyPressedResult
}
func (s *stubAccentHandler) OnKeyReleased(_ context.Context, _ rune) {}
func (s *stubAccentHandler) IsPickerVisible() bool                   { return false }
func (s *stubAccentHandler) Cancel(_ context.Context)                {}

func TestIsShortcutModified(t *testing.T) {
	tests := []struct {
		name     string
		mods     Modifier
		expected bool
	}{
		{"no modifier", ModNone, false},
		{"shift only", ModShift, false},
		{"ctrl", ModCtrl, true},
		{"alt", ModAlt, true},
		{"ctrl+shift", ModCtrl | ModShift, true},
		{"alt+shift", ModAlt | ModShift, true},
		{"ctrl+alt", ModCtrl | ModAlt, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsShortcutModified(tt.mods))
		})
	}
}

func TestIsTextInputKey(t *testing.T) {
	tests := []struct {
		name     string
		keyval   uint
		expected bool
	}{
		// Printable characters
		{"lowercase a", uint('a'), true},
		{"lowercase z", uint('z'), true},
		{"uppercase A", uint('A'), true},
		{"digit 1", uint('1'), true},
		{"space", 0x020, true},
		{"period", uint('.'), true},
		{"slash", uint('/'), true},
		{"apostrophe", 0x027, true},
		{"grave accent", 0x060, true},
		{"tilde", 0x07e, true},
		{"quotedbl", 0x022, true},
		{"caret", 0x05e, true},

		// Dead keys (used by US International layout)
		{"dead acute", 0xfe51, true},
		{"dead grave", 0xfe50, true},
		{"dead circumflex", 0xfe52, true},
		{"dead tilde", 0xfe53, true},
		{"dead diaeresis", 0xfe57, true},

		// Dead key upper-bound boundary cases
		{"dead_greek (0xfe8c) - highest dead key", 0xfe8c, true},
		{"0xfe8d - one past highest dead key", 0xfe8d, false},

		// Non-text keys (should NOT be treated as text)
		{"escape", 0xff1b, false},
		{"return", 0xff0d, false},
		{"tab", 0xff09, false},
		{"F1", 0xffbe, false},
		{"F12", 0xffc9, false},
		{"left arrow", 0xff51, false},
		{"right arrow", 0xff53, false},
		{"backspace", 0xff08, false},
		{"delete", 0xffff, false},
		{"home", 0xff50, false},
		{"end", 0xff57, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsTextInputKey(tt.keyval), "keyval 0x%x", tt.keyval)
		})
	}
}

func TestHandleKeyPress_RoutePassToWidget(t *testing.T) {
	ctx := context.Background()

	h := NewKeyboardHandler(ctx, newTestWorkspace(), newTestSession())

	// Route all non-shortcut keys to widget (simulates WebView text input)
	h.SetRouteKey(func(kc KeyContext) KeyRoute {
		return RoutePassToWidget
	})

	tests := []struct {
		name   string
		keyval uint
		state  gdk.ModifierType
	}{
		{"plain 'e'", uint('e'), 0},
		{"plain 'a'", uint('a'), 0},
		{"shift 'E'", uint('E'), gdk.ShiftMaskValue},
		{"space", 0x020, 0},
		{"digit 1", uint('1'), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := h.handleKeyPress(tt.keyval, 0, tt.state)
			assert.False(t, result, "RoutePassToWidget should let key propagate (return false)")
		})
	}
}

func TestHandleKeyPress_RouteAccentDetection_NoAccent(t *testing.T) {
	ctx := context.Background()

	h := NewKeyboardHandler(ctx, newTestWorkspace(), newTestSession())

	// Set a stub accent handler that always returns false (simulates "no accents for this key")
	h.SetAccentHandler(&stubAccentHandler{onKeyPressedResult: false})

	// Route to accent detection
	h.SetRouteKey(func(kc KeyContext) KeyRoute {
		return RouteAccentDetection
	})

	// 'x' has no accents -> accent handler returns false -> key falls through
	result := h.handleKeyPress(uint('x'), 0, 0)
	assert.False(t, result, "'x' has no accents, should pass through")
}

func TestHandleKeyPress_ShortcutsWorkWithRouting(t *testing.T) {
	ctx := context.Background()

	h := NewKeyboardHandler(ctx, newTestWorkspace(), newTestSession())

	h.SetOnAction(func(ctx context.Context, action Action) error {
		return nil
	})

	// Route text keys to widget, shortcuts to handler
	h.SetRouteKey(func(kc KeyContext) KeyRoute {
		if IsShortcutModified(kc.Modifiers) {
			return RouteHandleShortcuts
		}
		return RoutePassToWidget
	})

	// Ctrl+L = open omnibox (registered global shortcut)
	result := h.handleKeyPress(uint('l'), 0, gdk.ControlMaskValue)
	assert.True(t, result, "Ctrl+L should be consumed by shortcut handler")
}

func TestHandleKeyPress_ModalModeIgnoresRoute(t *testing.T) {
	ctx := context.Background()

	h := NewKeyboardHandler(ctx, newTestWorkspace(), newTestSession())
	h.EnterTabMode()

	// Even if routeKey says pass to widget, modal mode ignores it
	h.SetRouteKey(func(kc KeyContext) KeyRoute {
		return RoutePassToWidget
	})

	// In modal mode, unknown keys are consumed (return true)
	result := h.handleKeyPress(uint('z'), 0, 0)
	assert.True(t, result, "modal mode should consume unknown keys regardless of route")
}

func TestKeyboardHandler_SetRouteKey(t *testing.T) {
	ctx := context.Background()
	h := NewKeyboardHandler(ctx, nil, nil)

	// Default: no callback set
	h.mu.RLock()
	fn := h.routeKey
	h.mu.RUnlock()
	assert.Nil(t, fn, "routeKey should be nil by default")

	// Set callback
	h.SetRouteKey(func(kc KeyContext) KeyRoute {
		return RoutePassToWidget
	})

	h.mu.RLock()
	fn = h.routeKey
	h.mu.RUnlock()
	assert.NotNil(t, fn, "routeKey should be set")
	assert.Equal(t, RoutePassToWidget, fn(KeyContext{}))
}

func TestHandleKeyPress_NoRouteCallback_DefaultsToShortcuts(t *testing.T) {
	ctx := context.Background()

	h := NewKeyboardHandler(ctx, newTestWorkspace(), newTestSession())

	// No routeKey set -- should default to shortcut handling
	// 'z' is not a registered shortcut in ModeNormal, so returns false
	result := h.handleKeyPress(uint('z'), 0, 0)
	assert.False(t, result, "unregistered key in ModeNormal returns false")
}
