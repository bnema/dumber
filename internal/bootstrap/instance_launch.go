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
	instanceElectionSlice  = 150 * time.Millisecond
)

// errInstanceNamespaceContended marks a lease acquisition that failed because
// another process holds the namespace lock. It is the only acquisition failure
// callers may retry; permanent failures (mkdir/open/flock) must surface at once.
var errInstanceNamespaceContended = errors.New("named instance namespace contended")

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
			return nil, fmt.Errorf("wait for named instance namespace: %w: %w", errInstanceNamespaceContended, ctx.Err())
		case <-time.After(instanceLockRetry):
		}
	}
}

// DeliverOrRunWithInstanceRelay retries delivery while another process owns the
// namespace, and only the process that acquires the lease starts the host.
func DeliverOrRunWithInstanceRelay(
	ctx context.Context,
	profile runtimeprofile.Profile,
	relay port.BrowserLaunchRelay,
	name, url string,
	run func() int,
) (int, error) {
	instanceRelay, ok := relay.(port.BrowserInstanceLaunchRelay)
	if !ok {
		return 1, fmt.Errorf("named instance requires an instance launch relay")
	}
	// Normalize before any acquisition attempt so a cold default profile locks
	// the shared IPC runtime directory instead of retrying a permanent mkdir/open
	// failure on an empty path.
	if profile.InstanceRoot == "" {
		profile.InstanceRoot = profile.IPC.RuntimeDir
	}
	for {
		deliveryCtx, cancel := context.WithTimeout(ctx, instanceElectionSlice)
		delivered, err := instanceRelay.DeliverOpenInstance(deliveryCtx, name, url)
		cancel()
		if delivered {
			// An unconfirmed ACK still means ownership may have transferred; retrying
			// the same request would rely on relay request-ID deduplication.
			return 0, err
		}

		acquireCtx, cancel := context.WithTimeout(ctx, instanceElectionSlice)
		lease, acquireErr := acquireInstanceNamespace(acquireCtx, profile)
		cancel()
		if acquireErr == nil {
			return runWithInstanceLease(profile, relay, lease, run)
		}
		if !errors.Is(acquireErr, errInstanceNamespaceContended) {
			// Permanent failure (mkdir, open, unexpected flock error): retrying
			// cannot succeed, so surface it instead of spinning.
			return 1, fmt.Errorf("elect named instance host: %w", acquireErr)
		}
		if ctx.Err() != nil {
			return 1, fmt.Errorf("elect named instance host: %w", ctx.Err())
		}
	}
}

// RunWithInstanceRelay owns the namespace lease through full GUI and engine teardown.
func RunWithInstanceRelay(ctx context.Context, profile runtimeprofile.Profile, relay port.BrowserLaunchRelay, run func() int) (int, error) {
	lease, err := acquireInstanceNamespace(ctx, profile)
	if err != nil {
		return 1, err
	}
	return runWithInstanceLease(profile, relay, lease, run)
}

func runWithInstanceLease(profile runtimeprofile.Profile, relay port.BrowserLaunchRelay, lease io.Closer, run func() int) (int, error) {
	preparer, ok := relay.(port.BrowserLaunchRelayPreparer)
	if !ok {
		return 1, fmt.Errorf("named instance requires a preparable browser launch relay")
	}
	if profile.InstanceRoot == "" {
		profile.InstanceRoot = profile.IPC.RuntimeDir
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
