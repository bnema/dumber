// Package input provides keyboard event handling and modal input mode management.
package input

import (
	"context"
	"strings"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gdk"
)

// Modifier represents keyboard modifier flags.
type Modifier uint

const (
	// ModNone indicates no modifier is pressed.
	ModNone Modifier = 0
	// ModShift indicates the Shift key is pressed.
	ModShift Modifier = Modifier(gdk.ShiftMaskValue)
	// ModCtrl indicates the Control key is pressed.
	ModCtrl Modifier = Modifier(gdk.ControlMaskValue)
	// ModAlt indicates the Alt key is pressed.
	ModAlt Modifier = Modifier(gdk.AltMaskValue)
)

// modifierMask filters out non-standard modifiers from GDK state.
const modifierMask = ModCtrl | ModShift | ModAlt

// KeyBinding represents a single key combination.
type KeyBinding struct {
	Keyval    uint     // GDK key value (e.g., gdk.KEY_t)
	Modifiers Modifier // Combined modifiers
}

// Action represents what happens when a shortcut is triggered.
type Action string

// Predefined actions for the keyboard system.
const (
	// Mode management
	ActionEnterTabMode  Action = "enter_tab_mode"
	ActionEnterPaneMode Action = "enter_pane_mode"
	ActionExitMode      Action = "exit_mode"

	// Tab actions (global and modal)
	ActionNewTab           Action = "new_tab"
	ActionCloseTab         Action = "close_tab"
	ActionNextTab          Action = "next_tab"
	ActionPreviousTab      Action = "previous_tab"
	ActionRenameTab        Action = "rename_tab"
	ActionSwitchLastTab    Action = "switch_last_tab" // Alt+Tab style switching
	ActionSwitchTabIndex1  Action = "switch_tab_1"
	ActionSwitchTabIndex2  Action = "switch_tab_2"
	ActionSwitchTabIndex3  Action = "switch_tab_3"
	ActionSwitchTabIndex4  Action = "switch_tab_4"
	ActionSwitchTabIndex5  Action = "switch_tab_5"
	ActionSwitchTabIndex6  Action = "switch_tab_6"
	ActionSwitchTabIndex7  Action = "switch_tab_7"
	ActionSwitchTabIndex8  Action = "switch_tab_8"
	ActionSwitchTabIndex9  Action = "switch_tab_9"
	ActionSwitchTabIndex10 Action = "switch_tab_10"

	// Pane actions (modal)
	ActionSplitRight Action = "split_right"
	ActionSplitLeft  Action = "split_left"
	ActionSplitUp    Action = "split_up"
	ActionSplitDown  Action = "split_down"
	ActionClosePane  Action = "close_pane"
	ActionStackPane  Action = "stack_pane"

	// Pane focus navigation
	ActionFocusRight Action = "focus_right"
	ActionFocusLeft  Action = "focus_left"
	ActionFocusUp    Action = "focus_up"
	ActionFocusDown  Action = "focus_down"

	// Stack navigation (within stacked panes)
	ActionStackNavUp   Action = "stack_nav_up"
	ActionStackNavDown Action = "stack_nav_down"

	// Page navigation
	ActionGoBack     Action = "go_back"
	ActionGoForward  Action = "go_forward"
	ActionReload     Action = "reload"
	ActionHardReload Action = "hard_reload"
	ActionStop       Action = "stop"

	// Zoom
	ActionZoomIn    Action = "zoom_in"
	ActionZoomOut   Action = "zoom_out"
	ActionZoomReset Action = "zoom_reset"

	// UI
	ActionOpenOmnibox      Action = "open_omnibox"
	ActionOpenDevTools     Action = "open_devtools"
	ActionToggleFullscreen Action = "toggle_fullscreen"

	// Clipboard
	ActionCopyURL Action = "copy_url"

	// Application
	ActionQuit Action = "quit"
)

// ShortcutTable maps KeyBinding to Action.
type ShortcutTable map[KeyBinding]Action

