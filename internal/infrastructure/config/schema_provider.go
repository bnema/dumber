package config

import (
	"fmt"

	"github.com/bnema/dumber/internal/domain/entity"
)

// Section names for grouping config keys.
const (
	SectionAppearance       = "Appearance"
	SectionLogging          = "Logging"
	SectionHistory          = "History"
	SectionWorkspace        = "Workspace"
	SectionSession          = "Session"
	SectionOmnibox          = "Omnibox"
	SectionClipboard        = "Clipboard"
	SectionContentFiltering = "Content Filtering"
	SectionRendering        = "Rendering"
	SectionMedia            = "Media"
	SectionUpdate           = "Update"
	SectionDmenu            = "Dmenu"
	SectionDebug            = "Debug"
	SectionPerformance      = "Performance"
	SectionRuntime          = "Runtime"
	SectionDatabase         = "Database"
	SectionSearch           = "Search"
	SectionDownloads        = "Downloads"
)

// SchemaProvider implements port.ConfigSchemaProvider.
type SchemaProvider struct{}

// NewSchemaProvider creates a new SchemaProvider.
func NewSchemaProvider() *SchemaProvider {
	return &SchemaProvider{}
}

// GetSchema returns all configuration keys with their metadata.
func (p *SchemaProvider) GetSchema() []entity.ConfigKeyInfo {
	defaults := DefaultConfig()

	keys := make([]entity.ConfigKeyInfo, 0, 100)

	// Appearance section
	keys = append(keys, p.getAppearanceKeys(defaults)...)

	// Logging section
	keys = append(keys, p.getLoggingKeys(defaults)...)

	// History section
	keys = append(keys, p.getHistoryKeys(defaults)...)

	// Search section
	keys = append(keys, p.getSearchKeys(defaults)...)

	// Dmenu section
	keys = append(keys, p.getDmenuKeys(defaults)...)

	// Workspace section
	keys = append(keys, p.getWorkspaceKeys(defaults)...)

	// Session section
	keys = append(keys, p.getSessionKeys(defaults)...)

	// Omnibox section
	keys = append(keys, p.getOmniboxKeys(defaults)...)

	// Clipboard section
	keys = append(keys, p.getClipboardKeys(defaults)...)

	// Content Filtering section
	keys = append(keys, p.getContentFilteringKeys(defaults)...)

	// Rendering section
	keys = append(keys, p.getRenderingKeys(defaults)...)

	// Media section
	keys = append(keys, p.getMediaKeys(defaults)...)

	// Update section
	keys = append(keys, p.getUpdateKeys(defaults)...)

	// Debug section
	keys = append(keys, p.getDebugKeys(defaults)...)

	// Performance section
	keys = append(keys, p.getPerformanceKeys(defaults)...)

	// Runtime section
	keys = append(keys, p.getRuntimeKeys(defaults)...)

	// Database section
	keys = append(keys, p.getDatabaseKeys()...)

	// Downloads section
	keys = append(keys, p.getDownloadsKeys(defaults)...)

	return keys
}

