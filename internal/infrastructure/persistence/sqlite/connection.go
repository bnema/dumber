package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/logging"
	_ "github.com/ncruces/go-sqlite3/driver" // SQLite driver (pure Go)
	_ "github.com/ncruces/go-sqlite3/embed"  // Embed SQLite WASM binary
)

// NewConnection creates a new SQLite database connection with optimized settings.
// It creates the database directory if it doesn't exist and applies performance pragmas.
func NewConnection(ctx context.Context, dbPath string) (*sql.DB, error) {
	const dbDirPerm = 0o750
	log := logging.FromContext(ctx)

	if dbPath == "" {
		return nil, fmt.Errorf("database path cannot be empty")
	}

	// Ensure database directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), dbDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Apply performance pragmas
	if err := applyPragmas(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	// Run migrations
	if err := RunMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Info().Str("path", dbPath).Msg("database connection established")

	return db, nil
}

// applyPragmas configures SQLite for optimal performance.
func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",      // Write-Ahead Logging for concurrent access
		"PRAGMA synchronous = NORMAL",    // Safe in WAL mode
		"PRAGMA cache_size = -64000",     // 64MB cache
		"PRAGMA temp_store = MEMORY",     // Temporary tables in RAM
		"PRAGMA mmap_size = 30000000000", // 30GB memory-mapped I/O
		"PRAGMA busy_timeout = 5000",     // Wait 5 seconds on lock contention
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to set pragma %q: %w", pragma, err)
		}
	}

	return nil
}

// Close closes the database connection gracefully.
func Close(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}
