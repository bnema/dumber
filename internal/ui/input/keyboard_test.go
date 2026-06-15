package input

import (
	"context"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
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

type fakeModalTimer struct{}

func (*fakeModalTimer) Stop() bool { return true }

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

func TestKeyboardHandlerDetachForDestroyClearsRetainedCallbacksAndState(t *testing.T) {
	h := NewKeyboardHandler(context.Background(), newTestWorkspace(), newTestSession())
	h.keyPressedCb = func(gtk.EventControllerKey, uint, uint, gdk.ModifierType) bool { return true }
	h.keyReleasedCb = func(gtk.EventControllerKey, uint, uint, gdk.ModifierType) {}
	h.keyPressedHandlerID = 1
	h.keyReleasedHandlerID = 2
	h.activePressedActions = map[Action]uint{ActionReload: 1}

	h.DetachForDestroy()

	assert.Nil(t, h.controller)
	assert.Nil(t, h.window)
	assert.Nil(t, h.keyPressedCb)
	assert.Nil(t, h.keyReleasedCb)
	assert.Zero(t, h.keyPressedHandlerID)
	assert.Zero(t, h.keyReleasedHandlerID)
	assert.Nil(t, h.activePressedActions)
}

func TestHandleKeyPress_SuppressesHeldHardwareFallbackTabSwitchUntilRelease(t *testing.T) {
	h := NewKeyboardHandler(context.Background(), newTestWorkspace(), newTestSession())
	calls := 0
	h.SetOnAction(func(_ context.Context, action Action) error {
		calls++
		if action != ActionSwitchTabIndex2 {
			t.Fatalf("action = %s, want %s", action, ActionSwitchTabIndex2)
		}
		return nil
	})

	const fallbackKeycode = 11
	if got := KeycodeToTabAction[fallbackKeycode]; got != ActionSwitchTabIndex2 {
		t.Fatalf("KeycodeToTabAction[%d] = %s, want %s", fallbackKeycode, got, ActionSwitchTabIndex2)
	}

	if !h.handleKeyPress(uint('x'), fallbackKeycode, gdk.AltMaskValue) {
		t.Fatal("first tab-switch key press should be consumed")
	}
	if !h.handleKeyPress(uint('x'), fallbackKeycode, gdk.AltMaskValue) {
		t.Fatal("repeated held tab-switch key press should still be consumed")
	}
	if calls != 1 {
		t.Fatalf("tab switch handler calls while key held = %d, want 1", calls)
	}

	h.handleKeyRelease(uint('x'))
	if !h.handleKeyPress(uint('x'), fallbackKeycode, gdk.AltMaskValue) {
		t.Fatal("tab-switch key press after release should be consumed")
	}
	if calls != 2 {
		t.Fatalf("tab switch handler calls after release = %d, want 2", calls)
	}
}

func TestHandleKeyPress_DoesNotExitResizeModeWhileActivationKeyIsHeld(t *testing.T) {
	h := NewKeyboardHandler(context.Background(), newTestWorkspace(), newTestSession())
	const resizeKeycode = 27
	mods := gdk.ControlMaskValue | gdk.AltMaskValue

	if !h.handleKeyPress(uint('r'), resizeKeycode, mods) {
		t.Fatal("first resize-mode activation should be consumed")
	}
	if got := h.Mode(); got != ModeResize {
		t.Fatalf("mode after first press = %v, want %v", got, ModeResize)
	}

	if !h.handleKeyPress(uint('r'), resizeKeycode, mods) {
		t.Fatal("repeated held resize-mode activation should be consumed")
	}
	if got := h.Mode(); got != ModeResize {
		t.Fatalf("mode after repeated held press = %v, want %v", got, ModeResize)
	}

	h.handleKeyRelease(uint('x'))
	if !h.handleKeyPress(uint('r'), resizeKeycode, mods) {
		t.Fatal("resize-mode activation after unrelated release should still be consumed")
	}
	if got := h.Mode(); got != ModeResize {
		t.Fatalf("mode after unrelated release = %v, want %v", got, ModeResize)
	}

	h.handleKeyRelease(uint('r'))
	if !h.handleKeyPress(uint('r'), resizeKeycode, mods) {
		t.Fatal("resize-mode activation after release should be consumed")
	}
	if got := h.Mode(); got != ModeNormal {
		t.Fatalf("mode after press following release = %v, want %v", got, ModeNormal)
	}
}

func TestHandleKeyPress_DoesNotRetoggleFloatingPaneAfterUnrelatedRelease(t *testing.T) {
	workspace := newTestWorkspace()
	workspace.Shortcuts.Actions = map[string]entity.ActionBinding{
		"toggle_floating_pane": {Keys: []string{"alt+f"}},
	}
	h := NewKeyboardHandler(context.Background(), workspace, newTestSession())
	calls := 0
	h.SetOnAction(func(_ context.Context, action Action) error {
		calls++
		if action != ActionToggleFloatingPane {
			t.Fatalf("action = %s, want %s", action, ActionToggleFloatingPane)
		}
		return nil
	})

	if !h.handleKeyPress(uint('f'), 41, gdk.AltMaskValue) {
		t.Fatal("first floating pane toggle should be consumed")
	}
	h.handleKeyRelease(uint('x'))
	if !h.handleKeyPress(uint('f'), 41, gdk.AltMaskValue) {
		t.Fatal("held floating pane toggle repeat should still be consumed")
	}
	if calls != 1 {
		t.Fatalf("floating pane toggle calls while key held = %d, want 1", calls)
	}

	h.handleKeyRelease(uint('f'))
	if !h.handleKeyPress(uint('f'), 41, gdk.AltMaskValue) {
		t.Fatal("floating pane toggle after release should be consumed")
	}
	if calls != 2 {
		t.Fatalf("floating pane toggle calls after release = %d, want 2", calls)
	}
}

func TestIsRepeatedKeyboardActionSuppressed_AllowsResizeStepRepeats(t *testing.T) {
	if isRepeatedKeyboardActionSuppressed(ActionResizeIncreaseLeft) {
		t.Fatal("ActionResizeIncreaseLeft should not be suppressed")
	}
}

func TestHandleKeyPress_EnterPageMode(t *testing.T) {
	ctx := context.Background()
	workspace := newTestWorkspace()
	workspace.PageMode = entity.PageModeConfig{
		ActivationShortcut: "ctrl+y",
	}

	h := NewKeyboardHandler(ctx, workspace, newTestSession())

	h.SetOnAction(func(ctx context.Context, action Action) error {
		return nil
	})

	// Ctrl+Y should enter page mode
	result := h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue)
	if !result {
		t.Fatal("Ctrl+Y should be consumed")
	}
	if h.Mode() != ModePage {
		t.Fatalf("mode = %v, want ModePage", h.Mode())
	}
}