func (*SchemaProvider) getAppearanceKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "appearance.color_scheme",
			Type:        "string",
			Default:     defaults.Appearance.ColorScheme,
			Description: "Theme preference (follows system by default)",
			Values:      []string{"default", "prefer-dark", "prefer-light"},
			Section:     SectionAppearance,
		},
		{
			Key:         "appearance.default_font_size",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Appearance.DefaultFontSize),
			Description: "Default font size in CSS pixels",
			Range:       "1-72",
			Section:     SectionAppearance,
		},
		{
			Key:         "appearance.sans_font",
			Type:        "string",
			Default:     defaults.Appearance.SansFont,
			Description: "Sans-serif font family for web pages",
			Section:     SectionAppearance,
		},
		{
			Key:         "appearance.serif_font",
			Type:        "string",
			Default:     defaults.Appearance.SerifFont,
			Description: "Serif font family for web pages",
			Section:     SectionAppearance,
		},
		{
			Key:         "appearance.monospace_font",
			Type:        "string",
			Default:     defaults.Appearance.MonospaceFont,
			Description: "Monospace font family for code",
			Section:     SectionAppearance,
		},
		{
			Key:         "default_webpage_zoom",
			Type:        "float64",
			Default:     fmt.Sprintf("%.1f", defaults.DefaultWebpageZoom),
			Description: "Default zoom level for web pages (1.0 = 100%%)",
			Range:       "0.1-5.0",
			Section:     SectionAppearance,
		},
		{
			Key:         "default_ui_scale",
			Type:        "float64",
			Default:     fmt.Sprintf("%.1f", defaults.DefaultUIScale),
			Description: "UI scale multiplier for GTK widgets",
			Range:       "0.5-3.0",
			Section:     SectionAppearance,
		},
		{
			Key:         "appearance.light_palette.*",
			Type:        "string",
			Default:     "(hex colors)",
			Description: "Light theme color palette (background, surface, surface_variant, text, muted, accent, border)",
			Section:     SectionAppearance,
		},
		{
			Key:         "appearance.dark_palette.*",
			Type:        "string",
			Default:     "(hex colors)",
			Description: "Dark theme color palette (background, surface, surface_variant, text, muted, accent, border)",
			Section:     SectionAppearance,
		},
	}
}

func (*SchemaProvider) getLoggingKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "logging.level",
			Type:        "string",
			Default:     defaults.Logging.Level,
			Description: "Log verbosity level",
			Values:      []string{"trace", "debug", "info", "warn", "error", "fatal"},
			Section:     SectionLogging,
		},
		{
			Key:         "logging.format",
			Type:        "string",
			Default:     defaults.Logging.Format,
			Description: "Log output format",
			Values:      []string{"text", "json", "console"},
			Section:     SectionLogging,
		},
		{
			Key:         "logging.max_age",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Logging.MaxAge),
			Description: "Maximum age of log files in days",
			Range:       ">=0",
			Section:     SectionLogging,
		},
		{
			Key:         "logging.log_dir",
			Type:        "string",
			Default:     defaults.Logging.LogDir,
			Description: "Directory for log files",
			Section:     SectionLogging,
		},
		{
			Key:         "logging.enable_file_log",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Logging.EnableFileLog),
			Description: "Enable logging to file",
			Section:     SectionLogging,
		},
		{
			Key:         "logging.capture_console",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Logging.CaptureConsole),
			Description: "Capture browser console messages to log",
			Section:     SectionLogging,
		},
	}
}

func (*SchemaProvider) getHistoryKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "history.max_entries",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.History.MaxEntries),
			Description: "Maximum number of history entries to keep",
			Range:       ">=0",
			Section:     SectionHistory,
		},
		{
			Key:         "history.retention_period_days",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.History.RetentionPeriodDays),
			Description: "Number of days to retain history",
			Range:       ">=0",
			Section:     SectionHistory,
		},
		{
			Key:         "history.cleanup_interval_days",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.History.CleanupIntervalDays),
			Description: "Days between automatic history cleanup",
			Range:       ">=0",
			Section:     SectionHistory,
		},
	}
}

func (*SchemaProvider) getSearchKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "default_search_engine",
			Type:        "string",
			Default:     defaults.DefaultSearchEngine,
			Description: "Default search engine URL (must contain %s placeholder)",
			Section:     SectionSearch,
		},
		{
			Key:         "search_shortcuts.<key>",
			Type:        "object",
			Default:     "(see defaults)",
			Description: "Search shortcuts map with url and description fields",
			Section:     SectionSearch,
		},
	}
}

