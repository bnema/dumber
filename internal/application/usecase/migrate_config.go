package usecase

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// CheckConfigMigrationInput holds the input for checking config migration.
type CheckConfigMigrationInput struct{}

// CheckConfigMigrationOutput holds the result of the migration check.
type CheckConfigMigrationOutput struct {
	// NeedsMigration is true if there are missing keys.
	NeedsMigration bool
	// MissingKeys contains info about each missing key.
	MissingKeys []port.KeyInfo
	// ConfigFile is the path to the config file.
	ConfigFile string
}

// DetectChangesInput holds the input for detecting config changes.
type DetectChangesInput struct{}

// DetectChangesOutput holds the result of change detection.
type DetectChangesOutput struct {
	// HasChanges is true if any changes were detected.
	HasChanges bool
	// Changes contains all detected changes.
	Changes []port.KeyChange
	// DiffText is a formatted diff-like string representation.
	DiffText string
}

// MigrateConfigInput holds the input for migrating config.
type MigrateConfigInput struct{}

// MigrateConfigOutput holds the result of the migration.
type MigrateConfigOutput struct {
	// AddedKeys contains the keys that were added.
	AddedKeys []string
	// ConfigFile is the path to the config file.
	ConfigFile string
}

// MigrateConfigUseCase handles config migration operations.
type MigrateConfigUseCase struct {
	migrator      port.ConfigMigrator
	diffFormatter port.DiffFormatter
}

// NewMigrateConfigUseCase creates a new migrate config use case.
func NewMigrateConfigUseCase(migrator port.ConfigMigrator, diffFormatter port.DiffFormatter) *MigrateConfigUseCase {
	return &MigrateConfigUseCase{
		migrator:      migrator,
		diffFormatter: diffFormatter,
	}
}

// Check checks if the user config is missing any default keys.
func (uc *MigrateConfigUseCase) Check(ctx context.Context, _ CheckConfigMigrationInput) (*CheckConfigMigrationOutput, error) {
	log := logging.FromContext(ctx)

	result, err := uc.migrator.CheckMigration()
	if err != nil {
		log.Warn().Err(err).Msg("config migration check failed")
		return nil, err
	}

	// No migration needed
	if result == nil || len(result.MissingKeys) == 0 {
		log.Debug().Msg("config is up to date, no migration needed")
		return &CheckConfigMigrationOutput{
			NeedsMigration: false,
			MissingKeys:    nil,
			ConfigFile:     "",
		}, nil
	}

	// Build key info for each missing key
	keyInfos := make([]port.KeyInfo, 0, len(result.MissingKeys))
	for _, key := range result.MissingKeys {
		keyInfos = append(keyInfos, uc.migrator.GetKeyInfo(key))
	}

	log.Debug().
		Int("missing_keys", len(result.MissingKeys)).
		Str("config_file", result.ConfigFile).
		Msg("config migration check completed")

	return &CheckConfigMigrationOutput{
		NeedsMigration: true,
		MissingKeys:    keyInfos,
		ConfigFile:     result.ConfigFile,
	}, nil
}

// DetectChanges detects all config changes and returns a diff-like output.
func (uc *MigrateConfigUseCase) DetectChanges(ctx context.Context, _ DetectChangesInput) (*DetectChangesOutput, error) {
	log := logging.FromContext(ctx)

	changes, err := uc.migrator.DetectChanges()
	if err != nil {
		log.Warn().Err(err).Msg("config change detection failed")
		return nil, err
	}

	if len(changes) == 0 {
		log.Debug().Msg("no config changes detected")
		return &DetectChangesOutput{
			HasChanges: false,
			Changes:    nil,
			DiffText:   "No changes detected.",
		}, nil
	}

	log.Debug().
		Int("changes", len(changes)).
		Msg("config changes detected")

	return &DetectChangesOutput{
		HasChanges: true,
		Changes:    changes,
		DiffText:   uc.diffFormatter.FormatChangesAsDiff(changes),
	}, nil
}

// Execute adds missing default keys and removes deprecated keys from the user's config file.
func (uc *MigrateConfigUseCase) Execute(ctx context.Context, _ MigrateConfigInput) (*MigrateConfigOutput, error) {
	log := logging.FromContext(ctx)

	// Check for any changes (added, removed, or renamed keys)
	changes, err := uc.migrator.DetectChanges()
	if err != nil {
		return nil, err
	}

	if len(changes) == 0 {
		log.Debug().Msg("no migration needed")
		return &MigrateConfigOutput{
			AddedKeys:  nil,
			ConfigFile: "",
		}, nil
	}

	// Get config file path for output
	configFile, _ := uc.migrator.GetConfigFile()

	// Perform migration
	appliedKeys, err := uc.migrator.Migrate()
	if err != nil {
		log.Error().Err(err).Msg("config migration failed")
		return nil, err
	}

	log.Info().
		Int("applied_keys", len(appliedKeys)).
		Str("config_file", configFile).
		Msg("config migration completed")

	return &MigrateConfigOutput{
		AddedKeys:  appliedKeys,
		ConfigFile: configFile,
	}, nil
}
