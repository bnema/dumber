// Package config provides configuration management for dumber with Viper integration.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	Dmenu             DmenuConfig               `mapstructure:"dmenu" yaml:"dmenu"`
	Logging           LoggingConfig             `mapstructure:"logging" yaml:"logging"`
	Appearance        AppearanceConfig          `mapstructure:"appearance" yaml:"appearance"`
	VideoAcceleration VideoAccelerationConfig   `mapstructure:"video_acceleration" yaml:"video_acceleration"`
	CodecPreferences  CodecConfig               `mapstructure:"codec_preferences" yaml:"codec_preferences"`
	WebkitMemory      WebkitMemoryConfig        `mapstructure:"webkit_memory" yaml:"webkit_memory"`
	Debug             DebugConfig               `mapstructure:"debug" yaml:"debug"`
	APISecurity       APISecurityConfig         `mapstructure:"api_security" yaml:"api_security"`
	// RenderingMode controls GPU/CPU rendering selection for WebKit
	RenderingMode RenderingMode `mapstructure:"rendering_mode" yaml:"rendering_mode"`
	// UseDomZoom toggles DOM-based zoom instead of native WebKit zoom.
	UseDomZoom bool `mapstructure:"use_dom_zoom" yaml:"use_dom_zoom"`
	// Workspace defines workspace, pane, and tab handling behaviour.
	Workspace WorkspaceConfig `mapstructure:"workspace" yaml:"workspace"`
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
	Path           string        `mapstructure:"path" yaml:"path"`
	MaxConnections int           `mapstructure:"max_connections" yaml:"max_connections"`
	MaxIdleTime    time.Duration `mapstructure:"max_idle_time" yaml:"max_idle_time"`
	QueryTimeout   time.Duration `mapstructure:"query_timeout" yaml:"query_timeout"`
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
	CaptureStdout  bool `mapstructure:"capture_stdout" yaml:"capture_stdout"`
	CaptureStderr  bool `mapstructure:"capture_stderr" yaml:"capture_stderr"`
	CaptureCOutput bool `mapstructure:"capture_c_output" yaml:"capture_c_output"`

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
	DefaultFontSize int `mapstructure:"default_font_size" yaml:"default_font_size"`
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

// WebkitMemoryConfig holds WebKit memory optimization settings
type WebkitMemoryConfig struct {
	// Cache model (document_viewer | web_browser | primary_web_browser)
	CacheModel string `mapstructure:"cache_model" yaml:"cache_model"`

	// Enable page cache for back/forward navigation
	EnablePageCache bool `mapstructure:"enable_page_cache" yaml:"enable_page_cache"`

	// Memory limit in MB (0 = system default)
	MemoryLimitMB int `mapstructure:"memory_limit_mb" yaml:"memory_limit_mb"`

	// Memory pressure thresholds (0.0-1.0)
	ConservativeThreshold float64 `mapstructure:"conservative_threshold" yaml:"conservative_threshold"`
	StrictThreshold       float64 `mapstructure:"strict_threshold" yaml:"strict_threshold"`
	KillThreshold         float64 `mapstructure:"kill_threshold" yaml:"kill_threshold"`

	// Memory monitoring interval in seconds
	PollIntervalSeconds float64 `mapstructure:"poll_interval_seconds" yaml:"poll_interval_seconds"`

	// Garbage collection interval in seconds (0 = disabled)
	EnableGCInterval int `mapstructure:"enable_gc_interval" yaml:"enable_gc_interval"`

	// Process recycling threshold (number of page loads)
	ProcessRecycleThreshold int `mapstructure:"process_recycle_threshold" yaml:"process_recycle_threshold"`

	// Enable memory monitoring logs
	EnableMemoryMonitoring bool `mapstructure:"enable_memory_monitoring" yaml:"enable_memory_monitoring"`
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
}

// APISecurityConfig holds optional API key protection for dumb://api endpoints
type APISecurityConfig struct {
	// If non-empty, token that requests must present via `token` query param
	Token string `mapstructure:"token" yaml:"token"`
	// If true, require token for all API endpoints (except a minimal allowlist)
	RequireToken bool `mapstructure:"require_token" yaml:"require_token"`
}

