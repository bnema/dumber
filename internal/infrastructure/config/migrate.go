package config

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/viper"

	"github.com/bnema/dumber/internal/application/port"
)

// Migrator implements port.ConfigMigrator for comparing and merging config files.
type Migrator struct {
	// defaultViper holds a Viper instance with all defaults set.
	defaultViper *viper.Viper
	// defaultConfig holds the default configuration.
	defaultConfig *Config
}

// NewMigrator creates a new Migrator instance.
func NewMigrator() *Migrator {
	v := viper.New()
	v.SetConfigType("toml")

	// Create a temporary manager to set defaults
	m := &Manager{viper: v}
	m.setDefaults()

	return &Migrator{
		defaultViper:  v,
		defaultConfig: DefaultConfig(),
	}
}

// CheckMigration checks if user config is missing any default keys.
// Returns nil if no migration is needed.
func (m *Migrator) CheckMigration() (*port.MigrationResult, error) {
	configFile, err := GetConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get config file path: %w", err)
	}

	// If config file doesn't exist, no migration needed (will be created with all defaults)
	if _, statErr := os.Stat(configFile); os.IsNotExist(statErr) {
		return nil, nil
	}

	// Get user-defined keys from the TOML file
	userKeys, err := m.getUserConfigKeys(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user config: %w", err)
	}

	// Get all default keys
	defaultKeys := m.getAllDefaultKeys()

	// Find missing keys (in defaults but not in user config)
	missingKeys := m.findMissingKeys(defaultKeys, userKeys)

	if len(missingKeys) == 0 {
		return nil, nil
	}

	return &port.MigrationResult{
		MissingKeys: missingKeys,
		ConfigFile:  configFile,
	}, nil
}

// Migrate adds missing default keys to the user's config file.
func (m *Migrator) Migrate() ([]string, error) {
	result, err := m.CheckMigration()
	if err != nil {
		return nil, err
	}

	if result == nil || len(result.MissingKeys) == 0 {
		return nil, nil
	}

	// Create a new Viper instance to read user config
	userViper := viper.New()
	userViper.SetConfigFile(result.ConfigFile)
	userViper.SetConfigType("toml")

	// Set all defaults first
	mgr := &Manager{viper: userViper}
	mgr.setDefaults()

	// Read existing config (this merges with defaults)
	if err := userViper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Write the merged config back
	if err := userViper.WriteConfig(); err != nil {
		return nil, fmt.Errorf("failed to write config file: %w", err)
	}

	return result.MissingKeys, nil
}

// GetKeyInfo returns detailed information about a config key.
func (m *Migrator) GetKeyInfo(key string) port.KeyInfo {
	value := m.defaultViper.Get(key)
	if value == nil {
		return port.KeyInfo{
			Key:          key,
			Type:         "unknown",
			DefaultValue: "unknown",
		}
	}

	return port.KeyInfo{
		Key:          key,
		Type:         m.getTypeName(value),
		DefaultValue: m.formatValue(value),
	}
}

// getAllDefaultKeys returns all keys from the default configuration.
func (m *Migrator) getAllDefaultKeys() []string {
	keys := m.defaultViper.AllKeys()

	// Filter out keys that should not be migrated
	filtered := make([]string, 0, len(keys))
	for _, key := range keys {
		// Skip database.path as it's set dynamically
		if key == "database.path" {
			continue
		}
		filtered = append(filtered, key)
	}

	sort.Strings(filtered)
	return filtered
}