// ShortcutSet holds all shortcut tables organized by context.
type ShortcutSet struct {
	// Global shortcuts are always active regardless of mode.
	Global ShortcutTable
	// TabMode shortcuts are only active in tab mode.
	TabMode ShortcutTable
	// PaneMode shortcuts are only active in pane mode.
	PaneMode ShortcutTable
}

// NewShortcutSet creates a ShortcutSet from the workspace configuration.
func NewShortcutSet(ctx context.Context, cfg *config.WorkspaceConfig) *ShortcutSet {
	log := logging.FromContext(ctx)
	set := &ShortcutSet{
		Global:   make(ShortcutTable),
		TabMode:  make(ShortcutTable),
		PaneMode: make(ShortcutTable),
	}

	set.buildGlobalShortcuts(ctx, cfg)
	set.buildTabModeShortcuts(ctx, cfg)
	set.buildPaneModeShortcuts(ctx, cfg)

	log.Debug().Int("global", len(set.Global)).Int("tab", len(set.TabMode)).Int("pane", len(set.PaneMode)).Msg("shortcuts registered")

	return set
}

// buildGlobalShortcuts populates global shortcuts from config.
func (s *ShortcutSet) buildGlobalShortcuts(ctx context.Context, cfg *config.WorkspaceConfig) {
	log := logging.FromContext(ctx)
	// Mode entry from activation shortcuts
	if binding, ok := ParseKeyString(cfg.TabMode.ActivationShortcut); ok {
		s.Global[binding] = ActionEnterTabMode
		log.Trace().Str("shortcut", cfg.TabMode.ActivationShortcut).Uint("keyval", binding.Keyval).Uint("mod", uint(binding.Modifiers)).Msg("tab mode activation registered")
	} else {
		log.Warn().Str("shortcut", cfg.TabMode.ActivationShortcut).Msg("failed to parse tab mode activation shortcut")
	}
	if binding, ok := ParseKeyString(cfg.PaneMode.ActivationShortcut); ok {
		s.Global[binding] = ActionEnterPaneMode
		log.Trace().Str("shortcut", cfg.PaneMode.ActivationShortcut).Uint("keyval", binding.Keyval).Uint("mod", uint(binding.Modifiers)).Msg("pane mode activation registered")
	} else {
		log.Warn().Str("shortcut", cfg.PaneMode.ActivationShortcut).Msg("failed to parse pane mode activation shortcut")
	}

	// Note: Ctrl+T is NOT registered globally - it enters tab mode.
	// In tab mode, use:
	//   n = new tab
	//   x = close tab
	//   l/tab = next tab
	//   h/shift+tab = previous tab
	//   r = rename tab
	// This follows Zellij-style modal keyboard interface.
	//
	// However, these standard browser shortcuts ARE global:
	if binding, ok := ParseKeyString(cfg.Tabs.CloseTab); ok {
		s.Global[binding] = ActionCloseTab // Ctrl+W
	}
	if binding, ok := ParseKeyString(cfg.Tabs.NextTab); ok {
		s.Global[binding] = ActionNextTab // Ctrl+Tab
	}
	if binding, ok := ParseKeyString(cfg.Tabs.PreviousTab); ok {
		s.Global[binding] = ActionPreviousTab // Ctrl+Shift+Tab
	}

	// Standard browser shortcuts (hardcoded defaults)
	s.Global[KeyBinding{uint(gdk.KEY_l), ModCtrl}] = ActionOpenOmnibox
	s.Global[KeyBinding{uint(gdk.KEY_r), ModCtrl}] = ActionReload
	s.Global[KeyBinding{uint(gdk.KEY_R), ModCtrl | ModShift}] = ActionHardReload
	s.Global[KeyBinding{uint(gdk.KEY_F5), ModNone}] = ActionReload
	s.Global[KeyBinding{uint(gdk.KEY_F5), ModCtrl}] = ActionHardReload
	s.Global[KeyBinding{uint(gdk.KEY_F12), ModNone}] = ActionOpenDevTools
	s.Global[KeyBinding{uint(gdk.KEY_Left), ModAlt}] = ActionGoBack
	s.Global[KeyBinding{uint(gdk.KEY_Right), ModAlt}] = ActionGoForward
	s.Global[KeyBinding{uint(gdk.KEY_plus), ModCtrl}] = ActionZoomIn
	s.Global[KeyBinding{uint(gdk.KEY_equal), ModCtrl}] = ActionZoomIn // Ctrl+= (no shift needed)
	s.Global[KeyBinding{uint(gdk.KEY_minus), ModCtrl}] = ActionZoomOut
	s.Global[KeyBinding{uint(gdk.KEY_0), ModCtrl}] = ActionZoomReset
	s.Global[KeyBinding{uint(gdk.KEY_q), ModCtrl}] = ActionQuit
	s.Global[KeyBinding{uint(gdk.KEY_F11), ModNone}] = ActionToggleFullscreen
	s.Global[KeyBinding{uint(gdk.KEY_C), ModCtrl | ModShift}] = ActionCopyURL

	// Pane navigation (Alt+HJKL like vim)
	s.Global[KeyBinding{uint(gdk.KEY_h), ModAlt}] = ActionFocusLeft
	s.Global[KeyBinding{uint(gdk.KEY_l), ModAlt}] = ActionFocusRight
	s.Global[KeyBinding{uint(gdk.KEY_k), ModAlt}] = ActionFocusUp
	s.Global[KeyBinding{uint(gdk.KEY_j), ModAlt}] = ActionFocusDown

	// Also support Alt+Arrow keys for pane navigation (except Left/Right used for back/forward)
	s.Global[KeyBinding{uint(gdk.KEY_Up), ModAlt}] = ActionFocusUp
	s.Global[KeyBinding{uint(gdk.KEY_Down), ModAlt}] = ActionFocusDown

	// Direct tab switching (Alt+1-9, Alt+0 for tab 10)
	s.Global[KeyBinding{uint(gdk.KEY_1), ModAlt}] = ActionSwitchTabIndex1
	s.Global[KeyBinding{uint(gdk.KEY_2), ModAlt}] = ActionSwitchTabIndex2
	s.Global[KeyBinding{uint(gdk.KEY_3), ModAlt}] = ActionSwitchTabIndex3
	s.Global[KeyBinding{uint(gdk.KEY_4), ModAlt}] = ActionSwitchTabIndex4
	s.Global[KeyBinding{uint(gdk.KEY_5), ModAlt}] = ActionSwitchTabIndex5
	s.Global[KeyBinding{uint(gdk.KEY_6), ModAlt}] = ActionSwitchTabIndex6
	s.Global[KeyBinding{uint(gdk.KEY_7), ModAlt}] = ActionSwitchTabIndex7
	s.Global[KeyBinding{uint(gdk.KEY_8), ModAlt}] = ActionSwitchTabIndex8
	s.Global[KeyBinding{uint(gdk.KEY_9), ModAlt}] = ActionSwitchTabIndex9
	s.Global[KeyBinding{uint(gdk.KEY_0), ModAlt}] = ActionSwitchTabIndex10

	// Alt+Tab style switching (switch to last active tab)
	s.Global[KeyBinding{uint(gdk.KEY_Tab), ModAlt}] = ActionSwitchLastTab
	s.Global[KeyBinding{uint(gdk.KEY_Tab), ModAlt | ModShift}] = ActionSwitchLastTab
}

