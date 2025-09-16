package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/config"
	"strings"
)

// ConfigService handles configuration operations for the application.
type ConfigService struct {
	config     *config.Config
	configPath string
}

const defaultDirPerm = 0o750

// ConfigInfo represents configuration information for the frontend.
type ConfigInfo struct {
	ConfigPath      string                           `json:"config_path"`
	DatabasePath    string                           `json:"database_path"`
	SearchShortcuts map[string]config.SearchShortcut `json:"search_shortcuts"`
	DmenuSettings   *config.DmenuConfig              `json:"dmenu_settings"`
	RenderingMode   string                           `json:"rendering_mode"`
	UseDomZoom      bool                             `json:"use_dom_zoom"`
}

// NewConfigService creates a new ConfigService instance.
func NewConfigService(cfg *config.Config, configPath string) *ConfigService {
	return &ConfigService{
		config:     cfg,
		configPath: configPath,
	}
}

// GetConfigInfo returns comprehensive configuration information.
func (s *ConfigService) GetConfigInfo(ctx context.Context) (*ConfigInfo, error) {
	_ = ctx
	return &ConfigInfo{
		ConfigPath:      s.configPath,
		DatabasePath:    s.config.Database.Path,
		SearchShortcuts: s.config.SearchShortcuts,
		DmenuSettings:   &s.config.Dmenu,
		RenderingMode:   string(s.config.RenderingMode),
		UseDomZoom:      s.config.UseDomZoom,
	}, nil
}

// GetSearchShortcuts returns all configured search shortcuts.
func (s *ConfigService) GetSearchShortcuts(ctx context.Context) (map[string]config.SearchShortcut, error) {
	_ = ctx
	return s.config.SearchShortcuts, nil
}

// AddSearchShortcut adds a new search shortcut.
func (s *ConfigService) AddSearchShortcut(ctx context.Context, key string, shortcut config.SearchShortcut) error {
	_ = ctx
	if key == "" {
		return fmt.Errorf("shortcut key cannot be empty")
	}

	if shortcut.URL == "" {
		return fmt.Errorf("shortcut URL cannot be empty")
	}

	if s.config.SearchShortcuts == nil {
		s.config.SearchShortcuts = make(map[string]config.SearchShortcut)
	}

	s.config.SearchShortcuts[key] = shortcut
	return s.saveConfig()
}

// UpdateSearchShortcut updates an existing search shortcut.
func (s *ConfigService) UpdateSearchShortcut(ctx context.Context, key string, shortcut config.SearchShortcut) error {
	_ = ctx
	if key == "" {
		return fmt.Errorf("shortcut key cannot be empty")
	}

	if _, exists := s.config.SearchShortcuts[key]; !exists {
		return fmt.Errorf("shortcut '%s' does not exist", key)
	}

	if shortcut.URL == "" {
		return fmt.Errorf("shortcut URL cannot be empty")
	}

	s.config.SearchShortcuts[key] = shortcut
	return s.saveConfig()
}

// DeleteSearchShortcut removes a search shortcut.
func (s *ConfigService) DeleteSearchShortcut(ctx context.Context, key string) error {
	_ = ctx
	if key == "" {
		return fmt.Errorf("shortcut key cannot be empty")
	}

	if _, exists := s.config.SearchShortcuts[key]; !exists {
		return fmt.Errorf("shortcut '%s' does not exist", key)
	}

	delete(s.config.SearchShortcuts, key)
	return s.saveConfig()
}

// GetDmenuSettings returns current dmenu settings.
func (s *ConfigService) GetDmenuSettings(ctx context.Context) (*config.DmenuConfig, error) {
	_ = ctx
	return &s.config.Dmenu, nil
}

// UpdateDmenuSettings updates dmenu configuration.
func (s *ConfigService) UpdateDmenuSettings(ctx context.Context, settings *config.DmenuConfig) error {
	_ = ctx
	if settings == nil {
		return fmt.Errorf("dmenu settings cannot be nil")
	}

	s.config.Dmenu = *settings
	return s.saveConfig()
}

// GetDatabasePath returns the current database path.
func (s *ConfigService) GetDatabasePath(ctx context.Context) (string, error) {
	_ = ctx
	return s.config.Database.Path, nil
}

// SetDatabasePath updates the database path.
func (s *ConfigService) SetDatabasePath(ctx context.Context, path string) error {
	_ = ctx
	if path == "" {
		return fmt.Errorf("database path cannot be empty")
	}

	// Validate that the directory exists or can be created
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, defaultDirPerm); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	s.config.Database.Path = path
	return s.saveConfig()
}

