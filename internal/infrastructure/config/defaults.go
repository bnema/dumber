package config

// Default configuration constants
const (
	// History defaults
	defaultMaxHistoryEntries = 10000 // entries
	defaultRetentionDays     = 365   // 1 year

	// Dmenu defaults
	defaultMaxHistoryDays = 30 // days

	// Logging defaults
	defaultMaxLogAgeDays = 7 // days

	// Appearance defaults
	defaultFontSize = 16 // points

	// Workspace defaults
	defaultNewPaneURL = "about:blank"

	// Omnibox defaults
	defaultOmniboxInitialBehavior   = "recent"
	defaultOmniboxAutoOpenOnNewPane = false

	// Workspace defaults
	defaultPaneActivationShortcut    = "ctrl+p"
	defaultPaneTimeoutMilliseconds   = 3000
	defaultTabActivationShortcut     = "ctrl+t"
	defaultTabTimeoutMilliseconds    = 3000
	defaultResizeActivationShortcut  = "ctrl+n"
	defaultResizeTimeoutMilliseconds = 3000
	defaultResizeStepPercent         = 5.0
	defaultResizeMinPanePercent      = 10.0
	defaultTabBarPosition            = "bottom"
	defaultPopupPlacement            = "right"

	// Session defaults
	defaultSessionActivationShortcut  = "ctrl+o"
	defaultSessionTimeoutMilliseconds = 3000
	defaultSnapshotIntervalMs         = 5000
	defaultMaxExitedSessions          = 50
	defaultMaxExitedSessionAgeDays    = 7

	// Workspace styling defaults
	// Active pane border (overlay)
	defaultBorderWidth = 1
	defaultBorderColor = "@theme_selected_bg_color"

	// Mode border width (consolidated for all modes)
	defaultModeBorderWidth = 4

	// Mode colors (used for both borders and toaster)
	defaultPaneModeColor    = "#4A90E2" // Blue for pane mode
	defaultTabModeColor     = "#FFA500" // Orange for tab mode
	defaultSessionModeColor = "#9B59B6" // Purple for session mode
	defaultResizeModeColor  = "#00D4AA" // Cyan/teal for resize mode

	// Mode indicator toaster
	defaultModeIndicatorToasterEnabled = true

	// Other styling
	defaultTransitionDuration = 120
	defaultUIScale            = 1.0 // UI scale multiplier (1.0 = 100%, 1.2 = 120%)

	// Performance defaults
	defaultZoomCacheSize           = 256 // domains to cache (~20KB memory)
	defaultWebViewPoolPrewarmCount = 4   // WebViews to pre-create at startup

	// Skia threading defaults (0 = unset, -1 = unset for GPU threads)
	defaultSkiaCPUPaintingThreads = 0
	defaultSkiaGPUPaintingThreads = -1 // -1 means unset; 0 would disable GPU tile painting
)

// getDefaultLogDir returns the default log directory, falls back to empty string on error
func getDefaultLogDir() string {
	logDir, err := GetLogDir()
	if err != nil {
		return ""
	}
	return logDir
}

