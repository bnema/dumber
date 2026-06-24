package bootstrap

import (
	"reflect"
	"testing"

	bootstrapmocks "github.com/bnema/dumber/internal/bootstrap/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/stretchr/testify/mock"
)

type configManagerState struct {
	current    *config.Config
	watchCalls int
	callbacks  []func(*config.Config)
}

func newMockConfigManager(t *testing.T, state *configManagerState) *bootstrapmocks.MockConfigChangeManager {
	t.Helper()

	manager := bootstrapmocks.NewMockConfigChangeManager(t)
	manager.EXPECT().Get().RunAndReturn(state.get).Maybe()
	manager.EXPECT().Watch().RunAndReturn(state.watch).Maybe()
	manager.EXPECT().OnConfigChange(mock.Anything).RunAndReturn(state.onConfigChange).Maybe()
	return manager
}

func (s *configManagerState) watch() error {
	s.watchCalls++
	return nil
}

func (s *configManagerState) onConfigChange(callback func(*config.Config)) {
	s.callbacks = append(s.callbacks, callback)
}

func (s *configManagerState) get() *config.Config {
	return s.current
}

func (s *configManagerState) emit(next *config.Config) {
	s.current = next
	for _, callback := range s.callbacks {
		callback(next)
	}
}

func TestEngineSettingsPayloadFromConfigMapsRuntimeFields(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DefaultUIScale = 1.25
	cfg.Appearance.SansFont = "Inter"
	cfg.Appearance.SerifFont = "Literata"
	cfg.Appearance.MonospaceFont = "Fira Code"
	cfg.Appearance.DefaultFontSize = 17
	cfg.Debug.EnableDevTools = true
	cfg.Logging.CaptureConsole = true
	cfg.Engine.WebKit.DrawCompositingIndicators = true
	cfg.Media.HardwareDecodingMode = config.HardwareDecodingForce
	cfg.Clipboard.AutoCopyOnSelection = true

	got := EngineSettingsPayloadFromConfig(cfg)

	if got.DefaultUIScale != 1.25 {
		t.Fatalf("DefaultUIScale=%v, want 1.25", got.DefaultUIScale)
	}
	if got.WebContent.SansFont != "Inter" ||
		got.WebContent.SerifFont != "Literata" ||
		got.WebContent.MonospaceFont != "Fira Code" ||
		got.WebContent.DefaultFontSize != 17 {
		t.Fatalf("font settings not mapped: %#v", got.WebContent)
	}
	if !got.WebContent.EnableDevTools ||
		!got.WebContent.CaptureConsole ||
		!got.WebContent.DrawCompositingIndicators {
		t.Fatalf("debug settings not mapped: %#v", got.WebContent)
	}
	if got.WebContent.HardwareDecoding != entity.EngineHardwareDecodingForce {
		t.Fatalf("HardwareDecoding=%q, want %q", got.WebContent.HardwareDecoding, entity.EngineHardwareDecodingForce)
	}
	autoCopyField := reflect.ValueOf(got.WebContent).FieldByName("AutoCopyOnSelection")
	if !autoCopyField.IsValid() || !autoCopyField.Bool() {
		t.Fatalf("AutoCopyOnSelection not mapped in web content payload: %#v", got.WebContent)
	}
}

func TestRuntimeConfigProviderUpdatesSnapshotBeforeCallback(t *testing.T) {
	initial := config.DefaultConfig()
	initial.DefaultUIScale = 1
	manager := &configManagerState{}
	provider := NewRuntimeConfigProvider(initial, newMockConfigManager(t, manager))

	var seen entity.RuntimeConfigSnapshot
	provider.OnChange(func(snapshot entity.RuntimeConfigSnapshot) {
		seen = snapshot
	})

	next := config.DefaultConfig()
	next.DefaultUIScale = 1.8
	manager.emit(next)

	if got := provider.Current().UI.DefaultUIScale; got != 1.8 {
		t.Fatalf("Current DefaultUIScale=%v, want 1.8", got)
	}
	if seen.UI.DefaultUIScale != 1.8 {
		t.Fatalf("callback DefaultUIScale=%v, want 1.8", seen.UI.DefaultUIScale)
	}
}

