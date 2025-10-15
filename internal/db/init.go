package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/migrations"
	_ "github.com/ncruces/go-sqlite3/driver" // SQLite driver
	_ "github.com/ncruces/go-sqlite3/embed"  // Embed SQLite
)

// InitDB initializes the database connection and schema
func InitDB(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("database path cannot be empty")
	}

	// Ensure database directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := database.Ping(); err != nil {
		if err := database.Close(); err != nil {
			log.Printf("Warning: failed to close database: %v", err)
		}
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Run embedded migrations - single source of truth for schema initialization
	if err := migrations.RunEmbeddedMigrations(database); err != nil {
		if err := database.Close(); err != nil {
			log.Printf("Warning: failed to close database: %v", err)
		}
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	// Verify all migrations are applied
	if err := migrations.VerifyAllMigrationsApplied(database); err != nil {
		if err := database.Close(); err != nil {
			log.Printf("Warning: failed to close database: %v", err)
		}
		return nil, fmt.Errorf("migration verification failed: %w", err)
	}

	return database, nil
}

// InitDBWithConfig initializes the database with configuration-based shortcuts
func InitDBWithConfig(dbPath string, cfg *config.Config) (*sql.DB, error) {
	db, err := InitDB(dbPath)
	if err != nil {
		return nil, err
	}

	// Initialize shortcuts from configuration
	if err := initializeShortcuts(db, cfg); err != nil {
		if err := db.Close(); err != nil {
			log.Printf("Warning: failed to close database: %v", err)
		}
		return nil, fmt.Errorf("failed to initialize shortcuts: %w", err)
	}

	return db, nil
}

// initializeShortcuts inserts configured shortcuts into the database
func initializeShortcuts(db *sql.DB, cfg *config.Config) error {
	if cfg == nil || cfg.SearchShortcuts == nil {
		return nil // No shortcuts to initialize
	}

	// Insert configured shortcuts
	for shortcut, shortcutCfg := range cfg.SearchShortcuts {
		_, err := db.Exec(
			"INSERT OR IGNORE INTO shortcuts (shortcut, url_template, description) VALUES (?, ?, ?)",
			shortcut, shortcutCfg.URL, shortcutCfg.Description,
		)
		if err != nil {
			return fmt.Errorf("failed to insert shortcut %s: %w", shortcut, err)
		}
	}

	return nil
}
