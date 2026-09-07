package sqlite

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLazyDB_CloseBlocksWhileInitInflight holds the once-initializer open
// with initGateForTest and proves Close waits for it to settle instead of
// returning early and orphaning the connection that finishes opening after
// Close returns.
func TestLazyDB_CloseBlocksWhileInitInflight(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var enteredOnce atomic.Bool
	initGateForTest = func() {
		if enteredOnce.CompareAndSwap(false, true) {
			close(entered)
		}
		<-release
	}
	defer func() { initGateForTest = nil }()

	lazy := NewLazyDB(filepath.Join(t.TempDir(), "test.db"))

	dbErr := make(chan error, 1)
	go func() {
		_, err := lazy.DB(context.Background())
		dbErr <- err
	}()

	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("initializer did not start")
	}

	closeReturned := make(chan error, 1)
	go func() {
		closeReturned <- lazy.Close()
	}()

	select {
	case err := <-closeReturned:
		t.Fatalf("Close returned while initialization was in flight: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-dbErr:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("initializer did not finish after release")
	}

	select {
	case err := <-closeReturned:
		require.NoError(t, err, "Close must settle and release the warmed connection")
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return after initialization settled")
	}

	assert.True(t, lazy.IsInitialized(), "settled initialization must be recorded")
	db, err := lazy.DB(context.Background())
	require.NoError(t, err)
	require.Error(t, db.Ping(), "settled connection must be closed, not orphaned")
}
