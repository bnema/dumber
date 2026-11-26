package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bnema.dev/dumber/internal/application/port"
	portmocks "bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/infrastructure/process"
	corelogging "bnema.dev/dumber/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// deadPID is above the Linux pid_max limit (4194304), so no process has it.
const deadPID = 99999999

// hostChecker probes the real /proc without session leases.
func hostChecker() previousProcessChecker {
	return previousProcessChecker{probe: process.NewLivenessProbe()}
}

func TestParsePIDFile(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantPID     int
		wantSession string
		wantErr     bool
	}{
		{name: "v1 one line", raw: "123\n", wantPID: 123},
		{name: "v2 two lines", raw: "123\nabc-session\n", wantPID: 123, wantSession: "abc-session"},
		{name: "unsafe session ignored", raw: "123\n../../etc\n", wantPID: 123},
		{name: "zero pid", raw: "0\nabc\n", wantErr: true},
		{name: "negative pid", raw: "-5\n", wantErr: true},
		{name: "garbage", raw: "not-a-pid\nabc\n", wantErr: true},
		{name: "empty", raw: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pid, sessionID, err := parsePIDFile(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPID, pid)
			assert.Equal(t, tt.wantSession, sessionID)
		})
	}
}

func writeSessionLog(t *testing.T, logDir, sessionID string, mod time.Time) {
	t.Helper()
	p := filepath.Join(logDir, corelogging.SessionFilename(sessionID))
	require.NoError(t, os.WriteFile(p, []byte(`{"level":"info"}`+"\n"), filePerm))
	require.NoError(t, os.Chtimes(p, mod, mod))
}

func TestDetectPreviousCrash_UsesPIDFileSession(t *testing.T) {
	logDir := t.TempDir()
	now := time.Now()
	writeSessionLog(t, logDir, "crashed", now.Add(-time.Hour))
	writeSessionLog(t, logDir, "other-newer", now.Add(-time.Minute))
	writeSessionLog(t, logDir, "current", now)
	require.NoError(t, os.WriteFile(pidFilePath(logDir), []byte(fmt.Sprintf("%d\ncrashed\n", deadPID)), filePerm))

	reports := detectPreviousCrash(logDir, "current", hostChecker(), nil)

	require.Len(t, reports, 1)
	assert.Equal(t, "session_crashed.crash.json", filepath.Base(reports[0]))
	assert.NoFileExists(t, pidFilePath(logDir))
}

func TestDetectPreviousCrash_V1FallbackExcludesCurrentSession(t *testing.T) {
	logDir := t.TempDir()
	now := time.Now()
	writeSessionLog(t, logDir, "previous", now.Add(-time.Hour))
	writeSessionLog(t, logDir, "current", now)
	require.NoError(t, os.WriteFile(pidFilePath(logDir), []byte(fmt.Sprintf("%d\n", deadPID)), filePerm))

	reports := detectPreviousCrash(logDir, "current", hostChecker(), nil)

	require.Len(t, reports, 1)
	assert.Equal(t, "session_previous.crash.json", filepath.Base(reports[0]))
}

func TestFindLastSessionLog_ExcludesSession(t *testing.T) {
	logDir := t.TempDir()
	now := time.Now()
	writeSessionLog(t, logDir, "older", now.Add(-time.Hour))
	writeSessionLog(t, logDir, "current", now)

	assert.Equal(t, "session_older.log", filepath.Base(findLastSessionLog(logDir, "current")))
	assert.Equal(t, "session_current.log", filepath.Base(findLastSessionLog(logDir, "")))
}

func TestDetectPreviousCrash_OwnPIDCountsAsDead(t *testing.T) {
	logDir := t.TempDir()
	writeSessionLog(t, logDir, "reused", time.Now().Add(-time.Hour))
	require.NoError(t, os.WriteFile(pidFilePath(logDir), []byte(fmt.Sprintf("%d\nreused\n", os.Getpid())), filePerm))

	reports := detectPreviousCrash(logDir, "current", hostChecker(), nil)

	require.Len(t, reports, 1)
	assert.Equal(t, "session_reused.crash.json", filepath.Base(reports[0]))
}

