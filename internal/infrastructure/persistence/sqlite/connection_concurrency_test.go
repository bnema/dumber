package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetryPragmaHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := retryPragma(ctx, func() error { return errors.New("database is locked") })
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(start), lockRetryInterval)
}

func TestLockDatabaseStartupTruncatesStaleContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "startup.lock")
	require.NoError(t, os.WriteFile(path, []byte("stale"), 0o600))
	lock, err := lockDatabaseStartup(context.Background(), path)
	require.NoError(t, err)
	require.NoError(t, lock.Close())
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Zero(t, info.Size())
}

func TestNewConnectionUsesWAL(t *testing.T) {
	db, err := NewConnection(context.Background(), filepath.Join(t.TempDir(), "wal.db"))
	require.NoError(t, err)
	defer db.Close()
	var mode string
	require.NoError(t, db.QueryRow("PRAGMA journal_mode").Scan(&mode))
	require.Equal(t, "wal", mode)
}

func TestNewConnectionConcurrentFirstOpen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shared.db")
	const openers = 8
	var wg sync.WaitGroup
	openErrors := make(chan error, openers)
	for range openers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := NewConnection(context.Background(), dbPath)
			if err == nil {
				err = db.Close()
			}
			openErrors <- err
		}()
	}
	wg.Wait()
	close(openErrors)
	for err := range openErrors {
		require.NoError(t, err)
	}
}
