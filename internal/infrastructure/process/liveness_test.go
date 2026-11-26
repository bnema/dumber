package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testBootTime = 1795408658

// writeProcStat writes a fake /proc tree; real /proc cannot hold crafted entries.
func writeProcStat(t *testing.T, root string, pid int, stat string) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o644))
}

func fakeProcRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	stat := "cpu  1 2 3 4\nbtime " + strconv.Itoa(testBootTime) + "\nprocesses 42\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "stat"), []byte(stat), 0o644))
	return root
}

func statLine(comm string, state string, startTicks string) string {
	// pid (comm) state, then fields 4..21 and starttime as field 22.
	return "7537 (" + comm + ") " + state + " 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 " + startTicks + " 23 24\n"
}

func TestLivenessProbe_ProcessStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		stat        string
		wantRunning bool
		wantStart   time.Time
		wantErr     bool
	}{
		{
			name:        "running thread with spaces and parens in name",
			stat:        statLine("Bun Pool (11)", "S", "30650154"),
			wantRunning: true,
			wantStart:   time.Unix(testBootTime, 0).Add(306501540 * time.Millisecond),
		},
		{name: "zombie is not running", stat: statLine("dumber", "Z", "100")},
		{name: "missing command name", stat: "7537 dumber S 1 2", wantErr: true},
		{name: "missing starttime field", stat: "7537 (dumber) S 1 2 3\n", wantErr: true},
		{name: "non-numeric starttime", stat: statLine("dumber", "S", "abc"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := fakeProcRoot(t)
			writeProcStat(t, root, 7537, tt.stat)
			probe := &LivenessProbe{procRoot: root}

			status, err := probe.ProcessStatus(context.Background(), 7537)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRunning, status.Running)
			assert.True(t, tt.wantStart.Equal(status.StartedAt), "start %s, want %s", status.StartedAt, tt.wantStart)
		})
	}
}

func TestLivenessProbe_ProcessStatusMissingPIDIsNotRunning(t *testing.T) {
	t.Parallel()

	probe := &LivenessProbe{procRoot: fakeProcRoot(t)}

	status, err := probe.ProcessStatus(context.Background(), 4242)

	require.NoError(t, err)
	assert.False(t, status.Running)
}

func TestLivenessProbe_ProcessStatusMissingBootTimeErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "stat"), []byte("cpu 1 2 3\n"), 0o644))
	writeProcStat(t, root, 7537, statLine("dumber", "S", "100"))
	probe := &LivenessProbe{procRoot: root}

	_, err := probe.ProcessStatus(context.Background(), 7537)

	require.Error(t, err)
}

func TestLivenessProbe_ProcessStatusRejectsInvalidPIDAndCanceledContext(t *testing.T) {
	t.Parallel()

	probe := &LivenessProbe{procRoot: fakeProcRoot(t)}

	_, err := probe.ProcessStatus(context.Background(), 0)
	require.ErrorContains(t, err, "invalid pid")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = probe.ProcessStatus(ctx, 7537)
	require.ErrorIs(t, err, context.Canceled)
}

// The host /proc tests pin the real kernel format and units against known processes.
func TestLivenessProbe_ProcessStatusHostCurrentProcess(t *testing.T) {
	t.Parallel()

	status, err := NewLivenessProbe().ProcessStatus(context.Background(), os.Getpid())

	require.NoError(t, err)
	assert.True(t, status.Running)
	assert.False(t, status.StartedAt.After(time.Now()), "started in the future: %s", status.StartedAt)
	assert.WithinDuration(t, time.Now(), status.StartedAt, time.Hour)
}

func TestLivenessProbe_ProcessStatusHostExitedProcess(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("true")
	require.NoError(t, cmd.Run())

	status, err := NewLivenessProbe().ProcessStatus(context.Background(), cmd.Process.Pid)

	require.NoError(t, err)
	assert.False(t, status.Running)
}