func (*SchemaProvider) getDmenuKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "dmenu.max_history_days",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Dmenu.MaxHistoryDays),
			Description: "Number of days of history to show in dmenu (0 = all)",
			Range:       ">=0",
			Section:     SectionDmenu,
		},
		{
			Key:         "dmenu.show_visit_count",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Dmenu.ShowVisitCount),
			Description: "Show visit count in dmenu entries",
			Section:     SectionDmenu,
		},
		{
			Key:         "dmenu.show_last_visited",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Dmenu.ShowLastVisited),
			Description: "Show last visited date in dmenu entries",
			Section:     SectionDmenu,
		},
		{
			Key:         "dmenu.history_prefix",
			Type:        "string",
			Default:     defaults.Dmenu.HistoryPrefix,
			Description: "Prefix for history entries in dmenu",
			Section:     SectionDmenu,
		},
		{
			Key:         "dmenu.shortcut_prefix",
			Type:        "string",
			Default:     defaults.Dmenu.ShortcutPrefix,
			Description: "Prefix for search shortcuts in dmenu",
			Section:     SectionDmenu,
		},
		{
			Key:         "dmenu.url_prefix",
			Type:        "string",
			Default:     defaults.Dmenu.URLPrefix,
			Description: "Prefix for URL entries in dmenu",
			Section:     SectionDmenu,
		},
		{
			Key:         "dmenu.date_format",
			Type:        "string",
			Default:     defaults.Dmenu.DateFormat,
			Description: "Go time format for dates in dmenu",
			Section:     SectionDmenu,
		},
		{
			Key:         "dmenu.sort_by_visit_count",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Dmenu.SortByVisitCount),
			Description: "Sort dmenu entries by visit count",
			Section:     SectionDmenu,
		},
	}
}

