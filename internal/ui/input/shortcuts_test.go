package input

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/puregotk/v4/gdk"
)

func TestParseKeyString_SingleKeys(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   KeyBinding
		wantOk bool
	}{
		{
			name:   "escape",
			input:  "escape",
			want:   KeyBinding{Keyval: uint(gdk.KEY_Escape), Modifiers: ModNone},
			wantOk: true,
		},
		{
			name:   "esc alias",
			input:  "esc",
			want:   KeyBinding{Keyval: uint(gdk.KEY_Escape), Modifiers: ModNone},
			wantOk: true,
		},
		{
			name:   "return",
			input:  "return",
			want:   KeyBinding{Keyval: uint(gdk.KEY_Return), Modifiers: ModNone},
			wantOk: true,
		},
		{
			name:   "enter alias",
			input:  "enter",
			want:   KeyBinding{Keyval: uint(gdk.KEY_Return), Modifiers: ModNone},
			wantOk: true,
		},
		{
			name:   "tab",
			input:  "tab",
			want:   KeyBinding{Keyval: uint(gdk.KEY_Tab), Modifiers: ModNone},
			wantOk: true,
		},
		{
			name:   "space",
			input:  "space",
			want:   KeyBinding{Keyval: uint(gdk.KEY_space), Modifiers: ModNone},
			wantOk: true,
		},
		{
			name:   "plus symbol",
			input:  "+",
			want:   KeyBinding{Keyval: uint(gdk.KEY_plus), Modifiers: ModNone},
			wantOk: true,
		},
		{
			name:   "f5",
			input:  "f5",
			want:   KeyBinding{Keyval: uint(gdk.KEY_F5), Modifiers: ModNone},
			wantOk: true,
		},
		{
			name:   "f12",
			input:  "f12",
			want:   KeyBinding{Keyval: uint(gdk.KEY_F12), Modifiers: ModNone},
			wantOk: true,
		},
		{
			name:   "single letter",
			input:  "t",
			want:   KeyBinding{Keyval: uint('t'), Modifiers: ModNone},
			wantOk: true,
		},
		{
			name:   "single number",
			input:  "0",
			want:   KeyBinding{Keyval: uint(gdk.KEY_0), Modifiers: ModNone},
			wantOk: true,
		},
		{
			name:   "arrow left",
			input:  "left",
			want:   KeyBinding{Keyval: uint(gdk.KEY_Left), Modifiers: ModNone},
			wantOk: true,
		},
		{
			name:   "arrow right",
			input:  "right",
			want:   KeyBinding{Keyval: uint(gdk.KEY_Right), Modifiers: ModNone},
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseKeyString(tt.input)
			if ok != tt.wantOk {
				t.Errorf("ParseKeyString(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
				return
			}
			if !tt.wantOk {
				return
			}
			if got.Keyval != tt.want.Keyval {
				t.Errorf("ParseKeyString(%q) keyval = %d, want %d", tt.input, got.Keyval, tt.want.Keyval)
			}
			if got.Modifiers != tt.want.Modifiers {
				t.Errorf("ParseKeyString(%q) modifiers = %d, want %d", tt.input, got.Modifiers, tt.want.Modifiers)
			}
		})
	}
}

func TestParseKeyString_WithModifiers(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   KeyBinding
		wantOk bool
	}{
		{
			name:   "ctrl+t",
			input:  "ctrl+t",
			want:   KeyBinding{Keyval: uint('t'), Modifiers: ModCtrl},
			wantOk: true,
		},
		{
			name:   "control+t",
			input:  "control+t",
			want:   KeyBinding{Keyval: uint('t'), Modifiers: ModCtrl},
			wantOk: true,
		},
		{
			name:   "shift+tab",
			input:  "shift+tab",
			want:   KeyBinding{Keyval: uint(gdk.KEY_Tab), Modifiers: ModShift},
			wantOk: true,
		},
		{
			name:   "alt+left",
			input:  "alt+left",
			want:   KeyBinding{Keyval: uint(gdk.KEY_Left), Modifiers: ModAlt},
			wantOk: true,
		},
		{
			name:   "ctrl+shift+t",
			input:  "ctrl+shift+t",
			want:   KeyBinding{Keyval: uint('t'), Modifiers: ModCtrl | ModShift},
			wantOk: true,
		},
		{
			name:   "ctrl+r",
			input:  "ctrl+r",
			want:   KeyBinding{Keyval: uint('r'), Modifiers: ModCtrl},
			wantOk: true,
		},
		{
			name:   "ctrl++",
			input:  "ctrl++",
			want:   KeyBinding{Keyval: uint(gdk.KEY_plus), Modifiers: ModCtrl},
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseKeyString(tt.input)
			if ok != tt.wantOk {
				t.Errorf("ParseKeyString(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
				return
			}
			if !tt.wantOk {
				return
			}
			if got.Keyval != tt.want.Keyval {
				t.Errorf("ParseKeyString(%q) keyval = %d, want %d", tt.input, got.Keyval, tt.want.Keyval)
			}
			if got.Modifiers != tt.want.Modifiers {
				t.Errorf("ParseKeyString(%q) modifiers = %d, want %d", tt.input, got.Modifiers, tt.want.Modifiers)
			}
		})
	}
}

