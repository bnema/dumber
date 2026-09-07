package sqlite_test

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLazyDB_NotInitializedByDefault(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	lazy := sqlite.NewLazyDB(dbPath)

	assert.False(t, lazy.IsInitialized(), "LazyDB should not be initialized before DB() is called")
}

func TestLazyDB_InitializesOnFirstAccess(t *testing.T) {
	ctx := testCtx()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	lazy := sqlite.NewLazyDB(dbPath)

	db, err := lazy.DB(ctx)
	require.NoError(t, err)
	require.NotNil(t, db)

	assert.True(t, lazy.IsInitialized(), "LazyDB should be initialized after DB() is called")

	// Cleanup
	require.NoError(t, lazy.Close())
}

func TestLazyDB_ReturnsSameConnection(t *testing.T) {
	ctx := testCtx()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	lazy := sqlite.NewLazyDB(dbPath)

	db1, err := lazy.DB(ctx)
	require.NoError(t, err)

	db2, err := lazy.DB(ctx)
	require.NoError(t, err)

	assert.Same(t, db1, db2, "DB() should return the same connection instance")

	require.NoError(t, lazy.Close())
}

func TestLazyDB_ConcurrentAccess(t *testing.T) {
	ctx := testCtx()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	lazy := sqlite.NewLazyDB(dbPath)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	results := make(chan *struct {
		db  any
		err error
	}, goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			db, err := lazy.DB(ctx)
			results <- &struct {
				db  any
				err error
			}{db, err}
		}()
	}

	wg.Wait()
	close(results)

	var firstDB any
	for result := range results {
		require.NoError(t, result.err)
		require.NotNil(t, result.db)

		if firstDB == nil {
			firstDB = result.db
		} else {
			assert.Same(t, firstDB, result.db, "All goroutines should receive the same DB instance")
		}
	}

	require.NoError(t, lazy.Close())
}

func TestLazyDB_CloseBeforeInit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	lazy := sqlite.NewLazyDB(dbPath)

	// Close without ever calling DB() should not error and must not trigger
	// initialization.
	require.NoError(t, lazy.Close())
	assert.False(t, lazy.IsInitialized(), "Close must not trigger initialization")
}

func TestLazyDB_Path(t *testing.T) {
	dbPath := "/some/path/to/db.sqlite"
	lazy := sqlite.NewLazyDB(dbPath)

	assert.Equal(t, dbPath, lazy.Path())
}

func TestLazyDB_DBIsUsable(t *testing.T) {
	ctx := testCtx()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	lazy := sqlite.NewLazyDB(dbPath)

	db, err := lazy.DB(ctx)
	require.NoError(t, err)

	// Verify we can execute a simple query
	var result int
	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, 1, result)

	require.NoError(t, lazy.Close())
}

func TestLazyDB_WarmupInitializesInBackground(t *testing.T) {
	ctx := testCtx()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	lazy := sqlite.NewLazyDB(dbPath)

	// Warmup returns immediately and initializes the provider concurrently.
	lazy.Warmup(ctx)

	require.Eventually(t, func() bool {
		return lazy.IsInitialized()
	}, 10*time.Second, 5*time.Millisecond, "warmup should initialize the provider without a direct DB call")

	db, err := lazy.DB(ctx)
	require.NoError(t, err)
	require.NotNil(t, db)
	require.NoError(t, lazy.Close())
}

func TestLazyDB_CloseWaitsForInflightInit(t *testing.T) {
	ctx := testCtx()
	// Warmup is asynchronous: Close called before the warmup goroutine runs
	// is a legitimate no-op, so wait for initialization to be underway
	// before asserting Close settles and releases the provider cleanly.
	dbPath := filepath.Join(t.TempDir(), "test.db")
	lazy := sqlite.NewLazyDB(dbPath)

	lazy.Warmup(ctx)
	require.Eventually(t, func() bool {
		return lazy.IsInitialized()
	}, 10*time.Second, 5*time.Millisecond, "warmup should initialize the provider")

	require.NoError(t, lazy.Close())
}

func TestLazyDB_ConcurrentWarmupAccessAndClose(t *testing.T) {
	ctx := testCtx()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	lazy := sqlite.NewLazyDB(dbPath)

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers + 1)
	for range workers {
		go func() {
			defer wg.Done()
			_, _ = lazy.DB(ctx)
		}()
	}
	go func() {
		defer wg.Done()
		lazy.Warmup(ctx)
	}()
	wg.Wait()

	db, err := lazy.DB(ctx)
	require.NoError(t, err)
	require.NotNil(t, db)
	require.NoError(t, lazy.Close())
}

func TestLazyDB_InitErrorPropagated(t *testing.T) {
	ctx := testCtx()
	// An empty path always fails connection setup; the error must be cached
	// and reported to every later caller without panicking.
	lazy := sqlite.NewLazyDB("")

	_, err := lazy.DB(ctx)
	require.Error(t, err, "initialization failure must propagate to the first caller")

	_, err = lazy.DB(ctx)
	require.Error(t, err, "initialization failure must propagate to later callers")

	// Closing a failed provider must not error: there is no connection.
	require.NoError(t, lazy.Close())
	assert.False(t, lazy.IsInitialized())
}