func TestRuntimeConfigProviderCurrentReflectsManagerGetWithoutSubscription(t *testing.T) {
	initial := config.DefaultConfig()
	initial.DefaultUIScale = 1
	manager := &configManagerState{current: initial}
	provider := NewRuntimeConfigProvider(initial, newMockConfigManager(t, manager))

	next := config.DefaultConfig()
	next.DefaultUIScale = 1.8
	manager.current = next

	if got := provider.Current().UI.DefaultUIScale; got != 1.8 {
		t.Fatalf("Current DefaultUIScale=%v, want 1.8", got)
	}
}

func TestRuntimeConfigProviderCurrentFallsBackWhenManagerGetReturnsNil(t *testing.T) {
	initial := config.DefaultConfig()
	initial.DefaultUIScale = 1.35
	manager := &configManagerState{}
	provider := NewRuntimeConfigProvider(initial, newMockConfigManager(t, manager))

	got := provider.Current()
	want := RuntimeConfigSnapshotFromConfig(initial)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Current()=%#v, want initial fallback %#v", got, want)
	}
}

func TestRuntimeConfigProviderOnChangeCallbackCanCallCurrent(t *testing.T) {
	initial := config.DefaultConfig()
	initial.DefaultUIScale = 1
	manager := &configManagerState{current: initial}
	provider := NewRuntimeConfigProvider(initial, newMockConfigManager(t, manager))

	completed := false
	var seen entity.RuntimeConfigSnapshot
	provider.OnChange(func(entity.RuntimeConfigSnapshot) {
		seen = provider.Current()
		completed = true
	})

	next := config.DefaultConfig()
	next.DefaultUIScale = 1.8
	manager.emit(next)

	if !completed {
		t.Fatal("callback did not complete")
	}
	if seen.UI.DefaultUIScale != 1.8 {
		t.Fatalf("callback Current DefaultUIScale=%v, want 1.8", seen.UI.DefaultUIScale)
	}
}

func TestRuntimeConfigProviderOnChangeFallsBackWhenPayloadIsNil(t *testing.T) {
	initial := config.DefaultConfig()
	initial.DefaultUIScale = 1.35
	manager := &configManagerState{current: initial}
	provider := NewRuntimeConfigProvider(initial, newMockConfigManager(t, manager))

	var seen entity.RuntimeConfigSnapshot
	provider.OnChange(func(snapshot entity.RuntimeConfigSnapshot) {
		seen = snapshot
	})

	manager.emit(nil)
	want := RuntimeConfigSnapshotFromConfig(initial)

	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("callback snapshot=%#v, want initial fallback %#v", seen, want)
	}
}

func TestRuntimeConfigProviderOnChangeNilCallbackDoesNotRegister(t *testing.T) {
	manager := &configManagerState{current: config.DefaultConfig()}
	provider := NewRuntimeConfigProvider(config.DefaultConfig(), newMockConfigManager(t, manager))

	provider.OnChange(nil)

	if len(manager.callbacks) != 0 {
		t.Fatalf("callbacks=%d, want 0", len(manager.callbacks))
	}
}

func TestRuntimeConfigProviderWatchDelegatesToManager(t *testing.T) {
	manager := &configManagerState{}
	provider := NewRuntimeConfigProvider(config.DefaultConfig(), newMockConfigManager(t, manager))

	if err := provider.Watch(); err != nil {
		t.Fatalf("Watch returned error: %v", err)
	}
	if manager.watchCalls != 1 {
		t.Fatalf("watchCalls=%d, want 1", manager.watchCalls)
	}
}

func TestRuntimeConfigProviderCurrentReturnsMapClone(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SearchShortcuts = map[string]config.SearchShortcut{
		"gh": {URL: "https://github.com/search?q=%s", Description: "GitHub"},
	}
	provider := NewRuntimeConfigProvider(cfg, newMockConfigManager(t, &configManagerState{current: cfg}))

	first := provider.Current()
	first.UI.SearchShortcuts["gh"] = entity.RuntimeSearchShortcut{URL: "mutated"}
	second := provider.Current()

	if second.UI.SearchShortcuts["gh"].URL == "mutated" {
		t.Fatal("Current must return a snapshot clone so callers cannot mutate provider state")
	}
}

