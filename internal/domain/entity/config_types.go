package entity

// Config value types shared between infrastructure/config and the UI layer.
// Keeping these in domain/entity allows UI packages to use them without
// importing infrastructure/config.

// AppearanceConfig holds visual appearance settings.
type AppearanceConfig struct {
	SansFont        string       `mapstructure:"sans_font" yaml:"sans_font" toml:"sans_font"`
	SerifFont       string       `mapstructure:"serif_font" yaml:"serif_font" toml:"serif_font"`
	MonospaceFont   string       `mapstructure:"monospace_font" yaml:"monospace_font" toml:"monospace_font"`
	DefaultFontSize int          `mapstructure:"default_font_size" yaml:"default_font_size" toml:"default_font_size"`
	LightPalette    ColorPalette `mapstructure:"light_palette" yaml:"light_palette" toml:"light_palette"`
	DarkPalette     ColorPalette `mapstructure:"dark_palette" yaml:"dark_palette" toml:"dark_palette"`
	ColorScheme     string       `mapstructure:"color_scheme" yaml:"color_scheme" toml:"color_scheme"`
}

// ColorPalette defines the color scheme for a theme variant (light/dark).
type ColorPalette struct {
	Background     string `mapstructure:"background" yaml:"background" toml:"background" json:"background"`
	Surface        string `mapstructure:"surface" yaml:"surface" toml:"surface" json:"surface"`
	SurfaceVariant string `mapstructure:"surface_variant" yaml:"surface_variant" toml:"surface_variant" json:"surface_variant"`
	Text           string `mapstructure:"text" yaml:"text" toml:"text" json:"text"`
	Muted          string `mapstructure:"muted" yaml:"muted" toml:"muted" json:"muted"`
	Accent         string `mapstructure:"accent" yaml:"accent" toml:"accent" json:"accent"`
	Border         string `mapstructure:"border" yaml:"border" toml:"border" json:"border"`
}

// ActionBinding maps a config action name to key bindings.
type ActionBinding struct {
	Keys []string `mapstructure:"keys" yaml:"keys" toml:"keys" json:"keys"`
	Desc string   `mapstructure:"desc" yaml:"desc" toml:"desc" json:"desc,omitempty"`
}

// PaneModeConfig holds pane mode shortcut configuration.
type PaneModeConfig struct {
	ActivationShortcut  string                   `mapstructure:"activation_shortcut" yaml:"activation_shortcut" toml:"activation_shortcut" json:"activation_shortcut"` //nolint:lll // struct tags must stay on one line
	TimeoutMilliseconds int                      `mapstructure:"timeout_ms" yaml:"timeout_ms" toml:"timeout_ms" json:"timeout_ms"`
	Actions             map[string]ActionBinding `mapstructure:"actions" yaml:"actions" toml:"actions" json:"actions"`
}

// keyBindingsFromActions converts an action binding map to a key→action map.
// This is the canonical implementation shared by all GetKeyBindings methods.
func keyBindingsFromActions(actions map[string]ActionBinding) map[string]string {
	keyToAction := make(map[string]string)
	for action, binding := range actions {
		for _, key := range binding.Keys {
			keyToAction[key] = action
		}
	}
	return keyToAction
}

// GetKeyBindings returns a map from key string to action name.
func (p *PaneModeConfig) GetKeyBindings() map[string]string {
	return keyBindingsFromActions(p.Actions)
}

// TabModeConfig holds tab mode shortcut configuration.
type TabModeConfig struct {
	ActivationShortcut  string                   `mapstructure:"activation_shortcut" yaml:"activation_shortcut" toml:"activation_shortcut" json:"activation_shortcut"` //nolint:lll // struct tags must stay on one line
	TimeoutMilliseconds int                      `mapstructure:"timeout_ms" yaml:"timeout_ms" toml:"timeout_ms" json:"timeout_ms"`
	Actions             map[string]ActionBinding `mapstructure:"actions" yaml:"actions" toml:"actions" json:"actions"`
}

// GetKeyBindings returns a map from key string to action name.
func (t *TabModeConfig) GetKeyBindings() map[string]string {
	return keyBindingsFromActions(t.Actions)
}

// ResizeModeConfig holds resize mode shortcut configuration.
type ResizeModeConfig struct {
	ActivationShortcut  string                   `mapstructure:"activation_shortcut" yaml:"activation_shortcut" toml:"activation_shortcut" json:"activation_shortcut"` //nolint:lll // struct tags must stay on one line
	TimeoutMilliseconds int                      `mapstructure:"timeout_ms" yaml:"timeout_ms" toml:"timeout_ms" json:"timeout_ms"`
	StepPercent         float64                  `mapstructure:"step_percent" yaml:"step_percent" toml:"step_percent" json:"step_percent"`
	MinPanePercent      float64                  `mapstructure:"min_pane_percent" yaml:"min_pane_percent" toml:"min_pane_percent" json:"min_pane_percent"` //nolint:lll // struct tags must stay on one line
	Actions             map[string]ActionBinding `mapstructure:"actions" yaml:"actions" toml:"actions" json:"actions"`
}

