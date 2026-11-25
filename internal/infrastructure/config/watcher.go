package config

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"

	"bnema.dev/dumber/internal/infrastructure/filewatch"
	"bnema.dev/dumber/internal/logging"
)

// Watch starts watching the config file for changes and reloads automatically.
//
// Events whose file content matches the last loaded or saved content are
// ignored, so the manager's own Save does not trigger a reload.
func (m *Manager) Watch() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.watching {
		return nil // Already watching
	}

	path := m.GetConfigFile()
	if path == "" {
		return fmt.Errorf("config file path is unknown")
	}
	if m.fileWatcher == nil {
		m.fileWatcher = filewatch.New(m.watchDelay)
	}
	if err := m.fileWatcher.Start(context.Background(), []string{path}, m.handleFileChange); err != nil {
		return fmt.Errorf("watch config file: %w", err)
	}

	m.watching = true
	return nil
}

// StopWatching stops the config file watcher. It is safe to call repeatedly.
func (m *Manager) StopWatching() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.watching = false
	if m.fileWatcher == nil {
		return nil
	}
	return m.fileWatcher.Stop()
}

// handleFileChange runs on the watcher goroutine for each debounced change.
func (m *Manager) handleFileChange(path string) {
	log := logging.NewFromEnv()

	content, err := os.ReadFile(path)
	if err != nil {
		log.Debug().Err(err).Str("file", path).Msg("config file unreadable, keeping current config")
		return
	}
	hash := sha256.Sum256(content)

	m.mu.Lock()
	if hash == m.lastContentHash {
		m.mu.Unlock()
		log.Debug().Msg("config file content unchanged, skipping reload")
		return
	}

	log.Debug().Msg("reloading config from external change")
	if err := m.reloadFrom(content); err != nil {
		m.mu.Unlock()
		log.Warn().Err(err).Msg("failed to reload config, keeping last good config")
		return
	}
	m.lastContentHash = hash
	m.notifyCallbacksLocked()
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

// reload reloads the configuration from disk (must be called with lock held for write).
func (m *Manager) reload() error {
	content, err := os.ReadFile(m.GetConfigFile())
	if err != nil {
		return err
	}
	if err := m.reloadFrom(content); err != nil {
		return err
	}
	m.lastContentHash = sha256.Sum256(content)
	return nil
}

// reloadFrom parses content as the config file and replaces the current
// config on success. Must be called with lock held for write. It does not
// update lastContentHash.
func (m *Manager) reloadFrom(content []byte) error {
	if err := m.viper.ReadConfig(bytes.NewReader(content)); err != nil {
		return err
	}

	config, err := m.unmarshalConfig()
	if err != nil {
		return err
	}
	if err := ensureDatabasePath(config); err != nil {
		return err
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
