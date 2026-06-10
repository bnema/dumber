package dto

import "github.com/bnema/dumber/internal/domain/entity"

// WebUIConfig represents the subset of configuration editable in dumb://config.
type WebUIConfig struct {
	Appearance          WebUIAppearanceConfig     `json:"appearance"`
	Performance         WebUIPerformanceConfig    `json:"performance"`
	DefaultUIScale      float64                   `json:"default_ui_scale"`
	DefaultSearchEngine string                    `json:"default_search_engine"`
	SearchShortcuts     map[string]SearchShortcut `json:"search_shortcuts"`
}

// SystemviewCustomPerformancePayload is the editable custom performance profile shape.
type SystemviewCustomPerformancePayload = WebUICustomPerformanceConfig

// SystemviewResolvedPerformancePayload is the resolved effective performance shape.
type SystemviewResolvedPerformancePayload struct {
	SkiaCPUThreads         int     `json:"skia_cpu_threads"`
	SkiaGPUThreads         int     `json:"skia_gpu_threads"`
	WebProcessMemoryMB     int     `json:"web_process_memory_mb"`
	NetworkProcessMemoryMB int     `json:"network_process_memory_mb"`
	WebViewPoolPrewarm     int     `json:"webview_pool_prewarm"`
	ConservativeThreshold  float64 `json:"conservative_threshold"`
	StrictThreshold        float64 `json:"strict_threshold"`
}

// SystemviewHardwarePayload exposes detected hardware to the settings UI.
type SystemviewHardwarePayload struct {
	CPUCores   int    `json:"cpu_cores"`
	CPUThreads int    `json:"cpu_threads"`
	TotalRAMMB int    `json:"total_ram_mb"`
	GPUVendor  string `json:"gpu_vendor"`
	GPUName    string `json:"gpu_name"`
	VRAMMB     int    `json:"vram_mb"`
}

// SystemviewPerformancePayload is the performance tab payload expected by the UI.
type SystemviewPerformancePayload struct {
	Profile  string                               `json:"profile"`
	Custom   SystemviewCustomPerformancePayload   `json:"custom"`
	Resolved SystemviewResolvedPerformancePayload `json:"resolved"`
	Hardware SystemviewHardwarePayload            `json:"hardware"`
}

// SystemviewConfigPayload is the full JSON shape expected by dumb://config.
type SystemviewConfigPayload struct {
	// Appearance is the editable config snapshot.
	Appearance WebUIAppearanceConfig `json:"appearance"`
	// ResolvedAppearance is optional, display-only appearance resolved from config
	// plus external theme sources. Forms must save Appearance, never this field.
	ResolvedAppearance  *WebUIAppearanceConfig       `json:"resolved_appearance,omitempty"`
	Performance         SystemviewPerformancePayload `json:"performance"`
	DefaultUIScale      float64                      `json:"default_ui_scale"`
	DefaultSearchEngine string                       `json:"default_search_engine"`
	SearchShortcuts     map[string]SearchShortcut    `json:"search_shortcuts"`
	EngineType          string                       `json:"engine_type"`
}

// WebUIPerformanceConfig represents performance settings editable in dumb://config.
type WebUIPerformanceConfig struct {
	Profile string                       `json:"profile"`
	Custom  WebUICustomPerformanceConfig `json:"custom"`
}

// WebUICustomPerformanceConfig holds user-editable fields for custom profile.
type WebUICustomPerformanceConfig struct {
	SkiaCPUThreads         int `json:"skia_cpu_threads"`
	SkiaGPUThreads         int `json:"skia_gpu_threads"`
	WebProcessMemoryMB     int `json:"web_process_memory_mb"`
	NetworkProcessMemoryMB int `json:"network_process_memory_mb"`
	WebViewPoolPrewarm     int `json:"webview_pool_prewarm"`
}

type WebUIAppearanceConfig = entity.AppearanceConfig

type WebUIExternalThemeConfig = entity.ExternalThemeConfig

type ColorPalette = entity.ColorPalette

type SearchShortcut struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// WebUIAppearanceWithResolvedTheme builds a display-only appearance payload from
// resolved theme values while preserving fields that are not theme-resolution
// outputs, such as ColorScheme and ExternalTheme.
func WebUIAppearanceWithResolvedTheme(base WebUIAppearanceConfig, resolved entity.ResolvedTheme) WebUIAppearanceConfig {
	base.SansFont = resolved.Fonts.SansFont
	base.SerifFont = resolved.Fonts.SerifFont
	base.MonospaceFont = resolved.Fonts.MonospaceFont
	base.GtkFont = resolved.Fonts.GtkFont
	base.DefaultFontSize = resolved.Fonts.DefaultSize
	base.LightPalette = resolved.LightPalette
	base.DarkPalette = resolved.DarkPalette
	return base
}