// DefaultConfig returns the default configuration values for dumber.
func DefaultConfig() *Config {
	return &Config{
		Database: DatabaseConfig{
			// Path is set dynamically in config.Load()
		},
		History: HistoryConfig{
			MaxEntries:          defaultMaxHistoryEntries,
			RetentionPeriodDays: defaultRetentionDays, // 1 year
			CleanupIntervalDays: 1,                    // daily cleanup
		},
		SearchShortcuts:     GetDefaultSearchShortcuts(),
		DefaultSearchEngine: "https://duckduckgo.com/?q=%s",
		Dmenu: DmenuConfig{
			MaxHistoryDays:   defaultMaxHistoryDays,
			ShowVisitCount:   true,
			ShowLastVisited:  true,
			HistoryPrefix:    "🕒",
			ShortcutPrefix:   "🔍",
			URLPrefix:        "🌐",
			DateFormat:       "2006-01-02 15:04",
			SortByVisitCount: true,
		},
		Logging: LoggingConfig{
			Level:          "info",
			Format:         "text", // text or json
			MaxAge:         defaultMaxLogAgeDays,
			LogDir:         getDefaultLogDir(),
			EnableFileLog:  true,
			CaptureConsole: false, // Disabled by default
			CaptureGTKLogs: false, // Disabled by default
		},
		Appearance: AppearanceConfig{
			SansFont:        "Fira Sans",
			SerifFont:       "Fira Sans",
			MonospaceFont:   "Fira Code",
			DefaultFontSize: defaultFontSize,
			LightPalette: ColorPalette{
				Background:     "#fafafa",
				Surface:        "#f4f4f5",
				SurfaceVariant: "#e4e4e7",
				Text:           "#18181b",
				Muted:          "#71717a",
				Accent:         "#22c55e", // Green-500 - vibrant primary
				Border:         "#d4d4d8",
			},
			DarkPalette: ColorPalette{
				Background:     "#0a0a0b",
				Surface:        "#18181b",
				SurfaceVariant: "#27272a",
				Text:           "#fafafa",
				Muted:          "#a1a1aa",
				Accent:         "#4ade80", // Green-400 - vibrant primary
				Border:         "#3f3f46",
			},
			ColorScheme: "default", // default follows system theme
		},
		Debug: DebugConfig{
			EnableDevTools: true,
		},
		Rendering: RenderingConfig{
			Mode:                      RenderingModeGPU,
			DisableDMABufRenderer:     false,
			ForceCompositingMode:      false,
			DisableCompositingMode:    false,
			GSKRenderer:               GSKRendererAuto, // Let GTK choose - Vulkan can conflict with WebKit's DMA-BUF
			DisableMipmaps:            false,
			PreferGL:                  false,
			DrawCompositingIndicators: false,
			ShowFPS:                   false,
			SampleMemory:              false,
			DebugFrames:               false,
		},
		Privacy: PrivacyConfig{
			CookiePolicy: CookiePolicyNoThirdParty,
			ITPEnabled:   true,
		},
		DefaultWebpageZoom: 1.2,            // 120% default zoom for better readability
		DefaultUIScale:     defaultUIScale, // 1.0 = 100%, 2.0 = 200%
		Workspace: WorkspaceConfig{
			NewPaneURL:        defaultNewPaneURL,
			SwitchToTabOnMove: true,
			PaneMode: PaneModeConfig{
				ActivationShortcut:  defaultPaneActivationShortcut,
				TimeoutMilliseconds: defaultPaneTimeoutMilliseconds,
				Actions: map[string]ActionBinding{
					"split-right":           {Keys: []string{"arrowright", "r"}, Desc: "Split pane to the right"},
					"split-left":            {Keys: []string{"arrowleft", "l"}, Desc: "Split pane to the left"},
					"split-up":              {Keys: []string{"arrowup", "u"}, Desc: "Split pane upward"},
					"split-down":            {Keys: []string{"arrowdown", "d"}, Desc: "Split pane downward"},
					"stack-pane":            {Keys: []string{"s"}, Desc: "Stack pane with sibling"},
					"close-pane":            {Keys: []string{"x"}, Desc: "Close current pane"},
					"move-pane-to-tab":      {Keys: []string{"m"}, Desc: "Move pane to different tab"},
					"move-pane-to-next-tab": {Keys: []string{"M", "shift+m"}, Desc: "Move pane to next tab"},

					"consume-or-expel-left":  {Keys: []string{"["}, Desc: "Consume/expel pane left"},
					"consume-or-expel-right": {Keys: []string{"]"}, Desc: "Consume/expel pane right"},
					"consume-or-expel-up":    {Keys: []string{"{"}, Desc: "Consume/expel pane up"},
					"consume-or-expel-down":  {Keys: []string{"}"}, Desc: "Consume/expel pane down"},

					"focus-right": {Keys: []string{"shift+arrowright", "shift+l"}, Desc: "Focus pane to the right"},
					"focus-left":  {Keys: []string{"shift+arrowleft", "shift+h"}, Desc: "Focus pane to the left"},
					"focus-up":    {Keys: []string{"shift+arrowup", "shift+k"}, Desc: "Focus pane above"},
					"focus-down":  {Keys: []string{"shift+arrowdown", "shift+j"}, Desc: "Focus pane below"},
					"confirm":     {Keys: []string{"enter"}, Desc: "Confirm action"},
					"cancel":      {Keys: []string{"escape"}, Desc: "Cancel/exit mode"},
				},
			},
			TabMode: TabModeConfig{
				ActivationShortcut:  defaultTabActivationShortcut,
				TimeoutMilliseconds: defaultTabTimeoutMilliseconds,
				Actions: map[string]ActionBinding{
					"new-tab":      {Keys: []string{"n", "c"}, Desc: "Create new tab"},
					"close-tab":    {Keys: []string{"x"}, Desc: "Close current tab"},
					"next-tab":     {Keys: []string{"l", "tab"}, Desc: "Switch to next tab"},
					"previous-tab": {Keys: []string{"h", "shift+tab"}, Desc: "Switch to previous tab"},
					"rename-tab":   {Keys: []string{"r"}, Desc: "Rename current tab"},
					"confirm":      {Keys: []string{"enter"}, Desc: "Confirm action"},
					"cancel":       {Keys: []string{"escape"}, Desc: "Cancel/exit mode"},
				},
			},
			ResizeMode: ResizeModeConfig{
				ActivationShortcut:  defaultResizeActivationShortcut,
				TimeoutMilliseconds: defaultResizeTimeoutMilliseconds,
				StepPercent:         defaultResizeStepPercent,
				MinPanePercent:      defaultResizeMinPanePercent,
				Actions: map[string]ActionBinding{
					"resize-increase-left":  {Keys: []string{"h", "arrowleft"}, Desc: "Increase size leftward"},
					"resize-increase-down":  {Keys: []string{"j", "arrowdown"}, Desc: "Increase size downward"},
					"resize-increase-up":    {Keys: []string{"k", "arrowup"}, Desc: "Increase size upward"},
					"resize-increase-right": {Keys: []string{"l", "arrowright"}, Desc: "Increase size rightward"},
					"resize-decrease-left":  {Keys: []string{"H"}, Desc: "Decrease size leftward"},
					"resize-decrease-down":  {Keys: []string{"J"}, Desc: "Decrease size downward"},
					"resize-decrease-up":    {Keys: []string{"K"}, Desc: "Decrease size upward"},
					"resize-decrease-right": {Keys: []string{"L"}, Desc: "Decrease size rightward"},
					"resize-increase":       {Keys: []string{"+", "="}, Desc: "Increase pane size"},
					"resize-decrease":       {Keys: []string{"-"}, Desc: "Decrease pane size"},
					"confirm":               {Keys: []string{"enter"}, Desc: "Confirm action"},
					"cancel":                {Keys: []string{"escape"}, Desc: "Cancel/exit mode"},
				},
			},
			Shortcuts: GlobalShortcutsConfig{
				Actions: map[string]ActionBinding{
					"close_pane":             {Keys: []string{"ctrl+w"}, Desc: "Close active pane"},
					"next_tab":               {Keys: []string{"ctrl+tab"}, Desc: "Switch to next tab"},
					"previous_tab":           {Keys: []string{"ctrl+shift+tab"}, Desc: "Switch to previous tab"},
					"consume_or_expel_left":  {Keys: []string{"alt+["}, Desc: "Consume/expel pane left"},
					"consume_or_expel_right": {Keys: []string{"alt+]"}, Desc: "Consume/expel pane right"},
					"consume_or_expel_up":    {Keys: []string{"alt+{"}, Desc: "Consume/expel pane up"},
					"consume_or_expel_down":  {Keys: []string{"alt+}"}, Desc: "Consume/expel pane down"},
				},
			},
			TabBarPosition:          defaultTabBarPosition,
			HideTabBarWhenSingleTab: true,
			Popups: PopupBehaviorConfig{
				Behavior:             PopupBehaviorSplit, // Default: open JavaScript popups in split panes
				Placement:            defaultPopupPlacement,
				OpenInNewPane:        true,
				FollowPaneContext:    true,
				BlankTargetBehavior:  "stacked", // Default: open _blank links in stacked mode
				EnableSmartDetection: true,      // Use WindowProperties to detect popup vs tab
				OAuthAutoClose:       true,      // Auto-close OAuth popups on success
			},
			Styling: WorkspaceStylingConfig{
				BorderWidth:                 defaultBorderWidth,
				BorderColor:                 defaultBorderColor,
				ModeBorderWidth:             defaultModeBorderWidth,
				PaneModeColor:               defaultPaneModeColor,
				TabModeColor:                defaultTabModeColor,
				SessionModeColor:            defaultSessionModeColor,
				ResizeModeColor:             defaultResizeModeColor,
				ModeIndicatorToasterEnabled: defaultModeIndicatorToasterEnabled,
				TransitionDuration:          defaultTransitionDuration,
			},
		},
		ContentFiltering: ContentFilteringConfig{
			Enabled:    true, // Ad blocking enabled by default
			AutoUpdate: true, // Auto-update filters from GitHub releases
		},
		Clipboard: ClipboardConfig{
			AutoCopyOnSelection: true, // Enabled by default (zellij-style)
		},
		Omnibox: OmniboxConfig{
			InitialBehavior:   defaultOmniboxInitialBehavior,
			AutoOpenOnNewPane: defaultOmniboxAutoOpenOnNewPane,
		},
		Session: SessionConfig{
			AutoRestore:             false,
			SnapshotIntervalMs:      defaultSnapshotIntervalMs,
			MaxExitedSessions:       defaultMaxExitedSessions,
			MaxExitedSessionAgeDays: defaultMaxExitedSessionAgeDays,
			SessionMode: SessionModeConfig{
				ActivationShortcut:  defaultSessionActivationShortcut,
				TimeoutMilliseconds: defaultSessionTimeoutMilliseconds,
				Actions: map[string]ActionBinding{
					"session-manager": {Keys: []string{"s", "w"}, Desc: "Open session manager"},
					"confirm":         {Keys: []string{"enter"}, Desc: "Confirm action"},
					"cancel":          {Keys: []string{"escape"}, Desc: "Cancel/exit mode"},
				},
			},
		},
		Media: MediaConfig{
			HardwareDecodingMode:     HardwareDecodingAuto, // auto allows sw fallback
			PreferAV1:                false,                // Don't force codec preference, let site choose
			ShowDiagnosticsOnStartup: false,                // Disabled - diagnostics can be noisy
			ForceVSync:               false,                // Let compositor handle VSync
			GLRenderingMode:          GLRenderingModeAuto,  // GStreamer picks best GL API
			GStreamerDebugLevel:      0,                    // Disabled by default
		},
		Runtime: RuntimeConfig{
			Prefix: "",
		},
		Performance: PerformanceConfig{
			Profile:                 ProfileLite, // Daily-use default: lighter profile
			ZoomCacheSize:           defaultZoomCacheSize,
			WebViewPoolPrewarmCount: defaultWebViewPoolPrewarmCount,
			// Skia threading - balanced defaults (unset = use WebKit defaults)
			// These only apply when Profile is "custom"
			SkiaCPUPaintingThreads: defaultSkiaCPUPaintingThreads,
			SkiaGPUPaintingThreads: defaultSkiaGPUPaintingThreads,
			SkiaEnableCPURendering: false,
			// Web process memory pressure - all unset by default
			WebProcessMemoryLimitMB:               0,
			WebProcessMemoryPollIntervalSec:       0,
			WebProcessMemoryConservativeThreshold: 0,
			WebProcessMemoryStrictThreshold:       0,
			// Network process memory pressure - all unset by default
			NetworkProcessMemoryLimitMB:               0,
			NetworkProcessMemoryPollIntervalSec:       0,
			NetworkProcessMemoryConservativeThreshold: 0,
			NetworkProcessMemoryStrictThreshold:       0,
		},
		Update: UpdateConfig{
			EnableOnStartup:     true,  // Check for updates on startup by default
			AutoDownload:        false, // Conservative: don't auto-download by default
			NotifyOnNewSettings: true,  // Show toast when new config settings available
		},
		Downloads: DownloadsConfig{
			Path: "", // Empty = use XDG_DOWNLOAD_DIR or ~/Downloads
		},
	}
}

