package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLivenessProbe_IsProcessAliveRejectsInvalidPID(t *testing.T) {
	t.Parallel()

	probe := &LivenessProbe{signal: func(_ int, _ syscall.Signal) error {
		t.Fatal("signal should not be called for invalid pid")
		return nil
	}}

	alive, err := probe.IsProcessAlive(context.Background(), 0)

	assert.False(t, alive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pid")
}

func TestLivenessProbe_IsProcessAliveReturnsContextErrorBeforeSignaling(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	probe := &LivenessProbe{signal: func(_ int, _ syscall.Signal) error {
		t.Fatal("signal should not be called when context is canceled")
		return nil
	}}

	alive, err := probe.IsProcessAlive(ctx, 1234)

	assert.False(t, alive)
	require.ErrorIs(t, err, context.Canceled)
}

func TestLivenessProbe_IsProcessAliveInterpretsSignalErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		signalErr error
		wantAlive bool
		wantErr   error
	}{
		{name: "signal succeeds", wantAlive: true},
		{name: "process missing", signalErr: syscall.ESRCH, wantAlive: false},
		{name: "permission denied means process exists", signalErr: syscall.EPERM, wantAlive: true},
		{name: "unexpected signal error propagates", signalErr: errors.New("signal failed"), wantAlive: false, wantErr: errors.New("signal failed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			probe := &LivenessProbe{signal: func(pid int, signal syscall.Signal) error {
				assert.Equal(t, 1234, pid)
				assert.Equal(t, syscall.Signal(0), signal)
				return tt.signalErr
			}}

			alive, err := probe.IsProcessAlive(context.Background(), 1234)

			assert.Equal(t, tt.wantAlive, alive)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestLivenessProbe_IsProcessAliveReportsExitedProcessAsDead(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("true")
	require.NoError(t, cmd.Run())

	alive, err := NewLivenessProbe().IsProcessAlive(context.Background(), cmd.Process.Pid)

	require.NoError(t, err)
	assert.False(t, alive)
}

func TestLivenessProbe_IsProcessAliveReportsCurrentProcessAsAlive(t *testing.T) {
	t.Parallel()

	alive, err := NewLivenessProbe().IsProcessAlive(context.Background(), os.Getpid())

	require.NoError(t, err)
	assert.True(t, alive)
}
