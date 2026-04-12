package config

import "github.com/bnema/dumber/internal/domain/entity"

// Type aliases for config value types now defined in domain/entity.
// These aliases keep all existing config-package callers working without
// any changes to their import paths.

// AppearanceConfig holds UI/rendering preferences.
type AppearanceConfig = entity.AppearanceConfig

// ColorPalette contains semantic color tokens for light/dark themes.
type ColorPalette = entity.ColorPalette

// ActionBinding defines a keybinding with optional description.
type ActionBinding = entity.ActionBinding

// PaneModeConfig defines modal behavior for pane management.
type PaneModeConfig = entity.PaneModeConfig

// TabModeConfig defines modal behavior for tab management (Zellij-style).
type TabModeConfig = entity.TabModeConfig

// ResizeModeConfig defines modal behavior for resizing panes (Zellij-style).
type ResizeModeConfig = entity.ResizeModeConfig

// SessionModeConfig defines modal behavior for session management.
type SessionModeConfig = entity.SessionModeConfig

// SessionConfig holds session persistence settings.
type SessionConfig = entity.SessionConfig

// GlobalShortcutsConfig defines global shortcuts (always active, not modal).
type GlobalShortcutsConfig = entity.GlobalShortcutsConfig

// FloatingPaneProfile defines a shortcut profile that opens a URL in the floating pane.
type FloatingPaneProfile = entity.FloatingPaneProfile

// FloatingPaneConfig defines persistent floating pane behavior.
type FloatingPaneConfig = entity.FloatingPaneConfig

// WorkspaceStylingConfig defines visual styling for workspace panes.
type WorkspaceStylingConfig = entity.WorkspaceStylingConfig

// PopupBehavior defines how popup windows should be opened.
type PopupBehavior = entity.PopupBehavior

const (
	// PopupBehaviorSplit opens popups in a split pane (default)
	PopupBehaviorSplit = entity.PopupBehaviorSplit
	// PopupBehaviorStacked opens popups in a stacked pane
	PopupBehaviorStacked = entity.PopupBehaviorStacked
	// PopupBehaviorTabbed opens popups as a new tab
	PopupBehaviorTabbed = entity.PopupBehaviorTabbed
	// PopupBehaviorWindowed opens popups in a new workspace window
	PopupBehaviorWindowed = entity.PopupBehaviorWindowed
)

// OmniboxInitialBehavior defines how omnibox history is ordered on open.
type OmniboxInitialBehavior = entity.OmniboxInitialBehavior

const (
	// OmniboxInitialBehaviorRecent shows recent visits first.
	OmniboxInitialBehaviorRecent = entity.OmniboxInitialBehaviorRecent
	// OmniboxInitialBehaviorMostVisited shows most visited sites first.
	OmniboxInitialBehaviorMostVisited = entity.OmniboxInitialBehaviorMostVisited
	// OmniboxInitialBehaviorNone shows no initial history.
	OmniboxInitialBehaviorNone = entity.OmniboxInitialBehaviorNone
)

// PopupBehaviorConfig defines handling for popup windows.
type PopupBehaviorConfig = entity.PopupBehaviorConfig

// WorkspaceConfig captures layout, pane, and tab behavior preferences.
type WorkspaceConfig = entity.WorkspaceConfig

// UpdateConfig holds automatic update settings.
type UpdateConfig = entity.UpdateConfig

