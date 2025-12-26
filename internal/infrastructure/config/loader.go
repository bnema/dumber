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

	if err := EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to ensure directories: %w", err)
	}

	m.setDefaults()

	if err := m.readConfigFile(); err != nil {
		return err
	}

	config, err := m.unmarshalConfig()
	if err != nil {
		return err
	}
	if err := ensureDatabasePath(config); err != nil {
		return err
	}
	normalizeConfig(config)

	if err := validateConfig(config); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	m.config = config
	return nil
}

func (m *Manager) readConfigFile() error {
	if err := m.viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			if createErr := m.createDefaultConfig(); createErr != nil {
				configDir, _ := GetConfigDir()
				return fmt.Errorf(
					"failed to create default config at %s: %w\nTry creating the directory manually or check permissions",
					configDir,
					createErr,
				)
			}
			if rereadErr := m.viper.ReadInConfig(); rereadErr != nil {
				return fmt.Errorf(
					"failed to read newly created config file: %w\nThe config file was created but couldn't be read. Please check the file format",
					rereadErr,
				)
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
	return nil
}

func (m *Manager) unmarshalConfig() (*Config, error) {
	config := &Config{}
	if err := m.viper.Unmarshal(config); err != nil {
		configFile := m.viper.ConfigFileUsed()
		return nil, fmt.Errorf(
			"failed to parse config file at %s: %w\nCheck for syntax errors, invalid values, or type mismatches",
			configFile,
			err,
		)
	}
	return config, nil
}

func ensureDatabasePath(config *Config) error {
	if config.Database.Path != "" {
		return nil
	}
	dbPath, err := GetDatabaseFile()
	if err != nil {
		return fmt.Errorf("failed to get database path: %w", err)
	}
	config.Database.Path = dbPath
	return nil
}

func normalizeConfig(config *Config) {
	switch strings.ToLower(string(config.Rendering.Mode)) {
	case "", string(RenderingModeAuto):
		config.Rendering.Mode = RenderingModeAuto
	case string(RenderingModeGPU):
		config.Rendering.Mode = RenderingModeGPU
	case string(RenderingModeCPU):
		config.Rendering.Mode = RenderingModeCPU
	default:
		config.Rendering.Mode = RenderingModeAuto
	}

	switch strings.ToLower(string(config.Rendering.GSKRenderer)) {
	case "", string(GSKRendererAuto):
		config.Rendering.GSKRenderer = GSKRendererAuto
	case string(GSKRendererOpenGL):
		config.Rendering.GSKRenderer = GSKRendererOpenGL
	case string(GSKRendererVulkan):
		config.Rendering.GSKRenderer = GSKRendererVulkan
	case string(GSKRendererCairo):
		config.Rendering.GSKRenderer = GSKRendererCairo
	default:
		config.Rendering.GSKRenderer = GSKRendererAuto
	}

	switch config.Appearance.ColorScheme {
	case ThemePreferDark, ThemePreferLight, ThemeDefault:
	case "":
		config.Appearance.ColorScheme = ThemeDefault
	default:
		config.Appearance.ColorScheme = ThemeDefault
	}

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

	switch strings.ToLower(string(config.Media.GLRenderingMode)) {
	case "", string(GLRenderingModeAuto):
		config.Media.GLRenderingMode = GLRenderingModeAuto
	case string(GLRenderingModeGLES2):
		config.Media.GLRenderingMode = GLRenderingModeGLES2
	case string(GLRenderingModeGL3):
		config.Media.GLRenderingMode = GLRenderingModeGL3
	case string(GLRenderingModeNone):
		config.Media.GLRenderingMode = GLRenderingModeNone
	default:
		config.Media.GLRenderingMode = GLRenderingModeAuto
	}

	config.Runtime.Prefix = strings.TrimSpace(config.Runtime.Prefix)
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

	m.setHistoryDefaults(defaults)
	m.setSearchDefaults(defaults)
	m.setDmenuDefaults(defaults)
	m.setLoggingDefaults(defaults)
	m.setDebugDefaults(defaults)
	m.setAppearanceDefaults(defaults)
	m.setRenderingDefaults(defaults)
	m.setWorkspaceDefaults(defaults)
	m.setContentFilteringDefaults(defaults)
	m.setOmniboxDefaults(defaults)
	m.setMediaDefaults(defaults)
	m.setRuntimeDefaults(defaults)
	m.setSessionDefaults(defaults)
	m.setUpdateDefaults(defaults)
}

func (m *Manager) setHistoryDefaults(defaults *Config) {
	m.viper.SetDefault("history.max_entries", defaults.History.MaxEntries)
	m.viper.SetDefault("history.retention_period_days", defaults.History.RetentionPeriodDays)
	m.viper.SetDefault("history.cleanup_interval_days", defaults.History.CleanupIntervalDays)
}

func (m *Manager) setSearchDefaults(defaults *Config) {
	m.viper.SetDefault("search_shortcuts", defaults.SearchShortcuts)
	m.viper.SetDefault("default_search_engine", defaults.DefaultSearchEngine)
}

func (m *Manager) setDmenuDefaults(defaults *Config) {
	m.viper.SetDefault("dmenu.max_history_items", defaults.Dmenu.MaxHistoryItems)
	m.viper.SetDefault("dmenu.show_visit_count", defaults.Dmenu.ShowVisitCount)
	m.viper.SetDefault("dmenu.show_last_visited", defaults.Dmenu.ShowLastVisited)
	m.viper.SetDefault("dmenu.history_prefix", defaults.Dmenu.HistoryPrefix)
	m.viper.SetDefault("dmenu.shortcut_prefix", defaults.Dmenu.ShortcutPrefix)
	m.viper.SetDefault("dmenu.url_prefix", defaults.Dmenu.URLPrefix)
	m.viper.SetDefault("dmenu.date_format", defaults.Dmenu.DateFormat)
	m.viper.SetDefault("dmenu.sort_by_visit_count", defaults.Dmenu.SortByVisitCount)
}

func (m *Manager) setLoggingDefaults(defaults *Config) {
	m.viper.SetDefault("logging.level", defaults.Logging.Level)
	m.viper.SetDefault("logging.format", defaults.Logging.Format)
	m.viper.SetDefault("logging.max_age", defaults.Logging.MaxAge)
	m.viper.SetDefault("logging.log_dir", defaults.Logging.LogDir)
	m.viper.SetDefault("logging.enable_file_log", defaults.Logging.EnableFileLog)
	m.viper.SetDefault("logging.capture_console", defaults.Logging.CaptureConsole)
}

func (m *Manager) setDebugDefaults(defaults *Config) {
	m.viper.SetDefault("debug.enable_devtools", defaults.Debug.EnableDevTools)
}

func (m *Manager) setAppearanceDefaults(defaults *Config) {
	m.viper.SetDefault("appearance.sans_font", defaults.Appearance.SansFont)
	m.viper.SetDefault("appearance.serif_font", defaults.Appearance.SerifFont)
	m.viper.SetDefault("appearance.monospace_font", defaults.Appearance.MonospaceFont)
	m.viper.SetDefault("appearance.default_font_size", defaults.Appearance.DefaultFontSize)
	m.viper.SetDefault("appearance.light_palette", defaults.Appearance.LightPalette)
	m.viper.SetDefault("appearance.dark_palette", defaults.Appearance.DarkPalette)
	m.viper.SetDefault("appearance.color_scheme", defaults.Appearance.ColorScheme)
}

func (m *Manager) setRenderingDefaults(defaults *Config) {
	m.viper.SetDefault("rendering.mode", string(defaults.Rendering.Mode))
	m.viper.SetDefault("rendering.disable_dmabuf_renderer", defaults.Rendering.DisableDMABufRenderer)
	m.viper.SetDefault("rendering.force_compositing_mode", defaults.Rendering.ForceCompositingMode)
	m.viper.SetDefault("rendering.disable_compositing_mode", defaults.Rendering.DisableCompositingMode)
	m.viper.SetDefault("rendering.gsk_renderer", string(defaults.Rendering.GSKRenderer))
	m.viper.SetDefault("rendering.disable_mipmaps", defaults.Rendering.DisableMipmaps)
	m.viper.SetDefault("rendering.prefer_gl", defaults.Rendering.PreferGL)
	m.viper.SetDefault("rendering.draw_compositing_indicators", defaults.Rendering.DrawCompositingIndicators)
	m.viper.SetDefault("rendering.show_fps", defaults.Rendering.ShowFPS)
	m.viper.SetDefault("rendering.sample_memory", defaults.Rendering.SampleMemory)
	m.viper.SetDefault("rendering.debug_frames", defaults.Rendering.DebugFrames)
	m.viper.SetDefault("default_webpage_zoom", defaults.DefaultWebpageZoom)
	m.viper.SetDefault("default_ui_scale", defaults.DefaultUIScale)
}

func (m *Manager) setWorkspaceDefaults(defaults *Config) {
	m.viper.SetDefault("workspace.pane_mode.activation_shortcut", defaults.Workspace.PaneMode.ActivationShortcut)
	m.viper.SetDefault("workspace.pane_mode.timeout_ms", defaults.Workspace.PaneMode.TimeoutMilliseconds)
	m.viper.SetDefault("workspace.pane_mode.actions", defaults.Workspace.PaneMode.Actions)
	m.viper.SetDefault("workspace.tab_mode.activation_shortcut", defaults.Workspace.TabMode.ActivationShortcut)
	m.viper.SetDefault("workspace.tab_mode.timeout_ms", defaults.Workspace.TabMode.TimeoutMilliseconds)
	m.viper.SetDefault("workspace.tab_mode.actions", defaults.Workspace.TabMode.Actions)
	m.viper.SetDefault("workspace.resize_mode.activation_shortcut", defaults.Workspace.ResizeMode.ActivationShortcut)
	m.viper.SetDefault("workspace.resize_mode.timeout_ms", defaults.Workspace.ResizeMode.TimeoutMilliseconds)
	m.viper.SetDefault("workspace.resize_mode.step_percent", defaults.Workspace.ResizeMode.StepPercent)
	m.viper.SetDefault("workspace.resize_mode.min_pane_percent", defaults.Workspace.ResizeMode.MinPanePercent)
	m.viper.SetDefault("workspace.resize_mode.actions", defaults.Workspace.ResizeMode.Actions)
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
	m.viper.SetDefault("workspace.styling.session_mode_border_width", defaults.Workspace.Styling.SessionModeBorderWidth)
	m.viper.SetDefault("workspace.styling.session_mode_border_color", defaults.Workspace.Styling.SessionModeBorderColor)
	m.viper.SetDefault("workspace.styling.resize_mode_border_width", defaults.Workspace.Styling.ResizeModeBorderWidth)
	m.viper.SetDefault("workspace.styling.resize_mode_border_color", defaults.Workspace.Styling.ResizeModeBorderColor)
	m.viper.SetDefault("workspace.styling.transition_duration", defaults.Workspace.Styling.TransitionDuration)
}

func (m *Manager) setContentFilteringDefaults(defaults *Config) {
	m.viper.SetDefault("content_filtering.enabled", defaults.ContentFiltering.Enabled)
	m.viper.SetDefault("content_filtering.auto_update", defaults.ContentFiltering.AutoUpdate)
}

func (m *Manager) setOmniboxDefaults(defaults *Config) {
	m.viper.SetDefault("omnibox.initial_behavior", defaults.Omnibox.InitialBehavior)
}

func (m *Manager) setMediaDefaults(defaults *Config) {
	m.viper.SetDefault("media.hardware_decoding", string(defaults.Media.HardwareDecodingMode))
	m.viper.SetDefault("media.prefer_av1", defaults.Media.PreferAV1)
	m.viper.SetDefault("media.show_diagnostics", defaults.Media.ShowDiagnosticsOnStartup)
	m.viper.SetDefault("media.force_vsync", defaults.Media.ForceVSync)
	m.viper.SetDefault("media.gl_rendering_mode", string(defaults.Media.GLRenderingMode))
	m.viper.SetDefault("media.gstreamer_debug_level", defaults.Media.GStreamerDebugLevel)
	m.viper.SetDefault("media.video_buffer_size_mb", defaults.Media.VideoBufferSizeMB)
	m.viper.SetDefault("media.queue_buffer_time_sec", defaults.Media.QueueBufferTimeSec)
}

func (m *Manager) setRuntimeDefaults(defaults *Config) {
	m.viper.SetDefault("runtime.prefix", defaults.Runtime.Prefix)
}

func (m *Manager) setSessionDefaults(defaults *Config) {
	m.viper.SetDefault("session.auto_restore", defaults.Session.AutoRestore)
	m.viper.SetDefault("session.snapshot_interval_ms", defaults.Session.SnapshotIntervalMs)
	m.viper.SetDefault("session.max_exited_sessions", defaults.Session.MaxExitedSessions)
	m.viper.SetDefault("session.max_exited_session_age_days", defaults.Session.MaxExitedSessionAgeDays)
	m.viper.SetDefault("session.session_mode.activation_shortcut", defaults.Session.SessionMode.ActivationShortcut)
	m.viper.SetDefault("session.session_mode.timeout_ms", defaults.Session.SessionMode.TimeoutMilliseconds)
	m.viper.SetDefault("session.session_mode.actions", defaults.Session.SessionMode.Actions)
}

func (m *Manager) setUpdateDefaults(defaults *Config) {
	m.viper.SetDefault("update.enable_on_startup", defaults.Update.EnableOnStartup)
	m.viper.SetDefault("update.auto_download", defaults.Update.AutoDownload)
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
