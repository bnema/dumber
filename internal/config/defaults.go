// Package config provides default configuration values for dumber.
package config

import (
	"time"
)

// Default configuration constants
const (
	// Database defaults
	defaultMaxIdleTimeMin  = 5  // minutes
	defaultQueryTimeoutSec = 30 // seconds

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
	defaultPaneActivationShortcut  = "cmdorctrl+p"
	defaultPaneTimeoutMilliseconds = 3000
	defaultPopupPlacement          = "right"

	// Workspace styling defaults
	defaultBorderWidth        = 2
	defaultBorderColor        = "@theme_selected_bg_color"
	defaultTransitionDuration = 120
	defaultBorderRadius       = 0
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
			MaxConnections: 1,
			MaxIdleTime:    time.Minute * defaultMaxIdleTimeMin,
			QueryTimeout:   time.Second * defaultQueryTimeoutSec,
		},
		History: HistoryConfig{
			MaxEntries:          defaultMaxHistoryEntries,
			RetentionPeriodDays: defaultRetentionDays, // 1 year
			CleanupIntervalDays: 1,                    // daily cleanup
		},
		SearchShortcuts: GetDefaultSearchShortcuts(),
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
			AV1MaxResolution:          "1080p",                                                                                                 // Optimal AV1 up to 1080p, fallback to VP9 for higher res
			DisableTwitchCodecControl: true,                                                                                                    // Disable codec control on Twitch by default (prevents theater/fullscreen freezing)
		},
		WebkitMemory: WebkitMemoryConfig{
			CacheModel:              "web_browser", // Aggressive caching for fast page loads
			EnablePageCache:         true,          // Instant back/forward navigation
			MemoryLimitMB:           0,             // Use system default
			ConservativeThreshold:   0.4,           // Start cleanup at 40%
			StrictThreshold:         0.6,           // Strict cleanup at 60%
			KillThreshold:           0.8,           // Kill processes at 80%
			PollIntervalSeconds:     45.0,          // Check every 45 seconds
			EnableGCInterval:        120,           // GC every 2 minutes
			ProcessRecycleThreshold: 50,            // Recycle after 50 page loads
			EnableMemoryMonitoring:  true,          // Monitor for production tuning
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
		APISecurity: APISecurityConfig{
			Token:        "",
			RequireToken: false,
		},
		RenderingMode: RenderingModeGPU,
		UseDomZoom:    false,
		DefaultZoom:   1.2, // 120% default zoom for better readability
		Workspace: WorkspaceConfig{
			EnableZellijControls: true,
			PaneMode: PaneModeConfig{
				ActivationShortcut:  defaultPaneActivationShortcut,
				TimeoutMilliseconds: defaultPaneTimeoutMilliseconds,
				ActionBindings: map[string]string{
					"arrowright": "split-right",
					"arrowleft":  "split-left",
					"arrowup":    "split-up",
					"arrowdown":  "split-down",
					"r":          "split-right",
					"l":          "split-left",
					"u":          "split-up",
					"d":          "split-down",
					"x":          "close-pane",
					"enter":      "confirm",
					"escape":     "cancel",
				},
			},
			Tabs: TabKeyConfig{
				NewTab:      "cmdorctrl+t",
				CloseTab:    "cmdorctrl+w",
				NextTab:     "cmdorctrl+tab",
				PreviousTab: "cmdorctrl+shift+tab",
			},
			Popups: PopupBehaviorConfig{
				Placement:            defaultPopupPlacement,
				OpenInNewPane:        true,
				FollowPaneContext:    true,
				BlankTargetBehavior:  "pane", // Default to pane, future: "tab"
				EnableSmartDetection: true,   // Use WindowProperties to detect popup vs tab
				OAuthAutoClose:       true,   // Auto-close OAuth popups on success
			},
			Styling: WorkspaceStylingConfig{
				BorderWidth:        defaultBorderWidth,
				BorderColor:        defaultBorderColor,
				TransitionDuration: defaultTransitionDuration,
				BorderRadius:       defaultBorderRadius,
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
