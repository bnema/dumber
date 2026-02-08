package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

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

// DetectChanges analyzes user config and returns all detected changes.
// This provides a detailed diff-like view of what migration would do.
func (m *Migrator) DetectChanges() ([]port.KeyChange, error) {
	configFile, err := GetConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get config file path: %w", err)
	}

	// If config file doesn't exist, no changes to detect
	if _, statErr := os.Stat(configFile); os.IsNotExist(statErr) {
		return nil, nil
	}

	// Get user-defined keys with values
	userKeysWithValues, err := m.getUserConfigKeysWithValues(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user config: %w", err)
	}

	// Get all default keys
	defaultKeys := m.getAllDefaultKeys()
	defaultKeySet := make(map[string]bool)
	for _, k := range defaultKeys {
		defaultKeySet[k] = true
	}

	// Build user key set for lookup
	userKeySet := make(map[string]bool)
	for k := range userKeysWithValues {
		userKeySet[k] = true
	}

	var changes []port.KeyChange

	// Find deprecated keys (in user config but not in defaults)
	deprecatedKeys := m.findDeprecatedKeys(userKeysWithValues, defaultKeySet)

	// Find missing keys (in defaults but not in user config)
	missingKeys := m.findMissingKeys(defaultKeys, userKeySet)
	missingKeys = appendUniqueKeys(missingKeys, m.detectMissingWorkspaceShortcutActions(userKeysWithValues))

	// Try to match deprecated keys with missing keys (detect renames)
	renames, unmatchedDeprecated, unmatchedMissing := m.matchRenamedKeys(deprecatedKeys, missingKeys)

	// Add rename changes
	for oldKey, newKey := range renames {
		oldValue := m.formatValue(userKeysWithValues[oldKey])
		newValue := m.formatValue(m.defaultValueForKey(newKey))
		changes = append(changes, port.KeyChange{
			Type:     port.KeyChangeRenamed,
			OldKey:   oldKey,
			NewKey:   newKey,
			OldValue: oldValue,
			NewValue: newValue,
		})
	}

	// Add removed changes (deprecated keys that couldn't be matched)
	for _, oldKey := range unmatchedDeprecated {
		changes = append(changes, port.KeyChange{
			Type:     port.KeyChangeRemoved,
			OldKey:   oldKey,
			OldValue: m.formatValue(userKeysWithValues[oldKey]),
		})
	}

	// Add new key changes (missing keys that couldn't be matched)
	for _, newKey := range unmatchedMissing {
		changes = append(changes, port.KeyChange{
			Type:     port.KeyChangeAdded,
			NewKey:   newKey,
			NewValue: m.formatValue(m.defaultValueForKey(newKey)),
		})
	}

	// Sort changes for consistent output
	sort.Slice(changes, func(i, j int) bool {
		// Sort by type first, then by key
		if changes[i].Type != changes[j].Type {
			return changes[i].Type < changes[j].Type
		}
		keyI := changes[i].NewKey
		if keyI == "" {
			keyI = changes[i].OldKey
		}
		keyJ := changes[j].NewKey
		if keyJ == "" {
			keyJ = changes[j].OldKey
		}
		return keyI < keyJ
	})

	return changes, nil
}

// findDeprecatedKeys returns user keys that don't exist in defaults.
func (m *Migrator) findDeprecatedKeys(userKeys map[string]any, defaultKeys map[string]bool) []string {
	var deprecated []string
	for userKey := range userKeys {
		if userKey == "database.path" {
			continue
		}
		if !m.keyOrRelatedExistsInDefaults(userKey, defaultKeys) {
			deprecated = append(deprecated, userKey)
		}
	}
	sort.Strings(deprecated)
	return deprecated
}

