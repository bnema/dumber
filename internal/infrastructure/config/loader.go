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
	if err := v.BindEnv("default_webpage_zoom", "DUMBER_DEFAULT_WEBPAGE_ZOOM"); err != nil {
		return nil, fmt.Errorf("failed to bind DUMBER_DEFAULT_WEBPAGE_ZOOM: %w", err)
	}
	if err := v.BindEnv("default_ui_scale", "DUMBER_DEFAULT_UI_SCALE"); err != nil {
		return nil, fmt.Errorf("failed to bind DUMBER_DEFAULT_UI_SCALE: %w", err)
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

	// Validate Media.HardwareDecodingMode using switch statement
	switch strings.ToLower(string(config.Media.HardwareDecodingMode)) {
	case "", string(HardwareDecodingAuto):
		config.Media.HardwareDecodingMode = HardwareDecodingAuto
	case string(HardwareDecodingForce):
		config.Media.HardwareDecodingMode = HardwareDecodingForce
	case string(HardwareDecodingDisable):
		config.Media.HardwareDecodingMode = HardwareDecodingDisable
	default:
		config.Media.HardwareDecodingMode = HardwareDecodingAuto
	}

	// Normalize runtime prefix
	config.Runtime.Prefix = strings.TrimSpace(config.Runtime.Prefix)

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

// Save saves the provided configuration to disk and updates Viper.
func (m *Manager) Save(cfg *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	// Validate before writing so UI gets immediate errors.
	if err := validateConfig(cfg); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Update Viper with the new values.
	// Since we can't easily update Viper from a struct automatically while preserving
	// all settings, we update the main keys we care about.
	m.viper.Set("appearance.sans_font", cfg.Appearance.SansFont)
	m.viper.Set("appearance.serif_font", cfg.Appearance.SerifFont)
	m.viper.Set("appearance.monospace_font", cfg.Appearance.MonospaceFont)
	m.viper.Set("appearance.default_font_size", cfg.Appearance.DefaultFontSize)
	m.viper.Set("appearance.color_scheme", cfg.Appearance.ColorScheme)
	m.viper.Set("appearance.light_palette", cfg.Appearance.LightPalette)
	m.viper.Set("appearance.dark_palette", cfg.Appearance.DarkPalette)
	m.viper.Set("default_webpage_zoom", cfg.DefaultWebpageZoom)
	m.viper.Set("default_ui_scale", cfg.DefaultUIScale)
	m.viper.Set("default_search_engine", cfg.DefaultSearchEngine)

	if err := m.viper.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// We don't update m.config here manually because viper.OnConfigChange
	// will trigger reload() if Watch() is active.
	// If Watch() is not active, we should call reload() manually.
	if !m.watching {
		if err := m.reload(); err != nil {
			return err
		}
	}

	return nil
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
	m.viper.SetDefault("logging.max_age", defaults.Logging.MaxAge)
	m.viper.SetDefault("logging.log_dir", defaults.Logging.LogDir)
	m.viper.SetDefault("logging.enable_file_log", defaults.Logging.EnableFileLog)
	m.viper.SetDefault("logging.capture_console", defaults.Logging.CaptureConsole)

	// Debug defaults
	m.viper.SetDefault("debug.enable_devtools", defaults.Debug.EnableDevTools)

	// Appearance defaults
	m.viper.SetDefault("appearance.sans_font", defaults.Appearance.SansFont)
	m.viper.SetDefault("appearance.serif_font", defaults.Appearance.SerifFont)
	m.viper.SetDefault("appearance.monospace_font", defaults.Appearance.MonospaceFont)
	m.viper.SetDefault("appearance.default_font_size", defaults.Appearance.DefaultFontSize)
	m.viper.SetDefault("appearance.light_palette", defaults.Appearance.LightPalette)
	m.viper.SetDefault("appearance.dark_palette", defaults.Appearance.DarkPalette)
	m.viper.SetDefault("appearance.color_scheme", defaults.Appearance.ColorScheme)

	// Rendering defaults
	m.viper.SetDefault("rendering_mode", string(RenderingModeGPU))
	m.viper.SetDefault("default_webpage_zoom", defaults.DefaultWebpageZoom)
	m.viper.SetDefault("default_ui_scale", defaults.DefaultUIScale)

	// Workspace defaults
	m.viper.SetDefault("workspace.pane_mode.activation_shortcut", defaults.Workspace.PaneMode.ActivationShortcut)
	m.viper.SetDefault("workspace.pane_mode.timeout_ms", defaults.Workspace.PaneMode.TimeoutMilliseconds)
	m.viper.SetDefault("workspace.pane_mode.actions", defaults.Workspace.PaneMode.Actions)
	m.viper.SetDefault("workspace.tab_mode.activation_shortcut", defaults.Workspace.TabMode.ActivationShortcut)
	m.viper.SetDefault("workspace.tab_mode.timeout_ms", defaults.Workspace.TabMode.TimeoutMilliseconds)
	m.viper.SetDefault("workspace.tab_mode.actions", defaults.Workspace.TabMode.Actions)
	m.viper.SetDefault("workspace.shortcuts.close_pane", defaults.Workspace.Shortcuts.ClosePane)
	m.viper.SetDefault("workspace.shortcuts.next_tab", defaults.Workspace.Shortcuts.NextTab)
	m.viper.SetDefault("workspace.shortcuts.previous_tab", defaults.Workspace.Shortcuts.PreviousTab)
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

	// Content filtering
	m.viper.SetDefault("content_filtering.enabled", defaults.ContentFiltering.Enabled)
	m.viper.SetDefault("content_filtering.auto_update", defaults.ContentFiltering.AutoUpdate)

	// Omnibox defaults
	m.viper.SetDefault("omnibox.initial_behavior", defaults.Omnibox.InitialBehavior)

	// Media defaults
	m.viper.SetDefault("media.hardware_decoding", string(defaults.Media.HardwareDecodingMode))
	m.viper.SetDefault("media.prefer_av1", defaults.Media.PreferAV1)
	m.viper.SetDefault("media.show_diagnostics", defaults.Media.ShowDiagnosticsOnStartup)

	// Runtime defaults
	m.viper.SetDefault("runtime.prefix", defaults.Runtime.Prefix)
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
