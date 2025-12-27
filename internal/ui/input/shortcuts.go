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

	"bracketleft":  uint(gdk.KEY_bracketleft),
	"bracketright": uint(gdk.KEY_bracketright),
	"[":            uint(gdk.KEY_bracketleft),
	"]":            uint(gdk.KEY_bracketright),
	"braceleft":    uint(gdk.KEY_braceleft),
	"braceright":   uint(gdk.KEY_braceright),
	"{":            uint(gdk.KEY_braceleft),
	"}":            uint(gdk.KEY_braceright),
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
	ActionEnterTabMode     Action = "enter_tab_mode"
	ActionEnterPaneMode    Action = "enter_pane_mode"
	ActionEnterSessionMode Action = "enter_session_mode"
	ActionEnterResizeMode  Action = "enter_resize_mode"
	ActionExitMode         Action = "exit_mode"

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

	ActionMovePaneToTab     Action = "move_pane_to_tab"
	ActionMovePaneToNextTab Action = "move_pane_to_next_tab"

	ActionConsumeOrExpelLeft  Action = "consume_or_expel_left"
	ActionConsumeOrExpelRight Action = "consume_or_expel_right"
	ActionConsumeOrExpelUp    Action = "consume_or_expel_up"
	ActionConsumeOrExpelDown  Action = "consume_or_expel_down"

	// Pane focus navigation
	ActionFocusRight Action = "focus_right"
	ActionFocusLeft  Action = "focus_left"
	ActionFocusUp    Action = "focus_up"
	ActionFocusDown  Action = "focus_down"

	// Resize actions (modal)
	ActionResizeIncreaseLeft  Action = "resize_increase_left"
	ActionResizeIncreaseRight Action = "resize_increase_right"
	ActionResizeIncreaseUp    Action = "resize_increase_up"
	ActionResizeIncreaseDown  Action = "resize_increase_down"
	ActionResizeDecreaseLeft  Action = "resize_decrease_left"
	ActionResizeDecreaseRight Action = "resize_decrease_right"
	ActionResizeDecreaseUp    Action = "resize_decrease_up"
	ActionResizeDecreaseDown  Action = "resize_decrease_down"
	ActionResizeIncrease      Action = "resize_increase"
	ActionResizeDecrease      Action = "resize_decrease"

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

	// Session management
	ActionOpenSessionManager Action = "open_session_manager"

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
	// SessionMode shortcuts are only active in session mode.
	SessionMode ShortcutTable
	// ResizeMode shortcuts are only active in resize mode.
	ResizeMode ShortcutTable
}

// NewShortcutSet creates a ShortcutSet from the configuration.
func NewShortcutSet(ctx context.Context, cfg *config.Config) *ShortcutSet {
	log := logging.FromContext(ctx)
	set := &ShortcutSet{
		Global:      make(ShortcutTable),
		TabMode:     make(ShortcutTable),
		PaneMode:    make(ShortcutTable),
		SessionMode: make(ShortcutTable),
		ResizeMode:  make(ShortcutTable),
	}

	set.buildGlobalShortcuts(ctx, cfg)
	set.buildTabModeShortcuts(ctx, &cfg.Workspace)
	set.buildPaneModeShortcuts(ctx, &cfg.Workspace)
	set.buildResizeModeShortcuts(ctx, &cfg.Workspace)
	set.buildSessionModeShortcuts(ctx, &cfg.Session)

	log.Debug().
		Int("global", len(set.Global)).
		Int("tab", len(set.TabMode)).
		Int("pane", len(set.PaneMode)).
		Int("resize", len(set.ResizeMode)).
		Int("session", len(set.SessionMode)).
		Msg("shortcuts registered")

	return set
}