// GetDefaultSearchShortcuts returns the default search shortcuts.
func GetDefaultSearchShortcuts() map[string]SearchShortcut {
	return map[string]SearchShortcut{
		"ddg": {
			URL:         "https://duckduckgo.com/?q=%s",
			Description: "DuckDuckGo search",
		},
		"g": {
			URL:         "https://www.google.com/search?q=%s",
			Description: "Google search",
		},
		"gi": {
			URL:         "https://www.google.com/search?tbm=isch&q=%s",
			Description: "Google Images search",
		},
		"gh": {
			URL:         "https://github.com/search?q=%s",
			Description: "GitHub search",
		},
		"go": {
			URL:         "https://pkg.go.dev/search?q=%s",
			Description: "Go package search",
		},
		"mdn": {
			URL:         "https://developer.mozilla.org/en-US/search?q=%s",
			Description: "MDN Web Docs search",
		},
		"npm": {
			URL:         "https://www.npmjs.com/search?q=%s",
			Description: "npm package search",
		},
		"r": {
			URL:         "https://www.reddit.com/search?q=%s",
			Description: "Reddit search",
		},
		"so": {
			URL:         "https://stackoverflow.com/search?q=%s",
			Description: "Stack Overflow search",
		},
		"w": {
			URL:         "https://en.wikipedia.org/wiki/%s",
			Description: "Wikipedia search",
		},
		"yt": {
			URL:         "https://www.youtube.com/results?search_query=%s",
			Description: "YouTube search",
		},
	}
}