// keyOrRelatedExistsInDefaults checks if a user key exists in defaults.
func (*Migrator) keyOrRelatedExistsInDefaults(userKey string, defaultKeys map[string]bool) bool {
	if defaultKeys[userKey] {
		return true
	}

	// Check if this is a parent of any default key
	userKeyPrefix := userKey + "."
	for defaultKey := range defaultKeys {
		if strings.HasPrefix(defaultKey, userKeyPrefix) {
			return true
		}
	}

	// Check if any parent of this user key exists
	parts := strings.Split(userKey, ".")
	for i := len(parts) - 1; i > 0; i-- {
		parentKey := strings.Join(parts[:i], ".")
		if defaultKeys[parentKey] {
			return true
		}
	}

	return false
}

// matchRenamedKeys attempts to match deprecated keys with missing keys.
// Returns: renames map, unmatched deprecated, unmatched missing.
func (m *Migrator) matchRenamedKeys(deprecated, missing []string) (map[string]string, []string, []string) {
	renames := make(map[string]string)
	usedDeprecated := make(map[string]bool)
	usedMissing := make(map[string]bool)

	// First pass: exact suffix matching (e.g., "foo_border_color" -> "foo_color")
	for _, oldKey := range deprecated {
		for _, newKey := range missing {
			if usedMissing[newKey] {
				continue
			}

			// Check if keys are similar (same parent path + related leaf names)
			// AND have compatible types (don't match int keys with string keys)
			if keysAreSimilar(oldKey, newKey) && m.typesAreCompatible(oldKey, newKey) {
				renames[oldKey] = newKey
				usedDeprecated[oldKey] = true
				usedMissing[newKey] = true
				break
			}
		}
	}

	// Collect unmatched
	var unmatchedDeprecated []string
	for _, k := range deprecated {
		if !usedDeprecated[k] {
			unmatchedDeprecated = append(unmatchedDeprecated, k)
		}
	}

	var unmatchedMissing []string
	for _, k := range missing {
		if !usedMissing[k] {
			unmatchedMissing = append(unmatchedMissing, k)
		}
	}

	return renames, unmatchedDeprecated, unmatchedMissing
}

// Type name constants for migration compatibility checks.
const (
	typeNameInt    = "int"
	typeNameString = "string"
)

// Config format constants.
const (
	configFormatTOML             = "toml"
	configFormatYAML             = "yaml"
	configFormatJSON             = "json"
	workspaceShortcutsActionsKey = "workspace.shortcuts.actions"
)

// typesAreCompatible checks if the default types for two keys are compatible.
// This prevents matching int keys with string keys during rename detection.
func (m *Migrator) typesAreCompatible(oldKey, newKey string) bool {
	// Get the type of the new key from defaults
	newValue := m.defaultViper.Get(newKey)
	if newValue == nil {
		return false
	}

	newType := m.getTypeName(newValue)

	// Infer expected old type from key name patterns
	// Keys ending in "_width" are typically int
	// Keys ending in "_color" are typically string
	if strings.HasSuffix(oldKey, "_width") && newType == typeNameString {
		return false
	}
	if strings.HasSuffix(oldKey, "_color") && newType == typeNameInt {
		return false
	}

	// Additional checks based on new key patterns
	if strings.HasSuffix(newKey, "_width") && newType != typeNameInt {
		return false
	}
	if strings.HasSuffix(newKey, "_color") && newType != typeNameString {
		return false
	}

	return true
}

// keysAreSimilar checks if two keys are likely renames of each other.
func keysAreSimilar(oldKey, newKey string) bool {
	oldParts := strings.Split(oldKey, ".")
	newParts := strings.Split(newKey, ".")

	// Must have same parent path (all but last part)
	if len(oldParts) != len(newParts) {
		return false
	}
	if len(oldParts) < 2 {
		return false
	}

	// Check parent path matches
	for i := 0; i < len(oldParts)-1; i++ {
		if oldParts[i] != newParts[i] {
			return false
		}
	}

	// Check if leaf names are similar (one is substring of the other, or share a common base)
	oldLeaf := oldParts[len(oldParts)-1]
	newLeaf := newParts[len(newParts)-1]

	// Direct substring check
	if strings.Contains(oldLeaf, newLeaf) || strings.Contains(newLeaf, oldLeaf) {
		return true
	}

	// Common prefix/suffix matching (e.g., "pane_mode_border_color" vs "pane_mode_color")
	// Extract common parts by splitting on underscore
	oldTokens := strings.Split(oldLeaf, "_")
	newTokens := strings.Split(newLeaf, "_")

	// Count matching tokens, ensuring each new token is matched at most once
	matches := 0
	matchedNew := make([]bool, len(newTokens))
	for _, ot := range oldTokens {
		for j, nt := range newTokens {
			if ot == nt && !matchedNew[j] {
				matches++
				matchedNew[j] = true
				break
			}
		}
	}

	// If most tokens match, consider similar
	minTokens := len(oldTokens)
	if len(newTokens) < minTokens {
		minTokens = len(newTokens)
	}
	return matches >= minTokens-1 && matches > 0
}

