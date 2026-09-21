package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
)

const (
	instanceRuntimeDirPerm = 0o700
	instanceLockFilePerm   = 0o600
	instanceLockRetry      = 25 * time.Millisecond
)

type namespaceLease struct{ file *os.File }

func (l *namespaceLease) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil
	return errors.Join(err, closeErr)
}

func acquireInstanceNamespace(ctx context.Context, profile runtimeprofile.Profile) (io.Closer, error) {
	if err := os.MkdirAll(profile.InstanceRoot, instanceRuntimeDirPerm); err != nil {
		return nil, fmt.Errorf("create named instance runtime directory: %w", err)
	}
	path := filepath.Join(profile.InstanceRoot, "namespace.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, instanceLockFilePerm)
	if err != nil {
		return nil, fmt.Errorf("open named instance namespace lock: %w", err)
	}
	for {
		if err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
			return &namespaceLease{file: file}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			_ = file.Close()
			return nil, fmt.Errorf("lock named instance namespace: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, fmt.Errorf("wait for named instance namespace: %w", ctx.Err())
		case <-time.After(instanceLockRetry):
		}
	}
}

// RunWithInstanceRelay owns the namespace lease through full GUI and engine teardown.
func RunWithInstanceRelay(ctx context.Context, profile runtimeprofile.Profile, relay port.BrowserLaunchRelay, run func() int) (int, error) {
	preparer, ok := relay.(port.BrowserLaunchRelayPreparer)
	if !ok {
		return 1, fmt.Errorf("named instance requires a preparable browser launch relay")
	}
	lease, err := acquireInstanceNamespace(ctx, profile)
	if err != nil {
		return 1, err
	}
	defer func() { _ = lease.Close() }()
	if err := preparer.Prepare(lease); err != nil {
		return 1, fmt.Errorf(
			"prepare named instance IPC; remove %q only after verifying no Dumber instance is running: %w",
			profile.IPC.BrowserLaunchSocket,
			err,
		)
	}
	return run(), nil
}
