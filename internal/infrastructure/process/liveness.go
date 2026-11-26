package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"bnema.dev/dumber/internal/application/port"
)

// userHZ is the unit of /proc time fields (USER_HZ). It is 100 on x86 and arm64,
// independent of the kernel HZ.
const userHZ = 100

// statStartTimeIndex is the position of starttime (field 22) counted from the state field (field 3).
const statStartTimeIndex = 22 - 3

// subprocessFlag marks a Chromium/CEF subprocess (renderer, GPU, zygote...) that re-runs
// the main binary.
const subprocessFlag = "--type="

// deletedExeSuffix marks /proc/<pid>/exe when the binary was replaced, e.g. by a package upgrade.
const deletedExeSuffix = " (deleted)"

// LivenessProbe reads /proc to tell whether a PID is held, by what, and since when.
type LivenessProbe struct {
	procRoot string
	selfPID  int
}

// NewLivenessProbe creates a LivenessProbe that reads the host /proc.
func NewLivenessProbe() *LivenessProbe {
	return &LivenessProbe{procRoot: "/proc", selfPID: os.Getpid()}
}

// ProcessStatus reports whether a live process holds pid, when it started, and whether
// it runs the same binary as the caller. A missing PID, a thread ID or a zombie is not
// running. Unreadable or malformed /proc data is an error so callers can keep the
// session rather than end it on doubt.
func (p *LivenessProbe) ProcessStatus(ctx context.Context, pid int) (port.SessionProcessStatus, error) {
	if err := ctx.Err(); err != nil {
		return port.SessionProcessStatus{}, err
	}
	if pid <= 0 {
		return port.SessionProcessStatus{}, fmt.Errorf("invalid pid %d", pid)
	}
	if err := p.checkOwnPIDNamespace(); err != nil {
		return port.SessionProcessStatus{}, err
	}

	stat, err := os.ReadFile(filepath.Join(p.procRoot, strconv.Itoa(pid), "stat"))
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
		return port.SessionProcessStatus{}, nil
	}
	if err != nil {
		return port.SessionProcessStatus{}, fmt.Errorf("read process stat: %w", err)
	}

	state, startTicks, err := parseProcessStat(stat)
	if err != nil {
		return port.SessionProcessStatus{}, err
	}
	if state == 'Z' || state == 'X' {
		return port.SessionProcessStatus{}, nil
	}
	leader, err := p.isThreadGroupLeader(pid)
	if err != nil || !leader {
		return port.SessionProcessStatus{}, err
	}

	bootTime, err := readBootTime(p.procRoot)
	if err != nil {
		return port.SessionProcessStatus{}, err
	}
	startedAt := bootTime.Add(time.Duration(startTicks) * (time.Second / userHZ))
	return port.SessionProcessStatus{
		Running:        true,
		StartedAt:      startedAt,
		SameExecutable: p.sameExecutable(pid) && !p.isSubprocess(pid),
	}, nil
}

// checkOwnPIDNamespace fails when /proc shows another PID namespace than ours, for
// example after unshare without remounting /proc: our PIDs would name other processes.
func (p *LivenessProbe) checkOwnPIDNamespace() error {
	self, err := os.Readlink(filepath.Join(p.procRoot, "self"))
	if err != nil {
		return fmt.Errorf("read /proc/self: %w", err)
	}
	if self != strconv.Itoa(p.selfPID) {
		return fmt.Errorf("/proc belongs to another pid namespace (self %s, pid %d)", self, p.selfPID)
	}
	return nil
}

// isSubprocess reports whether pid was started as a Chromium/CEF subprocess.
// An unreadable command line is not treated as a subprocess.
func (p *LivenessProbe) isSubprocess(pid int) bool {
	cmdline, err := os.ReadFile(filepath.Join(p.procRoot, strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return false
	}
	for arg := range bytes.SplitSeq(cmdline, []byte{0}) {
		if bytes.HasPrefix(arg, []byte(subprocessFlag)) {
			return true
		}
	}
	return false
}

// isThreadGroupLeader reports whether pid names a process rather than one of its threads.
// /proc/<tid> resolves thread IDs too, and a session PID is always a process.
func (p *LivenessProbe) isThreadGroupLeader(pid int) (bool, error) {
	status, err := os.ReadFile(filepath.Join(p.procRoot, strconv.Itoa(pid), "status"))
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read process status: %w", err)
	}
	for line := range bytes.SplitSeq(status, []byte{'\n'}) {
		value, ok := bytes.CutPrefix(line, []byte("Tgid:"))
		if !ok {
			continue
		}
		tgid, err := strconv.Atoi(string(bytes.TrimSpace(value)))
		if err != nil {
			return false, fmt.Errorf("malformed process tgid: %w", err)
		}
		return tgid == pid, nil
	}
	return false, errors.New("tgid not found in process status")
}

// sameExecutable reports whether pid runs the same binary as this process.
// An unreadable link (another user's process, or one that exited) is not the same binary.
func (p *LivenessProbe) sameExecutable(pid int) bool {
	own, err := os.Readlink(filepath.Join(p.procRoot, "self", "exe"))
	if err != nil {
		return false
	}
	other, err := os.Readlink(filepath.Join(p.procRoot, strconv.Itoa(pid), "exe"))
	if err != nil {
		return false
	}
	return strings.TrimSuffix(own, deletedExeSuffix) == strings.TrimSuffix(other, deletedExeSuffix)
}

// parseProcessStat extracts the state and starttime from /proc/<pid>/stat.
// The command name may hold spaces and parentheses, so fields start after the last ')'.
func parseProcessStat(stat []byte) (byte, uint64, error) {
	end := bytes.LastIndexByte(stat, ')')
	if end < 0 {
		return 0, 0, errors.New("malformed process stat: missing command name")
	}
	fields := bytes.Fields(stat[end+1:])
	if len(fields) <= statStartTimeIndex || len(fields[0]) != 1 {
		return 0, 0, errors.New("malformed process stat: missing fields")
	}
	startTicks, err := strconv.ParseUint(string(fields[statStartTimeIndex]), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("malformed process stat starttime: %w", err)
	}
	return fields[0][0], startTicks, nil
}

// readBootTime reads the boot time (btime) from /proc/stat.
func readBootTime(procRoot string) (time.Time, error) {
	data, err := os.ReadFile(filepath.Join(procRoot, "stat"))
	if err != nil {
		return time.Time{}, fmt.Errorf("read boot time: %w", err)
	}
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		value, ok := bytes.CutPrefix(line, []byte("btime "))
		if !ok {
			continue
		}
		seconds, err := strconv.ParseInt(string(bytes.TrimSpace(value)), 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("malformed boot time: %w", err)
		}
		return time.Unix(seconds, 0), nil
	}
	return time.Time{}, errors.New("boot time not found in /proc/stat")
}
