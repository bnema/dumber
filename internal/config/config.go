// Package config provides configuration management for dumber with Viper integration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// Config represents the complete configuration for dumber.
type Config struct {
	Database        DatabaseConfig            `mapstructure:"database" yaml:"database"`
	History         HistoryConfig             `mapstructure:"history" yaml:"history"`
	SearchShortcuts map[string]SearchShortcut `mapstructure:"search_shortcuts" yaml:"search_shortcuts"`
	Dmenu           DmenuConfig               `mapstructure:"dmenu" yaml:"dmenu"`
	Logging         LoggingConfig             `mapstructure:"logging" yaml:"logging"`
}

// DatabaseConfig holds database-related configuration.
type DatabaseConfig struct {
	Path           string        `mapstructure:"path" yaml:"path"`
	MaxConnections int           `mapstructure:"max_connections" yaml:"max_connections"`
	MaxIdleTime    time.Duration `mapstructure:"max_idle_time" yaml:"max_idle_time"`
	QueryTimeout   time.Duration `mapstructure:"query_timeout" yaml:"query_timeout"`
}

// HistoryConfig holds history-related configuration.
type HistoryConfig struct {
	MaxEntries          int `mapstructure:"max_entries" yaml:"max_entries"`
	RetentionPeriodDays int `mapstructure:"retention_period_days" yaml:"retention_period_days"`
	CleanupIntervalDays int `mapstructure:"cleanup_interval_days" yaml:"cleanup_interval_days"`
}

// SearchShortcut represents a search shortcut configuration.
type SearchShortcut struct {
	URL         string `mapstructure:"url" yaml:"url"`
	Description string `mapstructure:"description" yaml:"description"`
}

