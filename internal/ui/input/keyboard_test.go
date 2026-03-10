package input

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/stretchr/testify/assert"
)

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

		// Dead keys (used by US International layout)
		{"dead acute", 0xfe51, true},
		{"dead grave", 0xfe50, true},
		{"dead circumflex", 0xfe52, true},
		{"dead tilde", 0xfe53, true},
		{"dead diaeresis", 0xfe57, true},

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
	cfg := &config.Config{}
	cfg.Workspace.TabMode.ActivationShortcut = "ctrl+t"
	cfg.Workspace.PaneMode.ActivationShortcut = "ctrl+p"
	cfg.Workspace.ResizeMode.ActivationShortcut = "ctrl+alt+r"
	cfg.Session.SessionMode.ActivationShortcut = "ctrl+s"

	h := NewKeyboardHandler(ctx, cfg)

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
	cfg := &config.Config{}
	cfg.Workspace.TabMode.ActivationShortcut = "ctrl+t"
	cfg.Workspace.PaneMode.ActivationShortcut = "ctrl+p"
	cfg.Workspace.ResizeMode.ActivationShortcut = "ctrl+alt+r"
	cfg.Session.SessionMode.ActivationShortcut = "ctrl+s"

	h := NewKeyboardHandler(ctx, cfg)

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
	cfg := &config.Config{}
	cfg.Workspace.TabMode.ActivationShortcut = "ctrl+t"
	cfg.Workspace.PaneMode.ActivationShortcut = "ctrl+p"
	cfg.Workspace.ResizeMode.ActivationShortcut = "ctrl+alt+r"
	cfg.Session.SessionMode.ActivationShortcut = "ctrl+s"

	h := NewKeyboardHandler(ctx, cfg)

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
	cfg := &config.Config{}
	cfg.Workspace.TabMode.ActivationShortcut = "ctrl+t"
	cfg.Workspace.PaneMode.ActivationShortcut = "ctrl+p"
	cfg.Workspace.ResizeMode.ActivationShortcut = "ctrl+alt+r"
	cfg.Session.SessionMode.ActivationShortcut = "ctrl+s"

	h := NewKeyboardHandler(ctx, cfg)
	h.EnterTabMode()

	// Even if routeKey says pass to widget, modal mode ignores it
	h.SetRouteKey(func(kc KeyContext) KeyRoute {
		return RoutePassToWidget
	})

	// In modal mode, unknown keys are consumed (return true)
	result := h.handleKeyPress(uint('z'), 0, 0)
	assert.True(t, result, "modal mode should consume unknown keys regardless of route")
}

func TestHandleKeyPress_NoRouteCallback_DefaultsToShortcuts(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.Workspace.TabMode.ActivationShortcut = "ctrl+t"
	cfg.Workspace.PaneMode.ActivationShortcut = "ctrl+p"
	cfg.Workspace.ResizeMode.ActivationShortcut = "ctrl+alt+r"
	cfg.Session.SessionMode.ActivationShortcut = "ctrl+s"

	h := NewKeyboardHandler(ctx, cfg)

	// No routeKey set -- should default to shortcut handling
	// 'z' is not a registered shortcut in ModeNormal, so returns false
	result := h.handleKeyPress(uint('z'), 0, 0)
	assert.False(t, result, "unregistered key in ModeNormal returns false")
}
