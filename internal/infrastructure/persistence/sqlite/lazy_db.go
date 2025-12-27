package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// LazyDB implements port.DatabaseProvider with lazy initialization.
// The database connection is created on first access, deferring the ~300-400ms
// WASM compilation and migration overhead until actually needed.
//
// Currently unused: the application uses eager initialization via OpenDatabase
// in RunParallelDBWebKit. This implementation is kept for potential future use
// in scenarios where deferring database initialization past first paint provides
// additional latency benefits, or for CLI commands that may not need DB access.
type LazyDB struct {
	dbPath string
	db     *sql.DB
	err    error
	once   sync.Once
	mu     sync.RWMutex
}

// Compile-time interface check.
var _ port.DatabaseProvider = (*LazyDB)(nil)

// NewLazyDB creates a new lazy database provider.
// The actual connection is not established until DB() is called.
func NewLazyDB(dbPath string) *LazyDB {
	return &LazyDB{dbPath: dbPath}
}

// DB returns the database connection, initializing it if necessary.
// This method is thread-safe and will only initialize once.
func (l *LazyDB) DB(ctx context.Context) (*sql.DB, error) {
	l.once.Do(func() {
		log := logging.FromContext(ctx)
		log.Debug().Str("path", l.dbPath).Msg("lazy database initialization starting")

		l.db, l.err = NewConnection(ctx, l.dbPath)
		if l.err != nil {
			log.Error().Err(l.err).Msg("lazy database initialization failed")
		} else {
			log.Debug().Msg("lazy database initialized successfully")
		}
	})

	if l.err != nil {
		return nil, fmt.Errorf("database initialization failed: %w", l.err)
	}
	return l.db, nil
}

// Close closes the database connection if it was initialized.
func (l *LazyDB) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.db != nil {
		return l.db.Close()
	}
	return nil
}

// IsInitialized returns true if the database has been initialized.
func (l *LazyDB) IsInitialized() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.db != nil
}

// Path returns the database path.
func (l *LazyDB) Path() string {
	return l.dbPath
}