// WorkspaceConfig captures layout, pane, and tab behaviour preferences.
type WorkspaceConfig struct {
	// EnableZellijControls toggles Zellij-inspired keybindings.
	EnableZellijControls bool `mapstructure:"enable_zellij_controls" yaml:"enable_zellij_controls" json:"enable_zellij_controls"`
	// PaneMode defines modal pane behaviour and bindings.
	PaneMode PaneModeConfig `mapstructure:"pane_mode" yaml:"pane_mode" json:"pane_mode"`
	// Tabs holds classic browser tab shortcuts.
	Tabs TabKeyConfig `mapstructure:"tabs" yaml:"tabs" json:"tabs"`
	// Popups configures default popup placement rules.
	Popups PopupBehaviorConfig `mapstructure:"popups" yaml:"popups" json:"popups"`
}

// PaneModeConfig defines modal behaviour for pane management.
type PaneModeConfig struct {
	ActivationShortcut  string            `mapstructure:"activation_shortcut" yaml:"activation_shortcut" json:"activation_shortcut"`
	TimeoutMilliseconds int               `mapstructure:"timeout_ms" yaml:"timeout_ms" json:"timeout_ms"`
	ActionBindings      map[string]string `mapstructure:"action_bindings" yaml:"action_bindings" json:"action_bindings"`
}

// TabKeyConfig defines Zellij-inspired tab shortcuts.
type TabKeyConfig struct {
	NewTab      string `mapstructure:"new_tab" yaml:"new_tab" json:"new_tab"`
	CloseTab    string `mapstructure:"close_tab" yaml:"close_tab" json:"close_tab"`
	NextTab     string `mapstructure:"next_tab" yaml:"next_tab" json:"next_tab"`
	PreviousTab string `mapstructure:"previous_tab" yaml:"previous_tab" json:"previous_tab"`
}