// DmenuConfig holds dmenu/rofi integration configuration.
type DmenuConfig struct {
	MaxHistoryItems  int    `mapstructure:"max_history_items" yaml:"max_history_items"`
	ShowVisitCount   bool   `mapstructure:"show_visit_count" yaml:"show_visit_count"`
	ShowLastVisited  bool   `mapstructure:"show_last_visited" yaml:"show_last_visited"`
	HistoryPrefix    string `mapstructure:"history_prefix" yaml:"history_prefix"`
	ShortcutPrefix   string `mapstructure:"shortcut_prefix" yaml:"shortcut_prefix"`
	URLPrefix        string `mapstructure:"url_prefix" yaml:"url_prefix"`
	DateFormat       string `mapstructure:"date_format" yaml:"date_format"`
	SortByVisitCount bool   `mapstructure:"sort_by_visit_count" yaml:"sort_by_visit_count"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level      string `mapstructure:"level" yaml:"level"`
	Format     string `mapstructure:"format" yaml:"format"`
	Filename   string `mapstructure:"filename" yaml:"filename"`
	MaxSize    int    `mapstructure:"max_size" yaml:"max_size"`
	MaxBackups int    `mapstructure:"max_backups" yaml:"max_backups"`
	MaxAge     int    `mapstructure:"max_age" yaml:"max_age"`
	Compress   bool   `mapstructure:"compress" yaml:"compress"`
}

// Manager handles configuration loading, watching, and reloading.
type Manager struct {
	config    *Config
	viper     *viper.Viper
	mu        sync.RWMutex
	callbacks []func(*Config)
	watching  bool
}

// NewManager creates a new configuration manager.
func NewManager() (*Manager, error) {
	v := viper.New()

	// Configure Viper - supports yaml, json, toml automatically
	v.SetConfigName("config") // Will find config.yaml, config.json, config.toml, etc.

	// Add config paths
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}
	v.AddConfigPath(configDir)
	v.AddConfigPath(".") // Current directory for development

	// Set up environment variable support
	v.SetEnvPrefix("DUMB_BROWSER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Bind specific environment variables
	bindings := map[string]string{
		"database.path":             "DATABASE_PATH",
		"database.max_connections":  "DATABASE_MAX_CONNECTIONS",
		"database.max_idle_time":    "DATABASE_MAX_IDLE_TIME",
		"database.query_timeout":    "DATABASE_QUERY_TIMEOUT",
		"browser.command":           "BROWSER_COMMAND",
		"browser.timeout":           "BROWSER_TIMEOUT",
		"browser.detach_process":    "BROWSER_DETACH_PROCESS",
		"history.max_entries":       "HISTORY_MAX_ENTRIES",
		"history.retention_period":  "HISTORY_RETENTION_PERIOD",
		"history.cleanup_interval":  "HISTORY_CLEANUP_INTERVAL",
		"dmenu.max_history_items":   "DMENU_MAX_HISTORY_ITEMS",
		"dmenu.show_visit_count":    "DMENU_SHOW_VISIT_COUNT",
		"dmenu.show_last_visited":   "DMENU_SHOW_LAST_VISITED",
		"dmenu.history_prefix":      "DMENU_HISTORY_PREFIX",
		"dmenu.shortcut_prefix":     "DMENU_SHORTCUT_PREFIX",
		"dmenu.url_prefix":          "DMENU_URL_PREFIX",
		"dmenu.date_format":         "DMENU_DATE_FORMAT",
		"dmenu.sort_by_visit_count": "DMENU_SORT_BY_VISIT_COUNT",
		"logging.level":             "LOGGING_LEVEL",
		"logging.format":            "LOGGING_FORMAT",
		"logging.filename":          "LOGGING_FILENAME",
		"logging.max_size":          "LOGGING_MAX_SIZE",
		"logging.max_backups":       "LOGGING_MAX_BACKUPS",
		"logging.max_age":           "LOGGING_MAX_AGE",
		"logging.compress":          "LOGGING_COMPRESS",
	}

	for key, env := range bindings {
		if err := v.BindEnv(key, "DUMB_BROWSER_"+env); err != nil {
			return nil, fmt.Errorf("failed to bind environment variable %s: %w", env, err)
		}
	}

	return &Manager{
		viper:     v,
		callbacks: make([]func(*Config), 0),
	}, nil
}

// Load loads the configuration from file and environment variables.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure directories exist
	if err := EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to ensure directories: %w", err)
	}

	// Set defaults
	m.setDefaults()

	// Read config file if it exists
	if err := m.viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, create default one
			if err := m.createDefaultConfig(); err != nil {
				return fmt.Errorf("failed to create default config: %w", err)
			}
		} else {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Unmarshal into config struct
	config := &Config{}
	if err := m.viper.Unmarshal(config); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set database path if not specified
	if config.Database.Path == "" {
		dbPath, err := GetDatabaseFile()
		if err != nil {
			return fmt.Errorf("failed to get database path: %w", err)
		}
		config.Database.Path = dbPath
	}

	m.config = config
	return nil
}

// Get returns the current configuration (thread-safe).
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modification
	configCopy := *m.config
	return &configCopy
}

// Watch starts watching the config file for changes and reloads automatically.
func (m *Manager) Watch() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.watching {
		return nil // Already watching
	}

	m.viper.WatchConfig()
	m.viper.OnConfigChange(func(_ fsnotify.Event) {
		// Reload config
		if err := m.reload(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to reload config: %v\n", err)
			return
		}

		// Notify callbacks
		m.mu.RLock()
		config := m.config
		callbacks := make([]func(*Config), len(m.callbacks))
		copy(callbacks, m.callbacks)
		m.mu.RUnlock()

		for _, callback := range callbacks {
			callback(config)
		}
	})

	m.watching = true
	return nil
}

// OnConfigChange registers a callback function to be called when config changes.
func (m *Manager) OnConfigChange(callback func(*Config)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callbacks = append(m.callbacks, callback)
}

// reload reloads the configuration (internal method, must be called with lock).
func (m *Manager) reload() error {
	if err := m.viper.ReadInConfig(); err != nil {
		return err
	}

	config := &Config{}
	if err := m.viper.Unmarshal(config); err != nil {
		return err
	}

	// Set database path if not specified
	if config.Database.Path == "" {
		dbPath, err := GetDatabaseFile()
		if err != nil {
			return fmt.Errorf("failed to get database path: %w", err)
		}
		config.Database.Path = dbPath
	}

	m.config = config
	return nil
}

// setDefaults sets default configuration values in Viper.
func (m *Manager) setDefaults() {
	defaults := DefaultConfig()

	// Database defaults
	m.viper.SetDefault("database.max_connections", defaults.Database.MaxConnections)
	m.viper.SetDefault("database.max_idle_time", defaults.Database.MaxIdleTime)
	m.viper.SetDefault("database.query_timeout", defaults.Database.QueryTimeout)

	// Note: Browser config removed - we build our own browser with Wails

	// History defaults
	m.viper.SetDefault("history.max_entries", defaults.History.MaxEntries)
	m.viper.SetDefault("history.retention_period_days", defaults.History.RetentionPeriodDays)
	m.viper.SetDefault("history.cleanup_interval_days", defaults.History.CleanupIntervalDays)

	// Search shortcuts defaults
	m.viper.SetDefault("search_shortcuts", defaults.SearchShortcuts)

	// Dmenu defaults
	m.viper.SetDefault("dmenu.max_history_items", defaults.Dmenu.MaxHistoryItems)
	m.viper.SetDefault("dmenu.show_visit_count", defaults.Dmenu.ShowVisitCount)
	m.viper.SetDefault("dmenu.show_last_visited", defaults.Dmenu.ShowLastVisited)
	m.viper.SetDefault("dmenu.history_prefix", defaults.Dmenu.HistoryPrefix)
	m.viper.SetDefault("dmenu.shortcut_prefix", defaults.Dmenu.ShortcutPrefix)
	m.viper.SetDefault("dmenu.url_prefix", defaults.Dmenu.URLPrefix)
	m.viper.SetDefault("dmenu.date_format", defaults.Dmenu.DateFormat)
	m.viper.SetDefault("dmenu.sort_by_visit_count", defaults.Dmenu.SortByVisitCount)

	// Logging defaults
	m.viper.SetDefault("logging.level", defaults.Logging.Level)
	m.viper.SetDefault("logging.format", defaults.Logging.Format)
	m.viper.SetDefault("logging.filename", defaults.Logging.Filename)
	m.viper.SetDefault("logging.max_size", defaults.Logging.MaxSize)
	m.viper.SetDefault("logging.max_backups", defaults.Logging.MaxBackups)
	m.viper.SetDefault("logging.max_age", defaults.Logging.MaxAge)
	m.viper.SetDefault("logging.compress", defaults.Logging.Compress)
}

// createDefaultConfig creates a default configuration file.
func (m *Manager) createDefaultConfig() error {
	configFile, err := GetConfigFile()
	if err != nil {
		return err
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(configFile), 0755); err != nil {
		return err
	}

	// Get the default configuration
	defaultConfig := DefaultConfig()

	// Marshal to JSON with proper indentation
	configData, err := json.MarshalIndent(defaultConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal default config: %w", err)
	}

	// Write JSON config file
	if err := os.WriteFile(configFile, configData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Created default configuration file: %s\n", configFile)
	return nil
}

// GetConfigFile returns the path to the configuration file being used.
func (m *Manager) GetConfigFile() string {
	return m.viper.ConfigFileUsed()
}

// Global configuration manager instance
var globalManager *Manager
var globalManagerOnce sync.Once

// Init initializes the global configuration manager.
func Init() error {
	var err error
	globalManagerOnce.Do(func() {
		globalManager, err = NewManager()
		if err != nil {
			return
		}
		err = globalManager.Load()
	})
	return err
}

// Get returns the global configuration.
func Get() *Config {
	if globalManager == nil {
		// Return defaults if not initialized
		return DefaultConfig()
	}
	return globalManager.Get()
}

// Watch starts watching the global configuration for changes.
func Watch() error {
	if globalManager == nil {
		return fmt.Errorf("configuration not initialized")
	}
	return globalManager.Watch()
}

// OnConfigChange registers a callback for global configuration changes.
func OnConfigChange(callback func(*Config)) {
	if globalManager == nil {
		return
	}
	globalManager.OnConfigChange(callback)
}
