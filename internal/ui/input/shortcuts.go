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

var keyvalByName = map[string]uint{
	"escape":     uint(gdk.KEY_Escape),
	"esc":        uint(gdk.KEY_Escape),
	"return":     uint(gdk.KEY_Return),
	"enter":      uint(gdk.KEY_Return),
	"tab":        uint(gdk.KEY_Tab),
	"space":      uint(gdk.KEY_space),
	"backspace":  uint(gdk.KEY_BackSpace),
	"delete":     uint(gdk.KEY_Delete),
	"del":        uint(gdk.KEY_Delete),
	"home":       uint(gdk.KEY_Home),
	"end":        uint(gdk.KEY_End),
	"pageup":     uint(gdk.KEY_Page_Up),
	"page_up":    uint(gdk.KEY_Page_Up),
	"pagedown":   uint(gdk.KEY_Page_Down),
	"page_down":  uint(gdk.KEY_Page_Down),
	"left":       uint(gdk.KEY_Left),
	"arrowleft":  uint(gdk.KEY_Left),
	"right":      uint(gdk.KEY_Right),
	"arrowright": uint(gdk.KEY_Right),
	"up":         uint(gdk.KEY_Up),
	"arrowup":    uint(gdk.KEY_Up),
	"down":       uint(gdk.KEY_Down),
	"arrowdown":  uint(gdk.KEY_Down),
	"f1":         uint(gdk.KEY_F1),
	"f2":         uint(gdk.KEY_F2),
	"f3":         uint(gdk.KEY_F3),
	"f4":         uint(gdk.KEY_F4),
	"f5":         uint(gdk.KEY_F5),
	"f6":         uint(gdk.KEY_F6),
	"f7":         uint(gdk.KEY_F7),
	"f8":         uint(gdk.KEY_F8),
	"f9":         uint(gdk.KEY_F9),
	"f10":        uint(gdk.KEY_F10),
	"f11":        uint(gdk.KEY_F11),
	"f12":        uint(gdk.KEY_F12),
	"plus":       uint(gdk.KEY_plus),
	"+":          uint(gdk.KEY_plus),
	"minus":      uint(gdk.KEY_minus),
	"-":          uint(gdk.KEY_minus),
	"equal":      uint(gdk.KEY_equal),
	"=":          uint(gdk.KEY_equal),
	"0":          uint(gdk.KEY_0),
	"1":          uint(gdk.KEY_1),
	"2":          uint(gdk.KEY_2),
	"3":          uint(gdk.KEY_3),
	"4":          uint(gdk.KEY_4),
	"5":          uint(gdk.KEY_5),
	"6":          uint(gdk.KEY_6),
	"7":          uint(gdk.KEY_7),
	"8":          uint(gdk.KEY_8),
	"9":          uint(gdk.KEY_9),
}

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
	ActionOpenFind         Action = "open_find"
	ActionFindNext         Action = "find_next"
	ActionFindPrev         Action = "find_prev"
	ActionCloseFind        Action = "close_find"
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
	s.registerActivationShortcuts(ctx, cfg)
	s.registerConfiguredShortcuts(cfg)
	s.registerStandardShortcuts()
	s.registerPaneNavigationShortcuts()
	s.registerTabSwitchShortcuts()
}

// buildTabModeShortcuts populates tab mode shortcuts from config.
func (s *ShortcutSet) buildTabModeShortcuts(ctx context.Context, cfg *config.WorkspaceConfig) {
	s.buildModeShortcuts(ctx, cfg.TabMode.GetKeyBindings(), s.TabMode, "tab")
}

// buildPaneModeShortcuts populates pane mode shortcuts from config.
func (s *ShortcutSet) buildPaneModeShortcuts(ctx context.Context, cfg *config.WorkspaceConfig) {
	s.buildModeShortcuts(ctx, cfg.PaneMode.GetKeyBindings(), s.PaneMode, "pane")
}

func (s *ShortcutSet) registerActivationShortcuts(ctx context.Context, cfg *config.WorkspaceConfig) {
	log := logging.FromContext(ctx)
	if binding, ok := ParseKeyString(cfg.TabMode.ActivationShortcut); ok {
		s.Global[binding] = ActionEnterTabMode
		log.Trace().
			Str("shortcut", cfg.TabMode.ActivationShortcut).
			Uint("keyval", binding.Keyval).
			Uint("mod", uint(binding.Modifiers)).
			Msg("tab mode activation registered")
	} else {
		log.Warn().Str("shortcut", cfg.TabMode.ActivationShortcut).Msg("failed to parse tab mode activation shortcut")
	}
	if binding, ok := ParseKeyString(cfg.PaneMode.ActivationShortcut); ok {
		s.Global[binding] = ActionEnterPaneMode
		log.Trace().
			Str("shortcut", cfg.PaneMode.ActivationShortcut).
			Uint("keyval", binding.Keyval).
			Uint("mod", uint(binding.Modifiers)).
			Msg("pane mode activation registered")
	} else {
		log.Warn().Str("shortcut", cfg.PaneMode.ActivationShortcut).Msg("failed to parse pane mode activation shortcut")
	}
}