func TestHandleKeyPress_PageModeStaysActiveAfterScroll(t *testing.T) {
	ctx := context.Background()
	workspace := newTestWorkspace()
	workspace.PageMode = entity.PageModeConfig{
		ActivationShortcut: "ctrl+y",
		Actions: map[string]entity.ActionBinding{
			"page-scroll-down": {Keys: []string{"j"}},
			"page-scroll-up":   {Keys: []string{"k"}},
			"cancel":           {Keys: []string{"escape"}},
		},
	}

	h := NewKeyboardHandler(ctx, workspace, newTestSession())
	actionCalls := 0
	h.SetOnAction(func(ctx context.Context, action Action) error {
		actionCalls++
		return nil
	})

	// Enter page mode
	result := h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue)
	if !result || h.Mode() != ModePage {
		t.Fatal("failed to enter page mode")
	}

	// Repeated scroll actions should stay in page mode
	for i := 0; i < 5; i++ {
		scrolled := h.handleKeyPress(uint('j'), 0, 0)
		if !scrolled {
			t.Fatalf("scroll down iteration %d: key not consumed", i)
		}
		if h.Mode() != ModePage {
			t.Fatalf("scroll down iteration %d: mode = %v, want ModePage", i, h.Mode())
		}
	}

	// Scroll up also stays in page mode
	result = h.handleKeyPress(uint('k'), 0, 0)
	if !result || h.Mode() != ModePage {
		t.Fatal("scroll up should stay in page mode")
	}

	// Ensure scroll actions were dispatched
	if actionCalls != 6 {
		t.Fatalf("action calls = %d, want 6 (5 down + 1 up)", actionCalls)
	}
}

