package sqlite

import (
	"context"
	"database/sql"
	"errors"
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
// example engine initialization) runs concurrently. Warmup registers its
// background task synchronously, so a Close that observes Warmup's return
// always waits for the task to settle instead of racing goroutine
// scheduling. Initialization admission is serialized with closure: once
// Close begins, no new initialization can start and late callers receive a
// closed-provider error, so no connection can open with nobody left to
// close it. An initialization admitted before Close began runs to
// completion (or aborts early via the owned init context) and its
// connection is then released. Ownership contract: Warmup must return
// before Close is called; concurrent Warmup and Close from different
// goroutines without external sequencing is not supported.
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
	// warmTasks tracks one completion channel per Warmup call, registered
	// synchronously under mu. Close waits for every registered task, so a
	// Close that observes Warmup's return settles the background task even
	// when the once-initializer has not been scheduled yet. Each channel is
	// closed exactly once by its own goroutine after its DB call returns,
	// which happens after the once-initializer completes, so observing all
	// closures implies initialization has settled.
	warmTasks []chan struct{}
	// closed is set by Close under mu. The once-initializer checks it under
	// the same mutex before opening anything, which serializes admission
	// with closure: init admitted before Close runs to completion and is
	// then released; init arriving after is skipped with ErrLazyDBClosed.
	closed bool
	// dbClosed records that Close released the connection, making repeat
	// Close calls idempotent.
	dbClosed bool
	// initCtx carries the connection work; initCancel aborts an admitted
	// in-flight initialization when Close begins so shutdown (and the
	// engine-build error path) is not stuck behind a doomed migration.
	// Canceling a settled provider is a no-op for later callers because
	// closure already rejects them.
	initCtx    context.Context
	initCancel context.CancelFunc
}

// Compile-time interface check.
var _ port.DatabaseProvider = (*LazyDB)(nil)

// ErrLazyDBClosed is returned to callers arriving after the provider was
// closed, including initializations that lose the admission race.
var ErrLazyDBClosed = errors.New("sqlite: database provider closed")

// initGateForTest, when non-nil, runs inside the once-initializer after
// admission is granted and before the connection is opened. Tests use it to
// hold an admitted initialization in flight. It must remain nil in
// production.
var initGateForTest func()

// NewLazyDB creates a new lazy database provider.
// The actual connection is not established until DB() is called, or until a
// Warmup task triggers initialization in the background.
func NewLazyDB(dbPath string) *LazyDB {
	initCtx, initCancel := context.WithCancel(context.Background())
	return &LazyDB{
		dbPath:     dbPath,
		started:    make(chan struct{}),
		done:       make(chan struct{}),
		initCtx:    initCtx,
		initCancel: initCancel,
	}
}

// DB returns the database connection, initializing it if necessary.
// This method is thread-safe and will only initialize once. Calls arriving
// after Close receive ErrLazyDBClosed without starting initialization.
func (l *LazyDB) DB(ctx context.Context) (*sql.DB, error) {
	l.once.Do(func() {
		close(l.started)
		defer close(l.done)
		l.mu.Lock()
		admitted := !l.closed
		l.mu.Unlock()
		if !admitted {
			l.mu.Lock()
			l.err = ErrLazyDBClosed
			l.mu.Unlock()
			logging.FromContext(ctx).Debug().Msg("lazy database initialization skipped: provider closed")
			return
		}
		if initGateForTest != nil {
			initGateForTest()
		}
		log := logging.FromContext(ctx)
		log.Debug().Str("path", l.dbPath).Msg("lazy database initialization starting")

		// The owned init context carries the connection work so Close can
		// abort a doomed initialization; caller values are preserved for
		// logging above. Shutdown cancellation can no longer poison the
		// cache because closure rejects later callers.
		db, err := NewConnection(l.initCtx, l.dbPath)

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
	db, err, closed := l.db, l.err, l.closed
	l.mu.RUnlock()

	if err != nil {
		if errors.Is(err, ErrLazyDBClosed) {
			return nil, ErrLazyDBClosed
		}
		return nil, fmt.Errorf("database initialization failed: %w", err)
	}
	if closed {
		return nil, ErrLazyDBClosed
	}
	return db, nil
}

// Warmup triggers database initialization in an owned background task and
// returns immediately. Initialization errors are cached and reported to
// later DB callers; pass a detached context (for example
// context.WithoutCancel) so shutdown cancellation cannot poison the cached
// result. The task is registered synchronously, so Close called after
// Warmup returns settles it (see the ownership contract on LazyDB).
func (l *LazyDB) Warmup(ctx context.Context) {
	task := make(chan struct{})
	l.mu.Lock()
	l.warmTasks = append(l.warmTasks, task)
	l.mu.Unlock()
	go func() {
		defer close(task)
		_, _ = l.DB(ctx)
	}()
}

// Close seals the provider against new initialization, aborts admitted
// in-flight connection work via the owned init context, settles every
// registered Warmup task and awaited initialization, then releases the
// connection. No connection can open after Close begins: late initializers
// are skipped with ErrLazyDBClosed and admitted ones are awaited and
// closed. Close never triggers initialization itself, and repeat calls are
// idempotent.
func (l *LazyDB) Close() error {
	// Abort doomed connection work first so shutdown and the engine-build
	// error path are not stuck behind a hung open or migration. Safe to
	// call repeatedly and on settled providers.
	l.initCancel()

	l.mu.Lock()
	l.closed = true
	tasks := make([]chan struct{}, len(l.warmTasks))
	copy(tasks, l.warmTasks)
	l.mu.Unlock()
	for _, task := range tasks {
		<-task
	}

	select {
	case <-l.started:
		<-l.done
	default:
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.dbClosed || l.db == nil {
		return nil
	}
	err := l.db.Close()
	l.dbClosed = true
	return err
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
