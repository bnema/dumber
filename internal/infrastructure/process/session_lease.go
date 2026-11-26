package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
)

const (
	leaseDirPerm  = 0o700
	leaseFilePerm = 0o600
)

// FileSessionLeases keeps each session's lease as an flock on <dir>/<session id>.lock.
// flock locks belong to the open file, so the kernel drops them when the owner exits,
// and they work across PID namespaces that share the directory.
type FileSessionLeases struct {
	dir string
}

// NewFileSessionLeases stores session leases in dir.
func NewFileSessionLeases(dir string) *FileSessionLeases {
	return &FileSessionLeases{dir: dir}
}

type sessionLease struct {
	file *os.File
	path string
}

// Close removes the lease file, then releases the lock.
func (l *sessionLease) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	removeErr := os.Remove(l.path)
	if errors.Is(removeErr, fs.ErrNotExist) {
		removeErr = nil
	}
	closeErr := l.file.Close()
	l.file = nil
	return errors.Join(removeErr, closeErr)
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
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, leaseFilePerm)
	if err != nil {
		return nil, fmt.Errorf("open session lease: %w", err)
	}
	if err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock session lease: %w", err)
	}
	return &sessionLease{file: file, path: path}, nil
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
	file, err := os.OpenFile(path, os.O_RDWR, 0)
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

func (s *FileSessionLeases) leasePath(id entity.SessionID) (string, error) {
	name := string(id)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return "", fmt.Errorf("invalid session id %q", name)
	}
	return filepath.Join(s.dir, name+".lock"), nil
}