func TestHandleKeyPress_PageModeTimeoutReset(t *testing.T) {
	ctx := context.Background()
	workspace := newTestWorkspace()
	workspace.PageMode = entity.PageModeConfig{
		ActivationShortcut:  "ctrl+y",
		TimeoutMilliseconds: 100,
		Actions: map[string]entity.ActionBinding{
			"page-scroll-down": {Keys: []string{"j"}},
			"cancel":           {Keys: []string{"escape"}},
		},
	}

	h := NewKeyboardHandler(ctx, workspace, newTestSession())
	timerStarts := 0
	h.modal.afterFunc = func(d time.Duration, fn func()) modalTimer {
		timerStarts++
		return &fakeModalTimer{}
	}
	h.SetOnAction(func(ctx context.Context, action Action) error {
		return nil
	})

	// Enter page mode with timeout.
	h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue)
	if h.Mode() != ModePage {
		t.Fatal("failed to enter page mode")
	}
	if timerStarts != 1 {
		t.Fatalf("timer starts after entering page mode = %d, want 1", timerStarts)
	}

	// Scroll action should reset the timeout by starting a replacement timer.
	h.handleKeyPress(uint('j'), 0, 0)

	if h.Mode() != ModePage {
		t.Fatalf("mode after scroll = %v, want ModePage", h.Mode())
	}
	if timerStarts != 2 {
		t.Fatalf("timer starts after scroll reset = %d, want 2", timerStarts)
	}
}

func TestHandleKeyPress_PageModeEscapeExits(t *testing.T) {
	ctx := context.Background()
	workspace := newTestWorkspace()
	workspace.PageMode = entity.PageModeConfig{
		ActivationShortcut: "ctrl+y",
		Actions: map[string]entity.ActionBinding{
			"page-scroll-down": {Keys: []string{"j"}},
			"cancel":           {Keys: []string{"escape"}},
		},
	}

	h := NewKeyboardHandler(ctx, workspace, newTestSession())
	h.SetOnAction(func(ctx context.Context, action Action) error {
		return nil
	})

	// Enter page mode
	h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue)
	if h.Mode() != ModePage {
		t.Fatal("failed to enter page mode")
	}

	// Escape should exit page mode
	result := h.handleKeyPress(uint(gdk.KEY_Escape), 0, 0)
	if !result {
		t.Fatal("Escape should be consumed")
	}
	if h.Mode() != ModeNormal {
		t.Fatalf("mode after Escape = %v, want ModeNormal", h.Mode())
	}
}

func TestHandleKeyPress_PageModeEnterExits(t *testing.T) {
	ctx := context.Background()
	workspace := newTestWorkspace()
	workspace.PageMode = entity.PageModeConfig{
		ActivationShortcut: "ctrl+y",
		Actions: map[string]entity.ActionBinding{
			"confirm": {Keys: []string{"enter"}},
		},
	}

	h := NewKeyboardHandler(ctx, workspace, newTestSession())
	h.SetOnAction(func(ctx context.Context, action Action) error {
		return nil
	})

	// Enter page mode
	h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue)
	if h.Mode() != ModePage {
		t.Fatal("failed to enter page mode")
	}

	// Enter should exit page mode
	result := h.handleKeyPress(uint(gdk.KEY_Return), 0, 0)
	if !result {
		t.Fatal("Enter should be consumed")
	}
	if h.Mode() != ModeNormal {
		t.Fatalf("mode after Enter = %v, want ModeNormal", h.Mode())
	}
}