func TestParseKeyString_Invalid(t *testing.T) {
	tests := []string{
		"",
		"ctrl+",
		"unknownkey",
		"ctrl+unknownkey",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, ok := ParseKeyString(input)
			if ok {
				t.Errorf("ParseKeyString(%q) should have failed", input)
			}
		})
	}
}

func TestParseKeyString_CaseInsensitive(t *testing.T) {
	tests := []struct {
		input1 string
		input2 string
	}{
		// Modifiers and named keys are case-insensitive.
		{"ctrl+t", "CTRL+t"},
		{"Escape", "escape"},
		{"Tab", "TAB"},
		// If shift is explicitly present, the key letter case shouldn't matter.
		{"Ctrl+Shift+T", "ctrl+shift+t"},
	}

	for _, tt := range tests {
		t.Run(tt.input1+" vs "+tt.input2, func(t *testing.T) {
			got1, ok1 := ParseKeyString(tt.input1)
			got2, ok2 := ParseKeyString(tt.input2)

			if ok1 != ok2 {
				t.Errorf("ParseKeyString case sensitivity: %q=%v, %q=%v", tt.input1, ok1, tt.input2, ok2)
				return
			}

			if got1 != got2 {
				t.Errorf("ParseKeyString case sensitivity: %q=%v, %q=%v", tt.input1, got1, tt.input2, got2)
			}
		})
	}
}

func TestParseKeyString_UppercaseSingleLetterAddsShift(t *testing.T) {
	got1, ok1 := ParseKeyString("M")
	got2, ok2 := ParseKeyString("shift+m")
	if !ok1 || !ok2 {
		t.Fatalf("ParseKeyString should succeed for M and shift+m")
	}
	if got1 != got2 {
		t.Fatalf("ParseKeyString uppercase should imply shift: got %v want %v", got1, got2)
	}
}

