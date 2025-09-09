// Package config provides default configuration values for dumber.
package config

import (
	"time"
)

// DefaultConfig returns the default configuration values for dumber.
func DefaultConfig() *Config {
	return &Config{
		Database: DatabaseConfig{
			MaxConnections: 1,
			MaxIdleTime:    time.Minute * 5,
			QueryTimeout:   time.Second * 30,
		},
		History: HistoryConfig{
			MaxEntries:          10000,
			RetentionPeriodDays: 365, // 1 year
			CleanupIntervalDays: 1,   // daily cleanup
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
			MaxHistoryItems:  20,
			ShowVisitCount:   true,
			ShowLastVisited:  true,
			HistoryPrefix:    "🕒",
			ShortcutPrefix:   "🔍",
			URLPrefix:        "🌐",
			DateFormat:       "2006-01-02 15:04",
			SortByVisitCount: true,
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "text", // text or json
			Filename:   "",     // empty means stdout
			MaxSize:    100,    // MB
			MaxBackups: 3,
			MaxAge:     7, // days
			Compress:   true,
		},
		Appearance: AppearanceConfig{
			SansFont:        "Fira Sans",
			SerifFont:       "Fira Sans",
			MonospaceFont:   "Fira Code",
			DefaultFontSize: 16,
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
