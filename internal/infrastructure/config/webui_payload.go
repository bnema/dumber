package config

import (
	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
)

// BuildSystemviewConfigPayload projects the full config into the stable DTO used by
// the settings page across engines.
func BuildSystemviewConfigPayload(cfg *Config, hw *port.HardwareInfo) dto.SystemviewConfigPayload {
	payload := dto.SystemviewConfigPayload{
		SearchShortcuts: map[string]dto.SearchShortcut{},
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
	payload.Performance = dto.SystemviewPerformancePayload{
		Profile: string(cfg.Engine.Profile),
		Custom: dto.SystemviewCustomPerformancePayload{
			SkiaCPUThreads:         cfg.Engine.WebKit.SkiaCPUPaintingThreads,
			SkiaGPUThreads:         cfg.Engine.WebKit.SkiaGPUPaintingThreads,
			WebProcessMemoryMB:     cfg.Engine.WebKit.WebProcessMemoryLimitMB,
			NetworkProcessMemoryMB: cfg.Engine.WebKit.NetworkProcessMemoryLimitMB,
			WebViewPoolPrewarm:     cfg.Engine.PoolPrewarmCount,
		},
		Resolved: dto.SystemviewResolvedPerformancePayload{
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

func buildSystemviewAppearancePayload(appearance AppearanceConfig) dto.WebUIAppearanceConfig {
	defaults := DefaultConfig().Appearance
	return dto.WebUIAppearanceConfig{
		SansFont:        appearance.SansFont,
		SerifFont:       appearance.SerifFont,
		MonospaceFont:   appearance.MonospaceFont,
		GtkFont:         appearance.GtkFont,
		DefaultFontSize: appearance.DefaultFontSize,
		ColorScheme:     appearance.ColorScheme,
		LightPalette:    buildSystemviewPalettePayload(appearance.LightPalette, defaults.LightPalette),
		DarkPalette:     buildSystemviewPalettePayload(appearance.DarkPalette, defaults.DarkPalette),
		ExternalTheme: dto.WebUIExternalThemeConfig{
			Enabled:  appearance.ExternalTheme.Enabled,
			Provider: appearance.ExternalTheme.Provider,
			Format:   appearance.ExternalTheme.Format,
			Path:     appearance.ExternalTheme.Path,
		},
	}
}

func buildSystemviewPalettePayload(palette, fallback ColorPalette) dto.ColorPalette {
	return dto.ColorPalette{
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

func buildSystemviewSearchShortcutsPayload(shortcuts map[string]SearchShortcut) map[string]dto.SearchShortcut {
	if len(shortcuts) == 0 {
		return map[string]dto.SearchShortcut{}
	}

	result := make(map[string]dto.SearchShortcut, len(shortcuts))
	for key, shortcut := range shortcuts {
		result[key] = dto.SearchShortcut{URL: shortcut.URL, Description: shortcut.Description}
	}
	return result
}

func buildSystemviewHardwarePayload(hw *port.HardwareInfo) dto.SystemviewHardwarePayload {
	if hw == nil {
		return dto.SystemviewHardwarePayload{}
	}
	return dto.SystemviewHardwarePayload{
		CPUCores:   hw.CPUCores,
		CPUThreads: hw.CPUThreads,
		TotalRAMMB: hw.TotalRAMMB(),
		GPUVendor:  string(hw.GPUVendor),
		GPUName:    hw.GPUName,
		VRAMMB:     hw.VRAMMB(),
	}
}