// PopupBehaviorConfig defines handling for popup windows.
type PopupBehaviorConfig struct {
	Placement         string `mapstructure:"placement" yaml:"placement" json:"placement"`
	OpenInNewPane     bool   `mapstructure:"open_in_new_pane" yaml:"open_in_new_pane" json:"open_in_new_pane"`
	FollowPaneContext bool   `mapstructure:"follow_pane_context" yaml:"follow_pane_context" json:"follow_pane_context"`
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
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}
	v.AddConfigPath(configDir)
	v.AddConfigPath(".") // Current directory for development

	// Set up environment variable support
	v.SetEnvPrefix("DUMB_BROWSER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Bind specific environment variables
	bindings := map[string]string{
		"database.path":             "DATABASE_PATH",
		"database.max_connections":  "DATABASE_MAX_CONNECTIONS",
		"database.max_idle_time":    "DATABASE_MAX_IDLE_TIME",
		"database.query_timeout":    "DATABASE_QUERY_TIMEOUT",
		"browser.command":           "BROWSER_COMMAND",
		"browser.timeout":           "BROWSER_TIMEOUT",
		"browser.detach_process":    "BROWSER_DETACH_PROCESS",
		"history.max_entries":       "HISTORY_MAX_ENTRIES",
		"history.retention_period":  "HISTORY_RETENTION_PERIOD",
		"history.cleanup_interval":  "HISTORY_CLEANUP_INTERVAL",
		"dmenu.max_history_items":   "DMENU_MAX_HISTORY_ITEMS",
		"dmenu.show_visit_count":    "DMENU_SHOW_VISIT_COUNT",
		"dmenu.show_last_visited":   "DMENU_SHOW_LAST_VISITED",
		"dmenu.history_prefix":      "DMENU_HISTORY_PREFIX",
		"dmenu.shortcut_prefix":     "DMENU_SHORTCUT_PREFIX",
		"dmenu.url_prefix":          "DMENU_URL_PREFIX",
		"dmenu.date_format":         "DMENU_DATE_FORMAT",
		"dmenu.sort_by_visit_count": "DMENU_SORT_BY_VISIT_COUNT",
		"logging.level":             "LOGGING_LEVEL",
		"logging.format":            "LOGGING_FORMAT",
		"logging.filename":          "LOGGING_FILENAME",
		"logging.max_size":          "LOGGING_MAX_SIZE",
		"logging.max_backups":       "LOGGING_MAX_BACKUPS",
		"logging.max_age":           "LOGGING_MAX_AGE",
		"logging.compress":          "LOGGING_COMPRESS",
		// API security
		"api_security.token":         "API_TOKEN",
		"api_security.require_token": "API_REQUIRE_TOKEN",
	}

	for key, env := range bindings {
		if err := v.BindEnv(key, "DUMB_BROWSER_"+env); err != nil {
			return nil, fmt.Errorf("failed to bind environment variable %s: %w", env, err)
		}
	}

	// Explicit binding for rendering mode via dedicated env var
	if err := v.BindEnv("rendering_mode", "DUMBER_RENDERING_MODE"); err != nil {
		return nil, fmt.Errorf("failed to bind DUMBER_RENDERING_MODE: %w", err)
	}
	if err := v.BindEnv("use_dom_zoom", "DUMBER_USE_DOM_ZOOM"); err != nil {
		return nil, fmt.Errorf("failed to bind DUMBER_USE_DOM_ZOOM: %w", err)
	}

	// Video acceleration environment variable bindings
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

	// WebKit memory environment variable bindings
	memoryEnvBindings := map[string]string{
		"webkit_memory.cache_model":               "DUMBER_CACHE_MODEL",
		"webkit_memory.enable_page_cache":         "DUMBER_ENABLE_PAGE_CACHE",
		"webkit_memory.memory_limit_mb":           "DUMBER_MEMORY_LIMIT_MB",
		"webkit_memory.conservative_threshold":    "DUMBER_MEMORY_CONSERVATIVE",
		"webkit_memory.strict_threshold":          "DUMBER_MEMORY_STRICT",
		"webkit_memory.kill_threshold":            "DUMBER_MEMORY_KILL",
		"webkit_memory.poll_interval_seconds":     "DUMBER_MEMORY_POLL_INTERVAL",
		"webkit_memory.enable_gc_interval":        "DUMBER_GC_INTERVAL",
		"webkit_memory.process_recycle_threshold": "DUMBER_RECYCLE_THRESHOLD",
		"webkit_memory.enable_memory_monitoring":  "DUMBER_ENABLE_MEMORY_MONITORING",
	}

	for key, env := range memoryEnvBindings {
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
				return fmt.Errorf("failed to create default config: %w", err)
			}
		} else {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Unmarshal into config struct
	config := &Config{}
	if err := m.viper.Unmarshal(config); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
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

	m.ensurePersistedDefaults(config)

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

	m.ensurePersistedDefaults(config)

	m.config = config
	return nil
}

