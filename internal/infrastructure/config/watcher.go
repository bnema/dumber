package config

import (
	"fmt"

	"github.com/bnema/dumber/internal/logging"
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
	m.viper.OnConfigChange(func(e fsnotify.Event) {
		log := logging.NewFromEnv()
		log.Debug().Str("op", e.Op.String()).Str("file", e.Name).Msg("fsnotify config change detected")

		// Acquire write lock before reload (reload modifies m.config)
		m.mu.Lock()

		// Skip reload if this was triggered by our own Save() - the in-memory
		// config is already correct and viper may have stale cached data
		if m.skipNextReload {
			log.Debug().Msg("skipping reload (triggered by own Save)")
			m.skipNextReload = false
			// Still need to sync viper's internal state with the file we just wrote
			if err := m.viper.ReadInConfig(); err != nil {
				log.Warn().Err(err).Msg("failed to sync viper config after Save")
				// Continue anyway - in-memory config is correct, just viper internal state is off
			}
			m.notifyCallbacksLocked()
			return
		}

		log.Debug().Msg("reloading config from external change")
		if err := m.reload(); err != nil {
			log.Warn().Err(err).Msg("failed to reload config")
			m.mu.Unlock()
			return
		}
		m.notifyCallbacksLocked()
	})

	m.watching = true
	return nil
}

// notifyCallbacksLocked copies callbacks and config, releases lock, then notifies.
// Must be called with m.mu held for write. Releases the lock before calling callbacks.
func (m *Manager) notifyCallbacksLocked() {
	config := m.config
	callbacks := make([]func(*Config), len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.mu.Unlock()

	for _, callback := range callbacks {
		callback(config)
	}
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

	normalizeConfig(config)

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