// Migrate adds missing default keys to the user's config file and removes deprecated keys.
func (m *Migrator) Migrate() ([]string, error) {
	changes, err := m.DetectChanges()
	if err != nil {
		return nil, err
	}

	if len(changes) == 0 {
		return nil, nil
	}

	configFile, err := GetConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get config file path: %w", err)
	}

	// Read the raw config file into a nested map
	rawConfig, err := m.readRawConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Transform legacy action bindings (slice -> ActionBinding struct)
	transformer := NewLegacyConfigTransformer()
	transformer.TransformLegacyActions(rawConfig)

	// Build sets of keys to remove and renames to apply
	keysToRemove := make(map[string]bool)
	renames := make(map[string]string) // oldKey -> newKey

	var appliedKeys []string
	for _, change := range changes {
		switch change.Type {
		case port.KeyChangeRenamed:
			renames[change.OldKey] = change.NewKey
			keysToRemove[change.OldKey] = true
			appliedKeys = append(appliedKeys, fmt.Sprintf("%s -> %s", change.OldKey, change.NewKey))
		case port.KeyChangeAdded:
			appliedKeys = append(appliedKeys, change.NewKey)
		case port.KeyChangeRemoved:
			keysToRemove[change.OldKey] = true
			appliedKeys = append(appliedKeys, fmt.Sprintf("(removed: %s)", change.OldKey))
		}
	}

	// Apply renames: copy value from old key to new key before removing old key
	for oldKey, newKey := range renames {
		if value := m.getNestedValue(rawConfig, oldKey); value != nil {
			m.setNestedValue(rawConfig, newKey, value)
		}
	}

	// Remove deprecated and renamed keys from the raw config
	for key := range keysToRemove {
		m.deleteNestedKey(rawConfig, key)
	}
	m.mergeMissingWorkspaceShortcutActions(rawConfig)

	// Create a new Viper instance with defaults for added keys
	userViper := viper.New()
	userViper.SetConfigFile(configFile)
	userViper.SetConfigType(m.getConfigType(configFile))

	// Set all defaults first
	mgr := &Manager{viper: userViper}
	mgr.setDefaults()

	// Merge the cleaned raw config on top of defaults
	if err := userViper.MergeConfigMap(rawConfig); err != nil {
		return nil, fmt.Errorf("failed to merge config: %w", err)
	}

	// Unmarshal to Config struct for ordered writing
	var cfg Config
	if err := userViper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal merged config: %w", err)
	}

	// Write with ordered sections
	if err := WriteConfigOrdered(&cfg, configFile); err != nil {
		return nil, fmt.Errorf("failed to write config file: %w", err)
	}

	return appliedKeys, nil
}

// getConfigType returns the config type based on file extension.
func (*Migrator) getConfigType(configFile string) string {
	ext := strings.TrimPrefix(filepath.Ext(configFile), ".")
	switch ext {
	case configFormatYAML, "yml":
		return configFormatYAML
	case configFormatJSON:
		return configFormatJSON
	case configFormatTOML:
		return configFormatTOML
	default:
		return configFormatTOML // default to TOML
	}
}

