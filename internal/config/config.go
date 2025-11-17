// Package config provides configuration management for dumber with Viper integration.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/pkg/gpu"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// File permission constants
const (
	dirPerm  = 0755 // Standard directory permissions (rwxr-xr-x)
	filePerm = 0644 // Standard file permissions (rw-r--r--)
)

// Config represents the complete configuration for dumber.
type Config struct {
	Database          DatabaseConfig            `mapstructure:"database" yaml:"database"`
	History           HistoryConfig             `mapstructure:"history" yaml:"history"`
	SearchShortcuts   map[string]SearchShortcut `mapstructure:"search_shortcuts" yaml:"search_shortcuts"`
	// DefaultSearchEngine is the URL template for the default search engine (must contain %s placeholder)
	DefaultSearchEngine string `mapstructure:"default_search_engine" yaml:"default_search_engine"`
	Dmenu               DmenuConfig `mapstructure:"dmenu" yaml:"dmenu"`
	Logging           LoggingConfig             `mapstructure:"logging" yaml:"logging"`
	Appearance        AppearanceConfig          `mapstructure:"appearance" yaml:"appearance"`
	VideoAcceleration VideoAccelerationConfig   `mapstructure:"video_acceleration" yaml:"video_acceleration"`
	CodecPreferences  CodecConfig               `mapstructure:"codec_preferences" yaml:"codec_preferences"`
	Debug             DebugConfig               `mapstructure:"debug" yaml:"debug"`
	// RenderingMode controls GPU/CPU rendering selection for WebKit
	RenderingMode RenderingMode `mapstructure:"rendering_mode" yaml:"rendering_mode"`
	// UseDomZoom toggles DOM-based zoom instead of native WebKit zoom.
	UseDomZoom bool `mapstructure:"use_dom_zoom" yaml:"use_dom_zoom"`
	// DefaultZoom sets the default zoom level for pages without saved zoom settings (1.0 = 100%, 1.2 = 120%)
	DefaultZoom float64 `mapstructure:"default_zoom" yaml:"default_zoom"`
	// Workspace defines workspace, pane, and tab handling behaviour.
	Workspace WorkspaceConfig `mapstructure:"workspace" yaml:"workspace"`
	// ContentFilteringWhitelist domains skip ad blocking (e.g. Twitch breaks with bot detection)
	ContentFilteringWhitelist []string `mapstructure:"content_filtering_whitelist" yaml:"content_filtering_whitelist"`
}

// RenderingMode selects GPU vs CPU rendering.
type RenderingMode string

const (
	RenderingModeAuto RenderingMode = "auto"
	RenderingModeGPU  RenderingMode = "gpu"
	RenderingModeCPU  RenderingMode = "cpu"
)

// DatabaseConfig holds database-related configuration.
type DatabaseConfig struct {
	Path string `mapstructure:"path" yaml:"path"`
}

// HistoryConfig holds history-related configuration.
type HistoryConfig struct {
	MaxEntries          int `mapstructure:"max_entries" yaml:"max_entries"`
	RetentionPeriodDays int `mapstructure:"retention_period_days" yaml:"retention_period_days"`
	CleanupIntervalDays int `mapstructure:"cleanup_interval_days" yaml:"cleanup_interval_days"`
}

// SearchShortcut represents a search shortcut configuration.
type SearchShortcut struct {
	URL         string `mapstructure:"url" yaml:"url" json:"url"`
	Description string `mapstructure:"description" yaml:"description" json:"description"`
}

