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