func TestHandleKeyPress_PageModeActivationTogglesOff(t *testing.T) {
	ctx := context.Background()
	workspace := newTestWorkspace()
	workspace.PageMode = entity.PageModeConfig{
		ActivationShortcut: "ctrl+y",
	}

	h := NewKeyboardHandler(ctx, workspace, newTestSession())

	if !h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue) {
		t.Fatal("first Ctrl+Y should be consumed")
	}
	if h.Mode() != ModePage {
		t.Fatalf("mode after first Ctrl+Y = %v, want ModePage", h.Mode())
	}

	h.handleKeyRelease(uint('y'))
	if !h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue) {
		t.Fatal("second Ctrl+Y should be consumed")
	}
	if h.Mode() != ModeNormal {
		t.Fatalf("mode after second Ctrl+Y = %v, want ModeNormal", h.Mode())
	}
}

func TestHandleKeyPress_PageModeEscapeExitsWithoutCancelBinding(t *testing.T) {
	ctx := context.Background()
	workspace := newTestWorkspace()
	workspace.PageMode = entity.PageModeConfig{
		ActivationShortcut: "ctrl+y",
		Actions: map[string]entity.ActionBinding{
			"page-scroll-down": {Keys: []string{"j"}},
		},
	}

	h := NewKeyboardHandler(ctx, workspace, newTestSession())
	if !h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue) {
		t.Fatal("Ctrl+Y should be consumed")
	}
	if h.Mode() != ModePage {
		t.Fatal("failed to enter page mode")
	}

	if !h.handleKeyPress(uint(gdk.KEY_Escape), 0, 0) {
		t.Fatal("Escape should still be consumed without explicit cancel binding")
	}
	if h.Mode() != ModeNormal {
		t.Fatalf("mode after Escape fallback = %v, want ModeNormal", h.Mode())
	}
}

func TestHandleKeyPress_PageModeActivationNotInPassThrough(t *testing.T) {
	ctx := context.Background()
	workspace := newTestWorkspace()
	workspace.PageMode = entity.PageModeConfig{
		ActivationShortcut: "ctrl+y",
	}

	h := NewKeyboardHandler(ctx, workspace, newTestSession())

	// Route ALL keys to widget (editable pass-through context)
	h.SetRouteKey(func(kc KeyContext) KeyRoute {
		return RoutePassToWidget
	})

	// Ctrl+Y should NOT activate page mode because route says pass to widget
	result := h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue)
	if result {
		t.Fatal("Ctrl+Y should NOT be consumed when routed to widget")
	}
	if h.Mode() != ModeNormal {
		t.Fatalf("mode = %v, want ModeNormal (should not enter page mode)", h.Mode())
	}
}

func TestHandleKeyPress_PageModeActivationPassesThroughWhenBlocked(t *testing.T) {
	ctx := context.Background()
	workspace := newTestWorkspace()
	workspace.PageMode = entity.PageModeConfig{
		ActivationShortcut: "ctrl+y",
	}

	h := NewKeyboardHandler(ctx, workspace, newTestSession())
	h.SetPageModeActivationPassthrough(func() bool { return true })

	result := h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue)
	if result {
		t.Fatal("Ctrl+Y should pass through when page mode activation is blocked")
	}
	if h.Mode() != ModeNormal {
		t.Fatalf("mode = %v, want ModeNormal", h.Mode())
	}
}

func TestDispatchAction_PageModeActivationPassesThroughWhenBlocked(t *testing.T) {
	ctx := context.Background()
	workspace := newTestWorkspace()
	workspace.PageMode = entity.PageModeConfig{
		ActivationShortcut: "ctrl+y",
	}

	h := NewKeyboardHandler(ctx, workspace, newTestSession())
	h.SetPageModeActivationPassthrough(func() bool { return true })

	consumed := h.DispatchAction(ActionEnterPageMode)
	if consumed {
		t.Fatal("DispatchAction should not consume blocked page mode activation")
	}
	if h.Mode() != ModeNormal {
		t.Fatalf("mode = %v, want ModeNormal", h.Mode())
	}
}

