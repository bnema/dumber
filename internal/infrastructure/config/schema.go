package config

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
	// Omnibox controls the omnibox behavior (initial history display)
	Omnibox OmniboxConfig `mapstructure:"omnibox" yaml:"omnibox" toml:"omnibox"`
	// Rendering controls WebKit, GTK and compositor behavior.
	Rendering RenderingConfig `mapstructure:"rendering" yaml:"rendering" toml:"rendering"`
	// Media controls video playback and hardware acceleration
	Media MediaConfig `mapstructure:"media" yaml:"media" toml:"media"`
	// Runtime configures optional runtime overrides (e.g., /opt prefix for WebKitGTK/GTK).
	Runtime RuntimeConfig `mapstructure:"runtime" yaml:"runtime" toml:"runtime"`
	// Performance holds internal performance tuning options (not exposed in UI).
	Performance PerformanceConfig `mapstructure:"performance" yaml:"performance" toml:"performance"`
	// Update controls automatic update checking and downloading.
	Update UpdateConfig `mapstructure:"update" yaml:"update" toml:"update"`
}

// RenderingMode selects GPU vs CPU rendering.
type RenderingMode string

const (
	RenderingModeAuto RenderingMode = "auto"
	RenderingModeGPU  RenderingMode = "gpu"
	RenderingModeCPU  RenderingMode = "cpu"
)

// GSKRendererMode controls the GTK Scene Kit renderer selection.
type GSKRendererMode string

const (
	GSKRendererAuto   GSKRendererMode = "auto"
	GSKRendererOpenGL GSKRendererMode = "opengl"
	GSKRendererVulkan GSKRendererMode = "vulkan"
	GSKRendererCairo  GSKRendererMode = "cairo"
)

// RenderingConfig controls WebKit and GTK/GSK rendering behavior.
type RenderingConfig struct {
	// Mode controls GPU/CPU rendering selection for WebKit (auto, gpu, cpu).
	Mode RenderingMode `mapstructure:"mode" yaml:"mode" toml:"mode"`

	// DisableDMABufRenderer disables DMA-BUF accelerated renderer.
	// Env: WEBKIT_DISABLE_DMABUF_RENDERER
	DisableDMABufRenderer bool `mapstructure:"disable_dmabuf_renderer" yaml:"disable_dmabuf_renderer" toml:"disable_dmabuf_renderer"`

	// ForceCompositingMode forces accelerated compositing.
	// Env: WEBKIT_FORCE_COMPOSITING_MODE
	ForceCompositingMode bool `mapstructure:"force_compositing_mode" yaml:"force_compositing_mode" toml:"force_compositing_mode"`

	// DisableCompositingMode disables accelerated compositing.
	// Env: WEBKIT_DISABLE_COMPOSITING_MODE
	DisableCompositingMode bool `mapstructure:"disable_compositing_mode" yaml:"disable_compositing_mode" toml:"disable_compositing_mode"`

	// GSKRenderer overrides GTK Scene Kit renderer selection.
	// Env: GSK_RENDERER
	GSKRenderer GSKRendererMode `mapstructure:"gsk_renderer" yaml:"gsk_renderer" toml:"gsk_renderer"`

	// DisableMipmaps disables mipmap generation.
	// Env: GSK_GPU_DISABLE=mipmap
	DisableMipmaps bool `mapstructure:"disable_mipmaps" yaml:"disable_mipmaps" toml:"disable_mipmaps"`

	// PreferGL forces OpenGL over GLES.
	// Env: GDK_DEBUG=gl-prefer-gl
	PreferGL bool `mapstructure:"prefer_gl" yaml:"prefer_gl" toml:"prefer_gl"`

	// DrawCompositingIndicators shows compositing layer borders and repaint counters.
	DrawCompositingIndicators bool `mapstructure:"draw_compositing_indicators" yaml:"draw_compositing_indicators" toml:"draw_compositing_indicators"` //nolint:lll // struct tags exceed lll limit

	// ShowFPS displays WebKit FPS counter.
	// Env: WEBKIT_SHOW_FPS
	ShowFPS bool `mapstructure:"show_fps" yaml:"show_fps" toml:"show_fps"`

	// SampleMemory enables memory sampling.
	// Env: WEBKIT_SAMPLE_MEMORY
	SampleMemory bool `mapstructure:"sample_memory" yaml:"sample_memory" toml:"sample_memory"`

	// DebugFrames enables frame timing debug output.
	// Env: GDK_DEBUG=frames
	DebugFrames bool `mapstructure:"debug_frames" yaml:"debug_frames" toml:"debug_frames"`
}

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
	// ForceVSync forces vertical sync for video playback (may help with tearing)
	ForceVSync bool `mapstructure:"force_vsync" yaml:"force_vsync" toml:"force_vsync"`
	// GLRenderingMode controls OpenGL API selection for video rendering
	// Values: "auto" (default), "gles2", "gl3", "none"
	GLRenderingMode GLRenderingMode `mapstructure:"gl_rendering_mode" yaml:"gl_rendering_mode" toml:"gl_rendering_mode"`
	// GStreamerDebugLevel sets GStreamer debug verbosity (0=off, 1-5=increasing verbosity)
	GStreamerDebugLevel int `mapstructure:"gstreamer_debug_level" yaml:"gstreamer_debug_level" toml:"gstreamer_debug_level"`
	// VideoBufferSizeMB sets the video buffer size in megabytes for smoother streaming
	// Higher values reduce rebuffering but use more memory. Default: 64 MB
	VideoBufferSizeMB int `mapstructure:"video_buffer_size_mb" yaml:"video_buffer_size_mb" toml:"video_buffer_size_mb"`
	// QueueBufferTimeSec sets the queue buffer time in seconds
	// Higher values allow more prebuffering for bursty streams. Default: 20 seconds
	QueueBufferTimeSec int `mapstructure:"queue_buffer_time_sec" yaml:"queue_buffer_time_sec" toml:"queue_buffer_time_sec"`
}

