// Package port defines interfaces for infrastructure adapters.
package port

import (
	"context"
	"database/sql"
)

// DatabaseProvider provides access to the database connection.
// Implementations may initialize the database lazily on first access.
//
// Currently used by the lazy repository infrastructure (sqlite.LazyDB and lazy_repos.go),
// which is kept for potential future optimization scenarios where deferring database
// initialization past first paint provides latency benefits.
type DatabaseProvider interface {
	// DB returns the database connection, initializing it if necessary.
	// Returns an error if initialization fails.
	DB(ctx context.Context) (*sql.DB, error)

	// Close closes the database connection if it was initialized.
	Close() error

	// IsInitialized returns true if the database has been initialized.
	IsInitialized() bool
}
