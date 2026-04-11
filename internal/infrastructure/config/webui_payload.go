package config

import "github.com/bnema/dumber/internal/application/port"

// BuildSystemviewConfigPayload projects the full config into the stable DTO used by
// the settings page across engines.
func BuildSystemviewConfigPayload(cfg *Config, hw *port.HardwareInfo) port.SystemviewConfigPayload {
	if cfg == nil {
		return port.SystemviewConfigPayload{}
	}

	perfCfg := PerformanceConfigFromEngine(&cfg.Engine)
	resolved := ResolvePerformanceProfile(&perfCfg, hw)

	return port.SystemviewConfigPayload{
		Appearance:          buildSystemviewAppearancePayload(cfg.Appearance),
		DefaultUIScale:      cfg.DefaultUIScale,
		DefaultSearchEngine: cfg.DefaultSearchEngine,
		SearchShortcuts:     buildSystemviewSearchShortcutsPayload(cfg.SearchShortcuts),
		EngineType:          cfg.Engine.ResolveEngineType(),
		Performance: port.SystemviewPerformancePayload{
			Profile: string(cfg.Engine.Profile),
			Custom: port.SystemviewCustomPerformancePayload{
				SkiaCPUThreads:         cfg.Engine.WebKit.SkiaCPUPaintingThreads,
				SkiaGPUThreads:         cfg.Engine.WebKit.SkiaGPUPaintingThreads,
				WebProcessMemoryMB:     cfg.Engine.WebKit.WebProcessMemoryLimitMB,
				NetworkProcessMemoryMB: cfg.Engine.WebKit.NetworkProcessMemoryLimitMB,
				WebViewPoolPrewarm:     cfg.Engine.PoolPrewarmCount,
			},
			Resolved: port.SystemviewResolvedPerformancePayload{
				SkiaCPUThreads:         resolved.SkiaCPUPaintingThreads,
				SkiaGPUThreads:         resolved.SkiaGPUPaintingThreads,
				WebProcessMemoryMB:     resolved.WebProcessMemoryLimitMB,
				NetworkProcessMemoryMB: resolved.NetworkProcessMemoryLimitMB,
				WebViewPoolPrewarm:     resolved.WebViewPoolPrewarmCount,
				ConservativeThreshold:  resolved.WebProcessMemoryConservativeThreshold,
				StrictThreshold:        resolved.WebProcessMemoryStrictThreshold,
			},
			Hardware: buildSystemviewHardwarePayload(hw),
		},
	}
}

func buildSystemviewAppearancePayload(appearance AppearanceConfig) port.WebUIAppearanceConfig {
	return port.WebUIAppearanceConfig{
		SansFont:        appearance.SansFont,
		SerifFont:       appearance.SerifFont,
		MonospaceFont:   appearance.MonospaceFont,
		DefaultFontSize: appearance.DefaultFontSize,
		ColorScheme:     appearance.ColorScheme,
		LightPalette: port.ColorPalette{
			Background:     appearance.LightPalette.Background,
			Surface:        appearance.LightPalette.Surface,
			SurfaceVariant: appearance.LightPalette.SurfaceVariant,
			Text:           appearance.LightPalette.Text,
			Muted:          appearance.LightPalette.Muted,
			Accent:         appearance.LightPalette.Accent,
			Border:         appearance.LightPalette.Border,
		},
		DarkPalette: port.ColorPalette{
			Background:     appearance.DarkPalette.Background,
			Surface:        appearance.DarkPalette.Surface,
			SurfaceVariant: appearance.DarkPalette.SurfaceVariant,
			Text:           appearance.DarkPalette.Text,
			Muted:          appearance.DarkPalette.Muted,
			Accent:         appearance.DarkPalette.Accent,
			Border:         appearance.DarkPalette.Border,
		},
	}
}

func buildSystemviewSearchShortcutsPayload(shortcuts map[string]SearchShortcut) map[string]port.SearchShortcut {
	if len(shortcuts) == 0 {
		return nil
	}

	result := make(map[string]port.SearchShortcut, len(shortcuts))
	for key, shortcut := range shortcuts {
		result[key] = port.SearchShortcut{URL: shortcut.URL, Description: shortcut.Description}
	}
	return result
}

func buildSystemviewHardwarePayload(hw *port.HardwareInfo) port.SystemviewHardwarePayload {
	if hw == nil {
		return port.SystemviewHardwarePayload{}
	}
	return port.SystemviewHardwarePayload{
		CPUCores:   hw.CPUCores,
		CPUThreads: hw.CPUThreads,
		TotalRAMMB: hw.TotalRAMMB(),
		GPUVendor:  string(hw.GPUVendor),
		GPUName:    hw.GPUName,
		VRAMMB:     hw.VRAMMB(),
	}
}
