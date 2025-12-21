package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/fsnotify/fsnotify"
)

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

// reload reloads the configuration (internal method, must be called with lock held for write).
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

	// Normalize/validate rendering mode using switch statement
	switch strings.ToLower(string(config.RenderingMode)) {
	case "", string(RenderingModeAuto):
		config.RenderingMode = RenderingModeAuto
	case string(RenderingModeGPU):
		config.RenderingMode = RenderingModeGPU
	case string(RenderingModeCPU):
		config.RenderingMode = RenderingModeCPU
	default:
		config.RenderingMode = RenderingModeAuto
	}

	// Validate ColorScheme setting using switch statement
	switch config.Appearance.ColorScheme {
	case ThemePreferDark, ThemePreferLight, ThemeDefault:
		// Valid values - no changes needed
	case "":
		// Empty string is treated the same as "default"
		config.Appearance.ColorScheme = ThemeDefault
	default:
		// Invalid value - reset to default
		config.Appearance.ColorScheme = ThemeDefault
	}

	// Validate all config values
	if err := validateConfig(config); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	m.config = config
	return nil
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