func (s *ShortcutSet) registerConfiguredShortcuts(cfg *config.WorkspaceConfig) {
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
	if binding, ok := ParseKeyString(cfg.Shortcuts.ClosePane); ok {
		s.Global[binding] = ActionClosePane // Ctrl+W closes active pane
	}
	if binding, ok := ParseKeyString(cfg.Shortcuts.NextTab); ok {
		s.Global[binding] = ActionNextTab // Ctrl+Tab
	}
	if binding, ok := ParseKeyString(cfg.Shortcuts.PreviousTab); ok {
		s.Global[binding] = ActionPreviousTab // Ctrl+Shift+Tab
	}
}

func (s *ShortcutSet) registerStandardShortcuts() {
	s.Global[KeyBinding{uint(gdk.KEY_l), ModCtrl}] = ActionOpenOmnibox
	s.Global[KeyBinding{uint(gdk.KEY_f), ModCtrl}] = ActionOpenFind
	s.Global[KeyBinding{uint(gdk.KEY_F3), ModNone}] = ActionFindNext
	s.Global[KeyBinding{uint(gdk.KEY_F3), ModShift}] = ActionFindPrev
	s.Global[KeyBinding{uint(gdk.KEY_g), ModCtrl}] = ActionFindNext
	s.Global[KeyBinding{uint(gdk.KEY_g), ModCtrl | ModShift}] = ActionFindPrev
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
}

func (s *ShortcutSet) registerPaneNavigationShortcuts() {
	s.Global[KeyBinding{uint(gdk.KEY_h), ModAlt}] = ActionFocusLeft
	s.Global[KeyBinding{uint(gdk.KEY_l), ModAlt}] = ActionFocusRight
	s.Global[KeyBinding{uint(gdk.KEY_k), ModAlt}] = ActionFocusUp
	s.Global[KeyBinding{uint(gdk.KEY_j), ModAlt}] = ActionFocusDown

	s.Global[KeyBinding{uint(gdk.KEY_Up), ModAlt}] = ActionFocusUp
	s.Global[KeyBinding{uint(gdk.KEY_Down), ModAlt}] = ActionFocusDown
}

func (s *ShortcutSet) registerTabSwitchShortcuts() {
	// NOTE: Alt+1-9, Alt+0, and Alt+Tab are now handled by GlobalShortcutHandler
	// using GtkShortcutController with GTK_SHORTCUT_SCOPE_GLOBAL.
	// This is necessary because WebKitGTK's WebView consumes these key events
	// before they reach the EventControllerKey in capture phase.
	//
	// Only Alt+Shift+Tab remains here as a fallback binding.
	s.Global[KeyBinding{uint(gdk.KEY_Tab), ModAlt | ModShift}] = ActionSwitchLastTab
}

func (s *ShortcutSet) buildModeShortcuts(ctx context.Context, bindings map[string]string, dest map[KeyBinding]Action, mode string) {
	log := logging.FromContext(ctx)
	var registered, parseErrors, unknownActions int
	for key, configAction := range bindings {
		if binding, ok := ParseKeyString(key); ok {
			if action := mapConfigAction(configAction); action != "" {
				dest[binding] = action
				registered++
				log.Trace().
					Str("key", key).
					Str("configAction", configAction).
					Str("action", string(action)).
					Uint("keyval", binding.Keyval).
					Msg(mode + " mode shortcut registered")
			} else {
				unknownActions++
				log.Warn().Str("key", key).Str("configAction", configAction).Msg("unknown config action in " + mode + " mode")
			}
		} else {
			parseErrors++
			log.Warn().Str("key", key).Msg("failed to parse " + mode + " mode key")
		}
	}
	log.Debug().
		Int("registered", registered).
		Int("parseErrors", parseErrors).
		Int("unknownActions", unknownActions).
		Msg(mode + " mode shortcuts built")
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
	if keyval, ok := keyvalByName[s]; ok {
		return keyval, true
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
