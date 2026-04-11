package config

import "os"

const (
	EngineTypeWebKit = "webkit"
	EngineTypeCEF    = "cef"
)

// EngineConfig holds engine selection and universal engine options.
type EngineConfig struct {
	Type             string             `mapstructure:"type" toml:"type" yaml:"type"`
	PoolPrewarmCount int                `mapstructure:"pool_prewarm_count" toml:"pool_prewarm_count" yaml:"pool_prewarm_count"`
	ZoomCacheSize    int                `mapstructure:"zoom_cache_size" toml:"zoom_cache_size" yaml:"zoom_cache_size"`
	Profile          PerformanceProfile `mapstructure:"profile" toml:"profile" yaml:"profile"`
	CookiePolicy     CookiePolicy       `mapstructure:"cookie_policy" toml:"cookie_policy" yaml:"cookie_policy"`
	WebKit           WebKitEngineConfig `mapstructure:"webkit" toml:"webkit" yaml:"webkit"`
	CEF              CEFEngineConfig    `mapstructure:"cef" toml:"cef" yaml:"cef"`
}

// ResolveEngineType returns the effective engine type from config + env override.
// It defaults to "webkit" when no type is configured, and allows DUMBER_ENGINE
// to override the configured value for smoke testing.
func (e *EngineConfig) ResolveEngineType() string {
	engineType := e.Type
	if engineType == "" {
		engineType = EngineTypeWebKit
	}
	if envEngine := os.Getenv("DUMBER_ENGINE"); envEngine != "" {
		engineType = envEngine
	}
	return engineType
}

// WebKitEngineConfig holds WebKit-specific engine options.
type WebKitEngineConfig struct {
	// Skia rendering threads
	SkiaCPUPaintingThreads int  `mapstructure:"skia_cpu_painting_threads" toml:"skia_cpu_painting_threads" yaml:"skia_cpu_painting_threads"`
	SkiaGPUPaintingThreads int  `mapstructure:"skia_gpu_painting_threads" toml:"skia_gpu_painting_threads" yaml:"skia_gpu_painting_threads"`
	SkiaEnableCPURendering bool `mapstructure:"skia_enable_cpu_rendering" toml:"skia_enable_cpu_rendering" yaml:"skia_enable_cpu_rendering"`

	// WebKit compositor
	DisableDMABufRenderer  bool `mapstructure:"disable_dmabuf_renderer" toml:"disable_dmabuf_renderer" yaml:"disable_dmabuf_renderer"`
	ForceCompositingMode   bool `mapstructure:"force_compositing_mode" toml:"force_compositing_mode" yaml:"force_compositing_mode"`
	DisableCompositingMode bool `mapstructure:"disable_compositing_mode" toml:"disable_compositing_mode" yaml:"disable_compositing_mode"`

	// GTK/GSK
	GSKRenderer    GSKRendererMode `mapstructure:"gsk_renderer" toml:"gsk_renderer" yaml:"gsk_renderer"`
	DisableMipmaps bool            `mapstructure:"disable_mipmaps" toml:"disable_mipmaps" yaml:"disable_mipmaps"`
	PreferGL       bool            `mapstructure:"prefer_gl" toml:"prefer_gl" yaml:"prefer_gl"`

	// Debug
	ShowFPS                   bool `mapstructure:"show_fps" toml:"show_fps" yaml:"show_fps"`
	SampleMemory              bool `mapstructure:"sample_memory" toml:"sample_memory" yaml:"sample_memory"`
	DebugFrames               bool `mapstructure:"debug_frames" toml:"debug_frames" yaml:"debug_frames"`
	DrawCompositingIndicators bool `mapstructure:"draw_compositing_indicators" toml:"draw_compositing_indicators" yaml:"draw_compositing_indicators"` //nolint:lll // struct tags exceed lll limit

	// Privacy (WebKit-specific)
	ITPEnabled bool `mapstructure:"itp_enabled" toml:"itp_enabled" yaml:"itp_enabled"`

	// GStreamer
	ForceVSync          bool            `mapstructure:"force_vsync" toml:"force_vsync" yaml:"force_vsync"`
	GLRenderingMode     GLRenderingMode `mapstructure:"gl_rendering_mode" toml:"gl_rendering_mode" yaml:"gl_rendering_mode"`
	GStreamerDebugLevel int             `mapstructure:"gstreamer_debug_level" toml:"gstreamer_debug_level" yaml:"gstreamer_debug_level"`

	// Runtime prefix for WebKitGTK libraries
	Prefix string `mapstructure:"prefix" toml:"prefix" yaml:"prefix"`

	// Web process memory pressure
	WebProcessMemoryLimitMB               int     `mapstructure:"web_process_memory_limit_mb" toml:"web_process_memory_limit_mb" yaml:"web_process_memory_limit_mb"`                                           //nolint:lll // struct tags exceed lll limit
	WebProcessMemoryPollIntervalSec       float64 `mapstructure:"web_process_memory_poll_interval_sec" toml:"web_process_memory_poll_interval_sec" yaml:"web_process_memory_poll_interval_sec"`                //nolint:lll // struct tags exceed lll limit
	WebProcessMemoryConservativeThreshold float64 `mapstructure:"web_process_memory_conservative_threshold" toml:"web_process_memory_conservative_threshold" yaml:"web_process_memory_conservative_threshold"` //nolint:lll // struct tags exceed lll limit
	WebProcessMemoryStrictThreshold       float64 `mapstructure:"web_process_memory_strict_threshold" toml:"web_process_memory_strict_threshold" yaml:"web_process_memory_strict_threshold"`                   //nolint:lll // struct tags exceed lll limit

	// Network process memory pressure
	NetworkProcessMemoryLimitMB               int     `mapstructure:"network_process_memory_limit_mb" toml:"network_process_memory_limit_mb" yaml:"network_process_memory_limit_mb"`                                           //nolint:lll // struct tags exceed lll limit
	NetworkProcessMemoryPollIntervalSec       float64 `mapstructure:"network_process_memory_poll_interval_sec" toml:"network_process_memory_poll_interval_sec" yaml:"network_process_memory_poll_interval_sec"`                //nolint:lll // struct tags exceed lll limit
	NetworkProcessMemoryConservativeThreshold float64 `mapstructure:"network_process_memory_conservative_threshold" toml:"network_process_memory_conservative_threshold" yaml:"network_process_memory_conservative_threshold"` //nolint:lll // struct tags exceed lll limit
	NetworkProcessMemoryStrictThreshold       float64 `mapstructure:"network_process_memory_strict_threshold" toml:"network_process_memory_strict_threshold" yaml:"network_process_memory_strict_threshold"`                   //nolint:lll // struct tags exceed lll limit
}

