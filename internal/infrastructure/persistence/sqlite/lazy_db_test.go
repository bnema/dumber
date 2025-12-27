package sqlite_test

import (
	"path/filepath"
	"sync"
	"testing"

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
		db  interface{}
		err error
	}, goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			db, err := lazy.DB(ctx)
			results <- &struct {
				db  interface{}
				err error
			}{db, err}
		}()
	}

	wg.Wait()
	close(results)

	var firstDB interface{}
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

	// Close without ever calling DB() should not error
	err := lazy.Close()
	assert.NoError(t, err)
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