// Config represents the complete configuration for dumber.
type Config struct {
	Database        DatabaseConfig            `mapstructure:"database" yaml:"database" toml:"database"`
	History         HistoryConfig             `mapstructure:"history" yaml:"history" toml:"history"`
	SearchShortcuts map[string]SearchShortcut `mapstructure:"search_shortcuts" yaml:"search_shortcuts" toml:"search_shortcuts"`
	// DefaultSearchEngine is the URL template for the default search engine (must contain %s placeholder)
	DefaultSearchEngine string           `mapstructure:"default_search_engine" yaml:"default_search_engine" toml:"default_search_engine"`
	Dmenu               DmenuConfig      `mapstructure:"dmenu" yaml:"dmenu" toml:"dmenu"`
	Logging             LoggingConfig    `mapstructure:"logging" yaml:"logging" toml:"logging"`
	Appearance          AppearanceConfig `mapstructure:"appearance" yaml:"appearance" toml:"appearance"`
	Debug               DebugConfig      `mapstructure:"debug" yaml:"debug" toml:"debug"`
	// DefaultWebpageZoom sets the default zoom level for pages without saved zoom settings (1.0 = 100%, 1.2 = 120%)
	DefaultWebpageZoom float64 `mapstructure:"default_webpage_zoom" yaml:"default_webpage_zoom" toml:"default_webpage_zoom"`
	// DefaultUIScale sets the default UI scale for GTK widgets (1.0 = 100%, 2.0 = 200%)
	DefaultUIScale float64 `mapstructure:"default_ui_scale" yaml:"default_ui_scale" toml:"default_ui_scale"`
	// Workspace defines workspace, pane, and tab handling behavior.
	Workspace WorkspaceConfig `mapstructure:"workspace" yaml:"workspace" toml:"workspace"`
	// Session controls session persistence and restoration.
	Session SessionConfig `mapstructure:"session" yaml:"session" toml:"session"`
	// ContentFiltering controls ad blocking and content filtering
	ContentFiltering ContentFilteringConfig `mapstructure:"content_filtering" yaml:"content_filtering" toml:"content_filtering"`
	// Clipboard controls clipboard-related behavior
	Clipboard ClipboardConfig `mapstructure:"clipboard" yaml:"clipboard" toml:"clipboard"`
	// Omnibox controls the omnibox behavior (initial history display)
	Omnibox OmniboxConfig `mapstructure:"omnibox" yaml:"omnibox" toml:"omnibox"`
	// Media controls video playback and hardware acceleration
	Media MediaConfig `mapstructure:"media" yaml:"media" toml:"media"`
	// Update controls automatic update checking and downloading.
	Update UpdateConfig `mapstructure:"update" yaml:"update" toml:"update"`
	// Downloads configures file download behavior.
	Downloads DownloadsConfig `mapstructure:"downloads" yaml:"downloads" toml:"downloads"`
	// Engine holds engine selection and unified engine options.
	Engine EngineConfig `mapstructure:"engine" toml:"engine" yaml:"engine"`
	// Transcoding controls GPU-accelerated media transcoding for proprietary codecs.
	Transcoding TranscodingConfig `mapstructure:"transcoding" yaml:"transcoding" toml:"transcoding"`
}

// CookiePolicy controls cookie acceptance behavior.
type CookiePolicy string

const (
	// CookiePolicyAlways accepts all cookies.
	CookiePolicyAlways CookiePolicy = "always"
	// CookiePolicyNoThirdParty blocks third-party cookies.
	CookiePolicyNoThirdParty CookiePolicy = "no_third_party"
	// CookiePolicyNever blocks all cookies.
	CookiePolicyNever CookiePolicy = "never"
)

// GSKRendererMode controls the GTK Scene Kit renderer selection.
type GSKRendererMode string

const (
	GSKRendererAuto   GSKRendererMode = "auto"
	GSKRendererOpenGL GSKRendererMode = "opengl"
	GSKRendererVulkan GSKRendererMode = "vulkan"
	GSKRendererCairo  GSKRendererMode = "cairo"
)

// HardwareDecodingMode controls video hardware acceleration.
type HardwareDecodingMode string

const (
	// HardwareDecodingAuto lets GStreamer choose (hw preferred, sw fallback)
	HardwareDecodingAuto HardwareDecodingMode = "auto"
	// HardwareDecodingForce requires hardware decoding (fails if unavailable)
	HardwareDecodingForce HardwareDecodingMode = "force"
	// HardwareDecodingDisable uses software decoding only
	HardwareDecodingDisable HardwareDecodingMode = "disable"
)