func (*SchemaProvider) getWorkspaceKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "workspace.new_pane_url",
			Type:        "string",
			Default:     defaults.Workspace.NewPaneURL,
			Description: "URL loaded when creating a new pane",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.tab_bar_position",
			Type:        "string",
			Default:     defaults.Workspace.TabBarPosition,
			Description: "Position of the tab bar",
			Values:      []string{"top", "bottom"},
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.hide_tab_bar_when_single_tab",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Workspace.HideTabBarWhenSingleTab),
			Description: "Hide tab bar when only one tab exists",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.switch_to_tab_on_move",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Workspace.SwitchToTabOnMove),
			Description: "Switch focus to tab when moving pane to it",
			Section:     SectionWorkspace,
		},
		// Pane mode
		{
			Key:         "workspace.pane_mode.activation_shortcut",
			Type:        "string",
			Default:     defaults.Workspace.PaneMode.ActivationShortcut,
			Description: "Shortcut to enter pane mode",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.pane_mode.timeout_ms",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Workspace.PaneMode.TimeoutMilliseconds),
			Description: "Pane mode timeout in milliseconds (0 = no timeout)",
			Range:       ">=0",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.pane_mode.actions.<action>",
			Type:        "[]string",
			Default:     "(see defaults)",
			Description: "Key bindings for pane mode actions",
			Section:     SectionWorkspace,
		},
		// Tab mode
		{
			Key:         "workspace.tab_mode.activation_shortcut",
			Type:        "string",
			Default:     defaults.Workspace.TabMode.ActivationShortcut,
			Description: "Shortcut to enter tab mode",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.tab_mode.timeout_ms",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Workspace.TabMode.TimeoutMilliseconds),
			Description: "Tab mode timeout in milliseconds (0 = no timeout)",
			Range:       ">=0",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.tab_mode.actions.<action>",
			Type:        "[]string",
			Default:     "(see defaults)",
			Description: "Key bindings for tab mode actions",
			Section:     SectionWorkspace,
		},
		// Resize mode
		{
			Key:         "workspace.resize_mode.activation_shortcut",
			Type:        "string",
			Default:     defaults.Workspace.ResizeMode.ActivationShortcut,
			Description: "Shortcut to enter resize mode",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.resize_mode.timeout_ms",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Workspace.ResizeMode.TimeoutMilliseconds),
			Description: "Resize mode timeout in milliseconds (0 = no timeout)",
			Range:       ">=0",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.resize_mode.step_percent",
			Type:        "float64",
			Default:     fmt.Sprintf("%.1f", defaults.Workspace.ResizeMode.StepPercent),
			Description: "Percentage to resize panes per step",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.resize_mode.min_pane_percent",
			Type:        "float64",
			Default:     fmt.Sprintf("%.1f", defaults.Workspace.ResizeMode.MinPanePercent),
			Description: "Minimum pane size as percentage",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.resize_mode.actions.<action>",
			Type:        "[]string",
			Default:     "(see defaults)",
			Description: "Key bindings for resize mode actions",
			Section:     SectionWorkspace,
		},
		// Global shortcuts - managed via dedicated keybindings UI
		{
			Key:         "workspace.shortcuts.actions",
			Type:        "object",
			Default:     "(see defaults)",
			Description: "Global keyboard shortcuts (use Keybindings tab to edit)",
			Section:     SectionWorkspace,
		},
		// Popups
		{
			Key:         "workspace.popups.behavior",
			Type:        "string",
			Default:     string(defaults.Workspace.Popups.Behavior),
			Description: "How to handle JavaScript popup windows",
			Values:      []string{"split", "stacked", "tabbed", "windowed"},
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.popups.placement",
			Type:        "string",
			Default:     defaults.Workspace.Popups.Placement,
			Description: "Placement direction for split popups",
			Values:      []string{"right", "left", "top", "bottom"},
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.popups.blank_target_behavior",
			Type:        "string",
			Default:     defaults.Workspace.Popups.BlankTargetBehavior,
			Description: "How to handle target=\"_blank\" links",
			Values:      []string{"split", "stacked", "tabbed"},
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.popups.open_in_new_pane",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Workspace.Popups.OpenInNewPane),
			Description: "Open popups in workspace instead of blocking",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.popups.follow_pane_context",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Workspace.Popups.FollowPaneContext),
			Description: "Popup placement follows parent pane context",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.popups.enable_smart_detection",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Workspace.Popups.EnableSmartDetection),
			Description: "Use window properties to detect popup intent",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.popups.oauth_auto_close",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Workspace.Popups.OAuthAutoClose),
			Description: "Auto-close OAuth popups after success",
			Section:     SectionWorkspace,
		},
		// Styling
		{
			Key:         "workspace.styling.border_width",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Workspace.Styling.BorderWidth),
			Description: "Active pane border width in pixels",
			Range:       ">=0",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.styling.border_color",
			Type:        "string",
			Default:     defaults.Workspace.Styling.BorderColor,
			Description: "Active pane border color (CSS or @theme variable)",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.styling.mode_border_width",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Workspace.Styling.ModeBorderWidth),
			Description: "Border width for modal mode indicators",
			Range:       ">=0",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.styling.pane_mode_color",
			Type:        "string",
			Default:     defaults.Workspace.Styling.PaneModeColor,
			Description: "Color for pane mode indicator",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.styling.tab_mode_color",
			Type:        "string",
			Default:     defaults.Workspace.Styling.TabModeColor,
			Description: "Color for tab mode indicator",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.styling.session_mode_color",
			Type:        "string",
			Default:     defaults.Workspace.Styling.SessionModeColor,
			Description: "Color for session mode indicator",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.styling.resize_mode_color",
			Type:        "string",
			Default:     defaults.Workspace.Styling.ResizeModeColor,
			Description: "Color for resize mode indicator",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.styling.mode_indicator_toaster_enabled",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Workspace.Styling.ModeIndicatorToasterEnabled),
			Description: "Show toaster notification for modal modes",
			Section:     SectionWorkspace,
		},
		{
			Key:         "workspace.styling.transition_duration",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Workspace.Styling.TransitionDuration),
			Description: "Border animation duration in milliseconds",
			Range:       ">=0",
			Section:     SectionWorkspace,
		},
	}
}

