package entity

// ConfigKeyInfo describes a single configuration key for schema documentation.
type ConfigKeyInfo struct {
	// Key is the full dotted path to the config key (e.g., "appearance.color_scheme")
	Key string `json:"key"`

	// Type is the Go type name (e.g., "string", "int", "bool", "float64")
	Type string `json:"type"`

	// Default is the default value as a string representation
	Default string `json:"default"`

	// Description explains the purpose of this config key
	Description string `json:"description"`

	// Values contains valid enum values (for string enums)
	// Empty if not an enum type
	Values []string `json:"values,omitempty"`

	// Range describes numeric constraints (e.g., "1-72", "0.1-5.0")
	// Empty if no range constraint
	Range string `json:"range,omitempty"`

	// Section groups related keys (e.g., "Appearance", "Logging")
	Section string `json:"section"`
}
