package config

// Default configuration constants
const (
	// History defaults
	defaultMaxHistoryEntries = 10000 // entries
	defaultRetentionDays     = 365   // 1 year

	// Dmenu defaults
	defaultMaxHistoryItems = 20 // items

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

	// Pane mode border (Ctrl+P N - overlay)
	defaultPaneModeBorderWidth = 4
	defaultPaneModeBorderColor = "#4A90E2" // Blue for pane mode indicator

	// Tab mode border (Ctrl+P T - overlay)
	defaultTabModeBorderWidth = 4
	defaultTabModeBorderColor = "#FFA500" // Orange for tab mode indicator

	// Session mode border (Ctrl+O - overlay)
	defaultSessionModeBorderWidth = 4
	defaultSessionModeBorderColor = "#9B59B6" // Purple for session mode indicator

	// Resize mode border (Ctrl+N - overlay)
	defaultResizeModeBorderWidth = 4
	defaultResizeModeBorderColor = "#00D4AA" // Cyan/teal for resize mode indicator

	// Other styling
	defaultTransitionDuration = 120
	defaultUIScale            = 1.0 // UI scale multiplier (1.0 = 100%, 1.2 = 120%)

	// Performance defaults
	defaultZoomCacheSize           = 256 // domains to cache (~20KB memory)
	defaultWebViewPoolPrewarmCount = 4   // WebViews to pre-create at startup
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
			MaxHistoryItems:  defaultMaxHistoryItems,
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
		DefaultWebpageZoom: 1.2,            // 120% default zoom for better readability
		DefaultUIScale:     defaultUIScale, // 1.0 = 100%, 2.0 = 200%
		Workspace: WorkspaceConfig{
			NewPaneURL:        defaultNewPaneURL,
			SwitchToTabOnMove: true,
			PaneMode: PaneModeConfig{
				ActivationShortcut:  defaultPaneActivationShortcut,
				TimeoutMilliseconds: defaultPaneTimeoutMilliseconds,
				Actions: map[string][]string{
					"split-right":           {"arrowright", "r"},
					"split-left":            {"arrowleft", "l"},
					"split-up":              {"arrowup", "u"},
					"split-down":            {"arrowdown", "d"},
					"stack-pane":            {"s"},
					"close-pane":            {"x"},
					"move-pane-to-tab":      {"m"},
					"move-pane-to-next-tab": {"M", "shift+m"},

					"consume-or-expel-left":  {"bracketleft", "["},
					"consume-or-expel-right": {"bracketright", "]"},
					"consume-or-expel-up":    {"shift+bracketleft", "braceleft", "{"},
					"consume-or-expel-down":  {"shift+bracketright", "braceright", "}"},

					"focus-right": {"shift+arrowright", "shift+l"},
					"focus-left":  {"shift+arrowleft", "shift+h"},
					"focus-up":    {"shift+arrowup", "shift+k"},
					"focus-down":  {"shift+arrowdown", "shift+j"},
					"confirm":     {"enter"},
					"cancel":      {"escape"},
				},
			},
			TabMode: TabModeConfig{
				ActivationShortcut:  defaultTabActivationShortcut,
				TimeoutMilliseconds: defaultTabTimeoutMilliseconds,
				Actions: map[string][]string{
					"new-tab":      {"n", "c"},
					"close-tab":    {"x"},
					"next-tab":     {"l", "tab"},
					"previous-tab": {"h", "shift+tab"},
					"rename-tab":   {"r"},
					"confirm":      {"enter"},
					"cancel":       {"escape"},
				},
			},
			ResizeMode: ResizeModeConfig{
				ActivationShortcut:  defaultResizeActivationShortcut,
				TimeoutMilliseconds: defaultResizeTimeoutMilliseconds,
				StepPercent:         defaultResizeStepPercent,
				MinPanePercent:      defaultResizeMinPanePercent,
				Actions: map[string][]string{
					"resize-increase-left":  {"h", "arrowleft"},
					"resize-increase-down":  {"j", "arrowdown"},
					"resize-increase-up":    {"k", "arrowup"},
					"resize-increase-right": {"l", "arrowright"},
					"resize-decrease-left":  {"H"},
					"resize-decrease-down":  {"J"},
					"resize-decrease-up":    {"K"},
					"resize-decrease-right": {"L"},
					"resize-increase":       {"+", "="},
					"resize-decrease":       {"-"},
					"confirm":               {"enter"},
					"cancel":                {"escape"},
				},
			},
			Shortcuts: GlobalShortcutsConfig{
				ClosePane:   "ctrl+w",
				NextTab:     "ctrl+tab",
				PreviousTab: "ctrl+shift+tab",

				ConsumeOrExpelLeft:  "alt+bracketleft",
				ConsumeOrExpelRight: "alt+bracketright",
				ConsumeOrExpelUp:    "alt+shift+bracketleft",
				ConsumeOrExpelDown:  "alt+shift+bracketright",
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
				BorderWidth:            defaultBorderWidth,
				BorderColor:            defaultBorderColor,
				PaneModeBorderWidth:    defaultPaneModeBorderWidth,
				PaneModeBorderColor:    defaultPaneModeBorderColor,
				TabModeBorderWidth:     defaultTabModeBorderWidth,
				TabModeBorderColor:     defaultTabModeBorderColor,
				SessionModeBorderWidth: defaultSessionModeBorderWidth,
				SessionModeBorderColor: defaultSessionModeBorderColor,
				ResizeModeBorderWidth:  defaultResizeModeBorderWidth,
				ResizeModeBorderColor:  defaultResizeModeBorderColor,
				TransitionDuration:     defaultTransitionDuration,
			},
		},
		ContentFiltering: ContentFilteringConfig{
			Enabled:    true, // Ad blocking enabled by default
			AutoUpdate: true, // Auto-update filters from GitHub releases
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
				Actions: map[string][]string{
					"session-manager": {"s", "w"},
					"confirm":         {"enter"},
					"cancel":          {"escape"},
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
			VideoBufferSizeMB:        0,                    // Not a valid GStreamer env var, removed
			QueueBufferTimeSec:       0,                    // Not a valid GStreamer env var, removed
		},
		Runtime: RuntimeConfig{
			Prefix: "",
		},
		Performance: PerformanceConfig{
			ZoomCacheSize:           defaultZoomCacheSize,
			WebViewPoolPrewarmCount: defaultWebViewPoolPrewarmCount,
		},
		Update: UpdateConfig{
			EnableOnStartup:     true,  // Check for updates on startup by default
			AutoDownload:        false, // Conservative: don't auto-download by default
			NotifyOnNewSettings: true,  // Show toast when new config settings available
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