func (*SchemaProvider) getSessionKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "session.auto_restore",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Session.AutoRestore),
			Description: "Automatically restore last session on startup",
			Section:     SectionSession,
		},
		{
			Key:         "session.snapshot_interval_ms",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Session.SnapshotIntervalMs),
			Description: "Minimum interval between session snapshots",
			Range:       ">=0",
			Section:     SectionSession,
		},
		{
			Key:         "session.max_exited_sessions",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Session.MaxExitedSessions),
			Description: "Maximum number of exited sessions to keep",
			Range:       ">=0",
			Section:     SectionSession,
		},
		{
			Key:         "session.max_exited_session_age_days",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Session.MaxExitedSessionAgeDays),
			Description: "Maximum age of exited sessions in days",
			Range:       ">=0",
			Section:     SectionSession,
		},
		{
			Key:         "session.session_mode.activation_shortcut",
			Type:        "string",
			Default:     defaults.Session.SessionMode.ActivationShortcut,
			Description: "Shortcut to enter session mode",
			Section:     SectionSession,
		},
		{
			Key:         "session.session_mode.timeout_ms",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Session.SessionMode.TimeoutMilliseconds),
			Description: "Session mode timeout in milliseconds",
			Range:       ">=0",
			Section:     SectionSession,
		},
		{
			Key:         "session.session_mode.actions.<action>",
			Type:        "[]string",
			Default:     "(see defaults)",
			Description: "Key bindings for session mode actions",
			Section:     SectionSession,
		},
	}
}

func (*SchemaProvider) getOmniboxKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "omnibox.initial_behavior",
			Type:        "string",
			Default:     defaults.Omnibox.InitialBehavior,
			Description: "What to show when omnibox opens with empty input",
			Values:      []string{"recent", "most_visited", "none"},
			Section:     SectionOmnibox,
		},
		{
			Key:         "omnibox.auto_open_on_new_pane",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Omnibox.AutoOpenOnNewPane),
			Description: "Auto-open omnibox when creating new pane",
			Section:     SectionOmnibox,
		},
	}
}

func (*SchemaProvider) getContentFilteringKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "content_filtering.enabled",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.ContentFiltering.Enabled),
			Description: "Enable ad blocking and content filtering",
			Section:     SectionContentFiltering,
		},
		{
			Key:         "content_filtering.auto_update",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.ContentFiltering.AutoUpdate),
			Description: "Auto-update filter lists from GitHub",
			Section:     SectionContentFiltering,
		},
	}
}

func (*SchemaProvider) getClipboardKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "clipboard.auto_copy_on_selection",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Clipboard.AutoCopyOnSelection),
			Description: "Auto-copy selected text to clipboard (zellij-style)",
			Section:     SectionClipboard,
		},
	}
}

func (*SchemaProvider) getRenderingKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "rendering.mode",
			Type:        "string",
			Default:     string(defaults.Rendering.Mode),
			Description: "GPU/CPU rendering selection for WebKit",
			Values:      []string{"auto", "gpu", "cpu"},
			Section:     SectionRendering,
		},
		{
			Key:         "rendering.gsk_renderer",
			Type:        "string",
			Default:     string(defaults.Rendering.GSKRenderer),
			Description: "GTK Scene Kit renderer selection",
			Values:      []string{"auto", "opengl", "vulkan", "cairo"},
			Section:     SectionRendering,
		},
		{
			Key:         "rendering.disable_dmabuf_renderer",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Rendering.DisableDMABufRenderer),
			Description: "Disable DMA-BUF accelerated renderer",
			Section:     SectionRendering,
		},
		{
			Key:         "rendering.force_compositing_mode",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Rendering.ForceCompositingMode),
			Description: "Force accelerated compositing",
			Section:     SectionRendering,
		},
		{
			Key:         "rendering.disable_compositing_mode",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Rendering.DisableCompositingMode),
			Description: "Disable accelerated compositing",
			Section:     SectionRendering,
		},
		{
			Key:         "rendering.disable_mipmaps",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Rendering.DisableMipmaps),
			Description: "Disable mipmap generation",
			Section:     SectionRendering,
		},
		{
			Key:         "rendering.prefer_gl",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Rendering.PreferGL),
			Description: "Force OpenGL over GLES",
			Section:     SectionRendering,
		},
		{
			Key:         "rendering.draw_compositing_indicators",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Rendering.DrawCompositingIndicators),
			Description: "Show compositing layer borders",
			Section:     SectionRendering,
		},
		{
			Key:         "rendering.show_fps",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Rendering.ShowFPS),
			Description: "Display WebKit FPS counter",
			Section:     SectionRendering,
		},
		{
			Key:         "rendering.sample_memory",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Rendering.SampleMemory),
			Description: "Enable memory sampling",
			Section:     SectionRendering,
		},
		{
			Key:         "rendering.debug_frames",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Rendering.DebugFrames),
			Description: "Enable frame timing debug output",
			Section:     SectionRendering,
		},
	}
}