func TestSweepFatalOutputs(t *testing.T) {
	logDir := t.TempDir()
	now := time.Now()
	writeSessionLog(t, logDir, "startup-crash", now.Add(-time.Hour))
	writeSessionLog(t, logDir, "current", now)
	self := os.Getpid()

	orphan := FatalOutputPath(logDir, deadPID)
	empty := FatalOutputPath(logDir, deadPID-1)
	own := FatalOutputPath(logDir, self)
	rotated := rotatedFatalOutputPath(logDir, self, 1700000000)
	writeFatalFile(t, orphan, "panic: startup\n")
	writeFatalFile(t, empty, "")
	writeFatalFile(t, own, "")
	writeFatalFile(t, rotated, "panic: reused pid\n")

	reports := sweepFatalOutputs(logDir, self, "current", hostChecker(), nil)

	names := make([]string, 0, len(reports))
	for _, r := range reports {
		names = append(names, filepath.Base(r))
	}
	// The first orphan (glob order) takes the newest other session; the
	// second falls back to a pid_<pid>[_<mtime>] ID.
	require.Len(t, names, 2)
	assert.Contains(t, names, "session_startup-crash.crash.json")
	assert.Condition(t, func() bool {
		for _, n := range names {
			if n == fmt.Sprintf("session_pid_%d.crash.json", deadPID) ||
				n == fmt.Sprintf("session_pid_%d_1700000000.crash.json", self) {
				return true
			}
		}
		return false
	}, "one report uses the pid fallback ID: %v", names)
	assert.NoFileExists(t, orphan)
	assert.NoFileExists(t, empty)
	assert.NoFileExists(t, rotated)
	assert.FileExists(t, own, "the current process's own file is left alone")
	assert.FileExists(t, filepath.Join(logDir, "crashes", "session_startup-crash.fatal.txt"))
}

func TestPreviousProcessChecker_Alive(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		lease     *port.SessionLeaseState // nil: lease not probed
		leaseErr  error
		status    *port.SessionProcessStatus // nil: pid not probed
		probeErr  error
		want      bool
	}{
		{name: "held lease is alive", sessionID: "s", lease: leaseState(port.SessionLeaseHeld), want: true},
		{name: "free lease is dead", sessionID: "s", lease: leaseState(port.SessionLeaseFree), want: false},
		{
			name: "missing lease probes a dumber main process", sessionID: "s", lease: leaseState(port.SessionLeaseMissing),
			status: &port.SessionProcessStatus{Running: true, SameExecutable: true}, want: true,
		},
		{
			name: "lease error probes the pid", sessionID: "s", lease: leaseState(port.SessionLeaseMissing), leaseErr: errors.New("io"),
			status: &port.SessionProcessStatus{}, want: false,
		},
		{name: "no session id: pid held by another program is reused", status: &port.SessionProcessStatus{Running: true}, want: false},
		{name: "no session id: free pid is dead", status: &port.SessionProcessStatus{}, want: false},
		{name: "probe error counts as alive", status: &port.SessionProcessStatus{}, probeErr: errors.New("foreign /proc"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			leases := portmocks.NewMockSessionLeaseStore(t)
			probe := portmocks.NewMockSessionProcessProbe(t)
			if tt.lease != nil {
				leases.EXPECT().Probe(mock.Anything, entity.SessionID(tt.sessionID)).Return(*tt.lease, tt.leaseErr)
			}
			if tt.status != nil {
				probe.EXPECT().ProcessStatus(mock.Anything, 4321).Return(*tt.status, tt.probeErr)
			}
			checker := previousProcessChecker{leases: leases, probe: probe}

			assert.Equal(t, tt.want, checker.alive(context.Background(), 4321, tt.sessionID))
		})
	}
}

func leaseState(state port.SessionLeaseState) *port.SessionLeaseState { return &state }

func TestDetectPreviousCrash_HeldLeaseKeepsPIDFile(t *testing.T) {
	logDir := t.TempDir()
	writeSessionLog(t, logDir, "running", time.Now().Add(-time.Hour))
	require.NoError(t, os.WriteFile(pidFilePath(logDir), []byte(fmt.Sprintf("%d\nrunning\n", deadPID)), filePerm))
	leases := portmocks.NewMockSessionLeaseStore(t)
	leases.EXPECT().Probe(mock.Anything, entity.SessionID("running")).Return(port.SessionLeaseHeld, nil)

	reports := detectPreviousCrash(logDir, "current", previousProcessChecker{leases: leases}, nil)

	assert.Empty(t, reports)
	assert.FileExists(t, pidFilePath(logDir))
}

func TestDetectPreviousCrash_FreeLeaseReportsCrashOfLiveLookingPID(t *testing.T) {
	logDir := t.TempDir()
	writeSessionLog(t, logDir, "crashed", time.Now().Add(-time.Hour))
	// Our parent holds this PID, as a reused PID would; the free lease decides.
	require.NoError(t, os.WriteFile(pidFilePath(logDir), []byte(fmt.Sprintf("%d\ncrashed\n", os.Getppid())), filePerm))
	leases := portmocks.NewMockSessionLeaseStore(t)
	leases.EXPECT().Probe(mock.Anything, entity.SessionID("crashed")).Return(port.SessionLeaseFree, nil)

	reports := detectPreviousCrash(logDir, "current", previousProcessChecker{leases: leases}, nil)

	require.Len(t, reports, 1)
	assert.Equal(t, "session_crashed.crash.json", filepath.Base(reports[0]))
	assert.NoFileExists(t, pidFilePath(logDir))
}