func TestRuntimeConfigProviderCurrentReturnsNestedConfigClone(t *testing.T) {
	cfg := runtimeConfigWithNestedMutableFields()
	provider := NewRuntimeConfigProvider(cfg, newMockConfigManager(t, &configManagerState{current: cfg}))

	first := provider.Current()
	mutateNestedRuntimeConfigSnapshot(first)
	second := provider.Current()

	assertNestedRuntimeConfigSnapshotUnchanged(t, second)
}

func TestRuntimeConfigProviderOnChangePassesNestedConfigClone(t *testing.T) {
	manager := &configManagerState{}
	provider := NewRuntimeConfigProvider(config.DefaultConfig(), newMockConfigManager(t, manager))

	provider.OnChange(func(snapshot entity.RuntimeConfigSnapshot) {
		mutateNestedRuntimeConfigSnapshot(snapshot)
	})

	manager.emit(runtimeConfigWithNestedMutableFields())

	assertNestedRuntimeConfigSnapshotUnchanged(t, provider.Current())
}

func TestEngineSettingsPayloadFromNilConfigReturnsZeroPayload(t *testing.T) {
	got := EngineSettingsPayloadFromConfig(nil)
	if got != (entity.EngineSettingsPayload{}) {
		t.Fatalf("payload=%#v, want zero value", got)
	}
}

func runtimeConfigWithNestedMutableFields() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Workspace.PaneMode.Actions = map[string]entity.ActionBinding{
		"split-right": {Keys: []string{"r"}, Desc: "Split right"},
	}
	cfg.Workspace.TabMode.Actions = map[string]entity.ActionBinding{
		"next-tab": {Keys: []string{"l"}, Desc: "Next tab"},
	}
	cfg.Workspace.ResizeMode.Actions = map[string]entity.ActionBinding{
		"grow-right": {Keys: []string{"right"}, Desc: "Grow right"},
	}
	cfg.Workspace.Shortcuts.Actions = map[string]entity.ActionBinding{
		"toggle-history-systemview": {Keys: []string{"ctrl+h"}, Desc: "History"},
	}
	cfg.Workspace.FloatingPane.Profiles = map[string]entity.FloatingPaneProfile{
		"docs": {Keys: []string{"alt+d"}, URL: "https://docs.example", Desc: "Docs"},
	}
	cfg.Session.SessionMode.Actions = map[string]entity.ActionBinding{
		"session-manager": {Keys: []string{"s"}, Desc: "Session manager"},
	}
	return cfg
}

func mutateNestedRuntimeConfigSnapshot(snapshot entity.RuntimeConfigSnapshot) {
	mutateActionBinding(snapshot.UI.Workspace.PaneMode.Actions, "split-right", "mutated-pane")
	snapshot.UI.Workspace.PaneMode.Actions["added-pane"] = entity.ActionBinding{Keys: []string{"added"}}

	mutateActionBinding(snapshot.UI.Workspace.TabMode.Actions, "next-tab", "mutated-tab")
	snapshot.UI.Workspace.TabMode.Actions["added-tab"] = entity.ActionBinding{Keys: []string{"added"}}

	mutateActionBinding(snapshot.UI.Workspace.ResizeMode.Actions, "grow-right", "mutated-resize")
	snapshot.UI.Workspace.ResizeMode.Actions["added-resize"] = entity.ActionBinding{Keys: []string{"added"}}

	mutateActionBinding(snapshot.UI.Workspace.Shortcuts.Actions, "toggle-history-systemview", "mutated-shortcut")
	snapshot.UI.Workspace.Shortcuts.Actions["added-shortcut"] = entity.ActionBinding{Keys: []string{"added"}}

	profile := snapshot.UI.Workspace.FloatingPane.Profiles["docs"]
	profile.Keys[0] = "mutated-floating"
	snapshot.UI.Workspace.FloatingPane.Profiles["docs"] = profile
	snapshot.UI.Workspace.FloatingPane.Profiles["added-floating"] = entity.FloatingPaneProfile{Keys: []string{"added"}}

	mutateActionBinding(snapshot.UI.Session.SessionMode.Actions, "session-manager", "mutated-session")
	snapshot.UI.Session.SessionMode.Actions["added-session"] = entity.ActionBinding{Keys: []string{"added"}}
}

