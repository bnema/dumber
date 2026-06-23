package bootstrap

import (
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

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
			Workspace:           cfg.Workspace,
			Session:             cfg.Session,
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
