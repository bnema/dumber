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

const (
	testBootTime = 1700000000
	testPID      = 4321
	testSelfPID  = 100
)

// writeProcEntry writes a fake /proc/<pid>; real /proc cannot hold crafted entries.
func writeProcEntry(t *testing.T, root string, pid, tgid int, stat string) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o644))
	status := "Name:\tworker\nTgid:\t" + strconv.Itoa(tgid) + "\nPid:\t" + strconv.Itoa(pid) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o644))
}

// linkExe points a fake /proc/<name>/exe at target.
func linkExe(t *testing.T, root, name, target string) {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "exe")))
}

func fakeProcRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	stat := "cpu  1 2 3 4\nbtime " + strconv.Itoa(testBootTime) + "\nprocesses 42\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "stat"), []byte(stat), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, strconv.Itoa(testSelfPID)), 0o755))
	require.NoError(t, os.Symlink(strconv.Itoa(testSelfPID), filepath.Join(root, "self")))
	return root
}

func fakeProbe(root string) *LivenessProbe {
	return &LivenessProbe{procRoot: root, selfPID: testSelfPID}
}

func statLine(comm, state, startTicks string) string {
	// pid (comm) state, then fields 4..21 and starttime as field 22.
	return strconv.Itoa(testPID) + " (" + comm + ") " + state + " 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 " + startTicks + " 23 24\n"
}

func TestLivenessProbe_ProcessStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		stat        string
		tgid        int
		selfExe     string
		holderExe   string
		cmdline     string
		wantRunning bool
		wantStart   time.Time
		wantSameExe bool
		wantErr     bool
	}{
		{
			name:        "process with spaces and parens in name",
			stat:        statLine("worker (1)", "S", "250000"),
			tgid:        testPID,
			wantRunning: true,
			wantStart:   time.Unix(testBootTime, 0).Add(2500 * time.Second),
		},
		{
			name:        "same executable after binary upgrade",
			stat:        statLine("dumber", "S", "100"),
			tgid:        testPID,
			selfExe:     "/usr/bin/dumber",
			holderExe:   "/usr/bin/dumber (deleted)",
			cmdline:     "/usr/bin/dumber\x00browse\x00",
			wantRunning: true,
			wantStart:   time.Unix(testBootTime+1, 0),
			wantSameExe: true,
		},
		{
			name:        "other executable",
			stat:        statLine("worker", "S", "100"),
			tgid:        testPID,
			selfExe:     "/usr/bin/dumber",
			holderExe:   "/usr/bin/worker",
			wantRunning: true,
			wantStart:   time.Unix(testBootTime+1, 0),
		},
		{
			name:        "cef subprocess of same executable",
			stat:        statLine("dumber", "S", "100"),
			tgid:        testPID,
			selfExe:     "/usr/bin/dumber",
			holderExe:   "/usr/bin/dumber",
			cmdline:     "/usr/bin/dumber\x00--type=renderer\x00",
			wantRunning: true,
			wantStart:   time.Unix(testBootTime+1, 0),
		},
		{name: "thread id is not a process", stat: statLine("worker", "S", "100"), tgid: 1000},
		{name: "zombie is not running", stat: statLine("dumber", "Z", "100"), tgid: testPID},
		{name: "missing command name", stat: "4321 dumber S 1 2", wantErr: true},
		{name: "missing starttime field", stat: "4321 (dumber) S 1 2 3\n", wantErr: true},
		{name: "non-numeric starttime", stat: statLine("dumber", "S", "abc"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := fakeProcRoot(t)
			writeProcEntry(t, root, testPID, tt.tgid, tt.stat)
			if tt.selfExe != "" {
				linkExe(t, root, "self", tt.selfExe)
				linkExe(t, root, strconv.Itoa(testPID), tt.holderExe)
			}
			if tt.cmdline != "" {
				cmdlinePath := filepath.Join(root, strconv.Itoa(testPID), "cmdline")
				require.NoError(t, os.WriteFile(cmdlinePath, []byte(tt.cmdline), 0o644))
			}
			probe := fakeProbe(root)

			status, err := probe.ProcessStatus(context.Background(), testPID)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRunning, status.Running)
			assert.True(t, tt.wantStart.Equal(status.StartedAt), "start %s, want %s", status.StartedAt, tt.wantStart)
			assert.Equal(t, tt.wantSameExe, status.SameExecutable)
		})
	}
}

func TestLivenessProbe_ProcessStatusMissingPIDIsNotRunning(t *testing.T) {
	t.Parallel()

	probe := fakeProbe(fakeProcRoot(t))

	status, err := probe.ProcessStatus(context.Background(), 4242)

	require.NoError(t, err)
	assert.False(t, status.Running)
}

func TestLivenessProbe_ProcessStatusMissingBootTimeErrors(t *testing.T) {
	t.Parallel()

	root := fakeProcRoot(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "stat"), []byte("cpu 1 2 3\n"), 0o644))
	writeProcEntry(t, root, testPID, testPID, statLine("dumber", "S", "100"))
	probe := fakeProbe(root)

	_, err := probe.ProcessStatus(context.Background(), testPID)

	require.ErrorContains(t, err, "boot time not found")
}

func TestLivenessProbe_ProcessStatusRejectsInvalidPIDAndCanceledContext(t *testing.T) {
	t.Parallel()

	probe := fakeProbe(fakeProcRoot(t))

	_, err := probe.ProcessStatus(context.Background(), 0)
	require.ErrorContains(t, err, "invalid pid")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = probe.ProcessStatus(ctx, testPID)
	require.ErrorIs(t, err, context.Canceled)
}

// The host /proc tests pin the real kernel format and units against known processes.
func TestLivenessProbe_ProcessStatusHostCurrentProcess(t *testing.T) {
	t.Parallel()

	status, err := NewLivenessProbe().ProcessStatus(context.Background(), os.Getpid())

	require.NoError(t, err)
	assert.True(t, status.Running)
	assert.True(t, status.SameExecutable)
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

func TestLivenessProbe_ProcessStatusHostThreadIsNotProcess(t *testing.T) {
	t.Parallel()

	tasks, err := os.ReadDir("/proc/self/task")
	require.NoError(t, err)
	tid := 0
	for _, task := range tasks {
		id, convErr := strconv.Atoi(task.Name())
		if convErr == nil && id != os.Getpid() {
			tid = id
			break
		}
	}
	require.NotZero(t, tid, "go test process has more than one thread")

	status, err := NewLivenessProbe().ProcessStatus(context.Background(), tid)

	require.NoError(t, err)
	assert.False(t, status.Running)
}

func TestLivenessProbe_ProcessStatusRejectsForeignPIDNamespace(t *testing.T) {
	t.Parallel()

	root := fakeProcRoot(t)
	writeProcEntry(t, root, testPID, testPID, statLine("dumber", "S", "100"))
	probe := &LivenessProbe{procRoot: root, selfPID: testSelfPID + 1}

	_, err := probe.ProcessStatus(context.Background(), testPID)

	require.ErrorContains(t, err, "another pid namespace")
}