func mutateActionBinding(actions map[string]entity.ActionBinding, action, key string) {
	binding := actions[action]
	binding.Keys[0] = key
	actions[action] = binding
}

func assertNestedRuntimeConfigSnapshotUnchanged(t *testing.T, snapshot entity.RuntimeConfigSnapshot) {
	t.Helper()

	assertActionBinding(t, snapshot.UI.Workspace.PaneMode.Actions, "split-right", "r")
	assertMapEntryAbsent(t, snapshot.UI.Workspace.PaneMode.Actions, "added-pane")

	assertActionBinding(t, snapshot.UI.Workspace.TabMode.Actions, "next-tab", "l")
	assertMapEntryAbsent(t, snapshot.UI.Workspace.TabMode.Actions, "added-tab")

	assertActionBinding(t, snapshot.UI.Workspace.ResizeMode.Actions, "grow-right", "right")
	assertMapEntryAbsent(t, snapshot.UI.Workspace.ResizeMode.Actions, "added-resize")

	assertActionBinding(t, snapshot.UI.Workspace.Shortcuts.Actions, "toggle-history-systemview", "ctrl+h")
	assertMapEntryAbsent(t, snapshot.UI.Workspace.Shortcuts.Actions, "added-shortcut")

	profile := snapshot.UI.Workspace.FloatingPane.Profiles["docs"]
	if got := profile.Keys[0]; got != "alt+d" {
		t.Fatalf("floating profile key=%q, want %q", got, "alt+d")
	}
	if _, ok := snapshot.UI.Workspace.FloatingPane.Profiles["added-floating"]; ok {
		t.Fatal("floating pane profiles must not include entries added through a returned snapshot")
	}

	assertActionBinding(t, snapshot.UI.Session.SessionMode.Actions, "session-manager", "s")
	assertMapEntryAbsent(t, snapshot.UI.Session.SessionMode.Actions, "added-session")
}

func assertActionBinding(t *testing.T, actions map[string]entity.ActionBinding, action, wantKey string) {
	t.Helper()

	binding, ok := actions[action]
	if !ok {
		t.Fatalf("action %q missing", action)
	}
	if got := binding.Keys[0]; got != wantKey {
		t.Fatalf("%s key=%q, want %q", action, got, wantKey)
	}
}

func assertMapEntryAbsent(t *testing.T, actions map[string]entity.ActionBinding, action string) {
	t.Helper()

	if _, ok := actions[action]; ok {
		t.Fatalf("actions must not include entry %q added through a returned snapshot", action)
	}
}

func assertNestedSourceConfigUnchanged(t *testing.T, cfg *config.Config) {
	t.Helper()

	assertActionBinding(t, cfg.Workspace.PaneMode.Actions, "split-right", "r")
	assertMapEntryAbsent(t, cfg.Workspace.PaneMode.Actions, "added-pane")

	assertActionBinding(t, cfg.Workspace.TabMode.Actions, "next-tab", "l")
	assertMapEntryAbsent(t, cfg.Workspace.TabMode.Actions, "added-tab")

	assertActionBinding(t, cfg.Workspace.ResizeMode.Actions, "grow-right", "right")
	assertMapEntryAbsent(t, cfg.Workspace.ResizeMode.Actions, "added-resize")

	assertActionBinding(t, cfg.Workspace.Shortcuts.Actions, "toggle-history-systemview", "ctrl+h")
	assertMapEntryAbsent(t, cfg.Workspace.Shortcuts.Actions, "added-shortcut")

	profile := cfg.Workspace.FloatingPane.Profiles["docs"]
	if got := profile.Keys[0]; got != "alt+d" {
		t.Fatalf("floating profile key=%q, want %q", got, "alt+d")
	}
	if _, ok := cfg.Workspace.FloatingPane.Profiles["added-floating"]; ok {
		t.Fatal("floating pane profiles must not include entries added through a returned snapshot")
	}

	assertActionBinding(t, cfg.Session.SessionMode.Actions, "session-manager", "s")
	assertMapEntryAbsent(t, cfg.Session.SessionMode.Actions, "added-session")
}