// GetKeyBindings returns a map from key string to action name.
func (r *ResizeModeConfig) GetKeyBindings() map[string]string {
	return keyBindingsFromActions(r.Actions)
}

// SessionModeConfig holds session mode shortcut configuration.
type SessionModeConfig struct {
	ActivationShortcut  string                   `mapstructure:"activation_shortcut" yaml:"activation_shortcut" toml:"activation_shortcut" json:"activation_shortcut"` //nolint:lll // struct tags must stay on one line
	TimeoutMilliseconds int                      `mapstructure:"timeout_ms" yaml:"timeout_ms" toml:"timeout_ms" json:"timeout_ms"`
	Actions             map[string]ActionBinding `mapstructure:"actions" yaml:"actions" toml:"actions" json:"actions"`
}

// GetKeyBindings returns a map from key string to action name.
func (s *SessionModeConfig) GetKeyBindings() map[string]string {
	return keyBindingsFromActions(s.Actions)
}

// SessionConfig holds session persistence and restoration settings.
type SessionConfig struct {
	AutoRestore bool `mapstructure:"auto_restore" yaml:"auto_restore" toml:"auto_restore" json:"auto_restore"`

	SnapshotIntervalMs int `mapstructure:"snapshot_interval_ms" yaml:"snapshot_interval_ms" toml:"snapshot_interval_ms" json:"snapshot_interval_ms"` //nolint:lll // struct tags must stay on one line

	MaxExitedSessions int `mapstructure:"max_exited_sessions" yaml:"max_exited_sessions" toml:"max_exited_sessions" json:"max_exited_sessions"` //nolint:lll // struct tags must stay on one line

	MaxExitedSessionAgeDays int `mapstructure:"max_exited_session_age_days" yaml:"max_exited_session_age_days" toml:"max_exited_session_age_days" json:"max_exited_session_age_days"` //nolint:lll // struct tags must stay on one line

	SessionMode SessionModeConfig `mapstructure:"session_mode" yaml:"session_mode" toml:"session_mode" json:"session_mode"`
}

// GlobalShortcutsConfig holds workspace-level global shortcut bindings.
type GlobalShortcutsConfig struct {
	Actions map[string]ActionBinding `mapstructure:"actions" yaml:"actions" toml:"actions" json:"actions"`
}

// GetKeyBindings returns a map from key string to action name.
func (g *GlobalShortcutsConfig) GetKeyBindings() map[string]string {
	return keyBindingsFromActions(g.Actions)
}

// FloatingPaneProfile defines a named floating pane preset.
type FloatingPaneProfile struct {
	Keys []string `mapstructure:"keys" yaml:"keys" toml:"keys" json:"keys"`
	URL  string   `mapstructure:"url" yaml:"url" toml:"url" json:"url"`
	Desc string   `mapstructure:"desc" yaml:"desc" toml:"desc" json:"desc,omitempty"`
}

// FloatingPaneConfig holds floating pane layout and profile settings.
type FloatingPaneConfig struct {
	WidthPct  float64                        `mapstructure:"width_pct" yaml:"width_pct" toml:"width_pct" json:"width_pct"`
	HeightPct float64                        `mapstructure:"height_pct" yaml:"height_pct" toml:"height_pct" json:"height_pct"`
	Profiles  map[string]FloatingPaneProfile `mapstructure:"profiles" yaml:"profiles" toml:"profiles" json:"profiles"`
}

// WorkspaceStylingConfig defines visual styling for workspace elements.
type WorkspaceStylingConfig struct {
	BorderWidth     int    `mapstructure:"border_width" yaml:"border_width" toml:"border_width" json:"border_width"`
	BorderColor     string `mapstructure:"border_color" yaml:"border_color" toml:"border_color" json:"border_color"`
	ModeBorderWidth int    `mapstructure:"mode_border_width" yaml:"mode_border_width" toml:"mode_border_width" json:"mode_border_width"` //nolint:lll // struct tags must stay on one line

	PaneModeColor    string `mapstructure:"pane_mode_color" yaml:"pane_mode_color" toml:"pane_mode_color" json:"pane_mode_color"`             //nolint:lll // struct tags must stay on one line
	TabModeColor     string `mapstructure:"tab_mode_color" yaml:"tab_mode_color" toml:"tab_mode_color" json:"tab_mode_color"`                 //nolint:lll // struct tags must stay on one line
	SessionModeColor string `mapstructure:"session_mode_color" yaml:"session_mode_color" toml:"session_mode_color" json:"session_mode_color"` //nolint:lll // struct tags must stay on one line
	ResizeModeColor  string `mapstructure:"resize_mode_color" yaml:"resize_mode_color" toml:"resize_mode_color" json:"resize_mode_color"`     //nolint:lll // struct tags must stay on one line

	ModeIndicatorToasterEnabled bool `mapstructure:"mode_indicator_toaster_enabled" yaml:"mode_indicator_toaster_enabled" toml:"mode_indicator_toaster_enabled" json:"mode_indicator_toaster_enabled"` //nolint:lll // struct tags must stay on one line

	TransitionDuration int `mapstructure:"transition_duration" yaml:"transition_duration" toml:"transition_duration" json:"transition_duration"` //nolint:lll // struct tags must stay on one line
}