// GLRenderingMode controls OpenGL API selection for video rendering.
type GLRenderingMode string

const (
	// GLRenderingModeAuto lets GStreamer choose the best GL API
	GLRenderingModeAuto GLRenderingMode = "auto"
	// GLRenderingModeGLES2 forces GLES2 (better for some mobile/embedded GPUs)
	GLRenderingModeGLES2 GLRenderingMode = "gles2"
	// GLRenderingModeGL3 forces OpenGL 3.x desktop
	GLRenderingModeGL3 GLRenderingMode = "gl3"
	// GLRenderingModeNone disables GL-based rendering
	GLRenderingModeNone GLRenderingMode = "none"
)

// PerformanceProfile selects preset performance tuning settings.
type PerformanceProfile string

const (
	// ProfileDefault uses WebKit defaults (no tuning applied).
	ProfileDefault PerformanceProfile = "default"
	// ProfileLite reduces resource usage for low-RAM systems.
	ProfileLite PerformanceProfile = "lite"
	// ProfileMax maximizes responsiveness for heavy pages (GitHub PRs, etc).
	ProfileMax PerformanceProfile = "max"
	// ProfileCustom allows full control over individual performance settings.
	ProfileCustom PerformanceProfile = "custom"
)

// ThemeDefault is the default theme setting (follows system).
const ThemeDefault = "default"

const (
	// ThemePreferDark explicitly selects dark theme.
	ThemePreferDark = "prefer-dark"
	// ThemePreferLight explicitly selects light theme.
	ThemePreferLight = "prefer-light"
)

// MediaConfig holds video playback and hardware acceleration preferences.
type MediaConfig struct {
	// HardwareDecodingMode controls hardware vs software video decoding
	// Values: "auto" (default), "force", "disable"
	HardwareDecodingMode HardwareDecodingMode `mapstructure:"hardware_decoding" yaml:"hardware_decoding" toml:"hardware_decoding"`
	// PreferAV1 prioritizes AV1 codec (most efficient) when available
	PreferAV1 bool `mapstructure:"prefer_av1" yaml:"prefer_av1" toml:"prefer_av1"`
	// ShowDiagnosticsOnStartup shows media capability warnings at startup
	ShowDiagnosticsOnStartup bool `mapstructure:"show_diagnostics" yaml:"show_diagnostics" toml:"show_diagnostics"`
	// ForceVSync forces vertical sync for video playback (may help with tearing).
	//
	// Deprecated: moved to [engine.webkit]. Kept for read compatibility during migration.
	ForceVSync bool `mapstructure:"force_vsync" yaml:"force_vsync" toml:"-"`
	// GLRenderingMode controls OpenGL API selection for video rendering.
	// Values: "auto" (default), "gles2", "gl3", "none"
	//
	// Deprecated: moved to [engine.webkit]. Kept for read compatibility during migration.
	GLRenderingMode GLRenderingMode `mapstructure:"gl_rendering_mode" yaml:"gl_rendering_mode" toml:"-"`
	// GStreamerDebugLevel sets GStreamer debug verbosity (0=off, 1-5=increasing verbosity).
	//
	// Deprecated: moved to [engine.webkit]. Kept for read compatibility during migration.
	GStreamerDebugLevel int `mapstructure:"gstreamer_debug_level" yaml:"gstreamer_debug_level" toml:"-"`
}

// DatabaseConfig holds database-related configuration.
type DatabaseConfig struct {
	Path string `mapstructure:"path" yaml:"path" toml:"path"`
}

// HistoryConfig holds history-related configuration.
type HistoryConfig struct {
	MaxEntries          int `mapstructure:"max_entries" yaml:"max_entries" toml:"max_entries"`
	RetentionPeriodDays int `mapstructure:"retention_period_days" yaml:"retention_period_days" toml:"retention_period_days"`
	CleanupIntervalDays int `mapstructure:"cleanup_interval_days" yaml:"cleanup_interval_days" toml:"cleanup_interval_days"`
}