// setDefaults sets default configuration values in Viper.
func (m *Manager) setDefaults() {
	defaults := DefaultConfig()

	// Database defaults
	m.viper.SetDefault("database.max_connections", defaults.Database.MaxConnections)
	m.viper.SetDefault("database.max_idle_time", defaults.Database.MaxIdleTime)
	m.viper.SetDefault("database.query_timeout", defaults.Database.QueryTimeout)

	// Note: Browser config removed - we build our own browser

	// History defaults
	m.viper.SetDefault("history.max_entries", defaults.History.MaxEntries)
	m.viper.SetDefault("history.retention_period_days", defaults.History.RetentionPeriodDays)
	m.viper.SetDefault("history.cleanup_interval_days", defaults.History.CleanupIntervalDays)

	// Search shortcuts defaults
	m.viper.SetDefault("search_shortcuts", defaults.SearchShortcuts)

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

	// Appearance defaults
	m.viper.SetDefault("appearance.sans_font", defaults.Appearance.SansFont)
	m.viper.SetDefault("appearance.serif_font", defaults.Appearance.SerifFont)
	m.viper.SetDefault("appearance.monospace_font", defaults.Appearance.MonospaceFont)
	m.viper.SetDefault("appearance.default_font_size", defaults.Appearance.DefaultFontSize)

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

	// WebKit memory defaults
	m.viper.SetDefault("webkit_memory.cache_model", defaults.WebkitMemory.CacheModel)
	m.viper.SetDefault("webkit_memory.enable_page_cache", defaults.WebkitMemory.EnablePageCache)
	m.viper.SetDefault("webkit_memory.memory_limit_mb", defaults.WebkitMemory.MemoryLimitMB)
	m.viper.SetDefault("webkit_memory.conservative_threshold", defaults.WebkitMemory.ConservativeThreshold)
	m.viper.SetDefault("webkit_memory.strict_threshold", defaults.WebkitMemory.StrictThreshold)
	m.viper.SetDefault("webkit_memory.kill_threshold", defaults.WebkitMemory.KillThreshold)
	m.viper.SetDefault("webkit_memory.poll_interval_seconds", defaults.WebkitMemory.PollIntervalSeconds)
	m.viper.SetDefault("webkit_memory.enable_gc_interval", defaults.WebkitMemory.EnableGCInterval)
	m.viper.SetDefault("webkit_memory.process_recycle_threshold", defaults.WebkitMemory.ProcessRecycleThreshold)
	m.viper.SetDefault("webkit_memory.enable_memory_monitoring", defaults.WebkitMemory.EnableMemoryMonitoring)

	// Rendering defaults
	m.viper.SetDefault("rendering_mode", string(RenderingModeGPU))
	m.viper.SetDefault("use_dom_zoom", defaults.UseDomZoom)

	// Workspace defaults
	m.viper.SetDefault("workspace.enable_zellij_controls", defaults.Workspace.EnableZellijControls)
	m.viper.SetDefault("workspace.pane_mode.activation_shortcut", defaults.Workspace.PaneMode.ActivationShortcut)
	m.viper.SetDefault("workspace.pane_mode.timeout_ms", defaults.Workspace.PaneMode.TimeoutMilliseconds)
	m.viper.SetDefault("workspace.pane_mode.action_bindings", defaults.Workspace.PaneMode.ActionBindings)
	m.viper.SetDefault("workspace.tabs.new_tab", defaults.Workspace.Tabs.NewTab)
	m.viper.SetDefault("workspace.tabs.close_tab", defaults.Workspace.Tabs.CloseTab)
	m.viper.SetDefault("workspace.tabs.next_tab", defaults.Workspace.Tabs.NextTab)
	m.viper.SetDefault("workspace.tabs.previous_tab", defaults.Workspace.Tabs.PreviousTab)
	m.viper.SetDefault("workspace.popups.placement", defaults.Workspace.Popups.Placement)
	m.viper.SetDefault("workspace.popups.open_in_new_pane", defaults.Workspace.Popups.OpenInNewPane)
	m.viper.SetDefault("workspace.popups.follow_pane_context", defaults.Workspace.Popups.FollowPaneContext)
}

func (m *Manager) ensurePersistedDefaults(cfg *Config) {
	if cfg == nil {
		return
	}

	if m.viper == nil {
		return
	}

	// Only persist when the key is missing from the user config file.
	if m.viper.InConfig("use_dom_zoom") {
		return
	}

	cfgFile := m.viper.ConfigFileUsed()
	if cfgFile == "" {
		return
	}

	m.viper.Set("use_dom_zoom", cfg.UseDomZoom)
	if err := m.viper.WriteConfig(); err != nil {
		logging.Warn(fmt.Sprintf("Config: failed to persist use_dom_zoom default to %s: %v", cfgFile, err))
		return
	}

	logging.Info(fmt.Sprintf("Config: persisted missing use_dom_zoom default (%v) to %s", cfg.UseDomZoom, cfgFile))
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

	// Get the default configuration
	defaultConfig := DefaultConfig()

	// Marshal to JSON with proper indentation
	configData, err := json.MarshalIndent(defaultConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal default config: %w", err)
	}

	// Write JSON config file
	if err := os.WriteFile(configFile, configData, filePerm); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Created default configuration file: %s\n", configFile)
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
