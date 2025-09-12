// Package config provides default configuration values for dumber.
package config

import (
	"time"
)

// Default configuration constants
const (
	// Database defaults
	defaultMaxIdleTimeMin   = 5  // minutes
	defaultQueryTimeoutSec  = 30 // seconds

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
		SearchShortcuts: map[string]SearchShortcut{
			"g": {
				URL:         "https://www.google.com/search?q=%s",
				Description: "Google search",
			},
			"gh": {
				URL:         "https://github.com/search?q=%s",
				Description: "GitHub search",
			},
			"yt": {
				URL:         "https://www.youtube.com/results?search_query=%s",
				Description: "YouTube search",
			},
			"w": {
				URL:         "https://en.wikipedia.org/wiki/%s",
				Description: "Wikipedia search",
			},
		},
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
			Level:         "info",
			Format:        "text", // text or json
			Filename:      "",     // empty means stdout
			MaxSize:       defaultMaxLogSizeMB, // MB
			MaxBackups:    defaultMaxBackups,
			MaxAge:        defaultMaxLogAgeDays, // days
			Compress:      true,
			LogDir:        getDefaultLogDir(),
			EnableFileLog: true,
			CaptureStdout: false,
			CaptureStderr: false,
			CaptureCOutput: false, // Disabled by default to avoid performance impact
			DebugFile:     "debug.log",
			VerboseWebKit: false,
		},
		Appearance: AppearanceConfig{
			SansFont:        "Fira Sans",
			SerifFont:       "Fira Sans",
			MonospaceFont:   "Fira Code",
			DefaultFontSize: defaultFontSize,
		},
		RenderingMode: RenderingModeAuto,
	}
}

// GetDefaultSearchShortcuts returns the default search shortcuts.
func GetDefaultSearchShortcuts() map[string]SearchShortcut {
	return map[string]SearchShortcut{
		"g": {
			URL:         "https://www.google.com/search?q=%s",
			Description: "Google search",
		},
		"gh": {
			URL:         "https://github.com/search?q=%s",
			Description: "GitHub search",
		},
		"yt": {
			URL:         "https://www.youtube.com/results?search_query=%s",
			Description: "YouTube search",
		},
		"w": {
			URL:         "https://en.wikipedia.org/wiki/%s",
			Description: "Wikipedia search",
		},
		"ddg": {
			URL:         "https://duckduckgo.com/?q=%s",
			Description: "DuckDuckGo search",
		},
		"so": {
			URL:         "https://stackoverflow.com/search?q=%s",
			Description: "Stack Overflow search",
		},
		"r": {
			URL:         "https://www.reddit.com/search?q=%s",
			Description: "Reddit search",
		},
		"npm": {
			URL:         "https://www.npmjs.com/search?q=%s",
			Description: "npm package search",
		},
		"go": {
			URL:         "https://pkg.go.dev/search?q=%s",
			Description: "Go package search",
		},
		"mdn": {
			URL:         "https://developer.mozilla.org/en-US/search?q=%s",
			Description: "MDN Web Docs search",
		},
	}
}

// Note: We build our own browser, so no external browser commands needed