// CEFEngineConfig holds CEF-specific engine options.
type CEFEngineConfig struct {
	// CEFDir is the path to the CEF framework directory containing
	// libcef.so and the Resources/locales subdirectories.
	CEFDir string `mapstructure:"cef_dir" toml:"cef_dir" yaml:"cef_dir"`
	// LogFile overrides the CEF internal log destination. When empty,
	// Dumber writes CEF startup logs to .dev/dumber/logs/cef_runtime.log.
	LogFile string `mapstructure:"log_file" toml:"log_file" yaml:"log_file"`
	// LogSeverity controls CEF's internal log verbosity.
	// 0 = default, 1 = verbose, 2 = info, 3 = warning, 4 = error, 99 = disable.
	LogSeverity int32 `mapstructure:"log_severity" toml:"log_severity" yaml:"log_severity"`
	// WindowlessFrameRate is the maximum frame rate for off-screen rendering.
	// Default: 60. Higher values increase CPU usage.
	WindowlessFrameRate int32 `mapstructure:"windowless_frame_rate" toml:"windowless_frame_rate" yaml:"windowless_frame_rate"`
	// EnableAudioHandler opts into the experimental CEF AudioHandler bridge.
	EnableAudioHandler bool `mapstructure:"enable_audio_handler" toml:"enable_audio_handler" yaml:"enable_audio_handler"`
	// EnableContextMenuHandler opts into the experimental CEF ContextMenuHandler bridge.
	EnableContextMenuHandler bool `mapstructure:"enable_context_menu_handler" toml:"enable_context_menu_handler" yaml:"enable_context_menu_handler"` //nolint:lll // struct tags exceed lll limit
	// TraceHandlers enables purego-cef handler/refcount tracing for diagnostics.
	TraceHandlers bool `mapstructure:"trace_handlers" toml:"trace_handlers" yaml:"trace_handlers"`
}

// CEFWindowlessFrameRate returns the effective OSR frame rate.
// A non-positive value falls back to the built-in default to avoid accidentally
// matching high-refresh monitors and burning CPU while idling.
func (c CEFEngineConfig) CEFWindowlessFrameRate() int32 {
	if c.WindowlessFrameRate > 0 {
		return c.WindowlessFrameRate
	}
	return defaultCEFWindowlessFrameRate
}

// PerformanceConfigFromEngine constructs a PerformanceConfig from the
// new [engine] config section, for use with ResolvePerformanceProfile.
func PerformanceConfigFromEngine(e *EngineConfig) PerformanceConfig {
	if e == nil {
		return PerformanceConfig{}
	}
	return PerformanceConfig{
		Profile:                                   e.Profile,
		ZoomCacheSize:                             e.ZoomCacheSize,
		WebViewPoolPrewarmCount:                   e.PoolPrewarmCount,
		SkiaCPUPaintingThreads:                    e.WebKit.SkiaCPUPaintingThreads,
		SkiaGPUPaintingThreads:                    e.WebKit.SkiaGPUPaintingThreads,
		SkiaEnableCPURendering:                    e.WebKit.SkiaEnableCPURendering,
		WebProcessMemoryLimitMB:                   e.WebKit.WebProcessMemoryLimitMB,
		WebProcessMemoryPollIntervalSec:           e.WebKit.WebProcessMemoryPollIntervalSec,
		WebProcessMemoryConservativeThreshold:     e.WebKit.WebProcessMemoryConservativeThreshold,
		WebProcessMemoryStrictThreshold:           e.WebKit.WebProcessMemoryStrictThreshold,
		NetworkProcessMemoryLimitMB:               e.WebKit.NetworkProcessMemoryLimitMB,
		NetworkProcessMemoryPollIntervalSec:       e.WebKit.NetworkProcessMemoryPollIntervalSec,
		NetworkProcessMemoryConservativeThreshold: e.WebKit.NetworkProcessMemoryConservativeThreshold,
		NetworkProcessMemoryStrictThreshold:       e.WebKit.NetworkProcessMemoryStrictThreshold,
	}
}
