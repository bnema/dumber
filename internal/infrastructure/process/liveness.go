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
	"syscall"
	"time"

	"bnema.dev/dumber/internal/application/port"
)

// userHZ is the fixed unit of /proc time fields on Linux (USER_HZ), independent of the kernel HZ.
const userHZ = 100

// statStartTimeIndex is the position of starttime (field 22) counted from the state field (field 3).
const statStartTimeIndex = 22 - 3

// LivenessProbe reads /proc to tell whether a PID is held and since when.
type LivenessProbe struct {
	procRoot string
}

// NewLivenessProbe creates a LivenessProbe that reads the host /proc.
func NewLivenessProbe() *LivenessProbe {
	return &LivenessProbe{procRoot: "/proc"}
}

// ProcessStatus reports whether a live process or thread holds pid and when it started.
// A missing PID or a zombie is not running. Unreadable or malformed /proc data is an
// error so callers can keep the session rather than end it on doubt.
func (p *LivenessProbe) ProcessStatus(ctx context.Context, pid int) (port.SessionProcessStatus, error) {
	if err := ctx.Err(); err != nil {
		return port.SessionProcessStatus{}, err
	}
	if pid <= 0 {
		return port.SessionProcessStatus{}, fmt.Errorf("invalid pid %d", pid)
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

	bootTime, err := readBootTime(p.procRoot)
	if err != nil {
		return port.SessionProcessStatus{}, err
	}
	startedAt := bootTime.Add(time.Duration(startTicks) * (time.Second / userHZ))
	return port.SessionProcessStatus{Running: true, StartedAt: startedAt}, nil
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