// PerformanceConfig holds internal performance tuning options.
// These are not exposed in dumb://config UI but can be set in config file.
type PerformanceConfig struct {
	// Profile selects a preset performance configuration.
	// Values: "default" (no tuning), "lite" (low RAM), "max" (heavy pages), "custom" (manual control)
	// When profile is not "custom", individual tuning fields below are ignored and computed from the profile.
	// Default: "default"
	Profile PerformanceProfile `mapstructure:"profile" yaml:"profile" toml:"profile"`

	// ZoomCacheSize is the number of domain zoom levels to cache in memory.
	// Higher values reduce database queries but use more memory.
	// Default: 256 domains (~20KB memory)
	ZoomCacheSize int `mapstructure:"zoom_cache_size" yaml:"zoom_cache_size" toml:"zoom_cache_size"`

	// WebViewPoolPrewarmCount is the number of WebViews to pre-create at startup.
	// Higher values speed up initial tab creation but use more memory.
	// Default: 4 (custom mode), computed from profile otherwise
	WebViewPoolPrewarmCount int `mapstructure:"webview_pool_prewarm_count" yaml:"webview_pool_prewarm_count" toml:"webview_pool_prewarm_count"` //nolint:lll // struct tags must stay on one line

	// --- Skia rendering threads (env vars) ---
	// NOTE: Fields below only apply when Profile is "custom".
	// SkiaCPUPaintingThreads sets WEBKIT_SKIA_CPU_PAINTING_THREADS.
	// Default: 0 (unset, uses WebKit default)
	SkiaCPUPaintingThreads int `mapstructure:"skia_cpu_painting_threads" yaml:"skia_cpu_painting_threads" toml:"skia_cpu_painting_threads"` //nolint:lll // struct tags must stay on one line

	// SkiaGPUPaintingThreads sets WEBKIT_SKIA_GPU_PAINTING_THREADS.
	// Default: -1 (unset). Value 0 disables GPU tile painting.
	SkiaGPUPaintingThreads int `mapstructure:"skia_gpu_painting_threads" yaml:"skia_gpu_painting_threads" toml:"skia_gpu_painting_threads"` //nolint:lll // struct tags must stay on one line

	// SkiaEnableCPURendering forces CPU rendering via WEBKIT_SKIA_ENABLE_CPU_RENDERING=1.
	// Default: false
	SkiaEnableCPURendering bool `mapstructure:"skia_enable_cpu_rendering" yaml:"skia_enable_cpu_rendering" toml:"skia_enable_cpu_rendering"` //nolint:lll // struct tags must stay on one line

	// --- Web process memory pressure ---
	// WebProcessMemoryLimitMB sets memory limit in MB for web processes.
	// Default: 0 (unset, uses WebKit default: system RAM capped at 3GB)
	WebProcessMemoryLimitMB int `mapstructure:"web_process_memory_limit_mb" yaml:"web_process_memory_limit_mb" toml:"web_process_memory_limit_mb"` //nolint:lll // struct tags must stay on one line

	// WebProcessMemoryPollIntervalSec sets poll interval for memory checks.
	// Default: 0 (unset, uses WebKit default: 30 seconds)
	WebProcessMemoryPollIntervalSec float64 `mapstructure:"web_process_memory_poll_interval_sec" yaml:"web_process_memory_poll_interval_sec" toml:"web_process_memory_poll_interval_sec"` //nolint:lll // struct tags must stay on one line

	// WebProcessMemoryConservativeThreshold sets threshold for conservative memory release.
	// Valid: (0, 1). Default: 0 (unset, uses WebKit default: 0.33)
	WebProcessMemoryConservativeThreshold float64 `mapstructure:"web_process_memory_conservative_threshold" yaml:"web_process_memory_conservative_threshold" toml:"web_process_memory_conservative_threshold"` //nolint:lll // struct tags must stay on one line

	// WebProcessMemoryStrictThreshold sets threshold for strict memory release.
	// Valid: (0, 1). Default: 0 (unset, uses WebKit default: 0.5)
	WebProcessMemoryStrictThreshold float64 `mapstructure:"web_process_memory_strict_threshold" yaml:"web_process_memory_strict_threshold" toml:"web_process_memory_strict_threshold"` //nolint:lll // struct tags must stay on one line

	// --- Network process memory pressure ---
	// NetworkProcessMemoryLimitMB sets memory limit in MB for network process.
	// Default: 0 (unset)
	NetworkProcessMemoryLimitMB int `mapstructure:"network_process_memory_limit_mb" yaml:"network_process_memory_limit_mb" toml:"network_process_memory_limit_mb"` //nolint:lll // struct tags must stay on one line

	// NetworkProcessMemoryPollIntervalSec sets poll interval for memory checks.
	// Default: 0 (unset)
	NetworkProcessMemoryPollIntervalSec float64 `mapstructure:"network_process_memory_poll_interval_sec" yaml:"network_process_memory_poll_interval_sec" toml:"network_process_memory_poll_interval_sec"` //nolint:lll // struct tags must stay on one line

	// NetworkProcessMemoryConservativeThreshold sets threshold for conservative memory release.
	// Valid: (0, 1). Default: 0 (unset)
	NetworkProcessMemoryConservativeThreshold float64 `mapstructure:"network_process_memory_conservative_threshold" yaml:"network_process_memory_conservative_threshold" toml:"network_process_memory_conservative_threshold"` //nolint:lll // struct tags must stay on one line

	// NetworkProcessMemoryStrictThreshold sets threshold for strict memory release.
	// Valid: (0, 1). Default: 0 (unset)
	NetworkProcessMemoryStrictThreshold float64 `mapstructure:"network_process_memory_strict_threshold" yaml:"network_process_memory_strict_threshold" toml:"network_process_memory_strict_threshold"` //nolint:lll // struct tags must stay on one line
}