// buildTabModeShortcuts populates tab mode shortcuts from config.
func (s *ShortcutSet) buildTabModeShortcuts(ctx context.Context, cfg *config.WorkspaceConfig) {
	log := logging.FromContext(ctx)
	bindings := cfg.TabMode.GetKeyBindings()
	var registered, parseErrors, unknownActions int
	for key, configAction := range bindings {
		if binding, ok := ParseKeyString(key); ok {
			if action := mapConfigAction(configAction); action != "" {
				s.TabMode[binding] = action
				registered++
				log.Trace().Str("key", key).Str("configAction", configAction).Str("action", string(action)).Uint("keyval", binding.Keyval).Msg("tab mode shortcut registered")
			} else {
				unknownActions++
				log.Warn().Str("key", key).Str("configAction", configAction).Msg("unknown config action in tab mode")
			}
		} else {
			parseErrors++
			log.Warn().Str("key", key).Msg("failed to parse tab mode key")
		}
	}
	log.Debug().Int("registered", registered).Int("parseErrors", parseErrors).Int("unknownActions", unknownActions).Msg("tab mode shortcuts built")
}

// buildPaneModeShortcuts populates pane mode shortcuts from config.
func (s *ShortcutSet) buildPaneModeShortcuts(ctx context.Context, cfg *config.WorkspaceConfig) {
	log := logging.FromContext(ctx)
	bindings := cfg.PaneMode.GetKeyBindings()
	var registered, parseErrors, unknownActions int
	for key, configAction := range bindings {
		if binding, ok := ParseKeyString(key); ok {
			if action := mapConfigAction(configAction); action != "" {
				s.PaneMode[binding] = action
				registered++
				log.Trace().Str("key", key).Str("configAction", configAction).Str("action", string(action)).Uint("keyval", binding.Keyval).Msg("pane mode shortcut registered")
			} else {
				unknownActions++
				log.Warn().Str("key", key).Str("configAction", configAction).Msg("unknown config action in pane mode")
			}
		} else {
			parseErrors++
			log.Warn().Str("key", key).Msg("failed to parse pane mode key")
		}
	}
	log.Debug().Int("registered", registered).Int("parseErrors", parseErrors).Int("unknownActions", unknownActions).Msg("pane mode shortcuts built")
}

