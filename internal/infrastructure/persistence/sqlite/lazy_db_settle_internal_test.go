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

// TestLazyDB_CloseSettlesAdmittedInit holds an admitted initialization open
// with initGateForTest and proves Close seals admission, settles the
// in-flight work, and leaves no usable connection behind: after Close,
// every DB call fails instead of escaping with an orphaned pool.
func TestLazyDB_CloseSettlesAdmittedInit(t *testing.T) {
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
		t.Fatal("admitted initializer did not start")
	}

	closeReturned := make(chan error, 1)
	go func() {
		closeReturned <- lazy.Close()
	}()

	select {
	case err := <-closeReturned:
		t.Fatalf("Close returned while admitted initialization was in flight: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-dbErr:
		require.Error(t, err, "admitted init racing Close must not yield a usable connection")
	case <-time.After(10 * time.Second):
		t.Fatal("initializer did not finish after release")
	}

	select {
	case err := <-closeReturned:
		require.NoError(t, err, "Close must settle admitted init and release its connection")
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return after initialization settled")
	}

	_, err := lazy.DB(context.Background())
	require.Error(t, err, "no usable connection may escape Close")

	require.NoError(t, lazy.Close(), "repeat Close must be idempotent")
}

// TestLazyDB_CloseRejectsLateAdmission proves the admission/closure
// serialization: a DB call arriving after Close began never opens anything
// and receives the closed-provider error.
func TestLazyDB_CloseRejectsLateAdmission(t *testing.T) {
	entered := make(chan struct{}, 1)
	initGateForTest = func() { close(entered) }
	defer func() { initGateForTest = nil }()

	lazy := NewLazyDB(filepath.Join(t.TempDir(), "test.db"))

	require.NoError(t, lazy.Close())

	_, err := lazy.DB(context.Background())
	require.ErrorIs(t, err, ErrLazyDBClosed)

	select {
	case <-entered:
		t.Fatal("initialization must not be admitted after Close")
	default:
	}
	assert.False(t, lazy.IsInitialized())
}