// readRawConfig reads the config file into a nested map without applying defaults.
func (m *Migrator) readRawConfig(configFile string) (map[string]any, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	var rawConfig map[string]any
	configType := m.getConfigType(configFile)

	switch configType {
	case configFormatTOML:
		if err := toml.Unmarshal(data, &rawConfig); err != nil {
			return nil, fmt.Errorf("failed to parse TOML: %w", err)
		}
	case configFormatYAML:
		if err := yaml.Unmarshal(data, &rawConfig); err != nil {
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}
	case configFormatJSON:
		if err := json.Unmarshal(data, &rawConfig); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config type: %s", configType)
	}

	return rawConfig, nil
}

// getNestedValue retrieves a value from a nested map using dot-notation key.
func (*Migrator) getNestedValue(m map[string]any, key string) any {
	parts := strings.Split(key, ".")
	current := m

	for i, part := range parts {
		val, ok := current[part]
		if !ok {
			return nil
		}

		if i == len(parts)-1 {
			return val
		}

		nested, ok := val.(map[string]any)
		if !ok {
			return nil
		}
		current = nested
	}
	return nil
}

// setNestedValue sets a value in a nested map using dot-notation key.
func (*Migrator) setNestedValue(m map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	current := m

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}

		val, ok := current[part]
		if !ok {
			// Create intermediate map
			newMap := make(map[string]any)
			current[part] = newMap
			current = newMap
			continue
		}

		nested, ok := val.(map[string]any)
		if !ok {
			// Overwrite non-map value with a map
			newMap := make(map[string]any)
			current[part] = newMap
			current = newMap
			continue
		}
		current = nested
	}
}

// deleteNestedKey removes a key from a nested map using dot-notation key.
// It also cleans up empty parent maps.
func (m *Migrator) deleteNestedKey(config map[string]any, key string) {
	parts := strings.Split(key, ".")
	m.deleteNestedKeyRecursive(config, parts)
}

// deleteNestedKeyRecursive recursively deletes a key and cleans up empty parents.
func (m *Migrator) deleteNestedKeyRecursive(current map[string]any, parts []string) bool {
	if len(parts) == 0 {
		return false
	}

	part := parts[0]
	val, ok := current[part]
	if !ok {
		return false
	}

	if len(parts) == 1 {
		// This is the leaf key, delete it
		delete(current, part)
		return true
	}

	// Navigate deeper
	nested, ok := val.(map[string]any)
	if !ok {
		return false
	}

	// Recursively delete in the nested map
	deleted := m.deleteNestedKeyRecursive(nested, parts[1:])

	// If the nested map is now empty, remove it too
	if deleted && len(nested) == 0 {
		delete(current, part)
	}

	return deleted
}

// GetKeyInfo returns detailed information about a config key.
func (m *Migrator) GetKeyInfo(key string) port.KeyInfo {
	value := m.defaultValueForKey(key)
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

// GetConfigFile returns the path to the user's config file.
func (*Migrator) GetConfigFile() (string, error) {
	return GetConfigFile()
}

func (m *Migrator) defaultValueForKey(key string) any {
	if value := m.defaultViper.Get(key); value != nil {
		return value
	}

	if strings.HasPrefix(key, workspaceShortcutsActionsKey+".") {
		actionName := strings.TrimPrefix(key, workspaceShortcutsActionsKey+".")
		if action, ok := m.defaultConfig.Workspace.Shortcuts.Actions[actionName]; ok {
			return action
		}
	}

	return nil
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
	keysWithValues, err := m.getUserConfigKeysWithValues(configFile)
	if err != nil {
		return nil, err
	}

	keys := make(map[string]bool)
	for k := range keysWithValues {
		keys[k] = true
	}
	return keys, nil
}

// getUserConfigKeysWithValues parses the user's TOML file and returns keys with their values.
func (m *Migrator) getUserConfigKeysWithValues(configFile string) (map[string]any, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse TOML into a generic map
	var rawConfig map[string]any
	if err := toml.Unmarshal(data, &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	// Transform legacy action bindings before flattening
	transformer := NewLegacyConfigTransformer()
	transformer.TransformLegacyActions(rawConfig)

	// Flatten the map to dot-notation keys with values
	result := make(map[string]any)
	m.flattenMapWithValues(rawConfig, "", result)

	return result, nil
}

// flattenMapWithValues recursively flattens a nested map to dot-notation keys with values.
func (m *Migrator) flattenMapWithValues(data map[string]any, prefix string, result map[string]any) {
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
				result[key] = v
			} else {
				// Recurse into nested config sections
				m.flattenMapWithValues(val, key, result)
			}
		default:
			result[key] = v
		}
	}
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
		if strings.HasSuffix(keyPath, suffix) {
			return true
		}
	}

	return false
}

