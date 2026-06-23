package bootstrap

import (
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

type configChangeManager interface {
	Get() *config.Config
	Watch() error
	OnConfigChange(func(*config.Config))
}

type runtimeConfigProvider struct {
	fallback port.RuntimeConfigSnapshot
	manager  configChangeManager
}

func NewRuntimeConfigProvider(cfg *config.Config, manager configChangeManager) port.RuntimeConfigProvider {
	return &runtimeConfigProvider{
		fallback: RuntimeConfigSnapshotFromConfig(cfg),
		manager:  manager,
	}
}

func (p *runtimeConfigProvider) Current() port.RuntimeConfigSnapshot {
	if p == nil {
		return port.RuntimeConfigSnapshot{}
	}
	if p.manager == nil {
		return cloneRuntimeConfigSnapshot(p.fallback)
	}
	return RuntimeConfigSnapshotFromConfig(p.manager.Get())
}

func (p *runtimeConfigProvider) Watch() error {
	if p == nil || p.manager == nil {
		return nil
	}
	return p.manager.Watch()
}

func (p *runtimeConfigProvider) OnChange(callback func(port.RuntimeConfigSnapshot)) {
	if p == nil || p.manager == nil || callback == nil {
		return
	}
	p.manager.OnConfigChange(func(cfg *config.Config) {
		callback(RuntimeConfigSnapshotFromConfig(cfg))
	})
}

func EngineSettingsPayloadFromConfig(cfg *config.Config) port.EngineSettingsPayload {
	if cfg == nil {
		return port.EngineSettingsPayload{}
	}
	return port.EngineSettingsPayload{
		DefaultUIScale: cfg.DefaultUIScale,
		WebContent: port.EngineWebContentSettingsPayload{
			SansFont:                  cfg.Appearance.SansFont,
			SerifFont:                 cfg.Appearance.SerifFont,
			MonospaceFont:             cfg.Appearance.MonospaceFont,
			DefaultFontSize:           cfg.Appearance.DefaultFontSize,
			EnableDevTools:            cfg.Debug.EnableDevTools,
			CaptureConsole:            cfg.Logging.CaptureConsole,
			DrawCompositingIndicators: cfg.Engine.WebKit.DrawCompositingIndicators,
			HardwareDecoding:          engineHardwareDecodingModeFromConfig(cfg.Media.HardwareDecodingMode),
		},
	}
}

func RuntimeConfigSnapshotFromConfig(cfg *config.Config) port.RuntimeConfigSnapshot {
	if cfg == nil {
		return port.RuntimeConfigSnapshot{}
	}
	return port.RuntimeConfigSnapshot{
		EngineSettings: EngineSettingsPayloadFromConfig(cfg),
		UI: port.RuntimeUIConfig{
			DefaultUIScale:      cfg.DefaultUIScale,
			SidebarWidth:        cfg.SidebarWidth,
			Appearance:          cfg.Appearance,
			Workspace:           cloneWorkspaceConfig(cfg.Workspace),
			Session:             cloneSessionConfig(cfg.Session),
			Clipboard:           port.RuntimeClipboardConfig{AutoCopyOnSelection: cfg.Clipboard.AutoCopyOnSelection},
			SearchShortcuts:     runtimeSearchShortcutsFromConfig(cfg.SearchShortcuts),
			DefaultSearchEngine: cfg.DefaultSearchEngine,
			Omnibox: port.RuntimeOmniboxConfig{
				InitialBehavior:   cfg.Omnibox.InitialBehavior,
				MostVisitedDays:   cfg.Omnibox.MostVisitedDays,
				AutoOpenOnNewPane: cfg.Omnibox.AutoOpenOnNewPane,
			},
			Update: port.RuntimeUpdateConfig{
				EnableOnStartup:     cfg.Update.EnableOnStartup,
				AutoDownload:        cfg.Update.AutoDownload,
				NotifyOnNewSettings: cfg.Update.NotifyOnNewSettings,
			},
			Downloads: port.RuntimeDownloadsConfig{Path: cfg.Downloads.Path},
		},
	}
}

func runtimeSearchShortcutsFromConfig(in map[string]config.SearchShortcut) map[string]port.RuntimeSearchShortcut {
	out := make(map[string]port.RuntimeSearchShortcut, len(in))
	for key, shortcut := range in {
		out[key] = port.RuntimeSearchShortcut{
			URL:         shortcut.URL,
			Description: shortcut.Description,
		}
	}
	return out
}

func cloneRuntimeConfigSnapshot(snapshot port.RuntimeConfigSnapshot) port.RuntimeConfigSnapshot {
	snapshot.UI.SearchShortcuts = cloneRuntimeSearchShortcuts(snapshot.UI.SearchShortcuts)
	snapshot.UI.Workspace = cloneWorkspaceConfig(snapshot.UI.Workspace)
	snapshot.UI.Session = cloneSessionConfig(snapshot.UI.Session)
	return snapshot
}

func cloneRuntimeSearchShortcuts(in map[string]port.RuntimeSearchShortcut) map[string]port.RuntimeSearchShortcut {
	if in == nil {
		return nil
	}
	out := make(map[string]port.RuntimeSearchShortcut, len(in))
	for key, shortcut := range in {
		out[key] = shortcut
	}
	return out
}

func cloneWorkspaceConfig(in entity.WorkspaceConfig) entity.WorkspaceConfig {
	in.PaneMode.Actions = cloneActionBindings(in.PaneMode.Actions)
	in.TabMode.Actions = cloneActionBindings(in.TabMode.Actions)
	in.ResizeMode.Actions = cloneActionBindings(in.ResizeMode.Actions)
	in.Shortcuts.Actions = cloneActionBindings(in.Shortcuts.Actions)
	in.FloatingPane.Profiles = cloneFloatingPaneProfiles(in.FloatingPane.Profiles)
	return in
}

func cloneSessionConfig(in entity.SessionConfig) entity.SessionConfig {
	in.SessionMode.Actions = cloneActionBindings(in.SessionMode.Actions)
	return in
}

func cloneActionBindings(in map[string]entity.ActionBinding) map[string]entity.ActionBinding {
	if in == nil {
		return nil
	}
	out := make(map[string]entity.ActionBinding, len(in))
	for action, binding := range in {
		binding.Keys = cloneStringSlice(binding.Keys)
		out[action] = binding
	}
	return out
}

func cloneFloatingPaneProfiles(in map[string]entity.FloatingPaneProfile) map[string]entity.FloatingPaneProfile {
	if in == nil {
		return nil
	}
	out := make(map[string]entity.FloatingPaneProfile, len(in))
	for name, profile := range in {
		profile.Keys = cloneStringSlice(profile.Keys)
		out[name] = profile
	}
	return out
}

func cloneStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func engineHardwareDecodingModeFromConfig(mode config.HardwareDecodingMode) port.EngineHardwareDecodingMode {
	switch mode {
	case config.HardwareDecodingForce:
		return port.EngineHardwareDecodingForce
	case config.HardwareDecodingDisable:
		return port.EngineHardwareDecodingDisable
	default:
		return port.EngineHardwareDecodingAuto
	}
}
