package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
)

const (
	leaseDirPerm    = 0o700
	leaseFilePerm   = 0o600
	leaseFileSuffix = ".lock"
	// A prober holds a free lease for microseconds; retrying outlasts it.
	leaseAcquireAttempts = 20
	leaseAcquireRetry    = 5 * time.Millisecond
)

// FileSessionLeases keeps each session's lease as an flock on <dir>/<session id>.lock.
// flock locks belong to the open file, so the kernel drops them when the owner exits,
// and they work across PID namespaces that share the directory.
type FileSessionLeases struct {
	dir string
	// waitRetry pauses between lock attempts; tests replace it to stay deterministic.
	waitRetry func(context.Context) error
}

// NewFileSessionLeases stores session leases in dir.
func NewFileSessionLeases(dir string) *FileSessionLeases {
	return &FileSessionLeases{dir: dir, waitRetry: waitLeaseRetry}
}

func waitLeaseRetry(ctx context.Context) error {
	timer := time.NewTimer(leaseAcquireRetry)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type sessionLease struct {
	file *os.File
}

// Close releases the lock and keeps the file; Remove deletes it once the session has ended.
func (l *sessionLease) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// Acquire creates and locks the session's lease file.
func (s *FileSessionLeases) Acquire(ctx context.Context, id entity.SessionID) (io.Closer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.leasePath(id)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(s.dir, leaseDirPerm); err != nil {
		return nil, fmt.Errorf("create session lease directory: %w", err)
	}
	for attempt := 1; ; attempt++ {
		file, locked, err := lockLeaseFile(path)
		if err == nil && locked {
			return &sessionLease{file: file}, nil
		}
		// A file we could not lock would read as free while this session runs:
		// remove it so the session reads as missing and falls back to process probing.
		if err != nil {
			_ = os.Remove(path)
			return nil, fmt.Errorf("lock session lease: %w", err)
		}
		// A prober holds a new session's lease for microseconds. A lock still held
		// after every attempt belongs to a live owner of a colliding session ID:
		// leave its file alone.
		if attempt == leaseAcquireAttempts {
			return nil, errors.New("lock session lease: held by another process")
		}
		if err := s.waitRetry(ctx); err != nil {
			return nil, err
		}
	}
}

// lockLeaseFile opens or creates path and locks it without blocking. It reports
// locked=false when another process holds the lock, or when the locked file was
// unlinked or replaced meanwhile, so the caller retries on the current file.
func lockLeaseFile(path string) (*os.File, bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, leaseFilePerm)
	if err != nil {
		return nil, false, fmt.Errorf("open session lease: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !isCurrentFile(file, path) {
		_ = file.Close()
		return nil, false, nil
	}
	return file, true, nil
}

// isCurrentFile reports whether file is still the one at path, not an unlinked inode.
func isCurrentFile(file *os.File, path string) bool {
	opened, err := file.Stat()
	if err != nil {
		return false
	}
	current, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return os.SameFile(opened, current)
}

// Probe tries the session's lock without blocking: a held lock means a live owner.
func (s *FileSessionLeases) Probe(ctx context.Context, id entity.SessionID) (port.SessionLeaseState, error) {
	if err := ctx.Err(); err != nil {
		return port.SessionLeaseMissing, err
	}
	path, err := s.leasePath(id)
	if err != nil {
		return port.SessionLeaseMissing, err
	}
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return port.SessionLeaseMissing, nil
	}
	if err != nil {
		return port.SessionLeaseMissing, fmt.Errorf("open session lease: %w", err)
	}
	defer func() { _ = file.Close() }()

	err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return port.SessionLeaseHeld, nil
	}
	if err != nil {
		return port.SessionLeaseMissing, fmt.Errorf("probe session lease: %w", err)
	}
	return port.SessionLeaseFree, nil
}

// RemoveIfFree deletes the session's lease file while holding its lock, so a live
// owner's lease is never removed. It reports whether the file was removed.
func (s *FileSessionLeases) RemoveIfFree(ctx context.Context, id entity.SessionID) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	path, err := s.leasePath(id)
	if err != nil {
		return false, err
	}
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open session lease: %w", err)
	}
	defer func() { _ = file.Close() }()

	err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock session lease: %w", err)
	}
	if !isCurrentFile(file, path) {
		return false, nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("remove session lease: %w", err)
	}
	return true, nil
}

// Remove deletes the lease file of an ended session.
func (s *FileSessionLeases) Remove(ctx context.Context, id entity.SessionID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.leasePath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove session lease: %w", err)
	}
	return nil
}

// List returns the session IDs of the lease files in the directory.
func (s *FileSessionLeases) List(ctx context.Context) ([]entity.SessionID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list session leases: %w", err)
	}
	ids := make([]entity.SessionID, 0, len(entries))
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), leaseFileSuffix)
		if !ok || !entry.Type().IsRegular() {
			continue
		}
		id := entity.SessionID(name)
		if _, err := s.leasePath(id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *FileSessionLeases) leasePath(id entity.SessionID) (string, error) {
	name := string(id)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return "", fmt.Errorf("invalid session id %q", name)
	}
	return filepath.Join(s.dir, name+leaseFileSuffix), nil
}
