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

// KeyChangeType indicates the type of config key change.
type KeyChangeType int

const (
	// KeyChangeAdded indicates a new key was added to defaults.
	KeyChangeAdded KeyChangeType = iota
	// KeyChangeRemoved indicates a key in user config no longer exists in defaults.
	KeyChangeRemoved
	// KeyChangeRenamed indicates a key was renamed (detected via similarity).
	KeyChangeRenamed
)

// String returns a display symbol for the change type.
func (t KeyChangeType) String() string {
	switch t {
	case KeyChangeAdded:
		return "+"
	case KeyChangeRemoved:
		return "-"
	case KeyChangeRenamed:
		return "~"
	default:
		return "?"
	}
}

// KeyChange represents a detected change between user config and defaults.
type KeyChange struct {
	Type     KeyChangeType // Type of change
	OldKey   string        // Old key name (for removed/renamed)
	NewKey   string        // New key name (for added/renamed)
	OldValue string        // Old value (for renamed/consolidated)
	NewValue string        // New/default value
}

// ConfigMigrator checks for and applies config migrations.
type ConfigMigrator interface {
	// CheckMigration checks if user config is missing any default keys.
	// Returns nil if no migration is needed (config file doesn't exist or is complete).
	CheckMigration() (*MigrationResult, error)

	// DetectChanges analyzes user config and returns all detected changes.
	// This provides a detailed diff-like view of what migration would do.
	DetectChanges() ([]KeyChange, error)

	// Migrate adds missing default keys and removes deprecated keys from the user's config file.
	// Returns the list of keys that were added/renamed/removed.
	Migrate() ([]string, error)

	// GetKeyInfo returns detailed information about a config key.
	GetKeyInfo(key string) KeyInfo

	// GetConfigFile returns the path to the user's config file.
	GetConfigFile() (string, error)
}

// DiffFormatter formats config changes for display.
type DiffFormatter interface {
	// FormatChangesAsDiff returns changes formatted as a diff-like string for display.
	FormatChangesAsDiff(changes []KeyChange) string
}
