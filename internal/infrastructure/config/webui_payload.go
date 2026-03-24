package config

import "github.com/bnema/dumber/internal/application/port"

// WebUIConfigAppearancePayload is the JSON shape expected by dumb://config.
type WebUIConfigAppearancePayload struct {
	SansFont        string       `json:"sans_font"`
	SerifFont       string       `json:"serif_font"`
	MonospaceFont   string       `json:"monospace_font"`
	DefaultFontSize int          `json:"default_font_size"`
	ColorScheme     string       `json:"color_scheme"`
	LightPalette    ColorPalette `json:"light_palette"`
	DarkPalette     ColorPalette `json:"dark_palette"`
}

// WebUIConfigHardwarePayload exposes detected hardware to the settings UI.
type WebUIConfigHardwarePayload struct {
	CPUCores   int    `json:"cpu_cores"`
	CPUThreads int    `json:"cpu_threads"`
	TotalRAMMB int    `json:"total_ram_mb"`
	GPUVendor  string `json:"gpu_vendor"`
	GPUName    string `json:"gpu_name"`
	VRAMMB     int    `json:"vram_mb"`
}

// WebUIConfigCustomPerformancePayload holds user-editable custom profile values.
type WebUIConfigCustomPerformancePayload struct {
	SkiaCPUThreads         int `json:"skia_cpu_threads"`
	SkiaGPUThreads         int `json:"skia_gpu_threads"`
	WebProcessMemoryMB     int `json:"web_process_memory_mb"`
	NetworkProcessMemoryMB int `json:"network_process_memory_mb"`
	WebViewPoolPrewarm     int `json:"webview_pool_prewarm"`
}

// WebUIConfigResolvedPerformancePayload holds the resolved effective values.
type WebUIConfigResolvedPerformancePayload struct {
	SkiaCPUThreads         int     `json:"skia_cpu_threads"`
	SkiaGPUThreads         int     `json:"skia_gpu_threads"`
	WebProcessMemoryMB     int     `json:"web_process_memory_mb"`
	NetworkProcessMemoryMB int     `json:"network_process_memory_mb"`
	WebViewPoolPrewarm     int     `json:"webview_pool_prewarm"`
	ConservativeThreshold  float64 `json:"conservative_threshold"`
	StrictThreshold        float64 `json:"strict_threshold"`
}

// WebUIConfigPerformancePayload is the performance tab payload expected by the UI.
type WebUIConfigPerformancePayload struct {
	Profile  string                                `json:"profile"`
	Custom   WebUIConfigCustomPerformancePayload   `json:"custom"`
	Resolved WebUIConfigResolvedPerformancePayload `json:"resolved"`
	Hardware WebUIConfigHardwarePayload            `json:"hardware"`
}

// WebUIConfigPayload is the full JSON shape expected by dumb://config.
type WebUIConfigPayload struct {
	Appearance          WebUIConfigAppearancePayload  `json:"appearance"`
	Performance         WebUIConfigPerformancePayload `json:"performance"`
	DefaultUIScale      float64                       `json:"default_ui_scale"`
	DefaultSearchEngine string                        `json:"default_search_engine"`
	SearchShortcuts     map[string]SearchShortcut     `json:"search_shortcuts"`
	EngineType          string                        `json:"engine_type"`
}

// BuildWebUIConfigPayload projects the full config into the stable DTO used by
// the settings page across engines.
func BuildWebUIConfigPayload(cfg *Config, hw *port.HardwareInfo) WebUIConfigPayload {
	if cfg == nil {
		return WebUIConfigPayload{}
	}

	perfCfg := PerformanceConfigFromEngine(&cfg.Engine)
	resolved := ResolvePerformanceProfile(&perfCfg, hw)

	return WebUIConfigPayload{
		DefaultUIScale:      cfg.DefaultUIScale,
		DefaultSearchEngine: cfg.DefaultSearchEngine,
		SearchShortcuts:     cfg.SearchShortcuts,
		EngineType:          cfg.Engine.ResolveEngineType(),
		Appearance: WebUIConfigAppearancePayload{
			SansFont:        cfg.Appearance.SansFont,
			SerifFont:       cfg.Appearance.SerifFont,
			MonospaceFont:   cfg.Appearance.MonospaceFont,
			DefaultFontSize: cfg.Appearance.DefaultFontSize,
			ColorScheme:     cfg.Appearance.ColorScheme,
			LightPalette:    cfg.Appearance.LightPalette,
			DarkPalette:     cfg.Appearance.DarkPalette,
		},
		Performance: WebUIConfigPerformancePayload{
			Profile: string(cfg.Engine.Profile),
			Custom: WebUIConfigCustomPerformancePayload{
				SkiaCPUThreads:         cfg.Engine.WebKit.SkiaCPUPaintingThreads,
				SkiaGPUThreads:         cfg.Engine.WebKit.SkiaGPUPaintingThreads,
				WebProcessMemoryMB:     cfg.Engine.WebKit.WebProcessMemoryLimitMB,
				NetworkProcessMemoryMB: cfg.Engine.WebKit.NetworkProcessMemoryLimitMB,
				WebViewPoolPrewarm:     cfg.Engine.PoolPrewarmCount,
			},
			Resolved: WebUIConfigResolvedPerformancePayload{
				SkiaCPUThreads:         resolved.SkiaCPUPaintingThreads,
				SkiaGPUThreads:         resolved.SkiaGPUPaintingThreads,
				WebProcessMemoryMB:     resolved.WebProcessMemoryLimitMB,
				NetworkProcessMemoryMB: resolved.NetworkProcessMemoryLimitMB,
				WebViewPoolPrewarm:     resolved.WebViewPoolPrewarmCount,
				ConservativeThreshold:  resolved.WebProcessMemoryConservativeThreshold,
				StrictThreshold:        resolved.WebProcessMemoryStrictThreshold,
			},
			Hardware: buildWebUIHardwarePayload(hw),
		},
	}
}

func buildWebUIHardwarePayload(hw *port.HardwareInfo) WebUIConfigHardwarePayload {
	if hw == nil {
		return WebUIConfigHardwarePayload{}
	}
	return WebUIConfigHardwarePayload{
		CPUCores:   hw.CPUCores,
		CPUThreads: hw.CPUThreads,
		TotalRAMMB: hw.TotalRAMMB(),
		GPUVendor:  string(hw.GPUVendor),
		GPUName:    hw.GPUName,
		VRAMMB:     hw.VRAMMB(),
	}
}
