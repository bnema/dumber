package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/fonts"
	"github.com/bnema/dumber/internal/logging"
	"github.com/spf13/viper"
)

// Manager handles configuration loading, watching, and reloading.
type Manager struct {
	config         *Config
	viper          *viper.Viper
	mu             sync.RWMutex
	callbacks      []func(*Config)
	watching       bool
	skipNextReload bool // Set by Save() to prevent fsnotify reload from overwriting in-memory config
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

	if err := m.checkLegacyFormat(); err != nil {
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

	// Transform legacy action bindings before unmarshaling
	m.transformLegacyActionBindings()

	return nil
}

// transformLegacyActionBindings converts old-format action bindings to new format.
// This is called after reading config but before unmarshaling.
func (m *Manager) transformLegacyActionBindings() {
	rawConfig := m.viper.AllSettings()
	transformer := NewLegacyConfigTransformer()
	transformer.TransformLegacyActions(rawConfig)

	// Apply transformed config back to viper
	for key, value := range rawConfig {
		m.viper.Set(key, value)
	}
}

// checkLegacyFormat detects old config format and returns an error directing user to migrate.
func (m *Manager) checkLegacyFormat() error {
	// Check if old sections exist by looking for known keys.
	// IsSet works for legacy keys because they do not have defaults, while
	// engine.type has a default and must be checked with InConfig.
	hasOldSections := m.viper.IsSet("rendering.mode") ||
		m.viper.IsSet("rendering.disable_dmabuf_renderer") ||
		m.viper.IsSet("performance.profile") ||
		m.viper.IsSet("privacy.cookie_policy") ||
		m.viper.IsSet("runtime.prefix")

	hasEngineSection := m.viper.InConfig("engine.type")

	if hasOldSections && !hasEngineSection {
		return fmt.Errorf(
			"config format outdated: [rendering], [performance], [privacy] sections " +
				"have moved to [engine]/[engine.webkit] — " +
				"run \"dumber config migrate\" to update your config file",
		)
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
	normalizeAppearance(config)
	normalizeMedia(config)
	normalizeEngineConfig(config)
}

func normalizeEngineConfig(config *Config) {
	// Normalize GSK renderer (engine.webkit)
	switch strings.ToLower(string(config.Engine.WebKit.GSKRenderer)) {
	case "", string(GSKRendererAuto):
		config.Engine.WebKit.GSKRenderer = GSKRendererAuto
	case string(GSKRendererOpenGL):
		config.Engine.WebKit.GSKRenderer = GSKRendererOpenGL
	case string(GSKRendererVulkan):
		config.Engine.WebKit.GSKRenderer = GSKRendererVulkan
	case string(GSKRendererCairo):
		config.Engine.WebKit.GSKRenderer = GSKRendererCairo
	default:
		config.Engine.WebKit.GSKRenderer = GSKRendererAuto
	}

	// Normalize cookie policy (engine).
	// When no value (or an invalid value) is set, respect the ITP setting: if ITP is
	// disabled the effective default is to block third-party cookies; if ITP is enabled
	// all cookies are accepted by default.  An explicit "always" value always wins.
	itpDefault := func() CookiePolicy {
		if config.Engine.WebKit.ITPEnabled {
			return CookiePolicyAlways
		}
		return CookiePolicyNoThirdParty
	}
	switch strings.ToLower(string(config.Engine.CookiePolicy)) {
	case "":
		config.Engine.CookiePolicy = itpDefault()
	case string(CookiePolicyAlways):
		config.Engine.CookiePolicy = CookiePolicyAlways
	case string(CookiePolicyNoThirdParty):
		config.Engine.CookiePolicy = CookiePolicyNoThirdParty
	case string(CookiePolicyNever):
		config.Engine.CookiePolicy = CookiePolicyNever
	default:
		config.Engine.CookiePolicy = itpDefault()
	}

	// Normalize performance profile (engine)
	switch strings.ToLower(string(config.Engine.Profile)) {
	case "", string(ProfileDefault):
		config.Engine.Profile = ProfileDefault
	case string(ProfileLite):
		config.Engine.Profile = ProfileLite
	case string(ProfileMax):
		config.Engine.Profile = ProfileMax
	case string(ProfileCustom):
		config.Engine.Profile = ProfileCustom
	default:
		config.Engine.Profile = ProfileDefault
	}

	// Trim runtime prefix (engine.webkit)
	config.Engine.WebKit.Prefix = strings.TrimSpace(config.Engine.WebKit.Prefix)
}

func normalizeAppearance(config *Config) {
	switch strings.ToLower(strings.TrimSpace(config.Appearance.ColorScheme)) {
	case strings.ToLower(ThemePreferDark):
		config.Appearance.ColorScheme = ThemePreferDark
	case strings.ToLower(ThemePreferLight):
		config.Appearance.ColorScheme = ThemePreferLight
	case strings.ToLower(ThemeDefault):
		config.Appearance.ColorScheme = ThemeDefault
	default:
		config.Appearance.ColorScheme = ThemeDefault
	}
}

func normalizeMedia(config *Config) {
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
}

// Get returns the current configuration (thread-safe).
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modification
	configCopy := *m.config
	return &configCopy
}

// Save saves the provided configuration to disk with ordered sections.
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

	// Write config with ordered sections (bypasses Viper's random map ordering)
	configFile := m.viper.ConfigFileUsed()
	if configFile == "" {
		var err error
		configFile, err = GetConfigFile()
		if err != nil {
			return fmt.Errorf("failed to determine config file path: %w", err)
		}
	}

	// Set flag BEFORE writing to prevent fsnotify-triggered reload from
	// overwriting our in-memory config with stale viper cache
	m.skipNextReload = true

	if err := WriteConfigOrdered(cfg, configFile); err != nil {
		m.skipNextReload = false // Reset on error
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Update in-memory config immediately (don't wait for file watcher)
	// This ensures subsequent Get() calls return the new values
	m.config = cfg

	// Also reload from disk to sync Viper's internal state (if not watching)
	if !m.watching {
		m.skipNextReload = false // Clear flag since we're doing manual reload
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

// createDefaultConfig creates a default configuration file with ordered sections.
func (m *Manager) createDefaultConfig() error {
	configFile, err := GetConfigFile()
	if err != nil {
		return err
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(configFile), dirPerm); err != nil {
		return err
	}

	// Start with default config
	cfg := DefaultConfig()

	// Detect available fonts and override defaults
	m.detectAndSetFontsOnConfig(cfg)

	// Write config with ordered sections
	if err := WriteConfigOrdered(cfg, configFile); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Created default configuration file: %s (TOML format)\n", configFile)

	return nil
}

// detectAndSetFontsOnConfig detects available system fonts and sets the best available
// fonts from the fallback chains directly on the Config struct.
func (*Manager) detectAndSetFontsOnConfig(cfg *Config) {
	// Create context with logger for debugging first-run font detection.
	logger := logging.NewFromEnv()
	ctx := logging.WithContext(context.Background(), logger)

	detector := fonts.NewDetector()

	if !detector.IsAvailable(ctx) {
		// fc-list not available, keep hardcoded defaults
		return
	}

	cfg.Appearance.SansFont = detector.SelectBestFont(ctx, port.FontCategorySansSerif, fonts.SansSerifFallbackChain())
	cfg.Appearance.SerifFont = detector.SelectBestFont(ctx, port.FontCategorySerif, fonts.SerifFallbackChain())
	cfg.Appearance.MonospaceFont = detector.SelectBestFont(ctx, port.FontCategoryMonospace, fonts.MonospaceFallbackChain())
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
	m.setEngineDefaults(defaults)
	m.setZoomAndScaleDefaults(defaults)
	m.setWorkspaceDefaults(defaults)
	m.setContentFilteringDefaults(defaults)
	m.setClipboardDefaults(defaults)
	m.setOmniboxDefaults(defaults)
	m.setMediaDefaults(defaults)
	m.setSessionDefaults(defaults)
	m.setUpdateDefaults(defaults)
	m.setDownloadsDefaults(defaults)
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
	m.viper.SetDefault("dmenu.max_history_days", defaults.Dmenu.MaxHistoryDays)
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
	m.viper.SetDefault("logging.capture_gtk_logs", defaults.Logging.CaptureGTKLogs)
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

func (m *Manager) setZoomAndScaleDefaults(defaults *Config) {
	m.viper.SetDefault("default_webpage_zoom", defaults.DefaultWebpageZoom)
	m.viper.SetDefault("default_ui_scale", defaults.DefaultUIScale)
}

func (m *Manager) setWorkspaceDefaults(defaults *Config) {
	m.viper.SetDefault("workspace.new_pane_url", defaults.Workspace.NewPaneURL)
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
	m.viper.SetDefault("workspace.shortcuts.actions", defaults.Workspace.Shortcuts.Actions)
	m.viper.SetDefault("workspace.floating_pane.width_pct", defaults.Workspace.FloatingPane.WidthPct)
	m.viper.SetDefault("workspace.floating_pane.height_pct", defaults.Workspace.FloatingPane.HeightPct)
	m.viper.SetDefault("workspace.floating_pane.profiles", defaults.Workspace.FloatingPane.Profiles)
	m.viper.SetDefault("workspace.tab_bar_position", defaults.Workspace.TabBarPosition)
	m.viper.SetDefault("workspace.hide_tab_bar_when_single_tab", defaults.Workspace.HideTabBarWhenSingleTab)
	m.viper.SetDefault("workspace.switch_to_tab_on_move", defaults.Workspace.SwitchToTabOnMove)
	m.viper.SetDefault("workspace.popups.behavior", string(defaults.Workspace.Popups.Behavior))
	m.viper.SetDefault("workspace.popups.placement", defaults.Workspace.Popups.Placement)
	m.viper.SetDefault("workspace.popups.open_in_new_pane", defaults.Workspace.Popups.OpenInNewPane)
	m.viper.SetDefault("workspace.popups.follow_pane_context", defaults.Workspace.Popups.FollowPaneContext)
	m.viper.SetDefault("workspace.popups.blank_target_behavior", defaults.Workspace.Popups.BlankTargetBehavior)
	m.viper.SetDefault("workspace.popups.enable_smart_detection", defaults.Workspace.Popups.EnableSmartDetection)
	m.viper.SetDefault("workspace.popups.oauth_auto_close", defaults.Workspace.Popups.OAuthAutoClose)
	m.viper.SetDefault("workspace.styling.border_width", defaults.Workspace.Styling.BorderWidth)
	m.viper.SetDefault("workspace.styling.border_color", defaults.Workspace.Styling.BorderColor)
	m.viper.SetDefault("workspace.styling.mode_border_width", defaults.Workspace.Styling.ModeBorderWidth)
	m.viper.SetDefault("workspace.styling.pane_mode_color", defaults.Workspace.Styling.PaneModeColor)
	m.viper.SetDefault("workspace.styling.tab_mode_color", defaults.Workspace.Styling.TabModeColor)
	m.viper.SetDefault("workspace.styling.session_mode_color", defaults.Workspace.Styling.SessionModeColor)
	m.viper.SetDefault("workspace.styling.resize_mode_color", defaults.Workspace.Styling.ResizeModeColor)
	m.viper.SetDefault("workspace.styling.mode_indicator_toaster_enabled", defaults.Workspace.Styling.ModeIndicatorToasterEnabled)
	m.viper.SetDefault("workspace.styling.transition_duration", defaults.Workspace.Styling.TransitionDuration)
}

func (m *Manager) setContentFilteringDefaults(defaults *Config) {
	m.viper.SetDefault("content_filtering.enabled", defaults.ContentFiltering.Enabled)
	m.viper.SetDefault("content_filtering.auto_update", defaults.ContentFiltering.AutoUpdate)
}

func (m *Manager) setClipboardDefaults(defaults *Config) {
	m.viper.SetDefault("clipboard.auto_copy_on_selection", defaults.Clipboard.AutoCopyOnSelection)
}

func (m *Manager) setOmniboxDefaults(defaults *Config) {
	m.viper.SetDefault("omnibox.initial_behavior", defaults.Omnibox.InitialBehavior)
	m.viper.SetDefault("omnibox.auto_open_on_new_pane", defaults.Omnibox.AutoOpenOnNewPane)
}

func (m *Manager) setMediaDefaults(defaults *Config) {
	// GStreamer fields (force_vsync, gl_rendering_mode, gstreamer_debug_level)
	// have moved to [engine.webkit] — only non-migrated media fields remain here.
	m.viper.SetDefault("media.hardware_decoding", string(defaults.Media.HardwareDecodingMode))
	m.viper.SetDefault("media.prefer_av1", defaults.Media.PreferAV1)
	m.viper.SetDefault("media.show_diagnostics", defaults.Media.ShowDiagnosticsOnStartup)
}

// setRuntimeDefaults removed — runtime.prefix moved to [engine.webkit].

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
	m.viper.SetDefault("update.notify_on_new_settings", defaults.Update.NotifyOnNewSettings)
}

// setPerformanceDefaults removed — all fields moved to [engine]/[engine.webkit].

func (m *Manager) setDownloadsDefaults(defaults *Config) {
	m.viper.SetDefault("downloads.path", defaults.Downloads.Path)
}

func (m *Manager) setEngineDefaults(defaults *Config) {
	e := defaults.Engine
	m.viper.SetDefault("engine.type", e.Type)
	m.viper.SetDefault("engine.profile", string(e.Profile))
	m.viper.SetDefault("engine.pool_prewarm_count", e.PoolPrewarmCount)
	m.viper.SetDefault("engine.zoom_cache_size", e.ZoomCacheSize)
	m.viper.SetDefault("engine.cookie_policy", string(e.CookiePolicy))

	ce := e.CEF
	m.viper.SetDefault("engine.cef.cef_dir", ce.CEFDir)
	m.viper.SetDefault("engine.cef.log_file", ce.LogFile)
	m.viper.SetDefault("engine.cef.log_severity", ce.LogSeverity)
	m.viper.SetDefault("engine.cef.windowless_frame_rate", ce.CEFWindowlessFrameRate())
	m.viper.SetDefault("engine.cef.multi_threaded_message_loop", ce.CEFMultiThreadedMessageLoop())
	m.viper.SetDefault("engine.cef.manual_pump_interval_ms", ce.CEFManualPumpIntervalMs())
	m.viper.SetDefault("engine.cef.enable_audio_handler", ce.EnableAudioHandler)
	m.viper.SetDefault("engine.cef.enable_context_menu_handler", ce.EnableContextMenuHandler)
	m.viper.SetDefault("engine.cef.trace_handlers", ce.TraceHandlers)

	wk := e.WebKit
	m.viper.SetDefault("engine.webkit.itp_enabled", wk.ITPEnabled)
	m.viper.SetDefault("engine.webkit.skia_cpu_painting_threads", wk.SkiaCPUPaintingThreads)
	m.viper.SetDefault("engine.webkit.skia_gpu_painting_threads", wk.SkiaGPUPaintingThreads)
	m.viper.SetDefault("engine.webkit.skia_enable_cpu_rendering", wk.SkiaEnableCPURendering)
	m.viper.SetDefault("engine.webkit.disable_dmabuf_renderer", wk.DisableDMABufRenderer)
	m.viper.SetDefault("engine.webkit.force_compositing_mode", wk.ForceCompositingMode)
	m.viper.SetDefault("engine.webkit.disable_compositing_mode", wk.DisableCompositingMode)
	m.viper.SetDefault("engine.webkit.gsk_renderer", string(wk.GSKRenderer))
	m.viper.SetDefault("engine.webkit.disable_mipmaps", wk.DisableMipmaps)
	m.viper.SetDefault("engine.webkit.prefer_gl", wk.PreferGL)
	m.viper.SetDefault("engine.webkit.draw_compositing_indicators", wk.DrawCompositingIndicators)
	m.viper.SetDefault("engine.webkit.show_fps", wk.ShowFPS)
	m.viper.SetDefault("engine.webkit.sample_memory", wk.SampleMemory)
	m.viper.SetDefault("engine.webkit.debug_frames", wk.DebugFrames)
	m.viper.SetDefault("engine.webkit.force_vsync", wk.ForceVSync)
	m.viper.SetDefault("engine.webkit.gl_rendering_mode", string(wk.GLRenderingMode))
	m.viper.SetDefault("engine.webkit.gstreamer_debug_level", wk.GStreamerDebugLevel)
	m.viper.SetDefault("engine.webkit.prefix", wk.Prefix)
	m.viper.SetDefault("engine.webkit.web_process_memory_limit_mb", wk.WebProcessMemoryLimitMB)
	m.viper.SetDefault("engine.webkit.web_process_memory_poll_interval_sec", wk.WebProcessMemoryPollIntervalSec)
	m.viper.SetDefault("engine.webkit.web_process_memory_conservative_threshold", wk.WebProcessMemoryConservativeThreshold)
	m.viper.SetDefault("engine.webkit.web_process_memory_strict_threshold", wk.WebProcessMemoryStrictThreshold)
	m.viper.SetDefault("engine.webkit.network_process_memory_limit_mb", wk.NetworkProcessMemoryLimitMB)
	m.viper.SetDefault("engine.webkit.network_process_memory_poll_interval_sec", wk.NetworkProcessMemoryPollIntervalSec)
	m.viper.SetDefault("engine.webkit.network_process_memory_conservative_threshold", wk.NetworkProcessMemoryConservativeThreshold)
	m.viper.SetDefault("engine.webkit.network_process_memory_strict_threshold", wk.NetworkProcessMemoryStrictThreshold)

	// Transcoding defaults.
	m.viper.SetDefault("transcoding.enabled", true)
	m.viper.SetDefault("transcoding.hwaccel", "auto")
	m.viper.SetDefault("transcoding.max_concurrent", 3)
	m.viper.SetDefault("transcoding.quality", "medium")
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