// RuntimeConfig holds optional runtime overrides.
type RuntimeConfig struct {
	// Prefix points to a custom runtime prefix (e.g., /opt/webkitgtk).
	// This is used to influence pkg-config lookup and dynamic library loading.
	Prefix string `mapstructure:"prefix" yaml:"prefix" toml:"prefix"`
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
	// ZoomCacheSize is the number of domain zoom levels to cache in memory.
	// Higher values reduce database queries but use more memory.
	// Default: 256 domains (~20KB memory)
	ZoomCacheSize int `mapstructure:"zoom_cache_size" yaml:"zoom_cache_size" toml:"zoom_cache_size"`

	// WebViewPoolPrewarmCount is the number of WebViews to pre-create at startup.
	// Higher values speed up initial tab creation but use more memory.
	// Default: 4
	WebViewPoolPrewarmCount int `mapstructure:"webview_pool_prewarm_count" yaml:"webview_pool_prewarm_count" toml:"webview_pool_prewarm_count"` //nolint:lll // struct tags must stay on one line
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
	MaxHistoryItems  int    `mapstructure:"max_history_items" yaml:"max_history_items" toml:"max_history_items"`
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
}

// AppearanceConfig holds UI/rendering preferences.
type AppearanceConfig struct {
	// Default fonts for pages that do not specify fonts.
	SansFont      string `mapstructure:"sans_font" yaml:"sans_font" toml:"sans_font"`
	SerifFont     string `mapstructure:"serif_font" yaml:"serif_font" toml:"serif_font"`
	MonospaceFont string `mapstructure:"monospace_font" yaml:"monospace_font" toml:"monospace_font"`
	// Default font size in CSS pixels (approx).
	DefaultFontSize int          `mapstructure:"default_font_size" yaml:"default_font_size" toml:"default_font_size"`
	LightPalette    ColorPalette `mapstructure:"light_palette" yaml:"light_palette" toml:"light_palette"`
	DarkPalette     ColorPalette `mapstructure:"dark_palette" yaml:"dark_palette" toml:"dark_palette"`
	// ColorScheme controls the initial theme preference: "prefer-dark", "prefer-light", or "default" (follows system)
	ColorScheme string `mapstructure:"color_scheme" yaml:"color_scheme" toml:"color_scheme"`
}