// mapConfigAction maps config action names to Action constants.
func mapConfigAction(configAction string) Action {
	switch configAction {
	// Mode management
	case "cancel", "confirm":
		return ActionExitMode

	// Tab actions
	case "new-tab":
		return ActionNewTab
	case "close-tab":
		return ActionCloseTab
	case "next-tab":
		return ActionNextTab
	case "previous-tab":
		return ActionPreviousTab
	case "rename-tab":
		return ActionRenameTab

	// Pane actions
	case "split-right":
		return ActionSplitRight
	case "split-left":
		return ActionSplitLeft
	case "split-up":
		return ActionSplitUp
	case "split-down":
		return ActionSplitDown
	case "close-pane":
		return ActionClosePane
	case "stack-pane":
		return ActionStackPane

	// Focus navigation
	case "focus-right":
		return ActionFocusRight
	case "focus-left":
		return ActionFocusLeft
	case "focus-up":
		return ActionFocusUp
	case "focus-down":
		return ActionFocusDown

	// Stack navigation
	case "stack-nav-up", "stack-up":
		return ActionStackNavUp
	case "stack-nav-down", "stack-down":
		return ActionStackNavDown

	default:
		return ""
	}
}

// ParseKeyString converts a config key string like "ctrl+t" to a KeyBinding.
// Returns false if the string cannot be parsed.
func ParseKeyString(s string) (KeyBinding, bool) {
	if s == "" {
		return KeyBinding{}, false
	}

	s = strings.ToLower(strings.TrimSpace(s))
	parts := strings.Split(s, "+")

	var modifiers Modifier
	var keyPart string

	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch part {
		case "ctrl", "control":
			modifiers |= ModCtrl
		case "shift":
			modifiers |= ModShift
		case "alt":
			modifiers |= ModAlt
		default:
			keyPart = part
		}
	}

	if keyPart == "" {
		return KeyBinding{}, false
	}

	keyval, ok := stringToKeyval(keyPart)
	if !ok {
		return KeyBinding{}, false
	}

	return KeyBinding{
		Keyval:    keyval,
		Modifiers: modifiers,
	}, true
}