// DmenuConfig holds dmenu/rofi integration configuration.
type DmenuConfig struct {
	MaxHistoryItems  int    `mapstructure:"max_history_items" yaml:"max_history_items"`
	ShowVisitCount   bool   `mapstructure:"show_visit_count" yaml:"show_visit_count"`
	ShowLastVisited  bool   `mapstructure:"show_last_visited" yaml:"show_last_visited"`
	HistoryPrefix    string `mapstructure:"history_prefix" yaml:"history_prefix"`
	ShortcutPrefix   string `mapstructure:"shortcut_prefix" yaml:"shortcut_prefix"`
	URLPrefix        string `mapstructure:"url_prefix" yaml:"url_prefix"`
	DateFormat       string `mapstructure:"date_format" yaml:"date_format"`
	SortByVisitCount bool   `mapstructure:"sort_by_visit_count" yaml:"sort_by_visit_count"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level      string `mapstructure:"level" yaml:"level"`
	Format     string `mapstructure:"format" yaml:"format"`
	Filename   string `mapstructure:"filename" yaml:"filename"`
	MaxSize    int    `mapstructure:"max_size" yaml:"max_size"`
	MaxBackups int    `mapstructure:"max_backups" yaml:"max_backups"`
	MaxAge     int    `mapstructure:"max_age" yaml:"max_age"`
	Compress   bool   `mapstructure:"compress" yaml:"compress"`

	// File output configuration
	LogDir        string `mapstructure:"log_dir" yaml:"log_dir"`
	EnableFileLog bool   `mapstructure:"enable_file_log" yaml:"enable_file_log"`

	// Capture settings
	CaptureStdout  bool `mapstructure:"capture_stdout" yaml:"capture_stdout" json:"capture_stdout"`
	CaptureStderr  bool `mapstructure:"capture_stderr" yaml:"capture_stderr" json:"capture_stderr"`
	CaptureCOutput bool `mapstructure:"capture_c_output" yaml:"capture_c_output" json:"capture_c_output"`
	CaptureConsole bool `mapstructure:"capture_console" yaml:"capture_console" json:"capture_console"`

	// Debug output
	DebugFile     string `mapstructure:"debug_file" yaml:"debug_file"`
	VerboseWebKit bool   `mapstructure:"verbose_webkit" yaml:"verbose_webkit"`
}

// AppearanceConfig holds UI/rendering preferences.
type AppearanceConfig struct {
	// Default fonts for pages that do not specify fonts.
	SansFont      string `mapstructure:"sans_font" yaml:"sans_font"`
	SerifFont     string `mapstructure:"serif_font" yaml:"serif_font"`
	MonospaceFont string `mapstructure:"monospace_font" yaml:"monospace_font"`
	// Default font size in CSS pixels (approx).
	DefaultFontSize int          `mapstructure:"default_font_size" yaml:"default_font_size"`
	LightPalette    ColorPalette `mapstructure:"light_palette" yaml:"light_palette"`
	DarkPalette     ColorPalette `mapstructure:"dark_palette" yaml:"dark_palette"`
	// ColorScheme controls the initial theme preference: "prefer-dark", "prefer-light", or "default" (follows system)
	ColorScheme string `mapstructure:"color_scheme" yaml:"color_scheme"`
}

// ColorPalette contains semantic color tokens for light/dark themes.
type ColorPalette struct {
	Background     string `mapstructure:"background" yaml:"background" json:"background"`
	Surface        string `mapstructure:"surface" yaml:"surface" json:"surface"`
	SurfaceVariant string `mapstructure:"surface_variant" yaml:"surface_variant" json:"surface_variant"`
	Text           string `mapstructure:"text" yaml:"text" json:"text"`
	Muted          string `mapstructure:"muted" yaml:"muted" json:"muted"`
	Accent         string `mapstructure:"accent" yaml:"accent" json:"accent"`
	Border         string `mapstructure:"border" yaml:"border" json:"border"`
}

// VideoAccelerationConfig holds video hardware acceleration preferences.
type VideoAccelerationConfig struct {
	EnableVAAPI      bool   `mapstructure:"enable_vaapi" yaml:"enable_vaapi"`
	AutoDetectGPU    bool   `mapstructure:"auto_detect_gpu" yaml:"auto_detect_gpu"`
	VAAPIDriverName  string `mapstructure:"vaapi_driver_name" yaml:"vaapi_driver_name"`
	EnableAllDrivers bool   `mapstructure:"enable_all_drivers" yaml:"enable_all_drivers"`
	LegacyVAAPI      bool   `mapstructure:"legacy_vaapi" yaml:"legacy_vaapi"`
}

// CodecConfig holds video codec preferences and handling
type CodecConfig struct {
	// Codec preference order (e.g., "av1,h264,vp8")
	PreferredCodecs string `mapstructure:"preferred_codecs" yaml:"preferred_codecs"`

	// Force specific codec for platforms
	ForceAV1 bool `mapstructure:"force_av1" yaml:"force_av1"`

	// Block problematic codecs
	BlockVP9 bool `mapstructure:"block_vp9" yaml:"block_vp9"`
	BlockVP8 bool `mapstructure:"block_vp8" yaml:"block_vp8"`

	// Hardware acceleration per codec
	AV1HardwareOnly    bool `mapstructure:"av1_hardware_only" yaml:"av1_hardware_only"`
	DisableVP9Hardware bool `mapstructure:"disable_vp9_hardware" yaml:"disable_vp9_hardware"`

	// Buffer configuration for smooth playback
	VideoBufferSizeMB  int `mapstructure:"video_buffer_size_mb" yaml:"video_buffer_size_mb"`
	QueueBufferTimeSec int `mapstructure:"queue_buffer_time_sec" yaml:"queue_buffer_time_sec"`

	// Custom User-Agent for codec negotiation
	CustomUserAgent string `mapstructure:"custom_user_agent" yaml:"custom_user_agent"`

	// Maximum resolution for AV1 codec (720p, 1080p, 1440p, 4k, unlimited)
	AV1MaxResolution string `mapstructure:"av1_max_resolution" yaml:"av1_max_resolution"`

	// Site-specific codec control settings
	DisableTwitchCodecControl bool `mapstructure:"disable_twitch_codec_control" yaml:"disable_twitch_codec_control"`
}

// DebugConfig holds debug and troubleshooting options
type DebugConfig struct {
	// Enable WebKit internal debug logging
	EnableWebKitDebug bool `mapstructure:"enable_webkit_debug" yaml:"enable_webkit_debug"`

	// WebKit debug categories (comma-separated)
	// Common values: "Network:preconnectTo", "ContentFilters", "Loading", "JavaScript"
	WebKitDebugCategories string `mapstructure:"webkit_debug_categories" yaml:"webkit_debug_categories"`

	// Enable content filtering debug logs
	EnableFilteringDebug bool `mapstructure:"enable_filtering_debug" yaml:"enable_filtering_debug"`

	// Enable detailed WebView state logging
	EnableWebViewDebug bool `mapstructure:"enable_webview_debug" yaml:"enable_webview_debug"`

	// Log WebKit crashes and errors to file
	LogWebKitCrashes bool `mapstructure:"log_webkit_crashes" yaml:"log_webkit_crashes"`

	// Enable script injection debug logs
	EnableScriptDebug bool `mapstructure:"enable_script_debug" yaml:"enable_script_debug"`

	// Enable general debug mode (equivalent to DUMBER_DEBUG env var)
	EnableGeneralDebug bool `mapstructure:"enable_general_debug" yaml:"enable_general_debug"`

	// Enable workspace navigation and focus debug logs
	EnableWorkspaceDebug bool `mapstructure:"enable_workspace_debug" yaml:"enable_workspace_debug"`

	// Enable focus state machine debug logs
	EnableFocusDebug bool `mapstructure:"enable_focus_debug" yaml:"enable_focus_debug"`

	// Enable CSS reconciler debug logs
	EnableCSSDebug bool `mapstructure:"enable_css_debug" yaml:"enable_css_debug"`

	// Enable focus metrics tracking
	EnableFocusMetrics bool `mapstructure:"enable_focus_metrics" yaml:"enable_focus_metrics"`

	// Enable detailed pane close instrumentation and tree snapshots
	EnablePaneCloseDebug bool `mapstructure:"enable_pane_close_debug" yaml:"enable_pane_close_debug"`
}

// WorkspaceConfig captures layout, pane, and tab behaviour preferences.
type WorkspaceConfig struct {
	// PaneMode defines modal pane behaviour and bindings.
	PaneMode PaneModeConfig `mapstructure:"pane_mode" yaml:"pane_mode" json:"pane_mode"`
	// Tabs holds classic browser tab shortcuts.
	Tabs TabKeyConfig `mapstructure:"tabs" yaml:"tabs" json:"tabs"`
	// Popups configures default popup placement rules.
	Popups PopupBehaviorConfig `mapstructure:"popups" yaml:"popups" json:"popups"`
	// Styling configures workspace visual appearance.
	Styling WorkspaceStylingConfig `mapstructure:"styling" yaml:"styling" json:"styling"`
}

// PaneModeConfig defines modal behaviour for pane management.
type PaneModeConfig struct {
	ActivationShortcut  string              `mapstructure:"activation_shortcut" yaml:"activation_shortcut" json:"activation_shortcut"`
	TimeoutMilliseconds int                 `mapstructure:"timeout_ms" yaml:"timeout_ms" json:"timeout_ms"`
	Actions             map[string][]string `mapstructure:"actions" yaml:"actions" json:"actions"`
}

// GetKeyBindings returns an inverted map for O(1) key→action lookup.
// This is built from the action→keys structure in the config.
func (p *PaneModeConfig) GetKeyBindings() map[string]string {
	keyToAction := make(map[string]string)
	for action, keys := range p.Actions {
		for _, key := range keys {
			keyToAction[key] = action
		}
	}
	return keyToAction
}

// TabKeyConfig defines Zellij-inspired tab shortcuts.
type TabKeyConfig struct {
	NewTab      string `mapstructure:"new_tab" yaml:"new_tab" json:"new_tab"`
	CloseTab    string `mapstructure:"close_tab" yaml:"close_tab" json:"close_tab"`
	NextTab     string `mapstructure:"next_tab" yaml:"next_tab" json:"next_tab"`
	PreviousTab string `mapstructure:"previous_tab" yaml:"previous_tab" json:"previous_tab"`
}

// PopupBehavior defines how popup windows should be opened
type PopupBehavior string

const (
	// PopupBehaviorSplit opens popups in a split pane (default)
	PopupBehaviorSplit PopupBehavior = "split"
	// PopupBehaviorStacked opens popups in a stacked pane
	PopupBehaviorStacked PopupBehavior = "stacked"
	// PopupBehaviorTabbed opens popups as a new tab
	PopupBehaviorTabbed PopupBehavior = "tabbed"
	// PopupBehaviorWindowed opens popups in a new workspace window
	PopupBehaviorWindowed PopupBehavior = "windowed"
)

// PopupBehaviorConfig defines handling for popup windows.
type PopupBehaviorConfig struct {
	// Behavior determines how popups are opened (split/stacked/tabbed/windowed)
	Behavior PopupBehavior `mapstructure:"behavior" yaml:"behavior" json:"behavior"`

	// Placement specifies direction for split behavior ("right", "left", "top", "bottom")
	// Only used when Behavior is "split"
	Placement string `mapstructure:"placement" yaml:"placement" json:"placement"`

	// OpenInNewPane controls whether popups are opened in workspace or blocked
	OpenInNewPane bool `mapstructure:"open_in_new_pane" yaml:"open_in_new_pane" json:"open_in_new_pane"`

	// FollowPaneContext determines if popup placement follows parent pane context
	FollowPaneContext bool `mapstructure:"follow_pane_context" yaml:"follow_pane_context" json:"follow_pane_context"`

	// BlankTargetBehavior determines how to handle window.open(url, "_blank") intents
	// Accepted values: "pane" (default) or "tab" (future support)
	BlankTargetBehavior string `mapstructure:"blank_target_behavior" yaml:"blank_target_behavior" json:"blank_target_behavior"`

	// EnableSmartDetection uses WebKitWindowProperties to detect popup vs tab intents
	EnableSmartDetection bool `mapstructure:"enable_smart_detection" yaml:"enable_smart_detection" json:"enable_smart_detection"`

	// OAuthAutoClose enables auto-closing OAuth popups after successful auth redirects
	OAuthAutoClose bool `mapstructure:"oauth_auto_close" yaml:"oauth_auto_close" json:"oauth_auto_close"`
}

// WorkspaceStylingConfig defines visual styling for workspace panes.
type WorkspaceStylingConfig struct {
	// BorderWidth in pixels for active pane borders
	BorderWidth int `mapstructure:"border_width" yaml:"border_width" json:"border_width"`
	// BorderColor for focused panes (CSS color value or theme variable)
	BorderColor string `mapstructure:"border_color" yaml:"border_color" json:"border_color"`
	// InactiveBorderWidth in pixels for inactive pane borders (0 = hidden)
	InactiveBorderWidth int `mapstructure:"inactive_border_width" yaml:"inactive_border_width" json:"inactive_border_width"`
	// InactiveBorderColor for unfocused panes (CSS color value or theme variable)
	InactiveBorderColor string `mapstructure:"inactive_border_color" yaml:"inactive_border_color" json:"inactive_border_color"`
	// ShowStackedTitleBorder enables the separator line below stacked pane titles
	ShowStackedTitleBorder bool `mapstructure:"show_stacked_title_border" yaml:"show_stacked_title_border" json:"show_stacked_title_border"`
	// PaneModeBorderColor for the pane mode indicator border (CSS color value or theme variable)
	// Defaults to "#FFA500" (orange) if not set
	PaneModeBorderColor string `mapstructure:"pane_mode_border_color" yaml:"pane_mode_border_color" json:"pane_mode_border_color"`
	// TransitionDuration in milliseconds for border animations
	TransitionDuration int `mapstructure:"transition_duration" yaml:"transition_duration" json:"transition_duration"`
	// BorderRadius in pixels for pane border corners
	BorderRadius int `mapstructure:"border_radius" yaml:"border_radius" json:"border_radius"`
}

// Manager handles configuration loading, watching, and reloading.
type Manager struct {
	config    *Config
	viper     *viper.Viper
	mu        sync.RWMutex
	callbacks []func(*Config)
	watching  bool
}

// NewManager creates a new configuration manager.
func NewManager() (*Manager, error) {
	v := viper.New()

	// Configure Viper - supports yaml, json, toml automatically
	v.SetConfigName("config") // Will find config.yaml, config.json, config.toml, etc.

	// Add config paths
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine config directory: %w\nCheck XDG_CONFIG_HOME environment variable or HOME directory", err)
	}
	v.AddConfigPath(configDir)
	v.AddConfigPath(".") // Current directory for development

	// Set up environment variable support
	v.SetEnvPrefix("DUMB_BROWSER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Note: Most environment variables are handled automatically via AutomaticEnv()
	// with the DUMB_BROWSER_ prefix (e.g., DUMB_BROWSER_DATABASE_PATH).
	// The explicit bindings below are only for special cases with different naming patterns.

	// Explicit bindings for DUMBER_* prefix (different from DUMB_BROWSER_)
	if err := v.BindEnv("rendering_mode", "DUMBER_RENDERING_MODE"); err != nil {
		return nil, fmt.Errorf("failed to bind DUMBER_RENDERING_MODE: %w", err)
	}
	if err := v.BindEnv("use_dom_zoom", "DUMBER_USE_DOM_ZOOM"); err != nil {
		return nil, fmt.Errorf("failed to bind DUMBER_USE_DOM_ZOOM: %w", err)
	}
	if err := v.BindEnv("default_zoom", "DUMBER_DEFAULT_ZOOM"); err != nil {
		return nil, fmt.Errorf("failed to bind DUMBER_DEFAULT_ZOOM: %w", err)
	}

	// Video acceleration environment variable bindings
	// These use system-standard env vars (LIBVA_*, GST_*, WEBKIT_*) and DUMBER_* prefix
	videoAccelEnvBindings := map[string]string{
		"video_acceleration.enable_vaapi":       "DUMBER_VIDEO_ACCELERATION_ENABLE",
		"video_acceleration.auto_detect_gpu":    "DUMBER_VIDEO_AUTO_DETECT",
		"video_acceleration.vaapi_driver_name":  "LIBVA_DRIVER_NAME",
		"video_acceleration.enable_all_drivers": "GST_VAAPI_ALL_DRIVERS",
		"video_acceleration.legacy_vaapi":       "WEBKIT_GST_ENABLE_LEGACY_VAAPI",
	}

	for key, env := range videoAccelEnvBindings {
		if err := v.BindEnv(key, env); err != nil {
			return nil, fmt.Errorf("failed to bind environment variable %s: %w", env, err)
		}
	}

	// Codec preferences environment variable bindings
	// These use DUMBER_* prefix for shorter, more convenient env var names
	codecEnvBindings := map[string]string{
		"codec_preferences.preferred_codecs":             "DUMBER_PREFERRED_CODECS",
		"codec_preferences.force_av1":                    "DUMBER_FORCE_AV1",
		"codec_preferences.block_vp9":                    "DUMBER_BLOCK_VP9",
		"codec_preferences.block_vp8":                    "DUMBER_BLOCK_VP8",
		"codec_preferences.av1_hardware_only":            "DUMBER_AV1_HW_ONLY",
		"codec_preferences.disable_vp9_hardware":         "DUMBER_DISABLE_VP9_HW",
		"codec_preferences.video_buffer_size_mb":         "DUMBER_VIDEO_BUFFER_MB",
		"codec_preferences.queue_buffer_time_sec":        "DUMBER_QUEUE_BUFFER_SEC",
		"codec_preferences.custom_user_agent":            "DUMBER_CUSTOM_UA",
		"codec_preferences.av1_max_resolution":           "DUMBER_AV1_MAX_RES",
		"codec_preferences.disable_twitch_codec_control": "DUMBER_DISABLE_TWITCH_CODEC",
	}

	for key, env := range codecEnvBindings {
		if err := v.BindEnv(key, env); err != nil {
			return nil, fmt.Errorf("failed to bind environment variable %s: %w", env, err)
		}
	}

	return &Manager{
		viper:     v,
		callbacks: make([]func(*Config), 0),
	}, nil
}

// Load loads the configuration from file and environment variables.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure directories exist
	if err := EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to ensure directories: %w", err)
	}

	// Set defaults
	m.setDefaults()

	// Read config file if it exists
	if err := m.viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			// Config file not found, create default one
			if err := m.createDefaultConfig(); err != nil {
				configDir, _ := GetConfigDir()
				return fmt.Errorf("failed to create default config at %s: %w\nTry creating the directory manually or check permissions", configDir, err)
			}
			// Re-read the newly created config file
			if err := m.viper.ReadInConfig(); err != nil {
				return fmt.Errorf("failed to read newly created config file: %w\nThe config file was created but couldn't be read. Please check the file format", err)
			}
		} else {
			configFile := m.viper.ConfigFileUsed()
			if configFile == "" {
				configDir, _ := GetConfigDir()
				configFile = filepath.Join(configDir, "config.*")
			}
			return fmt.Errorf("failed to read config file at %s: %w\nCheck the file format (must be valid TOML, JSON, or YAML) and permissions", configFile, err)
		}
	}

	// Unmarshal into config struct
	config := &Config{}
	if err := m.viper.Unmarshal(config); err != nil {
		configFile := m.viper.ConfigFileUsed()
		return fmt.Errorf("failed to parse config file at %s: %w\nCheck for syntax errors, invalid values, or type mismatches", configFile, err)
	}

	// Set database path if not specified
	if config.Database.Path == "" {
		dbPath, err := GetDatabaseFile()
		if err != nil {
			return fmt.Errorf("failed to get database path: %w", err)
		}
		config.Database.Path = dbPath
	}

	// Normalize/validate rendering mode
	switch strings.ToLower(string(config.RenderingMode)) {
	case "", string(RenderingModeAuto):
		config.RenderingMode = RenderingModeAuto
	case string(RenderingModeGPU):
		config.RenderingMode = RenderingModeGPU
	case string(RenderingModeCPU):
		config.RenderingMode = RenderingModeCPU
	default:
		config.RenderingMode = RenderingModeAuto
	}

	// Auto-detect GPU if enabled and driver name is not set
	if config.VideoAcceleration.AutoDetectGPU && config.VideoAcceleration.VAAPIDriverName == "" {
		gpuInfo := gpu.DetectGPU()
		if gpuInfo.SupportsVAAPI() {
			config.VideoAcceleration.VAAPIDriverName = gpuInfo.GetVAAPIDriverName()
		}
	}

	// Validate and configure codec preferences
	config = m.validateAndConfigureCodecPreferences(config)

	// Validate ColorScheme setting
	m.validateColorScheme(config)

	// Validate all config values
	if err := validateConfig(config); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	m.config = config
	return nil
}

// Get returns the current configuration (thread-safe).
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modification
	configCopy := *m.config
	return &configCopy
}

// Watch starts watching the config file for changes and reloads automatically.
func (m *Manager) Watch() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.watching {
		return nil // Already watching
	}

	m.viper.WatchConfig()
	m.viper.OnConfigChange(func(_ fsnotify.Event) {
		// Reload config
		if err := m.reload(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to reload config: %v\n", err)
			return
		}

		// Notify callbacks
		m.mu.RLock()
		config := m.config
		callbacks := make([]func(*Config), len(m.callbacks))
		copy(callbacks, m.callbacks)
		m.mu.RUnlock()

		for _, callback := range callbacks {
			callback(config)
		}
	})

	m.watching = true
	return nil
}

// OnConfigChange registers a callback function to be called when config changes.
func (m *Manager) OnConfigChange(callback func(*Config)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callbacks = append(m.callbacks, callback)
}

// reload reloads the configuration (internal method, must be called with lock).
func (m *Manager) reload() error {
	if err := m.viper.ReadInConfig(); err != nil {
		return err
	}

	config := &Config{}
	if err := m.viper.Unmarshal(config); err != nil {
		return err
	}

	// Set database path if not specified
	if config.Database.Path == "" {
		dbPath, err := GetDatabaseFile()
		if err != nil {
			return fmt.Errorf("failed to get database path: %w", err)
		}
		config.Database.Path = dbPath
	}

	// Normalize/validate rendering mode
	switch strings.ToLower(string(config.RenderingMode)) {
	case "", string(RenderingModeAuto):
		config.RenderingMode = RenderingModeAuto
	case string(RenderingModeGPU):
		config.RenderingMode = RenderingModeGPU
	case string(RenderingModeCPU):
		config.RenderingMode = RenderingModeCPU
	default:
		config.RenderingMode = RenderingModeAuto
	}

	// Auto-detect GPU if enabled and driver name is not set
	if config.VideoAcceleration.AutoDetectGPU && config.VideoAcceleration.VAAPIDriverName == "" {
		gpuInfo := gpu.DetectGPU()
		if gpuInfo.SupportsVAAPI() {
			config.VideoAcceleration.VAAPIDriverName = gpuInfo.GetVAAPIDriverName()
		}
	}

	// Validate and configure codec preferences
	config = m.validateAndConfigureCodecPreferences(config)

	// Validate ColorScheme setting
	m.validateColorScheme(config)

	// Validate all config values
	if err := validateConfig(config); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	m.config = config
	return nil
}

// setDefaults sets default configuration values in Viper.
func (m *Manager) setDefaults() {
	defaults := DefaultConfig()

	// Note: Database.Path is set dynamically in Load(), no defaults needed

	// Note: Browser config removed - we build our own browser

	// History defaults
	m.viper.SetDefault("history.max_entries", defaults.History.MaxEntries)
	m.viper.SetDefault("history.retention_period_days", defaults.History.RetentionPeriodDays)
	m.viper.SetDefault("history.cleanup_interval_days", defaults.History.CleanupIntervalDays)

	// Search shortcuts defaults
	m.viper.SetDefault("search_shortcuts", defaults.SearchShortcuts)
	m.viper.SetDefault("default_search_engine", defaults.DefaultSearchEngine)

	// Dmenu defaults
	m.viper.SetDefault("dmenu.max_history_items", defaults.Dmenu.MaxHistoryItems)
	m.viper.SetDefault("dmenu.show_visit_count", defaults.Dmenu.ShowVisitCount)
	m.viper.SetDefault("dmenu.show_last_visited", defaults.Dmenu.ShowLastVisited)
	m.viper.SetDefault("dmenu.history_prefix", defaults.Dmenu.HistoryPrefix)
	m.viper.SetDefault("dmenu.shortcut_prefix", defaults.Dmenu.ShortcutPrefix)
	m.viper.SetDefault("dmenu.url_prefix", defaults.Dmenu.URLPrefix)
	m.viper.SetDefault("dmenu.date_format", defaults.Dmenu.DateFormat)
	m.viper.SetDefault("dmenu.sort_by_visit_count", defaults.Dmenu.SortByVisitCount)

	// Logging defaults
	m.viper.SetDefault("logging.level", defaults.Logging.Level)
	m.viper.SetDefault("logging.format", defaults.Logging.Format)
	m.viper.SetDefault("logging.filename", defaults.Logging.Filename)
	m.viper.SetDefault("logging.max_size", defaults.Logging.MaxSize)
	m.viper.SetDefault("logging.max_backups", defaults.Logging.MaxBackups)
	m.viper.SetDefault("logging.max_age", defaults.Logging.MaxAge)
	m.viper.SetDefault("logging.compress", defaults.Logging.Compress)
	m.viper.SetDefault("logging.log_dir", defaults.Logging.LogDir)
	m.viper.SetDefault("logging.enable_file_log", defaults.Logging.EnableFileLog)
	m.viper.SetDefault("logging.capture_stdout", defaults.Logging.CaptureStdout)
	m.viper.SetDefault("logging.capture_stderr", defaults.Logging.CaptureStderr)
	m.viper.SetDefault("logging.capture_c_output", defaults.Logging.CaptureCOutput)
	m.viper.SetDefault("logging.debug_file", defaults.Logging.DebugFile)
	m.viper.SetDefault("logging.verbose_webkit", defaults.Logging.VerboseWebKit)

	// Debug defaults
	m.viper.SetDefault("debug.enable_webkit_debug", defaults.Debug.EnableWebKitDebug)
	m.viper.SetDefault("debug.webkit_debug_categories", defaults.Debug.WebKitDebugCategories)
	m.viper.SetDefault("debug.enable_filtering_debug", defaults.Debug.EnableFilteringDebug)
	m.viper.SetDefault("debug.enable_webview_debug", defaults.Debug.EnableWebViewDebug)
	m.viper.SetDefault("debug.log_webkit_crashes", defaults.Debug.LogWebKitCrashes)
	m.viper.SetDefault("debug.enable_script_debug", defaults.Debug.EnableScriptDebug)
	m.viper.SetDefault("debug.enable_general_debug", defaults.Debug.EnableGeneralDebug)
	m.viper.SetDefault("debug.enable_workspace_debug", defaults.Debug.EnableWorkspaceDebug)
	m.viper.SetDefault("debug.enable_focus_debug", defaults.Debug.EnableFocusDebug)
	m.viper.SetDefault("debug.enable_css_debug", defaults.Debug.EnableCSSDebug)
	m.viper.SetDefault("debug.enable_focus_metrics", defaults.Debug.EnableFocusMetrics)
	m.viper.SetDefault("debug.enable_pane_close_debug", defaults.Debug.EnablePaneCloseDebug)

	// Appearance defaults
	m.viper.SetDefault("appearance.sans_font", defaults.Appearance.SansFont)
	m.viper.SetDefault("appearance.serif_font", defaults.Appearance.SerifFont)
	m.viper.SetDefault("appearance.monospace_font", defaults.Appearance.MonospaceFont)
	m.viper.SetDefault("appearance.default_font_size", defaults.Appearance.DefaultFontSize)
	m.viper.SetDefault("appearance.light_palette", defaults.Appearance.LightPalette)
	m.viper.SetDefault("appearance.dark_palette", defaults.Appearance.DarkPalette)
	m.viper.SetDefault("appearance.color_scheme", defaults.Appearance.ColorScheme)

	// Video acceleration defaults
	m.viper.SetDefault("video_acceleration.enable_vaapi", defaults.VideoAcceleration.EnableVAAPI)
	m.viper.SetDefault("video_acceleration.auto_detect_gpu", defaults.VideoAcceleration.AutoDetectGPU)
	m.viper.SetDefault("video_acceleration.vaapi_driver_name", defaults.VideoAcceleration.VAAPIDriverName)
	m.viper.SetDefault("video_acceleration.enable_all_drivers", defaults.VideoAcceleration.EnableAllDrivers)
	m.viper.SetDefault("video_acceleration.legacy_vaapi", defaults.VideoAcceleration.LegacyVAAPI)

	// Codec preferences defaults
	m.viper.SetDefault("codec_preferences.preferred_codecs", defaults.CodecPreferences.PreferredCodecs)
	m.viper.SetDefault("codec_preferences.force_av1", defaults.CodecPreferences.ForceAV1)
	m.viper.SetDefault("codec_preferences.block_vp9", defaults.CodecPreferences.BlockVP9)
	m.viper.SetDefault("codec_preferences.block_vp8", defaults.CodecPreferences.BlockVP8)
	m.viper.SetDefault("codec_preferences.av1_hardware_only", defaults.CodecPreferences.AV1HardwareOnly)
	m.viper.SetDefault("codec_preferences.disable_vp9_hardware", defaults.CodecPreferences.DisableVP9Hardware)
	m.viper.SetDefault("codec_preferences.video_buffer_size_mb", defaults.CodecPreferences.VideoBufferSizeMB)
	m.viper.SetDefault("codec_preferences.queue_buffer_time_sec", defaults.CodecPreferences.QueueBufferTimeSec)
	m.viper.SetDefault("codec_preferences.custom_user_agent", defaults.CodecPreferences.CustomUserAgent)
	m.viper.SetDefault("codec_preferences.av1_max_resolution", defaults.CodecPreferences.AV1MaxResolution)
	m.viper.SetDefault("codec_preferences.disable_twitch_codec_control", defaults.CodecPreferences.DisableTwitchCodecControl)

	// Rendering defaults
	m.viper.SetDefault("rendering_mode", string(RenderingModeGPU))
	m.viper.SetDefault("use_dom_zoom", defaults.UseDomZoom)
	m.viper.SetDefault("default_zoom", defaults.DefaultZoom)

	// Workspace defaults
	m.viper.SetDefault("workspace.pane_mode.activation_shortcut", defaults.Workspace.PaneMode.ActivationShortcut)
	m.viper.SetDefault("workspace.pane_mode.timeout_ms", defaults.Workspace.PaneMode.TimeoutMilliseconds)
	m.viper.SetDefault("workspace.pane_mode.actions", defaults.Workspace.PaneMode.Actions)
	m.viper.SetDefault("workspace.tabs.new_tab", defaults.Workspace.Tabs.NewTab)
	m.viper.SetDefault("workspace.tabs.close_tab", defaults.Workspace.Tabs.CloseTab)
	m.viper.SetDefault("workspace.tabs.next_tab", defaults.Workspace.Tabs.NextTab)
	m.viper.SetDefault("workspace.tabs.previous_tab", defaults.Workspace.Tabs.PreviousTab)
	m.viper.SetDefault("workspace.popups.behavior", string(defaults.Workspace.Popups.Behavior))
	m.viper.SetDefault("workspace.popups.placement", defaults.Workspace.Popups.Placement)
	m.viper.SetDefault("workspace.popups.open_in_new_pane", defaults.Workspace.Popups.OpenInNewPane)
	m.viper.SetDefault("workspace.popups.follow_pane_context", defaults.Workspace.Popups.FollowPaneContext)
	// New popup behaviour defaults
	m.viper.SetDefault("workspace.popups.blank_target_behavior", defaults.Workspace.Popups.BlankTargetBehavior)
	m.viper.SetDefault("workspace.popups.enable_smart_detection", defaults.Workspace.Popups.EnableSmartDetection)
	m.viper.SetDefault("workspace.popups.oauth_auto_close", defaults.Workspace.Popups.OAuthAutoClose)
	m.viper.SetDefault("workspace.styling.border_width", defaults.Workspace.Styling.BorderWidth)
	m.viper.SetDefault("workspace.styling.border_color", defaults.Workspace.Styling.BorderColor)
	m.viper.SetDefault("workspace.styling.inactive_border_width", defaults.Workspace.Styling.InactiveBorderWidth)
	m.viper.SetDefault("workspace.styling.inactive_border_color", defaults.Workspace.Styling.InactiveBorderColor)
	m.viper.SetDefault("workspace.styling.show_stacked_title_border", defaults.Workspace.Styling.ShowStackedTitleBorder)
	m.viper.SetDefault("workspace.styling.pane_mode_border_color", defaults.Workspace.Styling.PaneModeBorderColor)
	m.viper.SetDefault("workspace.styling.transition_duration", defaults.Workspace.Styling.TransitionDuration)
	m.viper.SetDefault("workspace.styling.border_radius", defaults.Workspace.Styling.BorderRadius)
}

// createDefaultConfig creates a default configuration file.
func (m *Manager) createDefaultConfig() error {
	configFile, err := GetConfigFile()
	if err != nil {
		return err
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(configFile), dirPerm); err != nil {
		return err
	}

	// Use Viper's SafeWriteConfigAs to avoid duplicate keys
	if err := m.viper.SafeWriteConfigAs(configFile); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Created default configuration file: %s (TOML format)\n", configFile)

	// Generate JSON schema file only for JSON configs (used for IDE autocompletion)
	if filepath.Ext(configFile) == ".json" {
		if err := GenerateSchemaFile(); err != nil {
			// Log error but don't fail config creation
			fmt.Fprintf(os.Stderr, "Warning: failed to generate config schema: %v\n", err)
		}
	}

	return nil
}

// GetConfigFile returns the path to the configuration file being used.
func (m *Manager) GetConfigFile() string {
	return m.viper.ConfigFileUsed()
}

// New returns a new default configuration instance.
// This is a convenience function for getting default config without the full manager.
func New() *Config {
	return DefaultConfig()
}

// Global configuration manager instance
var globalManager *Manager
var globalManagerOnce sync.Once

// Init initializes the global configuration manager.
func Init() error {
	var err error
	globalManagerOnce.Do(func() {
		globalManager, err = NewManager()
		if err != nil {
			return
		}
		err = globalManager.Load()
	})
	return err
}

// Get returns the global configuration.
func Get() *Config {
	if globalManager == nil {
		// Return defaults if not initialized
		return DefaultConfig()
	}
	return globalManager.Get()
}

// Watch starts watching the global configuration for changes.
func Watch() error {
	if globalManager == nil {
		return fmt.Errorf("configuration not initialized")
	}
	return globalManager.Watch()
}

// OnConfigChange registers a callback for global configuration changes.
func OnConfigChange(callback func(*Config)) {
	if globalManager == nil {
		return
	}
	globalManager.OnConfigChange(callback)
}

// validateAndConfigureCodecPreferences validates and auto-configures codec preferences based on GPU capabilities
func (m *Manager) validateAndConfigureCodecPreferences(config *Config) *Config {
	// Validate codec preferences
	if config.CodecPreferences.PreferredCodecs == "" {
		config.CodecPreferences.PreferredCodecs = "av1,h264,vp8"
		fmt.Printf("Config: Set default codec preferences: %s\n", config.CodecPreferences.PreferredCodecs)
	}

	// Auto-configure based on GPU capabilities
	if config.VideoAcceleration.AutoDetectGPU {
		gpuInfo := gpu.DetectGPU()

		// Check AV1 hardware support
		if config.CodecPreferences.AV1HardwareOnly && !gpuInfo.SupportsAV1Hardware() {
			// Disable AV1 hardware-only if GPU doesn't support it
			config.CodecPreferences.AV1HardwareOnly = false
			fmt.Printf("Config: GPU doesn't support AV1 hardware, enabling software fallback\n")
		}

		// Get detailed AV1 capabilities
		av1Caps := gpuInfo.GetAV1HardwareCapabilities()
		if av1Caps["decode"] {
			logging.Info("GPU supports AV1 hardware decode")

			// If GPU supports AV1, prefer it over VP9
			if !config.CodecPreferences.ForceAV1 &&
				!strings.HasPrefix(config.CodecPreferences.PreferredCodecs, "av1") {
				config.CodecPreferences.PreferredCodecs = "av1," + config.CodecPreferences.PreferredCodecs
				fmt.Printf("Config: Auto-enabled AV1 preference due to hardware support\n")
			}
		} else {
			// If no AV1 support, ensure VP9 hardware is disabled to prevent issues
			if !config.CodecPreferences.DisableVP9Hardware {
				config.CodecPreferences.DisableVP9Hardware = true
				fmt.Printf("Config: Auto-disabled VP9 hardware acceleration (no AV1 fallback)\n")
			}
		}

		// Log GPU-specific codec capabilities
		logging.Info(fmt.Sprintf("%s GPU codec capabilities - AV1: decode=%t, encode=%t, 10bit=%t",
			gpuInfo.Vendor, av1Caps["decode"], av1Caps["encode"], av1Caps["10bit"]))
	}

	return config
}

// validateColorScheme validates and normalizes the ColorScheme setting
func (m *Manager) validateColorScheme(config *Config) {
	switch config.Appearance.ColorScheme {
	case "prefer-dark", "prefer-light", ThemeDefault, "":
		// Valid values - no changes needed
		// Empty string is treated the same as "default"
		if config.Appearance.ColorScheme == "" {
			config.Appearance.ColorScheme = ThemeDefault
			logging.Info("Config: ColorScheme not set, defaulting to '" + ThemeDefault + "' (follows system)")
		}
	default:
		// Invalid value - warn and reset to default
		logging.Info(fmt.Sprintf("Config: Invalid color_scheme value '%s', valid values are: 'prefer-dark', 'prefer-light', '"+ThemeDefault+"'. Resetting to '"+ThemeDefault+"'",
			config.Appearance.ColorScheme))
		config.Appearance.ColorScheme = ThemeDefault
	}
}