func (*SchemaProvider) getMediaKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "media.hardware_decoding",
			Type:        "string",
			Default:     string(defaults.Media.HardwareDecodingMode),
			Description: "Hardware vs software video decoding",
			Values:      []string{"auto", "force", "disable"},
			Section:     SectionMedia,
		},
		{
			Key:         "media.gl_rendering_mode",
			Type:        "string",
			Default:     string(defaults.Media.GLRenderingMode),
			Description: "OpenGL API selection for video rendering",
			Values:      []string{"auto", "gles2", "gl3", "none"},
			Section:     SectionMedia,
		},
		{
			Key:         "media.prefer_av1",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Media.PreferAV1),
			Description: "Prioritize AV1 codec when available",
			Section:     SectionMedia,
		},
		{
			Key:         "media.show_diagnostics",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Media.ShowDiagnosticsOnStartup),
			Description: "Show media capability warnings at startup",
			Section:     SectionMedia,
		},
		{
			Key:         "media.force_vsync",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Media.ForceVSync),
			Description: "Force vertical sync for video playback",
			Section:     SectionMedia,
		},
		{
			Key:         "media.gstreamer_debug_level",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Media.GStreamerDebugLevel),
			Description: "GStreamer debug verbosity (0=off, 1-5)",
			Range:       "0-5",
			Section:     SectionMedia,
		},
	}
}

func (*SchemaProvider) getUpdateKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "update.enable_on_startup",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Update.EnableOnStartup),
			Description: "Check for updates on browser startup",
			Section:     SectionUpdate,
		},
		{
			Key:         "update.auto_download",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Update.AutoDownload),
			Description: "Auto-download updates in background",
			Section:     SectionUpdate,
		},
		{
			Key:         "update.notify_on_new_settings",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Update.NotifyOnNewSettings),
			Description: "Show toast when new config settings available",
			Section:     SectionUpdate,
		},
	}
}

func (*SchemaProvider) getDebugKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "debug.enable_devtools",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Debug.EnableDevTools),
			Description: "Enable browser developer tools (F12)",
			Section:     SectionDebug,
		},
	}
}

