package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
)

const testSessionID entity.SessionID = "20251222_100000_abcd"

// flock conflicts between open file descriptions even in one process, so an
// in-process lease stands in for another instance holding it.
//
// These tests do not run in parallel: a parallel test that forks (exec.Command)
// briefly inherits open lease fds until exec, which would make a just-released
// lock look held.
func TestFileSessionLeases_HeldThenFreeAfterCloseThenMissingAfterRemove(t *testing.T) {
	ctx := context.Background()
	leases := NewFileSessionLeases(filepath.Join(t.TempDir(), "leases"))

	lease, err := leases.Acquire(ctx, testSessionID)
	require.NoError(t, err)
	state, err := leases.Probe(ctx, testSessionID)
	require.NoError(t, err)
	assert.Equal(t, port.SessionLeaseHeld, state)

	require.NoError(t, lease.Close())
	state, err = leases.Probe(ctx, testSessionID)
	require.NoError(t, err)
	assert.Equal(t, port.SessionLeaseFree, state, "close releases the lock and keeps the file")

	require.NoError(t, leases.Remove(ctx, testSessionID))
	state, err = leases.Probe(ctx, testSessionID)
	require.NoError(t, err)
	assert.Equal(t, port.SessionLeaseMissing, state)
}

func TestFileSessionLeases_CrashedOwnerLeavesFreeLease(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	leases := NewFileSessionLeases(dir)

	// A crash drops the lock but leaves the file, like closing the fd without Close.
	file, err := os.OpenFile(filepath.Join(dir, string(testSessionID)+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	require.NoError(t, err)
	require.NoError(t, unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB))
	require.NoError(t, file.Close())

	state, err := leases.Probe(ctx, testSessionID)
	require.NoError(t, err)
	assert.Equal(t, port.SessionLeaseFree, state)

	require.NoError(t, leases.Remove(ctx, testSessionID))
	require.NoError(t, leases.Remove(ctx, testSessionID), "remove is idempotent")
	state, err = leases.Probe(ctx, testSessionID)
	require.NoError(t, err)
	assert.Equal(t, port.SessionLeaseMissing, state)
}

func TestFileSessionLeases_ProbeDoesNotKeepLease(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	leases := NewFileSessionLeases(dir)
	// A free, read-only lease file: probing must not need write access nor keep the lock.
	require.NoError(t, os.WriteFile(filepath.Join(dir, string(testSessionID)+".lock"), nil, 0o400))
	state, err := leases.Probe(ctx, testSessionID)
	require.NoError(t, err)
	assert.Equal(t, port.SessionLeaseFree, state)
	require.NoError(t, os.Chmod(filepath.Join(dir, string(testSessionID)+".lock"), 0o600))
	lease, err := leases.Acquire(ctx, testSessionID)
	require.NoError(t, err)
	require.NoError(t, lease.Close())
}

func TestFileSessionLeases_RejectsPathLikeSessionIDs(t *testing.T) {
	ctx := context.Background()
	leases := NewFileSessionLeases(t.TempDir())

	for _, id := range []entity.SessionID{"", ".", "..", "../escape", "a/b"} {
		_, err := leases.Acquire(ctx, id)
		require.Error(t, err, id)
		_, err = leases.Probe(ctx, id)
		require.Error(t, err, id)
		require.Error(t, leases.Remove(ctx, id), id)
	}
}

func TestFileSessionLeases_ListReturnsLeaseFilesOnly(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "leases")
	leases := NewFileSessionLeases(dir)

	ids, err := leases.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, ids, "a missing directory has no leases")

	held, err := leases.Acquire(ctx, testSessionID)
	require.NoError(t, err)
	t.Cleanup(func() { _ = held.Close() })
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20251222_110000_free.lock"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dir.lock"), 0o700))

	ids, err = leases.List(ctx)

	require.NoError(t, err)
	assert.ElementsMatch(t, []entity.SessionID{testSessionID, "20251222_110000_free"}, ids)
}

// holdLease locks the lease file through another open file description, as a
// prober or another instance would.
func holdLease(t *testing.T, dir string) *os.File {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
	file, err := os.OpenFile(filepath.Join(dir, string(testSessionID)+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	require.NoError(t, err)
	require.NoError(t, unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB))
	return file
}

func TestFileSessionLeases_AcquireOutlastsABriefProbe(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	leases := NewFileSessionLeases(dir)
	prober := holdLease(t, dir)
	waits := 0
	leases.waitRetry = func(context.Context) error {
		waits++
		return prober.Close() // the prober finishes while Acquire waits
	}

	lease, err := leases.Acquire(ctx, testSessionID)

	require.NoError(t, err)
	assert.Equal(t, 1, waits)
	t.Cleanup(func() { _ = lease.Close() })
	state, err := leases.Probe(ctx, testSessionID)
	require.NoError(t, err)
	assert.Equal(t, port.SessionLeaseHeld, state)
}

func TestFileSessionLeases_AcquireFailureLeavesNoFreeLease(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	leases := NewFileSessionLeases(dir)
	holder := holdLease(t, dir)
	t.Cleanup(func() { _ = holder.Close() })
	waits := 0
	leases.waitRetry = func(context.Context) error {
		waits++
		return nil
	}

	_, err := leases.Acquire(ctx, testSessionID)

	require.Error(t, err)
	assert.Equal(t, leaseAcquireAttempts-1, waits)
	assert.NoFileExists(t, filepath.Join(dir, string(testSessionID)+".lock"), "a failed acquire must read as missing, not free")
}

func TestFileSessionLeases_RemoveIfFree(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	leases := NewFileSessionLeases(dir)
	path := filepath.Join(dir, string(testSessionID)+".lock")

	removed, err := leases.RemoveIfFree(ctx, testSessionID)
	require.NoError(t, err)
	assert.False(t, removed, "missing lease")

	lease, err := leases.Acquire(ctx, testSessionID)
	require.NoError(t, err)
	removed, err = leases.RemoveIfFree(ctx, testSessionID)
	require.NoError(t, err)
	assert.False(t, removed, "held lease is kept")
	assert.FileExists(t, path)

	require.NoError(t, lease.Close())
	removed, err = leases.RemoveIfFree(ctx, testSessionID)
	require.NoError(t, err)
	assert.True(t, removed)
	assert.NoFileExists(t, path)
}
