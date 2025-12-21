package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/bnema/dumber/internal/logging"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// RunMigrations applies all pending migrations to the database.
// Uses goose with embedded SQL files for automatic schema management.
func RunMigrations(ctx context.Context, db *sql.DB) error {
	log := logging.FromContext(ctx)

	// Configure goose
	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	// Disable verbose logging from goose
	goose.SetLogger(goose.NopLogger())

	// Get current version before migration
	currentVersion, err := goose.GetDBVersion(db)
	if err != nil {
		log.Debug().Err(err).Msg("could not get current db version (may be new database)")
		currentVersion = 0
	}

	// Run migrations
	if migrateErr := goose.Up(db, "migrations"); migrateErr != nil {
		return fmt.Errorf("failed to run migrations: %w", migrateErr)
	}

	// Get new version after migration
	newVersion, err := goose.GetDBVersion(db)
	if err != nil {
		return fmt.Errorf("failed to get db version after migration: %w", err)
	}

	if newVersion > currentVersion {
		log.Info().
			Int64("from_version", currentVersion).
			Int64("to_version", newVersion).
			Msg("database migrations applied")
	} else {
		log.Debug().Int64("version", newVersion).Msg("database schema up to date")
	}

	return nil
}

// GetMigrationStatus returns the current migration version.
func GetMigrationStatus(db *sql.DB) (int64, error) {
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return 0, err
	}
	return goose.GetDBVersion(db)
}
