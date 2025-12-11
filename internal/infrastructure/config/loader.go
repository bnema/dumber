package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

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

	// Configure Viper for TOML as default format
	v.SetConfigName("config") // Name without extension
	v.SetConfigType("toml")   // TOML as default format

	// Add config paths
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine config directory: %w\nCheck XDG_CONFIG_HOME environment variable or HOME directory", err)
	}
	v.AddConfigPath(configDir)
	v.AddConfigPath(".") // Current directory for development

	// Set up environment variable support
	v.SetEnvPrefix("DUMBER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Note: Most environment variables are handled automatically via AutomaticEnv()
	// with the DUMBER_ prefix (e.g., DUMBER_DATABASE_PATH, DUMBER_LOGGING_LEVEL).
	// The explicit bindings below are only for special cases with different naming patterns.

	// Explicit bindings for legacy or system env vars
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

	// Logging environment variable bindings
	if err := v.BindEnv("logging.level", "DUMBER_LOG_LEVEL"); err != nil {
		return nil, fmt.Errorf("failed to bind DUMBER_LOG_LEVEL: %w", err)
	}
	if err := v.BindEnv("logging.format", "DUMBER_LOG_FORMAT"); err != nil {
		return nil, fmt.Errorf("failed to bind DUMBER_LOG_FORMAT: %w", err)
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
				configFile = filepath.Join(configDir, "config.toml")
			}
			return fmt.Errorf("failed to read config file at %s: %w\nCheck the file format (must be valid TOML) and permissions", configFile, err)
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

	// Normalize/validate rendering mode using switch statement
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

	// TODO: implement in Step 18 - Auto-detect GPU if enabled and driver name is not set
	// if config.VideoAcceleration.AutoDetectGPU && config.VideoAcceleration.VAAPIDriverName == "" {
	//     gpuInfo := gpu.DetectGPU()
	//     if gpuInfo.SupportsVAAPI() {
	//         config.VideoAcceleration.VAAPIDriverName = gpuInfo.GetVAAPIDriverName()
	//     }
	// }

	// TODO: implement in Step 18 - Validate and configure codec preferences based on GPU
	// config = m.validateAndConfigureCodecPreferences(config)

	// Validate ColorScheme setting using switch statement
	switch config.Appearance.ColorScheme {
	case "prefer-dark", "prefer-light", ThemeDefault:
		// Valid values - no changes needed
	case "":
		// Empty string is treated the same as "default"
		config.Appearance.ColorScheme = ThemeDefault
	default:
		// Invalid value - reset to default
		config.Appearance.ColorScheme = ThemeDefault
	}

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

// GetConfigFile returns the path to the configuration file being used.
func (m *Manager) GetConfigFile() string {
	return m.viper.ConfigFileUsed()
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

	// Ensure TOML format and write config
	m.viper.SetConfigType("toml")
	if err := m.viper.SafeWriteConfigAs(configFile); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Created default configuration file: %s (TOML format)\n", configFile)

	return nil
}

// setDefaults sets default configuration values in Viper.
func (m *Manager) setDefaults() {
	defaults := DefaultConfig()

	// Note: Database.Path is set dynamically in Load(), no defaults needed

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
	m.viper.SetDefault("logging.capture_console", defaults.Logging.CaptureConsole)
	m.viper.SetDefault("logging.debug_file", defaults.Logging.DebugFile)
	m.viper.SetDefault("logging.verbose_webkit", defaults.Logging.VerboseWebKit)

	// Debug defaults
	m.viper.SetDefault("debug.enable_devtools", defaults.Debug.EnableDevTools)
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
	m.viper.SetDefault("workspace.tab_mode.activation_shortcut", defaults.Workspace.TabMode.ActivationShortcut)
	m.viper.SetDefault("workspace.tab_mode.timeout_ms", defaults.Workspace.TabMode.TimeoutMilliseconds)
	m.viper.SetDefault("workspace.tab_mode.actions", defaults.Workspace.TabMode.Actions)
	m.viper.SetDefault("workspace.tabs.new_tab", defaults.Workspace.Tabs.NewTab)
	m.viper.SetDefault("workspace.tabs.close_tab", defaults.Workspace.Tabs.CloseTab)
	m.viper.SetDefault("workspace.tabs.next_tab", defaults.Workspace.Tabs.NextTab)
	m.viper.SetDefault("workspace.tabs.previous_tab", defaults.Workspace.Tabs.PreviousTab)
	m.viper.SetDefault("workspace.tab_bar_position", defaults.Workspace.TabBarPosition)
	m.viper.SetDefault("workspace.hide_tab_bar_when_single_tab", defaults.Workspace.HideTabBarWhenSingleTab)
	m.viper.SetDefault("workspace.popups.behavior", string(defaults.Workspace.Popups.Behavior))
	m.viper.SetDefault("workspace.popups.placement", defaults.Workspace.Popups.Placement)
	m.viper.SetDefault("workspace.popups.open_in_new_pane", defaults.Workspace.Popups.OpenInNewPane)
	m.viper.SetDefault("workspace.popups.follow_pane_context", defaults.Workspace.Popups.FollowPaneContext)
	m.viper.SetDefault("workspace.popups.blank_target_behavior", defaults.Workspace.Popups.BlankTargetBehavior)
	m.viper.SetDefault("workspace.popups.enable_smart_detection", defaults.Workspace.Popups.EnableSmartDetection)
	m.viper.SetDefault("workspace.popups.oauth_auto_close", defaults.Workspace.Popups.OAuthAutoClose)
	m.viper.SetDefault("workspace.styling.border_width", defaults.Workspace.Styling.BorderWidth)
	m.viper.SetDefault("workspace.styling.border_color", defaults.Workspace.Styling.BorderColor)
	m.viper.SetDefault("workspace.styling.pane_mode_border_width", defaults.Workspace.Styling.PaneModeBorderWidth)
	m.viper.SetDefault("workspace.styling.pane_mode_border_color", defaults.Workspace.Styling.PaneModeBorderColor)
	m.viper.SetDefault("workspace.styling.tab_mode_border_width", defaults.Workspace.Styling.TabModeBorderWidth)
	m.viper.SetDefault("workspace.styling.tab_mode_border_color", defaults.Workspace.Styling.TabModeBorderColor)
	m.viper.SetDefault("workspace.styling.transition_duration", defaults.Workspace.Styling.TransitionDuration)
	m.viper.SetDefault("workspace.styling.ui_scale", defaults.Workspace.Styling.UIScale)

	// Content filtering
	m.viper.SetDefault("content_filtering.enabled", defaults.ContentFiltering.Enabled)
	m.viper.SetDefault("content_filtering.filter_lists", defaults.ContentFiltering.FilterLists)

	// Omnibox defaults
	m.viper.SetDefault("omnibox.initial_behavior", defaults.Omnibox.InitialBehavior)
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

// GetManager returns the global configuration manager.
// This is useful for accessing watcher functionality.
func GetManager() *Manager {
	return globalManager
}