// detectMissingWorkspaceShortcutActions finds default global shortcut actions not present in user config.
func (m *Migrator) detectMissingWorkspaceShortcutActions(userKeysWithValues map[string]any) []string {
	if userKeysWithValues == nil {
		return nil
	}

	userActionsValue, ok := userKeysWithValues[workspaceShortcutsActionsKey]
	if !ok {
		return nil
	}

	userActions, ok := m.toStringAnyMap(userActionsValue)
	if !ok {
		return nil
	}

	defaultActions := m.defaultConfig.Workspace.Shortcuts.Actions
	if len(defaultActions) == 0 {
		return nil
	}

	missing := make([]string, 0)
	for actionName := range defaultActions {
		if _, exists := userActions[actionName]; exists {
			continue
		}
		missing = append(missing, workspaceShortcutsActionsKey+"."+actionName)
	}

	sort.Strings(missing)
	return missing
}

// mergeMissingWorkspaceShortcutActions injects new default shortcut actions while preserving user overrides.
func (m *Migrator) mergeMissingWorkspaceShortcutActions(rawConfig map[string]any) {
	actionsValue := m.getNestedValue(rawConfig, workspaceShortcutsActionsKey)
	if actionsValue == nil {
		return
	}

	userActions, ok := m.toStringAnyMap(actionsValue)
	if !ok {
		return
	}

	defaultActions := m.defaultConfig.Workspace.Shortcuts.Actions
	for actionName, defaultAction := range defaultActions {
		if _, exists := userActions[actionName]; exists {
			continue
		}
		userActions[actionName] = defaultAction
	}

	m.setNestedValue(rawConfig, workspaceShortcutsActionsKey, userActions)
}

func appendUniqueKeys(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}

	seen := make(map[string]bool, len(base)+len(extra))
	for _, key := range base {
		seen[key] = true
	}

	for _, key := range extra {
		if seen[key] {
			continue
		}
		base = append(base, key)
		seen[key] = true
	}

	sort.Strings(base)
	return base
}

func (*Migrator) toStringAnyMap(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}

	if m, ok := value.(map[string]any); ok {
		return m, true
	}

	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Map {
		return nil, false
	}

	out := make(map[string]any, rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		k := iter.Key()
		if k.Kind() != reflect.String {
			return nil, false
		}
		out[k.String()] = iter.Value().Interface()
	}

	return out, true
}

// findMissingKeys returns keys that are in defaults but not in user config.
func (m *Migrator) findMissingKeys(defaultKeys []string, userKeys map[string]bool) []string {
	missing := make([]string, 0)

	for _, key := range defaultKeys {
		// Check if key or any related key exists in user config
		if m.keyOrRelatedExists(key, userKeys) {
			continue
		}
		missing = append(missing, key)
	}

	// Sort for consistent output
	sort.Strings(missing)
	return missing
}

// keyOrRelatedExists checks if a key, any parent, or any child keys exist in the map.
// This handles cases where:
// - User has defined a parent section but not all children
// - Default is a struct key but user has individual sub-keys (e.g., palettes)
func (*Migrator) keyOrRelatedExists(key string, keys map[string]bool) bool {
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

	// Check if any child keys exist (e.g., default is "appearance.dark_palette"
	// but user has "appearance.dark_palette.background")
	keyPrefix := key + "."
	for userKey := range keys {
		if strings.HasPrefix(userKey, keyPrefix) {
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