// SearchShortcut represents a search shortcut configuration.
type SearchShortcut struct {
	URL         string `mapstructure:"url" toml:"url" yaml:"url" json:"url"`
	Description string `mapstructure:"description" toml:"description" yaml:"description" json:"description"`
}

// ShortcutURLs returns a simple map of shortcut keys to URL templates.
// This is useful for passing to url.BuildSearchURL.
func (c *Config) ShortcutURLs() map[string]string {
	result := make(map[string]string, len(c.SearchShortcuts))
	for key, shortcut := range c.SearchShortcuts {
		result[key] = shortcut.URL
	}
	return result
}

// DmenuConfig holds dmenu/rofi integration configuration.
type DmenuConfig struct {
	MaxHistoryDays   int    `mapstructure:"max_history_days" yaml:"max_history_days" toml:"max_history_days"`
	ShowVisitCount   bool   `mapstructure:"show_visit_count" yaml:"show_visit_count" toml:"show_visit_count"`
	ShowLastVisited  bool   `mapstructure:"show_last_visited" yaml:"show_last_visited" toml:"show_last_visited"`
	HistoryPrefix    string `mapstructure:"history_prefix" yaml:"history_prefix" toml:"history_prefix"`
	ShortcutPrefix   string `mapstructure:"shortcut_prefix" yaml:"shortcut_prefix" toml:"shortcut_prefix"`
	URLPrefix        string `mapstructure:"url_prefix" yaml:"url_prefix" toml:"url_prefix"`
	DateFormat       string `mapstructure:"date_format" yaml:"date_format" toml:"date_format"`
	SortByVisitCount bool   `mapstructure:"sort_by_visit_count" yaml:"sort_by_visit_count" toml:"sort_by_visit_count"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level  string `mapstructure:"level" yaml:"level" toml:"level"`
	Format string `mapstructure:"format" yaml:"format" toml:"format"`
	MaxAge int    `mapstructure:"max_age" yaml:"max_age" toml:"max_age"`

	// File output configuration
	LogDir        string `mapstructure:"log_dir" yaml:"log_dir" toml:"log_dir"`
	EnableFileLog bool   `mapstructure:"enable_file_log" yaml:"enable_file_log" toml:"enable_file_log"`

	// Capture browser console to logs
	CaptureConsole bool `mapstructure:"capture_console" yaml:"capture_console" toml:"capture_console" json:"capture_console"`

	// Capture GTK4/WebKitGTK6 messages to session logs
	CaptureGTKLogs bool `mapstructure:"capture_gtk_logs" yaml:"capture_gtk_logs" toml:"capture_gtk_logs" json:"capture_gtk_logs"`
}

// ContentFilteringConfig holds content filtering and ad blocking preferences
type ContentFilteringConfig struct {
	// Enabled controls whether ad blocking is active (default: true)
	Enabled bool `mapstructure:"enabled" yaml:"enabled" toml:"enabled"`
	// AutoUpdate controls whether filters are automatically updated (default: true)
	AutoUpdate bool `mapstructure:"auto_update" yaml:"auto_update" toml:"auto_update"`
	// Note: Filters are downloaded from bnema/ublock-webkit-filters GitHub releases
	// Note: Whitelist is managed via database (content_whitelist table)
}

// ClipboardConfig holds clipboard-related behavior preferences
type ClipboardConfig struct {
	// AutoCopyOnSelection automatically copies selected text to clipboard (zellij-style).
	// When enabled, selecting text in a webview immediately copies it to clipboard.
	// Does not apply to text selection in input fields or textareas.
	// Default: true
	AutoCopyOnSelection bool `mapstructure:"auto_copy_on_selection" yaml:"auto_copy_on_selection" toml:"auto_copy_on_selection" json:"autoCopyOnSelection"` //nolint:lll // struct tags must stay on one line
}

// OmniboxConfig holds omnibox behavior preferences
type OmniboxConfig struct {
	// InitialBehavior controls what to show when omnibox opens with empty input
	// Values: "recent" (recent visits), "most_visited" (most visited sites), "none" (nothing)
	InitialBehavior OmniboxInitialBehavior `mapstructure:"initial_behavior" yaml:"initial_behavior" toml:"initial_behavior"`
	// AutoOpenOnNewPane opens the omnibox automatically when a new pane is created.
	// Default: false
	AutoOpenOnNewPane bool `mapstructure:"auto_open_on_new_pane" yaml:"auto_open_on_new_pane" toml:"auto_open_on_new_pane"`
}

// DebugConfig holds debug and troubleshooting options
type DebugConfig struct {
	// Enable browser developer tools (F12, Inspect Element in context menu)
	EnableDevTools bool `mapstructure:"enable_devtools" yaml:"enable_devtools" toml:"enable_devtools"`
}

// DownloadsConfig holds file download preferences.
type DownloadsConfig struct {
	// Path is the directory where downloads are saved.
	// Empty string means use XDG_DOWNLOAD_DIR or ~/Downloads.
	Path string `mapstructure:"path" yaml:"path" toml:"path"`
}

// TranscodingConfig controls GPU-accelerated media transcoding.
// When enabled, proprietary video formats (H.264, HEVC) are transcoded
// to open codecs (VP9, AV1) that CEF can decode natively.
type TranscodingConfig struct {
	// Enabled controls whether transcoding is active (default: false).
	Enabled bool `mapstructure:"enabled" yaml:"enabled" toml:"enabled"`
	// HWAccel selects the hardware acceleration API: "auto", "vaapi", or "nvenc".
	// Default: "auto" (tries VAAPI first, then CUDA).
	HWAccel string `mapstructure:"hwaccel" yaml:"hwaccel" toml:"hwaccel"`
	// MaxConcurrent is the maximum number of simultaneous transcode sessions.
	// Default: 3.
	MaxConcurrent int `mapstructure:"max_concurrent" yaml:"max_concurrent" toml:"max_concurrent"`
	// Quality controls the encode quality preset: "low", "medium", or "high".
	// Default: "medium".
	Quality string `mapstructure:"quality" yaml:"quality" toml:"quality"`
}
