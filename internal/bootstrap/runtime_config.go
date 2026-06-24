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
	fallback entity.RuntimeConfigSnapshot
	manager  configChangeManager
}

func NewRuntimeConfigProvider(cfg *config.Config, manager configChangeManager) port.RuntimeConfigProvider {
	return &runtimeConfigProvider{
		fallback: RuntimeConfigSnapshotFromConfig(cfg),
		manager:  manager,
	}
}

func (p *runtimeConfigProvider) Current() entity.RuntimeConfigSnapshot {
	if p == nil {
		return entity.RuntimeConfigSnapshot{}
	}
	if p.manager == nil {
		return cloneRuntimeConfigSnapshot(p.fallback)
	}
	cfg := p.manager.Get()
	if cfg == nil {
		return cloneRuntimeConfigSnapshot(p.fallback)
	}
	return RuntimeConfigSnapshotFromConfig(cfg)
}

func (p *runtimeConfigProvider) Watch() error {
	if p == nil || p.manager == nil {
		return nil
	}
	return p.manager.Watch()
}

func (p *runtimeConfigProvider) OnChange(callback func(entity.RuntimeConfigSnapshot)) {
	if p == nil || p.manager == nil || callback == nil {
		return
	}
	p.manager.OnConfigChange(func(cfg *config.Config) {
		if cfg == nil {
			callback(cloneRuntimeConfigSnapshot(p.fallback))
			return
		}
		callback(RuntimeConfigSnapshotFromConfig(cfg))
	})
}

func EngineSettingsPayloadFromConfig(cfg *config.Config) entity.EngineSettingsPayload {
	if cfg == nil {
		return entity.EngineSettingsPayload{}
	}
	return entity.EngineSettingsPayload{
		DefaultUIScale: cfg.DefaultUIScale,
		WebContent: entity.EngineWebContentSettingsPayload{
			SansFont:                  cfg.Appearance.SansFont,
			SerifFont:                 cfg.Appearance.SerifFont,
			MonospaceFont:             cfg.Appearance.MonospaceFont,
			DefaultFontSize:           cfg.Appearance.DefaultFontSize,
			EnableDevTools:            cfg.Debug.EnableDevTools,
			CaptureConsole:            cfg.Logging.CaptureConsole,
			DrawCompositingIndicators: cfg.Engine.WebKit.DrawCompositingIndicators,
			HardwareDecoding:          engineHardwareDecodingModeFromConfig(cfg.Media.HardwareDecodingMode),
			AutoCopyOnSelection:       cfg.Clipboard.AutoCopyOnSelection,
		},
	}
}

func RuntimeConfigSnapshotFromConfig(cfg *config.Config) entity.RuntimeConfigSnapshot {
	if cfg == nil {
		return entity.RuntimeConfigSnapshot{}
	}
	return entity.RuntimeConfigSnapshot{
		EngineSettings: EngineSettingsPayloadFromConfig(cfg),
		UI: entity.RuntimeUIConfig{
			DefaultUIScale:      cfg.DefaultUIScale,
			SidebarWidth:        cfg.SidebarWidth,
			Appearance:          cfg.Appearance,
			Workspace:           cloneWorkspaceConfig(cfg.Workspace),
			Session:             cloneSessionConfig(cfg.Session),
			Clipboard:           entity.RuntimeClipboardConfig{AutoCopyOnSelection: cfg.Clipboard.AutoCopyOnSelection},
			SearchShortcuts:     runtimeSearchShortcutsFromConfig(cfg.SearchShortcuts),
			DefaultSearchEngine: cfg.DefaultSearchEngine,
			Omnibox: entity.RuntimeOmniboxConfig{
				InitialBehavior:   cfg.Omnibox.InitialBehavior,
				MostVisitedDays:   cfg.Omnibox.MostVisitedDays,
				AutoOpenOnNewPane: cfg.Omnibox.AutoOpenOnNewPane,
			},
			Update: entity.RuntimeUpdateConfig{
				EnableOnStartup:     cfg.Update.EnableOnStartup,
				AutoDownload:        cfg.Update.AutoDownload,
				NotifyOnNewSettings: cfg.Update.NotifyOnNewSettings,
			},
			Downloads: entity.RuntimeDownloadsConfig{Path: cfg.Downloads.Path},
		},
	}
}

func runtimeSearchShortcutsFromConfig(in map[string]config.SearchShortcut) map[string]entity.RuntimeSearchShortcut {
	out := make(map[string]entity.RuntimeSearchShortcut, len(in))
	for key, shortcut := range in {
		out[key] = entity.RuntimeSearchShortcut{
			URL:         shortcut.URL,
			Description: shortcut.Description,
		}
	}
	return out
}

func cloneRuntimeConfigSnapshot(snapshot entity.RuntimeConfigSnapshot) entity.RuntimeConfigSnapshot {
	snapshot.UI.SearchShortcuts = cloneRuntimeSearchShortcuts(snapshot.UI.SearchShortcuts)
	snapshot.UI.Workspace = cloneWorkspaceConfig(snapshot.UI.Workspace)
	snapshot.UI.Session = cloneSessionConfig(snapshot.UI.Session)
	return snapshot
}

func cloneRuntimeSearchShortcuts(in map[string]entity.RuntimeSearchShortcut) map[string]entity.RuntimeSearchShortcut {
	if in == nil {
		return nil
	}
	out := make(map[string]entity.RuntimeSearchShortcut, len(in))
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

func engineHardwareDecodingModeFromConfig(mode config.HardwareDecodingMode) entity.EngineHardwareDecodingMode {
	switch mode {
	case config.HardwareDecodingForce:
		return entity.EngineHardwareDecodingForce
	case config.HardwareDecodingDisable:
		return entity.EngineHardwareDecodingDisable
	default:
		return entity.EngineHardwareDecodingAuto
	}
}