// buildGlobalShortcuts populates global shortcuts from config.
func (s *ShortcutSet) buildGlobalShortcuts(ctx context.Context, cfg *config.Config) {
	s.registerActivationShortcuts(ctx, cfg)
	s.registerConfiguredShortcuts(&cfg.Workspace)
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

// buildSessionModeShortcuts populates session mode shortcuts from config.
func (s *ShortcutSet) buildSessionModeShortcuts(ctx context.Context, cfg *config.SessionConfig) {
	s.buildModeShortcuts(ctx, cfg.SessionMode.GetKeyBindings(), s.SessionMode, "session")
}

// buildResizeModeShortcuts populates resize mode shortcuts from config.
func (s *ShortcutSet) buildResizeModeShortcuts(ctx context.Context, cfg *config.WorkspaceConfig) {
	s.buildModeShortcuts(ctx, cfg.ResizeMode.GetKeyBindings(), s.ResizeMode, "resize")
}

func (s *ShortcutSet) registerActivationShortcuts(ctx context.Context, cfg *config.Config) {
	log := logging.FromContext(ctx)
	if binding, ok := ParseKeyString(cfg.Workspace.TabMode.ActivationShortcut); ok {
		s.Global[binding] = ActionEnterTabMode
		log.Trace().
			Str("shortcut", cfg.Workspace.TabMode.ActivationShortcut).
			Uint("keyval", binding.Keyval).
			Uint("mod", uint(binding.Modifiers)).
			Msg("tab mode activation registered")
	} else {
		log.Warn().Str("shortcut", cfg.Workspace.TabMode.ActivationShortcut).Msg("failed to parse tab mode activation shortcut")
	}
	if binding, ok := ParseKeyString(cfg.Workspace.PaneMode.ActivationShortcut); ok {
		s.Global[binding] = ActionEnterPaneMode
		log.Trace().
			Str("shortcut", cfg.Workspace.PaneMode.ActivationShortcut).
			Uint("keyval", binding.Keyval).
			Uint("mod", uint(binding.Modifiers)).
			Msg("pane mode activation registered")
	} else {
		log.Warn().Str("shortcut", cfg.Workspace.PaneMode.ActivationShortcut).Msg("failed to parse pane mode activation shortcut")
	}
	if binding, ok := ParseKeyString(cfg.Session.SessionMode.ActivationShortcut); ok {
		s.Global[binding] = ActionEnterSessionMode
		log.Trace().
			Str("shortcut", cfg.Session.SessionMode.ActivationShortcut).
			Uint("keyval", binding.Keyval).
			Uint("mod", uint(binding.Modifiers)).
			Msg("session mode activation registered")
	} else {
		log.Warn().Str("shortcut", cfg.Session.SessionMode.ActivationShortcut).Msg("failed to parse session mode activation shortcut")
	}
	if binding, ok := ParseKeyString(cfg.Workspace.ResizeMode.ActivationShortcut); ok {
		s.Global[binding] = ActionEnterResizeMode
		log.Trace().
			Str("shortcut", cfg.Workspace.ResizeMode.ActivationShortcut).
			Uint("keyval", binding.Keyval).
			Uint("mod", uint(binding.Modifiers)).
			Msg("resize mode activation registered")
	} else {
		log.Warn().Str("shortcut", cfg.Workspace.ResizeMode.ActivationShortcut).Msg("failed to parse resize mode activation shortcut")
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

	if binding, ok := ParseKeyString(cfg.Shortcuts.ConsumeOrExpelLeft); ok {
		s.Global[binding] = ActionConsumeOrExpelLeft
	}
	if binding, ok := ParseKeyString(cfg.Shortcuts.ConsumeOrExpelRight); ok {
		s.Global[binding] = ActionConsumeOrExpelRight
	}
	if binding, ok := ParseKeyString(cfg.Shortcuts.ConsumeOrExpelUp); ok {
		s.Global[binding] = ActionConsumeOrExpelUp
	}
	if binding, ok := ParseKeyString(cfg.Shortcuts.ConsumeOrExpelDown); ok {
		s.Global[binding] = ActionConsumeOrExpelDown
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
	// Session management - direct shortcut to open session manager
	s.Global[KeyBinding{uint(gdk.KEY_s), ModCtrl | ModShift}] = ActionOpenSessionManager
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

var configActionToAction = map[string]Action{
	// Tab actions
	"new-tab":      ActionNewTab,
	"close-tab":    ActionCloseTab,
	"next-tab":     ActionNextTab,
	"previous-tab": ActionPreviousTab,
	"rename-tab":   ActionRenameTab,

	// Pane actions
	"split-right":           ActionSplitRight,
	"split-left":            ActionSplitLeft,
	"split-up":              ActionSplitUp,
	"split-down":            ActionSplitDown,
	"close-pane":            ActionClosePane,
	"stack-pane":            ActionStackPane,
	"move-pane-to-tab":      ActionMovePaneToTab,
	"move-pane-to-next-tab": ActionMovePaneToNextTab,

	"consume-or-expel-left":  ActionConsumeOrExpelLeft,
	"consume-or-expel-right": ActionConsumeOrExpelRight,
	"consume-or-expel-up":    ActionConsumeOrExpelUp,
	"consume-or-expel-down":  ActionConsumeOrExpelDown,

	// Focus navigation
	"focus-right": ActionFocusRight,
	"focus-left":  ActionFocusLeft,
	"focus-up":    ActionFocusUp,
	"focus-down":  ActionFocusDown,

	// Stack navigation
	"stack-nav-up":   ActionStackNavUp,
	"stack-up":       ActionStackNavUp,
	"stack-nav-down": ActionStackNavDown,
	"stack-down":     ActionStackNavDown,

	// Resize actions
	"resize-increase-left":  ActionResizeIncreaseLeft,
	"resize-increase-right": ActionResizeIncreaseRight,
	"resize-increase-up":    ActionResizeIncreaseUp,
	"resize-increase-down":  ActionResizeIncreaseDown,
	"resize-decrease-left":  ActionResizeDecreaseLeft,
	"resize-decrease-right": ActionResizeDecreaseRight,
	"resize-decrease-up":    ActionResizeDecreaseUp,
	"resize-decrease-down":  ActionResizeDecreaseDown,
	"resize-increase":       ActionResizeIncrease,
	"resize-decrease":       ActionResizeDecrease,

	// Session actions
	"session-manager": ActionOpenSessionManager,
}

// mapConfigAction maps config action names to Action constants.
func mapConfigAction(configAction string) Action {
	if configAction == "cancel" || configAction == "confirm" {
		return ActionExitMode
	}
	return configActionToAction[configAction]
}

// ParseKeyString converts a config key string like "ctrl+t" to a KeyBinding.
// Returns false if the string cannot be parsed.
func ParseKeyString(s string) (KeyBinding, bool) {
	if s == "" {
		return KeyBinding{}, false
	}

	s = strings.TrimSpace(s)
	if s == "" {
		return KeyBinding{}, false
	}
	if s == "+" {
		return KeyBinding{Keyval: uint(gdk.KEY_plus), Modifiers: ModNone}, true
	}

	parts := strings.Split(s, "+")

	var modifiers Modifier
	var keyPart string

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		lower := strings.ToLower(part)
		switch lower {
		case "ctrl", "control":
			modifiers |= ModCtrl
		case "shift":
			modifiers |= ModShift
		case "alt":
			modifiers |= ModAlt
		default:
			if keyPart != "" {
				return KeyBinding{}, false
			}
			keyPart = part
		}
	}

	// Allow parsing "ctrl++" / "alt+shift++" where the key is "+".
	if keyPart == "" && strings.HasSuffix(s, "++") {
		keyPart = "+"
	}

	if keyPart == "" {
		return KeyBinding{}, false
	}

	// Treat uppercase single-letter keys as Shift+<letter>.
	if len(keyPart) == 1 && keyPart[0] >= 'A' && keyPart[0] <= 'Z' {
		modifiers |= ModShift
		keyPart = strings.ToLower(keyPart)
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
	if s == "" {
		return 0, false
	}

	if keyval, ok := keyvalByName[strings.ToLower(s)]; ok {
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
	case ModeResize:
		modeTable = s.ResizeMode
	case ModeSession:
		modeTable = s.SessionMode
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
		ActionClosePane, ActionStackPane,
		ActionMovePaneToTab, ActionMovePaneToNextTab,
		ActionConsumeOrExpelLeft, ActionConsumeOrExpelRight, ActionConsumeOrExpelUp, ActionConsumeOrExpelDown,
		ActionOpenSessionManager:
		return true
	default:
		return false
	}
}