func TestShortcutSet_Lookup(t *testing.T) {
	set := &ShortcutSet{
		Global: ShortcutTable{
			{uint(gdk.KEY_q), ModCtrl}: ActionQuit,
			{uint(gdk.KEY_l), ModCtrl}: ActionOpenOmnibox,
		},
		TabMode: ShortcutTable{
			{uint('n'), ModNone}:            ActionNewTab,
			{uint(gdk.KEY_Escape), ModNone}: ActionExitMode,
		},
		PaneMode: ShortcutTable{
			{uint('x'), ModNone}:            ActionClosePane,
			{uint(gdk.KEY_Escape), ModNone}: ActionExitMode,
		},
	}

	tests := []struct {
		name    string
		binding KeyBinding
		mode    Mode
		want    Action
		wantOk  bool
	}{
		{
			name:    "global in normal mode",
			binding: KeyBinding{uint(gdk.KEY_q), ModCtrl},
			mode:    ModeNormal,
			want:    ActionQuit,
			wantOk:  true,
		},
		{
			name:    "global in tab mode",
			binding: KeyBinding{uint(gdk.KEY_l), ModCtrl},
			mode:    ModeTab,
			want:    ActionOpenOmnibox,
			wantOk:  true,
		},
		{
			name:    "tab mode action",
			binding: KeyBinding{uint('n'), ModNone},
			mode:    ModeTab,
			want:    ActionNewTab,
			wantOk:  true,
		},
		{
			name:    "pane mode action",
			binding: KeyBinding{uint('x'), ModNone},
			mode:    ModePane,
			want:    ActionClosePane,
			wantOk:  true,
		},
		{
			name:    "escape exits tab mode",
			binding: KeyBinding{uint(gdk.KEY_Escape), ModNone},
			mode:    ModeTab,
			want:    ActionExitMode,
			wantOk:  true,
		},
		{
			name:    "escape exits pane mode",
			binding: KeyBinding{uint(gdk.KEY_Escape), ModNone},
			mode:    ModePane,
			want:    ActionExitMode,
			wantOk:  true,
		},
		{
			name:    "unknown key in normal mode",
			binding: KeyBinding{uint('z'), ModNone},
			mode:    ModeNormal,
			want:    "",
			wantOk:  false,
		},
		{
			name:    "tab action not in normal mode",
			binding: KeyBinding{uint('n'), ModNone},
			mode:    ModeNormal,
			want:    "",
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := set.Lookup(tt.binding, tt.mode)
			if ok != tt.wantOk {
				t.Errorf("Lookup() ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if got != tt.want {
				t.Errorf("Lookup() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldAutoExitMode(t *testing.T) {
	exitActions := []Action{
		ActionNewTab,
		ActionCloseTab,
		ActionRenameTab,
		ActionSplitRight,
		ActionSplitLeft,
		ActionSplitUp,
		ActionSplitDown,
		ActionClosePane,
		ActionStackPane,
		ActionMovePaneToTab,
		ActionMovePaneToNextTab,
		ActionEjectPaneToWindow,
	}

	stayActions := []Action{
		ActionNextTab,
		ActionPreviousTab,
		ActionFocusRight,
		ActionFocusLeft,
		ActionGoBack,
		ActionZoomIn,
		ActionOpenOmnibox,
	}

	for _, action := range exitActions {
		if !ShouldAutoExitMode(action) {
			t.Errorf("ShouldAutoExitMode(%s) = false, want true", action)
		}
	}

	for _, action := range stayActions {
		if ShouldAutoExitMode(action) {
			t.Errorf("ShouldAutoExitMode(%s) = true, want false", action)
		}
	}
}

func TestMapConfigAction_EjectPaneToWindowAliases(t *testing.T) {
	tests := []string{"eject_pane_to_window", "eject-pane-to-window"}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			if got := mapConfigAction(name); got != ActionEjectPaneToWindow {
				t.Fatalf("mapConfigAction(%s) = %s, want %s", name, got, ActionEjectPaneToWindow)
			}
		})
	}
}

func TestMapConfigAction_ToggleFloatingPane(t *testing.T) {
	action := mapConfigAction("toggle_floating_pane")
	if action != ActionToggleFloatingPane {
		t.Fatalf("mapConfigAction(toggle_floating_pane) = %s, want %s", action, ActionToggleFloatingPane)
	}
}

func TestMapConfigAction_ToggleSystemViews(t *testing.T) {
	tests := []struct {
		name string
		want Action
	}{
		{name: "toggle_history_systemview", want: ActionToggleHistorySystemView},
		{name: "toggle_favorites_systemview", want: ActionToggleFavoritesSystemView},
		{name: "toggle_config_systemview", want: ActionToggleConfigSystemView},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapConfigAction(tt.name); got != tt.want {
				t.Fatalf("mapConfigAction(%s) = %s, want %s", tt.name, got, tt.want)
			}
		})
	}
}

func TestMapConfigAction_ToggleSystemViewsHyphenAlias(t *testing.T) {
	tests := []struct {
		name string
		want Action
	}{
		{name: "toggle-history-systemview", want: ActionToggleHistorySystemView},
		{name: "toggle-favorites-systemview", want: ActionToggleFavoritesSystemView},
		{name: "toggle-config-systemview", want: ActionToggleConfigSystemView},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapConfigAction(tt.name); got != tt.want {
				t.Fatalf("mapConfigAction(%s) = %s, want %s", tt.name, got, tt.want)
			}
		})
	}
}

func TestMapConfigAction_ToggleFloatingPaneHyphenAlias(t *testing.T) {
	action := mapConfigAction("toggle-floating-pane")
	if action != ActionToggleFloatingPane {
		t.Fatalf("mapConfigAction(toggle-floating-pane) = %s, want %s", action, ActionToggleFloatingPane)
	}
}

func TestGlobalShortcutActionMap_ConsumeOrExpelHyphenAliases(t *testing.T) {
	actions := globalShortcutActionMap()
	tests := []struct {
		name string
		want Action
	}{
		{name: "consume-or-expel-left", want: ActionConsumeOrExpelLeft},
		{name: "consume-or-expel-right", want: ActionConsumeOrExpelRight},
		{name: "consume-or-expel-up", want: ActionConsumeOrExpelUp},
		{name: "consume-or-expel-down", want: ActionConsumeOrExpelDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := actions[tt.name]; got != tt.want {
				t.Fatalf("globalShortcutActionMap()[%s] = %s, want %s", tt.name, got, tt.want)
			}
		})
	}
}

