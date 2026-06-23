package bootstrap

import (
	"reflect"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

type fakeConfigManager struct {
	watchCalls int
	callbacks  []func(*config.Config)
}

func (f *fakeConfigManager) Watch() error {
	f.watchCalls++
	return nil
}

func (f *fakeConfigManager) OnConfigChange(callback func(*config.Config)) {
	f.callbacks = append(f.callbacks, callback)
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
	if got.WebContent.HardwareDecoding != port.EngineHardwareDecodingForce {
		t.Fatalf("HardwareDecoding=%q, want %q", got.WebContent.HardwareDecoding, port.EngineHardwareDecodingForce)
	}
}

func TestRuntimeConfigProviderUpdatesSnapshotBeforeCallback(t *testing.T) {
	initial := config.DefaultConfig()
	initial.DefaultUIScale = 1
	manager := &fakeConfigManager{}
	provider := NewRuntimeConfigProvider(initial, manager)

	var seen port.RuntimeConfigSnapshot
	provider.OnChange(func(snapshot port.RuntimeConfigSnapshot) {
		seen = snapshot
	})

	next := config.DefaultConfig()
	next.DefaultUIScale = 1.8
	manager.callbacks[0](next)

	if got := provider.Current().UI.DefaultUIScale; got != 1.8 {
		t.Fatalf("Current DefaultUIScale=%v, want 1.8", got)
	}
	if seen.UI.DefaultUIScale != 1.8 {
		t.Fatalf("callback DefaultUIScale=%v, want 1.8", seen.UI.DefaultUIScale)
	}
}

func TestRuntimeConfigProviderWatchDelegatesToManager(t *testing.T) {
	manager := &fakeConfigManager{}
	provider := NewRuntimeConfigProvider(config.DefaultConfig(), manager)

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
	provider := NewRuntimeConfigProvider(cfg, &fakeConfigManager{})

	first := provider.Current()
	first.UI.SearchShortcuts["gh"] = port.RuntimeSearchShortcut{URL: "mutated"}
	second := provider.Current()

	if second.UI.SearchShortcuts["gh"].URL == "mutated" {
		t.Fatal("Current must return a snapshot clone so callers cannot mutate provider state")
	}
}

func TestEngineSettingsPayloadFromNilConfigReturnsZeroPayload(t *testing.T) {
	got := EngineSettingsPayloadFromConfig(nil)
	if got != (port.EngineSettingsPayload{}) {
		t.Fatalf("payload=%#v, want zero value", got)
	}
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
	got.UI.SearchShortcuts["gh"] = port.RuntimeSearchShortcut{URL: "mutated"}
	if cfg.SearchShortcuts["gh"].URL == "mutated" {
		t.Fatal("snapshot must deep-copy search shortcut map")
	}
}

func TestRuntimeConfigSnapshotFromNilConfigReturnsZeroSnapshot(t *testing.T) {
	got := RuntimeConfigSnapshotFromConfig(nil)
	if !reflect.DeepEqual(got, port.RuntimeConfigSnapshot{}) {
		t.Fatalf("snapshot=%#v, want zero value", got)
	}
}

func TestEngineSettingsPayloadFromConfigMapsHardwareDecodingModes(t *testing.T) {
	tests := []struct {
		name string
		mode config.HardwareDecodingMode
		want port.EngineHardwareDecodingMode
	}{
		{
			name: "auto",
			mode: config.HardwareDecodingAuto,
			want: port.EngineHardwareDecodingAuto,
		},
		{
			name: "force",
			mode: config.HardwareDecodingForce,
			want: port.EngineHardwareDecodingForce,
		},
		{
			name: "disable",
			mode: config.HardwareDecodingDisable,
			want: port.EngineHardwareDecodingDisable,
		},
		{
			name: "unknown",
			mode: config.HardwareDecodingMode("surprise"),
			want: port.EngineHardwareDecodingAuto,
		},
		{
			name: "zero value",
			mode: "",
			want: port.EngineHardwareDecodingAuto,
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
