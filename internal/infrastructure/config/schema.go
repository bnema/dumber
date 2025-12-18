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
	// RenderingMode controls GPU/CPU rendering selection for WebKit
	RenderingMode RenderingMode `mapstructure:"rendering_mode" yaml:"rendering_mode" toml:"rendering_mode"`
	// DefaultWebpageZoom sets the default zoom level for pages without saved zoom settings (1.0 = 100%, 1.2 = 120%)
	DefaultWebpageZoom float64 `mapstructure:"default_webpage_zoom" yaml:"default_webpage_zoom" toml:"default_webpage_zoom"`
	// DefaultUIScale sets the default UI scale for GTK widgets (1.0 = 100%, 2.0 = 200%)
	DefaultUIScale float64 `mapstructure:"default_ui_scale" yaml:"default_ui_scale" toml:"default_ui_scale"`
	// Workspace defines workspace, pane, and tab handling behaviour.
	Workspace WorkspaceConfig `mapstructure:"workspace" yaml:"workspace" toml:"workspace"`
	// ContentFiltering controls ad blocking and content filtering
	ContentFiltering ContentFilteringConfig `mapstructure:"content_filtering" yaml:"content_filtering" toml:"content_filtering"`
	// Omnibox controls the omnibox behavior (initial history display)
	Omnibox OmniboxConfig `mapstructure:"omnibox" yaml:"omnibox" toml:"omnibox"`
}

// RenderingMode selects GPU vs CPU rendering.
type RenderingMode string

const (
	RenderingModeAuto RenderingMode = "auto"
	RenderingModeGPU  RenderingMode = "gpu"
	RenderingModeCPU  RenderingMode = "cpu"
)

// ThemeDefault is the default theme setting (follows system).
const ThemeDefault = "default"

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

// SearchShortcut represents a search shortcut configuration.
type SearchShortcut struct {
	URL         string `mapstructure:"url" toml:"url" yaml:"url" json:"url"`
	Description string `mapstructure:"description" toml:"description" yaml:"description" json:"description"`
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
	// FilterLists URLs of filter lists to use (EasyList, uBlock, etc.)
	FilterLists []string `mapstructure:"filter_lists" yaml:"filter_lists" toml:"filter_lists"`
	// Note: Whitelist is now managed via database (content_whitelist table)
}

// OmniboxConfig holds omnibox behavior preferences
type OmniboxConfig struct {
	// InitialBehavior controls what to show when omnibox opens with empty input
	// Values: "recent" (recent visits), "most_visited" (most visited sites), "none" (nothing)
	InitialBehavior string `mapstructure:"initial_behavior" yaml:"initial_behavior" toml:"initial_behavior"`
}

// DebugConfig holds debug and troubleshooting options
type DebugConfig struct {
	// Enable browser developer tools (F12, Inspect Element in context menu)
	EnableDevTools bool `mapstructure:"enable_devtools" yaml:"enable_devtools" toml:"enable_devtools"`
}

// WorkspaceConfig captures layout, pane, and tab behaviour preferences.
type WorkspaceConfig struct {
	// PaneMode defines modal pane behaviour and bindings.
	PaneMode PaneModeConfig `mapstructure:"pane_mode" yaml:"pane_mode" toml:"pane_mode" json:"pane_mode"`
	// TabMode defines modal tab behaviour and bindings (Alt+T).
	TabMode TabModeConfig `mapstructure:"tab_mode" yaml:"tab_mode" toml:"tab_mode" json:"tab_mode"`
	// Tabs holds classic browser tab shortcuts.
	Tabs TabKeyConfig `mapstructure:"tabs" yaml:"tabs" toml:"tabs" json:"tabs"`
	// TabBarPosition determines tab bar placement: "top" or "bottom".
	TabBarPosition string `mapstructure:"tab_bar_position" yaml:"tab_bar_position" toml:"tab_bar_position" json:"tab_bar_position"`
	// HideTabBarWhenSingleTab hides the tab bar when only one tab exists.
	HideTabBarWhenSingleTab bool `mapstructure:"hide_tab_bar_when_single_tab" yaml:"hide_tab_bar_when_single_tab" toml:"hide_tab_bar_when_single_tab" json:"hide_tab_bar_when_single_tab"`
	// Popups configures default popup placement rules.
	Popups PopupBehaviorConfig `mapstructure:"popups" yaml:"popups" toml:"popups" json:"popups"`
	// Styling configures workspace visual appearance.
	Styling WorkspaceStylingConfig `mapstructure:"styling" yaml:"styling" toml:"styling" json:"styling"`
}

// PaneModeConfig defines modal behaviour for pane management.
type PaneModeConfig struct {
	ActivationShortcut  string              `mapstructure:"activation_shortcut" yaml:"activation_shortcut" toml:"activation_shortcut" json:"activation_shortcut"`
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

// TabModeConfig defines modal behaviour for tab management (Zellij-style).
type TabModeConfig struct {
	ActivationShortcut  string              `mapstructure:"activation_shortcut" yaml:"activation_shortcut" toml:"activation_shortcut" json:"activation_shortcut"`
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

// TabKeyConfig defines Zellij-inspired tab shortcuts.
type TabKeyConfig struct {
	NewTab      string `mapstructure:"new_tab" yaml:"new_tab" toml:"new_tab" json:"new_tab"`
	CloseTab    string `mapstructure:"close_tab" yaml:"close_tab" toml:"close_tab" json:"close_tab"`
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
	FollowPaneContext bool `mapstructure:"follow_pane_context" yaml:"follow_pane_context" toml:"follow_pane_context" json:"follow_pane_context"`

	// BlankTargetBehavior determines how target="_blank" links are opened
	// Accepted values: "split", "stacked" (default), "tabbed"
	// This is separate from Behavior which controls JavaScript popups
	BlankTargetBehavior string `mapstructure:"blank_target_behavior" yaml:"blank_target_behavior" toml:"blank_target_behavior" json:"blank_target_behavior"`

	// EnableSmartDetection uses WebKitWindowProperties to detect popup vs tab intents
	EnableSmartDetection bool `mapstructure:"enable_smart_detection" yaml:"enable_smart_detection" toml:"enable_smart_detection" json:"enable_smart_detection"`

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
	PaneModeBorderWidth int `mapstructure:"pane_mode_border_width" yaml:"pane_mode_border_width" toml:"pane_mode_border_width" json:"pane_mode_border_width"`
	// PaneModeBorderColor for the pane mode indicator border (CSS color value or theme variable)
	// Defaults to "#4A90E2" (blue) if not set
	PaneModeBorderColor string `mapstructure:"pane_mode_border_color" yaml:"pane_mode_border_color" toml:"pane_mode_border_color" json:"pane_mode_border_color"`

	// TabModeBorderWidth in pixels for tab mode indicator border (Ctrl+P T overlay)
	TabModeBorderWidth int `mapstructure:"tab_mode_border_width" yaml:"tab_mode_border_width" toml:"tab_mode_border_width" json:"tab_mode_border_width"`
	// TabModeBorderColor for the tab mode indicator border (CSS color value or theme variable)
	// Defaults to "#FFA500" (orange) if not set - MUST be different from PaneModeBorderColor
	TabModeBorderColor string `mapstructure:"tab_mode_border_color" yaml:"tab_mode_border_color" toml:"tab_mode_border_color" json:"tab_mode_border_color"`

	// TransitionDuration in milliseconds for border animations
	TransitionDuration int `mapstructure:"transition_duration" yaml:"transition_duration" toml:"transition_duration" json:"transition_duration"`
}
