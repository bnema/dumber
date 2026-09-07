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
// The database connection is created on first access, deferring the
// connection and migration overhead until actually needed.
//
// Callers may warm the provider with Warmup while other startup work (for
// example engine initialization) runs concurrently. Close waits for an
// in-flight initialization to settle before closing, so cleanup never
// orphans a connection that finishes opening after Close returns.
type LazyDB struct {
	dbPath string
	db     *sql.DB
	err    error
	once   sync.Once
	mu     sync.RWMutex
	// started is closed when the first initialization begins; done is
	// closed when it completes. They let Close distinguish "never used"
	// from "initialization in flight" without triggering initialization.
	started chan struct{}
	done    chan struct{}
}

// Compile-time interface check.
var _ port.DatabaseProvider = (*LazyDB)(nil)

// initGateForTest, when non-nil, runs at the start of the once-initializer
// before the connection is opened. Tests use it to hold initialization in
// flight and prove Close waits for it to settle. It must remain nil in
// production.
var initGateForTest func()

// NewLazyDB creates a new lazy database provider.
// The actual connection is not established until DB() is called, or until a
// Warmup task triggers initialization in the background.
func NewLazyDB(dbPath string) *LazyDB {
	return &LazyDB{dbPath: dbPath, started: make(chan struct{}), done: make(chan struct{})}
}

// DB returns the database connection, initializing it if necessary.
// This method is thread-safe and will only initialize once.
func (l *LazyDB) DB(ctx context.Context) (*sql.DB, error) {
	l.once.Do(func() {
		close(l.started)
		defer close(l.done)
		if initGateForTest != nil {
			initGateForTest()
		}
		log := logging.FromContext(ctx)
		log.Debug().Str("path", l.dbPath).Msg("lazy database initialization starting")

		db, err := NewConnection(ctx, l.dbPath)

		// Acquire lock before assigning to ensure IsInitialized() sees
		// a consistent state when reading l.db under RLock.
		l.mu.Lock()
		l.db = db
		l.err = err
		l.mu.Unlock()

		if err != nil {
			log.Error().Err(err).Msg("lazy database initialization failed")
		} else {
			log.Debug().Msg("lazy database initialized successfully")
		}
	})

	l.mu.RLock()
	db, err := l.db, l.err
	l.mu.RUnlock()

	if err != nil {
		return nil, fmt.Errorf("database initialization failed: %w", err)
	}
	return db, nil
}

// Warmup triggers database initialization in an owned background task and
// returns immediately. Initialization errors are cached and reported to
// later DB callers; pass a detached context (for example
// context.WithoutCancel) so shutdown cancellation cannot poison the cached
// result.
func (l *LazyDB) Warmup(ctx context.Context) {
	go func() {
		_, _ = l.DB(ctx)
	}()
}

// Close closes the database connection if it was initialized. When an
// initialization is in flight, Close waits for it to settle first so the
// resulting connection is closed rather than orphaned. Close never triggers
// initialization: closing a provider that was never used is a no-op.
func (l *LazyDB) Close() error {
	select {
	case <-l.started:
		<-l.done
	default:
		return nil
	}

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