// ColorPalette contains semantic color tokens for light/dark themes.
type ColorPalette struct {
	Background     string `mapstructure:"background" yaml:"background" toml:"background" json:"background"`
	Surface        string `mapstructure:"surface" yaml:"surface" toml:"surface" json:"surface"`
	SurfaceVariant string `mapstructure:"surface_variant" yaml:"surface_variant" toml:"surface_variant" json:"surface_variant"`
	Text           string `mapstructure:"text" yaml:"text" toml:"text" json:"text"`
	Muted          string `mapstructure:"muted" yaml:"muted" toml:"muted" json:"muted"`
	Accent         string `mapstructure:"accent" yaml:"accent" toml:"accent" json:"accent"`
	Border         string `mapstructure:"border" yaml:"border" toml:"border" json:"border"`
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

// OmniboxConfig holds omnibox behavior preferences
type OmniboxConfig struct {
	// InitialBehavior controls what to show when omnibox opens with empty input
	// Values: "recent" (recent visits), "most_visited" (most visited sites), "none" (nothing)
	InitialBehavior string `mapstructure:"initial_behavior" yaml:"initial_behavior" toml:"initial_behavior"`
	// AutoOpenOnNewPane opens the omnibox automatically when a new pane is created.
	// Default: false
	AutoOpenOnNewPane bool `mapstructure:"auto_open_on_new_pane" yaml:"auto_open_on_new_pane" toml:"auto_open_on_new_pane"`
}

// DebugConfig holds debug and troubleshooting options
type DebugConfig struct {
	// Enable browser developer tools (F12, Inspect Element in context menu)
	EnableDevTools bool `mapstructure:"enable_devtools" yaml:"enable_devtools" toml:"enable_devtools"`
}

// UpdateConfig holds automatic update settings.
type UpdateConfig struct {
	// EnableOnStartup enables checking for updates when the browser starts.
	// Default: true
	EnableOnStartup bool `mapstructure:"enable_on_startup" yaml:"enable_on_startup" toml:"enable_on_startup"`
	// AutoDownload automatically downloads updates in the background.
	// When enabled, updates are applied on browser exit.
	// Default: false
	AutoDownload bool `mapstructure:"auto_download" yaml:"auto_download" toml:"auto_download"`
}

// WorkspaceConfig captures layout, pane, and tab behavior preferences.
type WorkspaceConfig struct {
	// NewPaneURL is the URL loaded when creating a new empty pane/tab.
	// Default: "about:blank"
	NewPaneURL string `mapstructure:"new_pane_url" yaml:"new_pane_url" toml:"new_pane_url"`
	// PaneMode defines modal pane behavior and bindings.
	PaneMode PaneModeConfig `mapstructure:"pane_mode" yaml:"pane_mode" toml:"pane_mode" json:"pane_mode"`
	// TabMode defines modal tab behavior and bindings (Alt+T).
	TabMode TabModeConfig `mapstructure:"tab_mode" yaml:"tab_mode" toml:"tab_mode" json:"tab_mode"`
	// ResizeMode defines modal pane resizing behavior and bindings (Ctrl+N).
	ResizeMode ResizeModeConfig `mapstructure:"resize_mode" yaml:"resize_mode" toml:"resize_mode" json:"resize_mode"`
	// Shortcuts holds global (non-modal) keyboard shortcuts.
	Shortcuts GlobalShortcutsConfig `mapstructure:"shortcuts" yaml:"shortcuts" toml:"shortcuts" json:"shortcuts"`
	// TabBarPosition determines tab bar placement: "top" or "bottom".
	TabBarPosition string `mapstructure:"tab_bar_position" yaml:"tab_bar_position" toml:"tab_bar_position" json:"tab_bar_position"`
	// HideTabBarWhenSingleTab hides the tab bar when only one tab exists.
	HideTabBarWhenSingleTab bool `mapstructure:"hide_tab_bar_when_single_tab" yaml:"hide_tab_bar_when_single_tab" toml:"hide_tab_bar_when_single_tab" json:"hide_tab_bar_when_single_tab"` //nolint:lll // struct tags must stay on one line
	// Popups configures default popup placement rules.
	Popups PopupBehaviorConfig `mapstructure:"popups" yaml:"popups" toml:"popups" json:"popups"`
	// Styling configures workspace visual appearance.
	Styling WorkspaceStylingConfig `mapstructure:"styling" yaml:"styling" toml:"styling" json:"styling"`
}

// PaneModeConfig defines modal behavior for pane management.
type PaneModeConfig struct {
	ActivationShortcut  string              `mapstructure:"activation_shortcut" yaml:"activation_shortcut" toml:"activation_shortcut" json:"activation_shortcut"` //nolint:lll // struct tags must stay on one line
	TimeoutMilliseconds int                 `mapstructure:"timeout_ms" yaml:"timeout_ms" toml:"timeout_ms" json:"timeout_ms"`
	Actions             map[string][]string `mapstructure:"actions" yaml:"actions" toml:"actions" json:"actions"`
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

// TabModeConfig defines modal behavior for tab management (Zellij-style).
type TabModeConfig struct {
	ActivationShortcut  string              `mapstructure:"activation_shortcut" yaml:"activation_shortcut" toml:"activation_shortcut" json:"activation_shortcut"` //nolint:lll // struct tags must stay on one line
	TimeoutMilliseconds int                 `mapstructure:"timeout_ms" yaml:"timeout_ms" toml:"timeout_ms" json:"timeout_ms"`
	Actions             map[string][]string `mapstructure:"actions" yaml:"actions" toml:"actions" json:"actions"`
}

// GetKeyBindings returns an inverted map for O(1) key→action lookup.
// This is built from the action→keys structure in the config.
func (t *TabModeConfig) GetKeyBindings() map[string]string {
	keyToAction := make(map[string]string)
	for action, keys := range t.Actions {
		for _, key := range keys {
			keyToAction[key] = action
		}
	}
	return keyToAction
}

// ResizeModeConfig defines modal behavior for resizing panes (Zellij-style).
type ResizeModeConfig struct {
	ActivationShortcut  string              `mapstructure:"activation_shortcut" yaml:"activation_shortcut" toml:"activation_shortcut" json:"activation_shortcut"` //nolint:lll // struct tags must stay on one line
	TimeoutMilliseconds int                 `mapstructure:"timeout_ms" yaml:"timeout_ms" toml:"timeout_ms" json:"timeout_ms"`
	StepPercent         float64             `mapstructure:"step_percent" yaml:"step_percent" toml:"step_percent" json:"step_percent"`
	MinPanePercent      float64             `mapstructure:"min_pane_percent" yaml:"min_pane_percent" toml:"min_pane_percent" json:"min_pane_percent"` //nolint:lll // struct tags must stay on one line
	Actions             map[string][]string `mapstructure:"actions" yaml:"actions" toml:"actions" json:"actions"`
}

// GetKeyBindings returns an inverted map for O(1) key→action lookup.
// This is built from the action→keys structure in the config.
func (r *ResizeModeConfig) GetKeyBindings() map[string]string {
	keyToAction := make(map[string]string)
	for action, keys := range r.Actions {
		for _, key := range keys {
			keyToAction[key] = action
		}
	}
	return keyToAction
}

// GlobalShortcutsConfig defines global shortcuts (always active, not modal).
type GlobalShortcutsConfig struct {
	ClosePane   string `mapstructure:"close_pane" yaml:"close_pane" toml:"close_pane" json:"close_pane"`
	NextTab     string `mapstructure:"next_tab" yaml:"next_tab" toml:"next_tab" json:"next_tab"`
	PreviousTab string `mapstructure:"previous_tab" yaml:"previous_tab" toml:"previous_tab" json:"previous_tab"`
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
	Behavior PopupBehavior `mapstructure:"behavior" yaml:"behavior" toml:"behavior" json:"behavior"`

	// Placement specifies direction for split behavior ("right", "left", "top", "bottom")
	// Only used when Behavior is "split"
	Placement string `mapstructure:"placement" yaml:"placement" toml:"placement" json:"placement"`

	// OpenInNewPane controls whether popups are opened in workspace or blocked
	OpenInNewPane bool `mapstructure:"open_in_new_pane" yaml:"open_in_new_pane" toml:"open_in_new_pane" json:"open_in_new_pane"`

	// FollowPaneContext determines if popup placement follows parent pane context
	FollowPaneContext bool `mapstructure:"follow_pane_context" yaml:"follow_pane_context" toml:"follow_pane_context" json:"follow_pane_context"` //nolint:lll // struct tags must stay on one line

	// BlankTargetBehavior determines how target="_blank" links are opened
	// Accepted values: "split", "stacked" (default), "tabbed"
	// This is separate from Behavior which controls JavaScript popups
	BlankTargetBehavior string `mapstructure:"blank_target_behavior" yaml:"blank_target_behavior" toml:"blank_target_behavior" json:"blank_target_behavior"` //nolint:lll // struct tags must stay on one line

	// EnableSmartDetection uses WebKitWindowProperties to detect popup vs tab intents
	EnableSmartDetection bool `mapstructure:"enable_smart_detection" yaml:"enable_smart_detection" toml:"enable_smart_detection" json:"enable_smart_detection"` //nolint:lll // struct tags must stay on one line

	// OAuthAutoClose enables auto-closing OAuth popups after successful auth redirects
	OAuthAutoClose bool `mapstructure:"oauth_auto_close" yaml:"oauth_auto_close" toml:"oauth_auto_close" json:"oauth_auto_close"`
}

// WorkspaceStylingConfig defines visual styling for workspace panes.
type WorkspaceStylingConfig struct {
	// BorderWidth in pixels for active pane borders (overlay)
	BorderWidth int `mapstructure:"border_width" yaml:"border_width" toml:"border_width" json:"border_width"`
	// BorderColor for focused panes (CSS color value or theme variable)
	BorderColor string `mapstructure:"border_color" yaml:"border_color" toml:"border_color" json:"border_color"`

	// PaneModeBorderWidth in pixels for pane mode indicator border (Ctrl+P N overlay)
	PaneModeBorderWidth int `mapstructure:"pane_mode_border_width" yaml:"pane_mode_border_width" toml:"pane_mode_border_width" json:"pane_mode_border_width"` //nolint:lll // struct tags must stay on one line
	// PaneModeBorderColor for the pane mode indicator border (CSS color value or theme variable)
	// Defaults to "#4A90E2" (blue) if not set
	PaneModeBorderColor string `mapstructure:"pane_mode_border_color" yaml:"pane_mode_border_color" toml:"pane_mode_border_color" json:"pane_mode_border_color"` //nolint:lll // struct tags must stay on one line

	// TabModeBorderWidth in pixels for tab mode indicator border (Ctrl+P T overlay)
	TabModeBorderWidth int `mapstructure:"tab_mode_border_width" yaml:"tab_mode_border_width" toml:"tab_mode_border_width" json:"tab_mode_border_width"` //nolint:lll // struct tags must stay on one line
	// TabModeBorderColor for the tab mode indicator border (CSS color value or theme variable)
	// Defaults to "#FFA500" (orange) if not set - MUST be different from PaneModeBorderColor
	TabModeBorderColor string `mapstructure:"tab_mode_border_color" yaml:"tab_mode_border_color" toml:"tab_mode_border_color" json:"tab_mode_border_color"` //nolint:lll // struct tags must stay on one line

	// SessionModeBorderWidth in pixels for session mode indicator border (Ctrl+O overlay)
	SessionModeBorderWidth int `mapstructure:"session_mode_border_width" yaml:"session_mode_border_width" toml:"session_mode_border_width" json:"session_mode_border_width"` //nolint:lll // struct tags must stay on one line
	// SessionModeBorderColor for the session mode indicator border (CSS color value or theme variable)
	// Defaults to "#9B59B6" (purple) if not set - MUST be different from other mode colors
	SessionModeBorderColor string `mapstructure:"session_mode_border_color" yaml:"session_mode_border_color" toml:"session_mode_border_color" json:"session_mode_border_color"` //nolint:lll // struct tags must stay on one line

	// ResizeModeBorderWidth in pixels for resize mode indicator border (Ctrl+N overlay)
	ResizeModeBorderWidth int `mapstructure:"resize_mode_border_width" yaml:"resize_mode_border_width" toml:"resize_mode_border_width" json:"resize_mode_border_width"` //nolint:lll // struct tags must stay on one line
	// ResizeModeBorderColor for the resize mode indicator border (CSS color value or theme variable)
	// Defaults to "#00D4AA" (cyan/teal) if not set - MUST be different from other mode colors
	ResizeModeBorderColor string `mapstructure:"resize_mode_border_color" yaml:"resize_mode_border_color" toml:"resize_mode_border_color" json:"resize_mode_border_color"` //nolint:lll // struct tags must stay on one line

	// TransitionDuration in milliseconds for border animations
	TransitionDuration int `mapstructure:"transition_duration" yaml:"transition_duration" toml:"transition_duration" json:"transition_duration"` //nolint:lll // struct tags must stay on one line
}

// SessionConfig holds session persistence settings.
type SessionConfig struct {
	// AutoRestore automatically restores the last session on startup.
	// Default: false
	AutoRestore bool `mapstructure:"auto_restore" yaml:"auto_restore" toml:"auto_restore" json:"auto_restore"`

	// SnapshotIntervalMs is the minimum interval between snapshots in milliseconds.
	// Default: 5000
	SnapshotIntervalMs int `mapstructure:"snapshot_interval_ms" yaml:"snapshot_interval_ms" toml:"snapshot_interval_ms" json:"snapshot_interval_ms"` //nolint:lll // struct tags must stay on one line

	// MaxExitedSessions is the maximum number of exited sessions to keep.
	// Default: 50
	MaxExitedSessions int `mapstructure:"max_exited_sessions" yaml:"max_exited_sessions" toml:"max_exited_sessions" json:"max_exited_sessions"` //nolint:lll // struct tags must stay on one line

	// MaxExitedSessionAgeDays is the maximum age in days for exited sessions.
	// Sessions older than this will be automatically deleted on startup.
	// Default: 7
	MaxExitedSessionAgeDays int `mapstructure:"max_exited_session_age_days" yaml:"max_exited_session_age_days" toml:"max_exited_session_age_days" json:"max_exited_session_age_days"` //nolint:lll // struct tags must stay on one line

	// SessionMode defines modal behavior for session management (Ctrl+O).
	SessionMode SessionModeConfig `mapstructure:"session_mode" yaml:"session_mode" toml:"session_mode" json:"session_mode"`
}

// SessionModeConfig defines modal behavior for session management.
type SessionModeConfig struct {
	ActivationShortcut  string              `mapstructure:"activation_shortcut" yaml:"activation_shortcut" toml:"activation_shortcut" json:"activation_shortcut"` //nolint:lll // struct tags must stay on one line
	TimeoutMilliseconds int                 `mapstructure:"timeout_ms" yaml:"timeout_ms" toml:"timeout_ms" json:"timeout_ms"`
	Actions             map[string][]string `mapstructure:"actions" yaml:"actions" toml:"actions" json:"actions"`
}

// GetKeyBindings returns an inverted map for O(1) key→action lookup.
// This is built from the action→keys structure in the config.
func (s *SessionModeConfig) GetKeyBindings() map[string]string {
	keyToAction := make(map[string]string)
	for action, keys := range s.Actions {
		for _, key := range keys {
			keyToAction[key] = action
		}
	}
	return keyToAction
}