func TestNewShortcutSet_PageModeActivationShortcut(t *testing.T) {
	workspace := &entity.WorkspaceConfig{
		TabMode:  entity.TabModeConfig{ActivationShortcut: "ctrl+t"},
		PaneMode: entity.PaneModeConfig{ActivationShortcut: "ctrl+p"},
		PageMode: entity.PageModeConfig{ActivationShortcut: "ctrl+y"},
	}

	set := NewShortcutSet(context.Background(), workspace, nil)

	binding, ok := ParseKeyString("ctrl+y")
	if !ok {
		t.Fatal("failed to parse ctrl+y")
	}

	action, found := set.Global[binding]
	if !found {
		t.Fatal("page mode activation shortcut not found in Global")
	}
	if action != ActionEnterPageMode {
		t.Fatalf("Global shortcut action = %s, want %s", action, ActionEnterPageMode)
	}
}

func TestShortcutSet_Lookup_PageMode(t *testing.T) {
	set := &ShortcutSet{
		Global: ShortcutTable{
			{uint(gdk.KEY_q), ModCtrl}: ActionQuit,
		},
		PageMode: ShortcutTable{
			{uint('h'), ModNone}:            ActionPageScrollLeft,
			{uint('j'), ModNone}:            ActionPageScrollDown,
			{uint('k'), ModNone}:            ActionPageScrollUp,
			{uint('l'), ModNone}:            ActionPageScrollRight,
			{uint(gdk.KEY_Escape), ModNone}: ActionExitMode,
			{uint(gdk.KEY_Return), ModNone}: ActionExitMode,
			{uint('y'), ModCtrl}:            ActionEnterPageMode,
		},
	}

	tests := []struct {
		name    string
		binding KeyBinding
		want    Action
		wantOk  bool
	}{
		{
			name:    "scroll left",
			binding: KeyBinding{uint('h'), ModNone},
			want:    ActionPageScrollLeft,
			wantOk:  true,
		},
		{
			name:    "scroll down",
			binding: KeyBinding{uint('j'), ModNone},
			want:    ActionPageScrollDown,
			wantOk:  true,
		},
		{
			name:    "scroll up",
			binding: KeyBinding{uint('k'), ModNone},
			want:    ActionPageScrollUp,
			wantOk:  true,
		},
		{
			name:    "scroll right",
			binding: KeyBinding{uint('l'), ModNone},
			want:    ActionPageScrollRight,
			wantOk:  true,
		},
		{
			name:    "escape exits page mode",
			binding: KeyBinding{uint(gdk.KEY_Escape), ModNone},
			want:    ActionExitMode,
			wantOk:  true,
		},
		{
			name:    "enter exits page mode",
			binding: KeyBinding{uint(gdk.KEY_Return), ModNone},
			want:    ActionExitMode,
			wantOk:  true,
		},
		{
			name:    "activation shortcut still toggles page mode",
			binding: KeyBinding{uint('y'), ModCtrl},
			want:    ActionEnterPageMode,
			wantOk:  true,
		},
		{
			name:    "global shortcut does not fire inside page mode",
			binding: KeyBinding{uint(gdk.KEY_q), ModCtrl},
			want:    "",
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := set.Lookup(tt.binding, ModePage)
			if ok != tt.wantOk {
				t.Errorf("Lookup() ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if got != tt.want {
				t.Errorf("Lookup() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapConfigAction_PageScrollActions(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		expected Action
	}{
		{name: "page_scroll_left", config: "page_scroll_left", expected: ActionPageScrollLeft},
		{name: "page-scroll-left", config: "page-scroll-left", expected: ActionPageScrollLeft},
		{name: "page_scroll_down", config: "page_scroll_down", expected: ActionPageScrollDown},
		{name: "page-scroll-down", config: "page-scroll-down", expected: ActionPageScrollDown},
		{name: "page_scroll_up", config: "page_scroll_up", expected: ActionPageScrollUp},
		{name: "page-scroll-up", config: "page-scroll-up", expected: ActionPageScrollUp},
		{name: "page_scroll_right", config: "page_scroll_right", expected: ActionPageScrollRight},
		{name: "page-scroll-right", config: "page-scroll-right", expected: ActionPageScrollRight},
		{name: "page_scroll_down_fast", config: "page_scroll_down_fast", expected: ActionPageScrollDownFast},
		{name: "page-scroll-down-fast", config: "page-scroll-down-fast", expected: ActionPageScrollDownFast},
		{name: "page_scroll_up_fast", config: "page_scroll_up_fast", expected: ActionPageScrollUpFast},
		{name: "page-scroll-up-fast", config: "page-scroll-up-fast", expected: ActionPageScrollUpFast},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapConfigAction(tt.config)
			if got != tt.expected {
				t.Fatalf("mapConfigAction(%s) = %s, want %s", tt.config, got, tt.expected)
			}
		})
	}
}
