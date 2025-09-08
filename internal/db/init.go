package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/config"
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
		database.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize database schema
	if err := initializeSchema(database); err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to initialize database schema: %w", err)
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
		db.Close()
		return nil, fmt.Errorf("failed to initialize shortcuts: %w", err)
	}

	return db, nil
}

// initializeSchema creates the database schema if it doesn't exist
func initializeSchema(db *sql.DB) error {
	schema := `
	-- History tracking for visited URLs
	CREATE TABLE IF NOT EXISTS history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT NOT NULL UNIQUE,
		title TEXT,
		visit_count INTEGER DEFAULT 1,
		last_visited DATETIME DEFAULT CURRENT_TIMESTAMP,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_history_url ON history(url);
	CREATE INDEX IF NOT EXISTS idx_history_last_visited ON history(last_visited);

	-- URL shortcuts configuration
	CREATE TABLE IF NOT EXISTS shortcuts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		shortcut TEXT NOT NULL UNIQUE,
		url_template TEXT NOT NULL,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	// Execute schema creation
	_, err := db.Exec(schema)
	return err
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