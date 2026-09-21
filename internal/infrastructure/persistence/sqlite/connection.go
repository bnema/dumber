package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/bnema/dumber/internal/logging"
	_ "github.com/bnema/purego-sqlite/driver" // SQLite driver (purego, no WASM/CGo)
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

	lock, err := lockDatabaseStartup(ctx, dbPath+".startup.lock")
	if err != nil {
		return nil, err
	}
	defer unlockDatabaseStartup(lock)

	// Open database connection
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool (must be done before any queries)
	configurePool(db)

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
	const (
		pragmaRetryTimeout = 5 * time.Second
		lockRetryInterval  = 25 * time.Millisecond
	)
	pragmas := []string{
		"PRAGMA journal_mode = WAL",    // Write-Ahead Logging for concurrent access
		"PRAGMA synchronous = NORMAL",  // Safe in WAL mode
		"PRAGMA cache_size = -64000",   // 64MB cache
		"PRAGMA temp_store = MEMORY",   // Temporary tables in RAM
		"PRAGMA mmap_size = 268435456", // 256MB memory-mapped I/O (reasonable for browser history)
		"PRAGMA busy_timeout = 5000",   // Wait 5 seconds on lock contention
		"PRAGMA foreign_keys = ON",     // Enable referential integrity
	}

	for _, pragma := range pragmas {
		deadline := time.Now().Add(pragmaRetryTimeout)
		for {
			if _, err := db.Exec(pragma); err != nil {
				if isSQLiteBusy(err) && time.Now().Before(deadline) {
					time.Sleep(lockRetryInterval)
					continue
				}
				return fmt.Errorf("failed to set pragma %q: %w", pragma, err)
			}
			break
		}
	}

	return nil
}

func isSQLiteBusy(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database is busy") ||
		strings.Contains(message, "sqlite_busy")
}

func lockDatabaseStartup(ctx context.Context, path string) (*os.File, error) {
	const (
		lockRetryInterval = 25 * time.Millisecond
		ownerOnlyFileMode = 0o600
	)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, ownerOnlyFileMode)
	if err != nil {
		return nil, fmt.Errorf("open database startup lock: %w", err)
	}
	for {
		if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
			return file, nil
		} else if !errors.Is(err, unix.EWOULDBLOCK) {
			_ = file.Close()
			return nil, fmt.Errorf("lock database startup: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, fmt.Errorf("lock database startup: %w", ctx.Err())
		case <-time.After(lockRetryInterval):
		}
	}
}

func unlockDatabaseStartup(file *os.File) {
	if file == nil {
		return
	}
	_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
	_ = file.Close()
}

// configurePool sets connection pool parameters optimized for SQLite.
// SQLite only supports one writer at a time, so we limit connections.
//
// Connections are configured to never expire or close while idle. This is safe
// because the application's lifecycle matches the database connection's lifecycle:
// the process terminates before the underlying database file is rotated or replaced.
func configurePool(db *sql.DB) {
	db.SetMaxOpenConns(1)    // SQLite is single-writer
	db.SetMaxIdleConns(1)    // Keep one connection alive
	db.SetConnMaxLifetime(0) // Never expire connections
	db.SetConnMaxIdleTime(0) // Never close idle connections
}

// Close closes the database connection gracefully.
func Close(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}