func TestRuntimeConfigSnapshotFromConfigMapsUIRuntimeFields(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DefaultUIScale = 1.4
	cfg.SidebarWidth = 333
	cfg.Downloads.Path = "/tmp/downloads"
	cfg.DefaultSearchEngine = "https://search.example/?q=%s"
	cfg.SearchShortcuts = map[string]config.SearchShortcut{
		"gh": {URL: "https://github.com/search?q=%s", Description: "GitHub"},
	}
	cfg.Workspace.NewPaneURL = "about:blank"
	cfg.Session.SnapshotIntervalMs = 7000
	cfg.Clipboard.AutoCopyOnSelection = true
	cfg.Omnibox.AutoOpenOnNewPane = true
	cfg.Update.NotifyOnNewSettings = true

	got := RuntimeConfigSnapshotFromConfig(cfg)

	if got.EngineSettings != EngineSettingsPayloadFromConfig(cfg) {
		t.Fatalf("EngineSettings=%#v, want %#v", got.EngineSettings, EngineSettingsPayloadFromConfig(cfg))
	}
	if got.UI.DefaultUIScale != 1.4 ||
		got.UI.SidebarWidth != 333 ||
		got.UI.Downloads.Path != "/tmp/downloads" ||
		got.UI.DefaultSearchEngine != "https://search.example/?q=%s" ||
		got.UI.Workspace.NewPaneURL != "about:blank" ||
		got.UI.Session.SnapshotIntervalMs != 7000 ||
		!got.UI.Clipboard.AutoCopyOnSelection ||
		!got.UI.Omnibox.AutoOpenOnNewPane ||
		!got.UI.Update.NotifyOnNewSettings {
		t.Fatalf("snapshot not mapped: %#v", got.UI)
	}
	if got.UI.SearchShortcuts["gh"].URL != "https://github.com/search?q=%s" {
		t.Fatalf("search shortcut not mapped: %#v", got.UI.SearchShortcuts)
	}
	got.UI.SearchShortcuts["gh"] = entity.RuntimeSearchShortcut{URL: "mutated"}
	if cfg.SearchShortcuts["gh"].URL == "mutated" {
		t.Fatal("snapshot must deep-copy search shortcut map")
	}
}

func TestRuntimeConfigSnapshotFromConfigDetachesNestedMutableFields(t *testing.T) {
	cfg := runtimeConfigWithNestedMutableFields()
	snapshot := RuntimeConfigSnapshotFromConfig(cfg)

	mutateNestedRuntimeConfigSnapshot(snapshot)

	assertNestedSourceConfigUnchanged(t, cfg)
}

func TestRuntimeConfigSnapshotFromNilConfigReturnsZeroSnapshot(t *testing.T) {
	got := RuntimeConfigSnapshotFromConfig(nil)
	if !reflect.DeepEqual(got, entity.RuntimeConfigSnapshot{}) {
		t.Fatalf("snapshot=%#v, want zero value", got)
	}
}

func TestEngineSettingsPayloadFromConfigMapsHardwareDecodingModes(t *testing.T) {
	tests := []struct {
		name string
		mode config.HardwareDecodingMode
		want entity.EngineHardwareDecodingMode
	}{
		{
			name: "auto",
			mode: config.HardwareDecodingAuto,
			want: entity.EngineHardwareDecodingAuto,
		},
		{
			name: "force",
			mode: config.HardwareDecodingForce,
			want: entity.EngineHardwareDecodingForce,
		},
		{
			name: "disable",
			mode: config.HardwareDecodingDisable,
			want: entity.EngineHardwareDecodingDisable,
		},
		{
			name: "unknown",
			mode: config.HardwareDecodingMode("surprise"),
			want: entity.EngineHardwareDecodingAuto,
		},
		{
			name: "zero value",
			mode: "",
			want: entity.EngineHardwareDecodingAuto,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Media.HardwareDecodingMode = tt.mode

			got := EngineSettingsPayloadFromConfig(cfg)

			if got.WebContent.HardwareDecoding != tt.want {
				t.Fatalf("HardwareDecoding=%q, want %q", got.WebContent.HardwareDecoding, tt.want)
			}
		})
	}
}