// PopupBehavior defines how a popup is placed in the workspace.
type PopupBehavior string

const (
	PopupBehaviorSplit    PopupBehavior = "split"
	PopupBehaviorStacked  PopupBehavior = "stacked"
	PopupBehaviorTabbed   PopupBehavior = "tabbed"
	PopupBehaviorWindowed PopupBehavior = "windowed"
)

// PopupBehaviorConfig controls how popup windows are handled.
type PopupBehaviorConfig struct {
	Behavior PopupBehavior `mapstructure:"behavior" yaml:"behavior" toml:"behavior" json:"behavior"`

	Placement string `mapstructure:"placement" yaml:"placement" toml:"placement" json:"placement"`

	OpenInNewPane bool `mapstructure:"open_in_new_pane" yaml:"open_in_new_pane" toml:"open_in_new_pane" json:"open_in_new_pane"`

	FollowPaneContext bool `mapstructure:"follow_pane_context" yaml:"follow_pane_context" toml:"follow_pane_context" json:"follow_pane_context"` //nolint:lll // struct tags must stay on one line

	BlankTargetBehavior string `mapstructure:"blank_target_behavior" yaml:"blank_target_behavior" toml:"blank_target_behavior" json:"blank_target_behavior"` //nolint:lll // struct tags must stay on one line

	EnableSmartDetection bool `mapstructure:"enable_smart_detection" yaml:"enable_smart_detection" toml:"enable_smart_detection" json:"enable_smart_detection"` //nolint:lll // struct tags must stay on one line

	OAuthAutoClose bool `mapstructure:"oauth_auto_close" yaml:"oauth_auto_close" toml:"oauth_auto_close" json:"oauth_auto_close"`
}

// WorkspaceConfig holds all workspace layout and behavior settings.
type WorkspaceConfig struct {
	NewPaneURL   string                `mapstructure:"new_pane_url" yaml:"new_pane_url" toml:"new_pane_url"`
	PaneMode     PaneModeConfig        `mapstructure:"pane_mode" yaml:"pane_mode" toml:"pane_mode" json:"pane_mode"`
	TabMode      TabModeConfig         `mapstructure:"tab_mode" yaml:"tab_mode" toml:"tab_mode" json:"tab_mode"`
	ResizeMode   ResizeModeConfig      `mapstructure:"resize_mode" yaml:"resize_mode" toml:"resize_mode" json:"resize_mode"`
	Shortcuts    GlobalShortcutsConfig `mapstructure:"shortcuts" yaml:"shortcuts" toml:"shortcuts" json:"shortcuts"`
	FloatingPane FloatingPaneConfig    `mapstructure:"floating_pane" yaml:"floating_pane" toml:"floating_pane" json:"floating_pane"`

	TabBarPosition          string `mapstructure:"tab_bar_position" yaml:"tab_bar_position" toml:"tab_bar_position" json:"tab_bar_position"`
	HideTabBarWhenSingleTab bool   `mapstructure:"hide_tab_bar_when_single_tab" yaml:"hide_tab_bar_when_single_tab" toml:"hide_tab_bar_when_single_tab" json:"hide_tab_bar_when_single_tab"` //nolint:lll // struct tags must stay on one line
	SwitchToTabOnMove       bool   `mapstructure:"switch_to_tab_on_move" yaml:"switch_to_tab_on_move" toml:"switch_to_tab_on_move" json:"switch_to_tab_on_move"`                             //nolint:lll // struct tags must stay on one line

	Popups  PopupBehaviorConfig    `mapstructure:"popups" yaml:"popups" toml:"popups" json:"popups"`
	Styling WorkspaceStylingConfig `mapstructure:"styling" yaml:"styling" toml:"styling" json:"styling"`
}

// UpdateConfig holds auto-update behavior settings.
type UpdateConfig struct {
	EnableOnStartup     bool `mapstructure:"enable_on_startup" yaml:"enable_on_startup" toml:"enable_on_startup"`
	AutoDownload        bool `mapstructure:"auto_download" yaml:"auto_download" toml:"auto_download"`
	NotifyOnNewSettings bool `mapstructure:"notify_on_new_settings" yaml:"notify_on_new_settings" toml:"notify_on_new_settings"`
}
