package port

import "context"

// WebUIConfig represents the subset of configuration editable in dumb://config.
type WebUIConfig struct {
	Appearance          WebUIAppearanceConfig     `json:"appearance"`
	Performance         WebUIPerformanceConfig    `json:"performance"`
	DefaultUIScale      float64                   `json:"default_ui_scale"`
	DefaultSearchEngine string                    `json:"default_search_engine"`
	SearchShortcuts     map[string]SearchShortcut `json:"search_shortcuts"`
}

// SystemviewCustomPerformancePayload is the editable custom performance profile shape.
type SystemviewCustomPerformancePayload struct {
	SkiaCPUThreads         int `json:"skia_cpu_threads"`
	SkiaGPUThreads         int `json:"skia_gpu_threads"`
	WebProcessMemoryMB     int `json:"web_process_memory_mb"`
	NetworkProcessMemoryMB int `json:"network_process_memory_mb"`
	WebViewPoolPrewarm     int `json:"webview_pool_prewarm"`
}

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
	Appearance          WebUIAppearanceConfig        `json:"appearance"`
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

type WebUIAppearanceConfig struct {
	SansFont        string       `json:"sans_font"`
	SerifFont       string       `json:"serif_font"`
	MonospaceFont   string       `json:"monospace_font"`
	DefaultFontSize int          `json:"default_font_size"`
	ColorScheme     string       `json:"color_scheme"`
	LightPalette    ColorPalette `json:"light_palette"`
	DarkPalette     ColorPalette `json:"dark_palette"`
}

type ColorPalette struct {
	Background     string `json:"background"`
	Surface        string `json:"surface"`
	SurfaceVariant string `json:"surface_variant"`
	Text           string `json:"text"`
	Muted          string `json:"muted"`
	Accent         string `json:"accent"`
	Border         string `json:"border"`
}

type SearchShortcut struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// SystemviewConfigReader reads the current and default systemview config payloads.
type SystemviewConfigReader interface {
	Current(ctx context.Context) (SystemviewConfigPayload, error)
	Default(ctx context.Context) (SystemviewConfigPayload, error)
}

// WebUIConfigSaver persists WebUI configuration changes.
type WebUIConfigSaver interface {
	SaveWebUIConfig(ctx context.Context, cfg WebUIConfig) error
}