// getUserConfigKeys parses the user's TOML file and returns all defined keys.
func (m *Migrator) getUserConfigKeys(configFile string) (map[string]bool, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse TOML into a generic map
	var rawConfig map[string]any
	if err := toml.Unmarshal(data, &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	// Flatten the map to dot-notation keys
	keys := make(map[string]bool)
	m.flattenMap(rawConfig, "", keys)

	return keys, nil
}

// flattenMap recursively flattens a nested map to dot-notation keys.
func (m *Migrator) flattenMap(data map[string]any, prefix string, keys map[string]bool) {
	for k, v := range data {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		switch val := v.(type) {
		case map[string]any:
			// Check if this key represents user data (like search_shortcuts, actions)
			// that should be treated as a single key rather than recursed into.
			if m.isUserDataSection(key) {
				keys[key] = true
			} else {
				// Recurse into nested config sections
				m.flattenMap(val, key, keys)
			}
		default:
			keys[key] = true
		}
	}
}

// isUserDataSection checks if a key path represents user-defined data
// that should not be compared key-by-key (e.g., search shortcuts, action bindings).
func (*Migrator) isUserDataSection(keyPath string) bool {
	// User data sections - treat entire section as one key
	switch keyPath {
	case "search_shortcuts":
		return true
	}

	// Action maps are user-customizable
	userDataSuffixes := []string{
		".actions", // pane_mode.actions, tab_mode.actions, etc.
	}

	for _, suffix := range userDataSuffixes {
		if len(keyPath) > len(suffix) && keyPath[len(keyPath)-len(suffix):] == suffix {
			return true
		}
	}

	return false
}

// findMissingKeys returns keys that are in defaults but not in user config.
func (m *Migrator) findMissingKeys(defaultKeys []string, userKeys map[string]bool) []string {
	missing := make([]string, 0)

	for _, key := range defaultKeys {
		// Check if key or any parent key exists in user config
		if m.keyOrParentExists(key, userKeys) {
			continue
		}
		missing = append(missing, key)
	}

	// Sort for consistent output
	sort.Strings(missing)
	return missing
}

// keyOrParentExists checks if a key or any of its parent keys exist in the map.
// This handles cases where a user has defined a parent section but not all children.
func (*Migrator) keyOrParentExists(key string, keys map[string]bool) bool {
	if keys[key] {
		return true
	}

	// Check if this is a child of a leaf map (like search_shortcuts.ddg)
	// In that case, we should check if the parent exists
	parts := strings.Split(key, ".")
	for i := len(parts) - 1; i > 0; i-- {
		parentKey := strings.Join(parts[:i], ".")
		if keys[parentKey] {
			// Parent exists as a leaf map, so child is implicitly defined
			return true
		}
	}

	return false
}

// getTypeName returns a human-readable type name for a value.
func (*Migrator) getTypeName(value any) string {
	if value == nil {
		return "unknown"
	}

	t := reflect.TypeOf(value)
	switch t.Kind() {
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "int"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.String:
		return "string"
	case reflect.Slice:
		return "list"
	case reflect.Map:
		return "map"
	default:
		return t.String()
	}
}

// formatValue returns a human-readable string representation of a value.
func (*Migrator) formatValue(value any) string {
	if value == nil {
		return "null"
	}

	switch v := value.(type) {
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%v", v)
	case string:
		if v == "" {
			return `""`
		}
		// Truncate long strings
		const maxStringLen = 50
		if len(v) > maxStringLen {
			return fmt.Sprintf("%q...", v[:maxStringLen-3])
		}
		return fmt.Sprintf("%q", v)
	case []any:
		if len(v) == 0 {
			return "[]"
		}
		return fmt.Sprintf("[%d items]", len(v))
	case map[string]any:
		if len(v) == 0 {
			return "{}"
		}
		return fmt.Sprintf("{%d entries}", len(v))
	default:
		// Handle slices of strings (common for action bindings)
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Slice {
			if rv.Len() == 0 {
				return "[]"
			}
			return fmt.Sprintf("[%d items]", rv.Len())
		}
		if rv.Kind() == reflect.Map {
			if rv.Len() == 0 {
				return "{}"
			}
			return fmt.Sprintf("{%d entries}", rv.Len())
		}
		return fmt.Sprintf("%v", v)
	}
}