// stringToKeyval converts a key name to its GDK keyval.
func stringToKeyval(s string) (uint, bool) {
	// Special keys
	switch s {
	case "escape", "esc":
		return uint(gdk.KEY_Escape), true
	case "return", "enter":
		return uint(gdk.KEY_Return), true
	case "tab":
		return uint(gdk.KEY_Tab), true
	case "space":
		return uint(gdk.KEY_space), true
	case "backspace":
		return uint(gdk.KEY_BackSpace), true
	case "delete", "del":
		return uint(gdk.KEY_Delete), true
	case "home":
		return uint(gdk.KEY_Home), true
	case "end":
		return uint(gdk.KEY_End), true
	case "pageup", "page_up":
		return uint(gdk.KEY_Page_Up), true
	case "pagedown", "page_down":
		return uint(gdk.KEY_Page_Down), true

	// Arrow keys
	case "left", "arrowleft":
		return uint(gdk.KEY_Left), true
	case "right", "arrowright":
		return uint(gdk.KEY_Right), true
	case "up", "arrowup":
		return uint(gdk.KEY_Up), true
	case "down", "arrowdown":
		return uint(gdk.KEY_Down), true

	// Function keys
	case "f1":
		return uint(gdk.KEY_F1), true
	case "f2":
		return uint(gdk.KEY_F2), true
	case "f3":
		return uint(gdk.KEY_F3), true
	case "f4":
		return uint(gdk.KEY_F4), true
	case "f5":
		return uint(gdk.KEY_F5), true
	case "f6":
		return uint(gdk.KEY_F6), true
	case "f7":
		return uint(gdk.KEY_F7), true
	case "f8":
		return uint(gdk.KEY_F8), true
	case "f9":
		return uint(gdk.KEY_F9), true
	case "f10":
		return uint(gdk.KEY_F10), true
	case "f11":
		return uint(gdk.KEY_F11), true
	case "f12":
		return uint(gdk.KEY_F12), true

	// Symbols
	case "plus", "+":
		return uint(gdk.KEY_plus), true
	case "minus", "-":
		return uint(gdk.KEY_minus), true
	case "equal", "=":
		return uint(gdk.KEY_equal), true

	// Numbers
	case "0":
		return uint(gdk.KEY_0), true
	case "1":
		return uint(gdk.KEY_1), true
	case "2":
		return uint(gdk.KEY_2), true
	case "3":
		return uint(gdk.KEY_3), true
	case "4":
		return uint(gdk.KEY_4), true
	case "5":
		return uint(gdk.KEY_5), true
	case "6":
		return uint(gdk.KEY_6), true
	case "7":
		return uint(gdk.KEY_7), true
	case "8":
		return uint(gdk.KEY_8), true
	case "9":
		return uint(gdk.KEY_9), true
	}

	// Single letter keys (a-z)
	if len(s) == 1 && s[0] >= 'a' && s[0] <= 'z' {
		// ASCII lowercase a=97, which matches gdk.KEY_a
		return uint(s[0]), true
	}

	return 0, false
}

// Lookup finds an action for the given key binding in the appropriate table.
// It first checks the mode-specific table, then falls back to global.
func (s *ShortcutSet) Lookup(binding KeyBinding, mode Mode) (Action, bool) {
	// Normalize the binding modifiers
	binding.Modifiers &= modifierMask

	// Check mode-specific table first
	var modeTable ShortcutTable
	switch mode {
	case ModeTab:
		modeTable = s.TabMode
	case ModePane:
		modeTable = s.PaneMode
	}

	if modeTable != nil {
		if action, ok := modeTable[binding]; ok {
			return action, true
		}
	}

	// Fall back to global
	if action, ok := s.Global[binding]; ok {
		return action, true
	}

	return "", false
}

// ShouldAutoExitMode returns true if the action should cause modal mode to exit.
func ShouldAutoExitMode(action Action) bool {
	switch action {
	case ActionNewTab, ActionCloseTab, ActionRenameTab,
		ActionSplitRight, ActionSplitLeft, ActionSplitUp, ActionSplitDown,
		ActionClosePane, ActionStackPane:
		return true
	default:
		return false
	}
}