func (*SchemaProvider) getPerformanceKeys(defaults *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "performance.profile",
			Type:        "string",
			Default:     string(defaults.Performance.Profile),
			Description: "Performance profile preset (custom enables manual tuning)",
			Values:      []string{"default", "lite", "max", "custom"},
			Section:     SectionPerformance,
		},
		{
			Key:         "performance.zoom_cache_size",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Performance.ZoomCacheSize),
			Description: "Number of domain zoom levels to cache",
			Range:       ">=0",
			Section:     SectionPerformance,
		},
		{
			Key:         "performance.webview_pool_prewarm_count",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Performance.WebViewPoolPrewarmCount),
			Description: "WebViews to pre-create at startup",
			Range:       ">=0",
			Section:     SectionPerformance,
		},
		{
			Key:         "performance.skia_cpu_painting_threads",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Performance.SkiaCPUPaintingThreads),
			Description: "Skia CPU rendering threads (0=unset, custom profile only)",
			Range:       ">=0",
			Section:     SectionPerformance,
		},
		{
			Key:         "performance.skia_gpu_painting_threads",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Performance.SkiaGPUPaintingThreads),
			Description: "Skia GPU rendering threads (-1=unset, 0=disable, custom profile only)",
			Range:       ">=-1",
			Section:     SectionPerformance,
		},
		{
			Key:         "performance.skia_enable_cpu_rendering",
			Type:        "bool",
			Default:     fmt.Sprintf("%t", defaults.Performance.SkiaEnableCPURendering),
			Description: "Force CPU rendering (custom profile only)",
			Section:     SectionPerformance,
		},
		{
			Key:         "performance.web_process_memory_limit_mb",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Performance.WebProcessMemoryLimitMB),
			Description: "Web process memory limit in MB (0=unset, custom profile only)",
			Range:       ">=0",
			Section:     SectionPerformance,
		},
		{
			Key:         "performance.web_process_memory_poll_interval_sec",
			Type:        "float64",
			Default:     fmt.Sprintf("%.1f", defaults.Performance.WebProcessMemoryPollIntervalSec),
			Description: "Memory check interval in seconds (0=default 30s, custom profile only)",
			Range:       ">=0",
			Section:     SectionPerformance,
		},
		{
			Key:         "performance.web_process_memory_conservative_threshold",
			Type:        "float64",
			Default:     fmt.Sprintf("%.2f", defaults.Performance.WebProcessMemoryConservativeThreshold),
			Description: "Conservative memory cleanup threshold (0-1, custom profile only)",
			Range:       "0-1",
			Section:     SectionPerformance,
		},
		{
			Key:         "performance.web_process_memory_strict_threshold",
			Type:        "float64",
			Default:     fmt.Sprintf("%.2f", defaults.Performance.WebProcessMemoryStrictThreshold),
			Description: "Strict memory cleanup threshold (0-1, custom profile only)",
			Range:       "0-1",
			Section:     SectionPerformance,
		},
		{
			Key:         "performance.network_process_memory_limit_mb",
			Type:        "int",
			Default:     fmt.Sprintf("%d", defaults.Performance.NetworkProcessMemoryLimitMB),
			Description: "Network process memory limit in MB (0=unset, custom profile only)",
			Range:       ">=0",
			Section:     SectionPerformance,
		},
		{
			Key:         "performance.network_process_memory_poll_interval_sec",
			Type:        "float64",
			Default:     fmt.Sprintf("%.1f", defaults.Performance.NetworkProcessMemoryPollIntervalSec),
			Description: "Network memory check interval in seconds (custom profile only)",
			Range:       ">=0",
			Section:     SectionPerformance,
		},
		{
			Key:         "performance.network_process_memory_conservative_threshold",
			Type:        "float64",
			Default:     fmt.Sprintf("%.2f", defaults.Performance.NetworkProcessMemoryConservativeThreshold),
			Description: "Network conservative memory threshold (0-1, custom profile only)",
			Range:       "0-1",
			Section:     SectionPerformance,
		},
		{
			Key:         "performance.network_process_memory_strict_threshold",
			Type:        "float64",
			Default:     fmt.Sprintf("%.2f", defaults.Performance.NetworkProcessMemoryStrictThreshold),
			Description: "Network strict memory threshold (0-1, custom profile only)",
			Range:       "0-1",
			Section:     SectionPerformance,
		},
	}
}

func (*SchemaProvider) getRuntimeKeys(_ *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "runtime.prefix",
			Type:        "string",
			Default:     "(empty)",
			Description: "Custom runtime prefix for WebKitGTK/GTK",
			Section:     SectionRuntime,
		},
	}
}

func (*SchemaProvider) getDatabaseKeys() []entity.ConfigKeyInfo {
	// Get actual default path for display
	dbPath := "$XDG_DATA_HOME/" + appName + "/" + databaseName
	if path, err := GetDatabaseFile(); err == nil {
		dbPath = path
	}

	return []entity.ConfigKeyInfo{
		{
			Key:         "database.path",
			Type:        "string",
			Default:     dbPath,
			Description: "Path to SQLite database file",
			Section:     SectionDatabase,
		},
	}
}

func (*SchemaProvider) getDownloadsKeys(_ *Config) []entity.ConfigKeyInfo {
	return []entity.ConfigKeyInfo{
		{
			Key:         "downloads.path",
			Type:        "string",
			Default:     "(empty = $XDG_DOWNLOAD_DIR or ~/Downloads)",
			Description: "Directory where downloads are saved",
			Section:     SectionDownloads,
		},
	}
}