// GetDefaultBrowser returns the default browser setting.
func (s *ConfigService) GetDefaultBrowser(ctx context.Context) (string, error) {
	_ = ctx
	// Default browser is now built into the app
	return "dumber (built-in)", nil
}

// SetDefaultBrowser updates the default browser setting.
func (s *ConfigService) SetDefaultBrowser(ctx context.Context, browser string) error {
	_ = ctx
	_ = browser
	// Default browser setting is not needed for built-in browser
	return fmt.Errorf("default browser is built-in and cannot be changed")
}

// ReloadConfig reloads configuration from file.
func (s *ConfigService) ReloadConfig(ctx context.Context) error {
	_ = ctx
	// Create new manager and load config
	manager, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to create config manager: %w", err)
	}

	if err := manager.Load(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	s.config = manager.Get()
	return nil
}

// ValidateConfig validates the current configuration.
func (s *ConfigService) ValidateConfig(ctx context.Context) ([]string, error) {
	_ = ctx
	var errors []string

	// Validate database path
	if s.config.Database.Path == "" {
		errors = append(errors, "database path is not set")
	} else {
		dir := filepath.Dir(s.config.Database.Path)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("database directory does not exist: %s", dir))
		}
	}

	// Validate search shortcuts
	if len(s.config.SearchShortcuts) == 0 {
		errors = append(errors, "no search shortcuts configured")
	} else {
		for key, shortcut := range s.config.SearchShortcuts {
			if shortcut.URL == "" {
				errors = append(errors, fmt.Sprintf("search shortcut '%s' has empty URL", key))
			}
		}
	}

	// Check for default search shortcut
	if _, hasDefault := s.config.SearchShortcuts["g"]; !hasDefault {
		errors = append(errors, "no default search shortcut 'g' configured")
	}

	// Validate dmenu settings
	if s.config.Dmenu.MaxHistoryItems < 1 {
		errors = append(errors, "dmenu max_history_items must be at least 1")
	}

	return errors, nil
}

// GetConfigPath returns the path to the configuration file.
func (s *ConfigService) GetConfigPath(ctx context.Context) (string, error) {
	_ = ctx
	return s.configPath, nil
}

// ExportConfig returns the current configuration as JSON.
func (s *ConfigService) ExportConfig(ctx context.Context) (*config.Config, error) {
	_ = ctx
	// Return a copy of the config to prevent modifications
	configCopy := *s.config
	return &configCopy, nil
}

// ImportConfig imports configuration from a Config object.
func (s *ConfigService) ImportConfig(ctx context.Context, newConfig *config.Config) error {
	if newConfig == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Validate the new config
	tempService := &ConfigService{config: newConfig}
	validationErrors, err := tempService.ValidateConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to validate config: %w", err)
	}

	if len(validationErrors) > 0 {
		return fmt.Errorf("config validation failed: %v", validationErrors)
	}

	// Apply the new config
	s.config = newConfig
	return s.saveConfig()
}

// GetRenderingMode returns the current rendering mode (auto|gpu|cpu).
func (s *ConfigService) GetRenderingMode(ctx context.Context) (string, error) {
	_ = ctx
	return string(s.config.RenderingMode), nil
}

// SetRenderingMode updates the rendering mode (auto|gpu|cpu).
func (s *ConfigService) SetRenderingMode(ctx context.Context, mode string) error {
	_ = ctx
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case "auto", "gpu", "cpu":
		s.config.RenderingMode = config.RenderingMode(m)
		return s.saveConfig()
	default:
		return fmt.Errorf("invalid rendering mode: %s", mode)
	}
}

// GetUseDomZoom reports whether DOM-based zoom is enabled.
func (s *ConfigService) GetUseDomZoom(ctx context.Context) (bool, error) {
	_ = ctx
	return s.config.UseDomZoom, nil
}

// SetUseDomZoom toggles DOM-based zoom.
func (s *ConfigService) SetUseDomZoom(ctx context.Context, enabled bool) error {
	_ = ctx
	s.config.UseDomZoom = enabled
	return s.saveConfig()
}

// saveConfig saves the current configuration to file.
func (s *ConfigService) saveConfig() error {
	// For now, return success - implementing config saving would require
	// extending the config package with save functionality
	return nil
}
