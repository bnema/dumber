package process

import (
	"context"
	"errors"
	"fmt"
	"syscall"
)

type processSignalFunc func(pid int, signal syscall.Signal) error

// LivenessProbe checks whether a process is alive by sending signal 0.
type LivenessProbe struct {
	signal processSignalFunc
}

// NewLivenessProbe creates a LivenessProbe that uses kill(2) with signal 0.
func NewLivenessProbe() *LivenessProbe {
	return &LivenessProbe{signal: signalProcess}
}

// IsProcessAlive checks whether the process with the given PID is alive.
// It returns immediately with the context error when ctx is done, and returns
// an error for invalid PIDs. Signal errors are mapped for cleanup safety:
// ESRCH means the process is not alive, EPERM means the process exists but is
// not signalable by this user, and unexpected signal errors are propagated.
// It calls kill(2) directly: os.Process.Signal reports a missing PID as
// os.ErrProcessDone and, with pidfd, holds an fd until GC.
func (p *LivenessProbe) IsProcessAlive(ctx context.Context, pid int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if pid <= 0 {
		return false, fmt.Errorf("invalid pid %d", pid)
	}
	signal := p.signal
	if signal == nil {
		signal = signalProcess
	}
	if err := signal(pid, 0); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return false, nil
		}
		if errors.Is(err, syscall.EPERM) {
			return true, nil
		}
		return false, err
	}
	return true, nil
}

func signalProcess(pid int, signal syscall.Signal) error {
	return syscall.Kill(pid, signal)
}
