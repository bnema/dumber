package config

import "github.com/bnema/dumber/internal/application/port"

// BuildSystemviewConfigPayload projects the full config into the stable DTO used by
// the settings page across engines.
func BuildSystemviewConfigPayload(cfg *Config, hw *port.HardwareInfo) port.SystemviewConfigPayload {
	payload := port.SystemviewConfigPayload{
		SearchShortcuts: map[string]port.SearchShortcut{},
	}
	if cfg == nil {
		return payload
	}

	perfCfg := PerformanceConfigFromEngine(&cfg.Engine)
	resolved := ResolvePerformanceProfile(&perfCfg, hw)

	payload.Appearance = buildSystemviewAppearancePayload(cfg.Appearance)
	payload.DefaultUIScale = cfg.DefaultUIScale
	payload.DefaultSearchEngine = cfg.DefaultSearchEngine
	payload.SearchShortcuts = buildSystemviewSearchShortcutsPayload(cfg.SearchShortcuts)
	payload.EngineType = cfg.Engine.ResolveEngineType()
	payload.Performance = port.SystemviewPerformancePayload{
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
	}

	return payload
}

func buildSystemviewAppearancePayload(appearance AppearanceConfig) port.WebUIAppearanceConfig {
	defaults := DefaultConfig().Appearance
	return port.WebUIAppearanceConfig{
		SansFont:        appearance.SansFont,
		SerifFont:       appearance.SerifFont,
		MonospaceFont:   appearance.MonospaceFont,
		DefaultFontSize: appearance.DefaultFontSize,
		ColorScheme:     appearance.ColorScheme,
		LightPalette:    buildSystemviewPalettePayload(appearance.LightPalette, defaults.LightPalette),
		DarkPalette:     buildSystemviewPalettePayload(appearance.DarkPalette, defaults.DarkPalette),
	}
}

func buildSystemviewPalettePayload(palette, fallback ColorPalette) port.ColorPalette {
	return port.ColorPalette{
		Background:     nonEmptyColor(palette.Background, fallback.Background),
		Surface:        nonEmptyColor(palette.Surface, fallback.Surface),
		SurfaceVariant: nonEmptyColor(palette.SurfaceVariant, fallback.SurfaceVariant),
		Text:           nonEmptyColor(palette.Text, fallback.Text),
		Muted:          nonEmptyColor(palette.Muted, fallback.Muted),
		Accent:         nonEmptyColor(palette.Accent, fallback.Accent),
		Border:         nonEmptyColor(palette.Border, fallback.Border),
	}
}

func nonEmptyColor(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func buildSystemviewSearchShortcutsPayload(shortcuts map[string]SearchShortcut) map[string]port.SearchShortcut {
	if len(shortcuts) == 0 {
		return map[string]port.SearchShortcut{}
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
