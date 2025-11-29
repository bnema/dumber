package migrations

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.sql
var migrationFiles embed.FS

// Migration represents a single database migration
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// GetMigrations returns all embedded migrations sorted by version
func GetMigrations() ([]Migration, error) {
	entries, err := migrationFiles.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded migrations directory: %w", err)
	}

	var migrations []Migration
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Parse version from filename (e.g., "001_initial.sql" -> version 1)
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			log.Printf("Warning: skipping migration file with invalid name format: %s", entry.Name())
			continue
		}

		versionStr := parts[0]
		version, err := strconv.Atoi(versionStr)
		if err != nil {
			log.Printf("Warning: skipping migration file with invalid version: %s", entry.Name())
			continue
		}

		// Extract name without extension
		name := strings.TrimSuffix(parts[1], ".sql")

		// Read SQL content
		content, err := migrationFiles.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			SQL:     string(content),
		})
	}

	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// RunEmbeddedMigrations applies all pending embedded migrations to the database
func RunEmbeddedMigrations(db *sql.DB) error {
	// Create migrations tracking table if it doesn't exist
	if err := createMigrationsTable(db); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// NOTE: Legacy databases without migration tracking should be deleted and recreated.
	// All new databases will have migration tracking from the start.
	// Backfill logic was removed to maintain idempotency - migrations are the single source of truth.

	migrations, err := GetMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	for _, migration := range migrations {
		if err := applyMigration(db, migration); err != nil {
			return fmt.Errorf("failed to apply migration %d (%s): %w", migration.Version, migration.Name, err)
		}
	}

	return nil
}

// createMigrationsTable creates the table for tracking applied migrations
func createMigrationsTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err := db.Exec(schema)
	return err
}

// applyMigration applies a single migration if it hasn't been applied yet
func applyMigration(db *sql.DB, migration Migration) error {
	// Check if migration has already been applied
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", migration.Version).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check migration status: %w", err)
	}

	if count > 0 {
		// Migration already applied, skip
		return nil
	}

	log.Printf("Applying migration %03d: %s", migration.Version, migration.Name)

	// Use transaction for safety
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Printf("failed to rollback migration transaction: %v", rollbackErr)
			}
		}
	}()

	// Execute migration SQL
	if _, err := tx.Exec(migration.SQL); err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Record migration as applied
	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (version, name) VALUES (?, ?)",
		migration.Version, migration.Name,
	); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration transaction: %w", err)
	}

	log.Printf("Successfully applied migration %03d: %s", migration.Version, migration.Name)
	return nil
}

// GetAppliedMigrations returns the list of applied migration versions
func GetAppliedMigrations(db *sql.DB) ([]int, error) {
	// First check if migrations table exists
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to check if migrations table exists: %w", err)
	}

	if count == 0 {
		// Migrations table doesn't exist, no migrations applied
		return []int{}, nil
	}

	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("failed to close migration query rows: %v", err)
		}
	}()

	var versions []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("failed to scan migration version: %w", err)
		}
		versions = append(versions, version)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over migration rows: %w", err)
	}

	return versions, nil
}

// VerifyAllMigrationsApplied scans all embedded migrations and verifies they have been applied
// Returns an error if any migrations are missing or if verification fails
func VerifyAllMigrationsApplied(db *sql.DB) error {
	// Get all embedded migrations
	allMigrations, err := GetMigrations()
	if err != nil {
		return fmt.Errorf("failed to get embedded migrations: %w", err)
	}

	// Get applied migrations from database
	appliedVersions, err := GetAppliedMigrations(db)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Create map for quick lookup
	appliedMap := make(map[int]bool)
	for _, version := range appliedVersions {
		appliedMap[version] = true
	}

	// Check each migration is applied
	var missingMigrations []Migration
	for _, migration := range allMigrations {
		if !appliedMap[migration.Version] {
			missingMigrations = append(missingMigrations, migration)
		}
	}

	// Report results
	if len(missingMigrations) > 0 {
		log.Printf("WARNING: %d migrations are not applied:", len(missingMigrations))
		for _, migration := range missingMigrations {
			log.Printf("  - Migration %03d: %s", migration.Version, migration.Name)
		}
		return fmt.Errorf("%d migrations are not applied", len(missingMigrations))
	}

	log.Printf("Migration verification: All %d migrations are applied", len(allMigrations))
	return nil
}