func TestHandleKeyPress_PageModeFastScrollLookup(t *testing.T) {
	tests := []struct {
		name         string
		configAction string
		key          uint
		expected     Action
	}{
		{
			name:         "scroll down fast",
			configAction: "page-scroll-down-fast",
			key:          uint('d'),
			expected:     ActionPageScrollDownFast,
		},
		{
			name:         "scroll up fast",
			configAction: "page-scroll-up-fast",
			key:          uint('u'),
			expected:     ActionPageScrollUpFast,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			workspace := newTestWorkspace()
			workspace.PageMode = entity.PageModeConfig{
				ActivationShortcut: "ctrl+y",
				Actions: map[string]entity.ActionBinding{
					tt.configAction: {Keys: []string{"ctrl+" + string(rune(tt.key))}},
				},
			}

			h := NewKeyboardHandler(ctx, workspace, newTestSession())
			actionCalls := 0
			var lastAction Action
			h.SetOnAction(func(ctx context.Context, action Action) error {
				actionCalls++
				lastAction = action
				return nil
			})

			// Enter page mode.
			h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue)
			if h.Mode() != ModePage {
				t.Fatal("failed to enter page mode")
			}

			// Fast-scroll shortcut in page mode should dispatch the expected action.
			result := h.handleKeyPress(tt.key, 0, gdk.ControlMaskValue)
			if !result {
				t.Fatal("fast scroll shortcut in page mode should be consumed")
			}
			if actionCalls != 1 || lastAction != tt.expected {
				t.Fatalf("expected %s, got action=%s calls=%d", tt.expected, lastAction, actionCalls)
			}
			if h.Mode() != ModePage {
				t.Fatal("should stay in page mode after scroll action")
			}
		})
	}
}

func TestHandleKeyPress_PageModeNoAutoExitForScrollActions(t *testing.T) {
	ctx := context.Background()
	workspace := newTestWorkspace()
	workspace.PageMode = entity.PageModeConfig{
		ActivationShortcut: "ctrl+y",
		Actions: map[string]entity.ActionBinding{
			"page-scroll-left":  {Keys: []string{"h"}},
			"page-scroll-right": {Keys: []string{"l"}},
		},
	}

	h := NewKeyboardHandler(ctx, workspace, newTestSession())
	actionCalls := 0
	h.SetOnAction(func(ctx context.Context, action Action) error {
		actionCalls++
		return nil
	})

	// Enter page mode
	h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue)
	if h.Mode() != ModePage {
		t.Fatal("failed to enter page mode")
	}

	// Scroll left should not auto-exit
	h.handleKeyPress(uint('h'), 0, 0)
	if h.Mode() != ModePage {
		t.Fatal("scroll left should not exit page mode")
	}

	// Scroll right should not auto-exit
	h.handleKeyPress(uint('l'), 0, 0)
	if h.Mode() != ModePage {
		t.Fatal("scroll right should not exit page mode")
	}

	if actionCalls != 2 {
		t.Fatalf("action calls = %d, want 2", actionCalls)
	}
}

func TestHandleKeyPress_PageModeArrowKeysPassThroughNatively(t *testing.T) {
	ctx := context.Background()
	workspace := newTestWorkspace()
	workspace.PageMode = entity.PageModeConfig{
		ActivationShortcut: "ctrl+y",
		Actions: map[string]entity.ActionBinding{
			"page-scroll-down": {Keys: []string{"j"}},
		},
	}

	h := NewKeyboardHandler(ctx, workspace, newTestSession())
	actionCalls := 0
	h.SetOnAction(func(ctx context.Context, action Action) error {
		actionCalls++
		return nil
	})

	h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue)
	if h.Mode() != ModePage {
		t.Fatal("failed to enter page mode")
	}

	for _, keyval := range []uint{uint(gdk.KEY_Left), uint(gdk.KEY_Right), uint(gdk.KEY_Up), uint(gdk.KEY_Down)} {
		consumed := h.handleKeyPress(keyval, 0, 0)
		if consumed {
			t.Fatalf("arrow key %d should pass through to native page handling in page mode", keyval)
		}
		if h.Mode() != ModePage {
			t.Fatalf("arrow key %d should keep page mode active", keyval)
		}
	}

	if actionCalls != 0 {
		t.Fatalf("arrow key passthrough should not dispatch page-mode action, got %d calls", actionCalls)
	}
}
