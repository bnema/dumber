// Package config provides default configuration values for dumber.
package config

// Default configuration constants
const (
	// History defaults
	defaultMaxHistoryEntries = 10000 // entries
	defaultRetentionDays     = 365   // 1 year

	// Dmenu defaults
	defaultMaxHistoryItems = 20 // items

	// Logging defaults
	defaultMaxLogSizeMB  = 100 // MB
	defaultMaxBackups    = 3   // backup files
	defaultMaxLogAgeDays = 7   // days

	// Appearance defaults
	defaultFontSize = 16 // points

	// Workspace defaults
	defaultPaneActivationShortcut  = "ctrl+p"
	defaultPaneTimeoutMilliseconds = 3000
	defaultTabActivationShortcut   = "ctrl+t"
	defaultTabTimeoutMilliseconds  = 3000
	defaultTabBarPosition          = "bottom"
	defaultPopupPlacement          = "right"

	// Workspace styling defaults
	defaultBorderWidth            = 1
	defaultBorderColor            = "@theme_selected_bg_color"
	defaultInactiveBorderWidth    = 1                             // Same width as active to prevent layout shift
	defaultInactiveBorderColor    = "@theme_unfocused_bg_color"   // GTK theme variable
	defaultShowStackedTitleBorder = false                         // Hidden by default
	defaultPaneModeBorderColor    = "#4A90E2"                     // Blue for pane mode indicator
	defaultTabModeBorderColor     = "#FFA500"                     // Orange for tab mode indicator (distinct from pane mode)
	defaultTransitionDuration     = 120
	defaultBorderRadius           = 0
	defaultUIScale                = 1.0 // UI scale multiplier (1.0 = 100%, 1.2 = 120%)
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
			Format:         "text",              // text or json
			Filename:       "",                  // empty means stdout
			MaxSize:        defaultMaxLogSizeMB, // MB
			MaxBackups:     defaultMaxBackups,
			MaxAge:         defaultMaxLogAgeDays, // days
			Compress:       true,
			LogDir:         getDefaultLogDir(),
			EnableFileLog:  true,
			CaptureStdout:  false,
			CaptureStderr:  false,
			CaptureCOutput: false, // Disabled by default to avoid performance impact
			CaptureConsole: false, // Disabled by default
			DebugFile:      "debug.log",
			VerboseWebKit:  false,
		},
		Appearance: AppearanceConfig{
			SansFont:        "Fira Sans",
			SerifFont:       "Fira Sans",
			MonospaceFont:   "Fira Code",
			DefaultFontSize: defaultFontSize,
			LightPalette: ColorPalette{
				Background:     "#f8f8f8",
				Surface:        "#f2f2f2",
				SurfaceVariant: "#ececec",
				Text:           "#1a1a1a",
				Muted:          "#6e6e6e",
				Accent:         "#404040",
				Border:         "#d2d2d2",
			},
			DarkPalette: ColorPalette{
				Background:     "#0e0e0e",
				Surface:        "#1a1a1a",
				SurfaceVariant: "#141414",
				Text:           "#e4e4e4",
				Muted:          "#848484",
				Accent:         "#a8a8a8",
				Border:         "#363636",
			},
			ColorScheme: "default", // default follows system theme
		},
		VideoAcceleration: VideoAccelerationConfig{
			EnableVAAPI:      true,
			AutoDetectGPU:    true,
			VAAPIDriverName:  "", // Will be auto-detected
			EnableAllDrivers: true,
			LegacyVAAPI:      false,
		},
		CodecPreferences: CodecConfig{
			PreferredCodecs:           "av1,h264",                                                                                              // AV1 first, but allow VP9 fallback for higher resolutions
			ForceAV1:                  false,                                                                                                   // Use smart AV1 negotiation instead of forcing
			BlockVP9:                  false,                                                                                                   // Allow VP9 for higher resolutions
			BlockVP8:                  true,                                                                                                    // Still block VP8 as it's outdated
			AV1HardwareOnly:           false,                                                                                                   // Allow software AV1 fallback
			DisableVP9Hardware:        false,                                                                                                   // Allow VP9 hardware for high res content
			VideoBufferSizeMB:         16,                                                                                                      // Larger buffer for AV1 streams
			QueueBufferTimeSec:        10,                                                                                                      // More buffering time for smooth playback
			CustomUserAgent:           "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36", // Chrome UA with AV1 support
			AV1MaxResolution:          "1080p", // Optimal AV1 up to 1080p, fallback to VP9 for higher res
			DisableTwitchCodecControl: true,    // Disable codec control on Twitch by default (prevents theater/fullscreen freezing)
		},
		Debug: DebugConfig{
			EnableWebKitDebug:     false,
			WebKitDebugCategories: "Network:preconnectTo,ContentFilters",
			EnableFilteringDebug:  false,
			EnableWebViewDebug:    false,
			LogWebKitCrashes:      true, // Always log crashes
			EnableScriptDebug:     false,
			EnableGeneralDebug:    false,
			EnableWorkspaceDebug:  false,
			EnableFocusDebug:      false,
			EnableCSSDebug:        false,
			EnableFocusMetrics:    false,
			EnablePaneCloseDebug:  false,
		},
		RenderingMode: RenderingModeGPU,
		UseDomZoom:    false,
		DefaultZoom:   1.2, // 120% default zoom for better readability
		Workspace: WorkspaceConfig{
			PaneMode: PaneModeConfig{
				ActivationShortcut:  defaultPaneActivationShortcut,
				TimeoutMilliseconds: defaultPaneTimeoutMilliseconds,
				Actions: map[string][]string{
					"split-right": {"arrowright", "r"},
					"split-left":  {"arrowleft", "l"},
					"split-up":    {"arrowup", "u"},
					"split-down":  {"arrowdown", "d"},
					"close-pane":  {"x"},
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
			Tabs: TabKeyConfig{
				NewTab:      "ctrl+t",
				CloseTab:    "ctrl+w",
				NextTab:     "ctrl+tab",
				PreviousTab: "ctrl+shift+tab",
			},
			TabBarPosition: defaultTabBarPosition,
			Popups: PopupBehaviorConfig{
				Behavior:             PopupBehaviorSplit, // Default: open JavaScript popups in split panes
				Placement:            defaultPopupPlacement,
				OpenInNewPane:        true,
				FollowPaneContext:    true,
				BlankTargetBehavior:  "stacked", // Default: open _blank links in stacked mode
				EnableSmartDetection: true,   // Use WindowProperties to detect popup vs tab
				OAuthAutoClose:       true,   // Auto-close OAuth popups on success
			},
			Styling: WorkspaceStylingConfig{
				BorderWidth:            defaultBorderWidth,
				BorderColor:            defaultBorderColor,
				InactiveBorderWidth:    defaultInactiveBorderWidth,
				InactiveBorderColor:    defaultInactiveBorderColor,
				ShowStackedTitleBorder: defaultShowStackedTitleBorder,
				PaneModeBorderColor:    defaultPaneModeBorderColor,
				TabModeBorderColor:     defaultTabModeBorderColor,
				TransitionDuration:     defaultTransitionDuration,
				BorderRadius:           defaultBorderRadius,
				UIScale:                defaultUIScale,
			},
		},
		ContentFilteringWhitelist: []string{
			"twitch.tv",          // Arkose Labs bot detection breaks with filtering
			"passport.twitch.tv", // Auth subdomain
			"gql.twitch.tv",      // GraphQL API
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

// Note: We build our own browser, so no external browser commands needed
