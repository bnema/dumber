package port

// MigrationResult contains the result of a config migration check.
type MigrationResult struct {
	// MissingKeys contains the keys that exist in defaults but not in user config.
	MissingKeys []string
	// ConfigFile is the path to the user's config file.
	ConfigFile string
}

// KeyInfo contains metadata about a config key for display purposes.
type KeyInfo struct {
	// Key is the dot-notation key path (e.g., "update.enable_on_startup").
	Key string
	// Type is the Go type of the value (e.g., "bool", "int", "string").
	Type string
	// DefaultValue is a string representation of the default value.
	DefaultValue string
}

// ConfigMigrator checks for and applies config migrations.
type ConfigMigrator interface {
	// CheckMigration checks if user config is missing any default keys.
	// Returns nil if no migration is needed (config file doesn't exist or is complete).
	CheckMigration() (*MigrationResult, error)

	// Migrate adds missing default keys to the user's config file.
	// Returns the list of keys that were added.
	Migrate() ([]string, error)

	// GetKeyInfo returns detailed information about a config key.
	GetKeyInfo(key string) KeyInfo
}
