package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/invopop/jsonschema"
)

// GenerateSchemaFile generates a JSON schema file for the configuration.
// This is called automatically when a default config is created.
func GenerateSchemaFile() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	schemaFile := filepath.Join(configDir, "config.schema.json")

	// Create reflector and generate schema
	r := new(jsonschema.Reflector)
	schema := r.Reflect(&Config{})

	// Set schema metadata
	schema.ID = "https://github.com/bnema/dumber/config.schema.json"
	schema.Title = "Dumber Browser Configuration"
	schema.Description = "Configuration schema for Dumber, a minimalist browser for tiling window managers"

	// Marshal to pretty JSON
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	// Write schema file
	if err := os.WriteFile(schemaFile, data, filePerm); err != nil {
		return fmt.Errorf("failed to write schema file: %w", err)
	}

	fmt.Printf("Generated JSON schema: %s\n", schemaFile)
	return nil
}
