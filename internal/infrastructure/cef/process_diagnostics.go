package cef

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/logging"
)

const (
	maxLoggedThreadsPerProcess       = 8
	maxDetailedThreadsPerProcess     = 48
	renderStallBacktraceCooldown     = 30 * time.Second
	renderStallBacktraceCommandLimit = 64 * 1024
)

var renderStallBacktraceLastUnixNS atomic.Int64

func (wv *WebView) logCEFProcessDiagnostics(reason string, classification renderStallClassification) {
	if wv == nil || wv.ctx == nil {
		return
	}
	pids := append([]int{os.Getpid()}, childPIDs(os.Getpid())...)
	for _, pid := range pids {
		cmdline := procCmdline(pid)
		processType := chromiumProcessType(cmdline)
		threads := procThreadSummaries(pid, maxLoggedThreadsPerProcess)
		detailedThreads := []string(nil)
		if classification.CEFUIBlocked && processType == "browser" {
			detailedThreads = procDetailedThreadSummaries(pid, maxDetailedThreadsPerProcess)
		}
		logging.FromContext(wv.ctx).Warn().
			Str("reason", reason).
			Str("classification", classification.Category).
			Bool("cef_ui_blocked", classification.CEFUIBlocked).
			Bool("cef_io_alive", classification.CEFIOAlive).
			Int("pid", pid).
			Str("process_type", processType).
			Str("cmdline", logging.TruncateURL(cmdline, 1024)).
			Int("thread_count", procThreadCount(pid)).
			Strs("hot_threads", threads).
			Strs("detailed_threads", detailedThreads).
			Strs("gpu_fdinfo", gpuFDInfoSummaries(pid)).
			Msg("cef: process diagnostic snapshot")
	}
	if classification.CEFUIBlocked {
		wv.maybeCaptureRenderStallBacktrace(reason, classification)
	}
}

func childPIDs(parent int) []int {
	if out := childPIDsFromProcChildren(parent); len(out) > 0 {
		return out
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		if procPPID(pid) == parent {
			out = append(out, pid)
		}
	}
	sort.Ints(out)
	return out
}

func childPIDsFromProcChildren(parent int) []int {
	path := filepath.Join("/proc", strconv.Itoa(parent), "task", strconv.Itoa(parent), "children")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(b))
	out := make([]int, 0, len(fields))
	for _, field := range fields {
		pid, err := strconv.Atoi(field)
		if err != nil {
			continue
		}
		out = append(out, pid)
	}
	sort.Ints(out)
	return out
}

func procPPID(pid int) int {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "PPid:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			ppid, _ := strconv.Atoi(fields[1])
			return ppid
		}
	}
	return 0
}

func procCmdline(pid int) string {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(bytes.ReplaceAll(b, []byte{0}, []byte{' '})))
}

func chromiumProcessType(cmdline string) string {
	for _, field := range strings.Fields(cmdline) {
		if strings.HasPrefix(field, "--type=") {
			return strings.TrimPrefix(field, "--type=")
		}
	}
	return "browser"
}

func procThreadCount(pid int) int {
	entries, err := os.ReadDir(filepath.Join("/proc", strconv.Itoa(pid), "task"))
	if err != nil {
		return 0
	}
	return len(entries)
}

type procThreadSummary struct {
	text  string
	ticks uint64
}

func procThreadSummaries(pid, limit int) []string {
	return procThreadSummariesWithDetails(pid, limit, false)
}

func procDetailedThreadSummaries(pid, limit int) []string {
	return procThreadSummariesWithDetails(pid, limit, true)
}

func procThreadSummariesWithDetails(pid, limit int, detailed bool) []string {
	entries, err := os.ReadDir(filepath.Join("/proc", strconv.Itoa(pid), "task"))
	if err != nil {
		return nil
	}
	items := make([]procThreadSummary, 0, len(entries))
	for _, entry := range entries {
		tid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "task", strconv.Itoa(tid), "stat"))
		if err != nil {
			continue
		}
		text, ticks := parseThreadStat(tid, string(b))
		if detailed {
			text = text + " wchan=" + procThreadWchan(pid, tid)
			if stack := procThreadKernelStack(pid, tid); stack != "" {
				text = text + " kernel_stack=" + stack
			}
		}
		items = append(items, procThreadSummary{text: text, ticks: ticks})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ticks > items[j].ticks })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.text)
	}
	return out
}

func parseThreadStat(tid int, stat string) (string, uint64) {
	open := strings.IndexByte(stat, '(')
	close := strings.LastIndexByte(stat, ')')
	if open < 0 || close <= open || close+2 >= len(stat) {
		return strconv.Itoa(tid), 0
	}
	comm := stat[open+1 : close]
	rest := strings.Fields(stat[close+2:])
	state := "?"
	if len(rest) > 0 {
		state = rest[0]
	}
	var ticks uint64
	if len(rest) > 12 {
		utime, _ := strconv.ParseUint(rest[11], 10, 64)
		stime, _ := strconv.ParseUint(rest[12], 10, 64)
		ticks = utime + stime
	}
	return "tid=" + strconv.Itoa(tid) + " state=" + state + " ticks=" + strconv.FormatUint(ticks, 10) + " comm=" + comm, ticks
}

func procThreadWchan(pid, tid int) string {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "task", strconv.Itoa(tid), "wchan"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func procThreadKernelStack(pid, tid int) string {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "task", strconv.Itoa(tid), "stack"))
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > 6 {
		lines = lines[:6]
	}
	return strings.Join(lines, " | ")
}

func (wv *WebView) maybeCaptureRenderStallBacktrace(reason string, classification renderStallClassification) {
	if wv == nil || wv.ctx == nil || wv.destroyed.Load() || !renderStallBacktraceEnabled() {
		return
	}
	now := time.Now()
	last := unixNSTime(renderStallBacktraceLastUnixNS.Load())
	if !last.IsZero() && now.Sub(last) < renderStallBacktraceCooldown {
		return
	}
	renderStallBacktraceLastUnixNS.Store(now.UnixNano())
	pid := os.Getpid()
	if _, err := exec.LookPath("gdb"); err != nil {
		logging.FromContext(wv.ctx).Warn().
			Err(err).
			Str("reason", reason).
			Str("classification", classification.Category).
			Int("pid", pid).
			Str("external_command", fmt.Sprintf("gdb -p %d -batch -ex 'thread apply all bt'", pid)).
			Msg("cef: render stall native backtrace skipped; gdb not available")
		return
	}
	if wv == nil || wv.ctx == nil || wv.destroyed.Load() || !renderStallBacktraceEnabled() {
		return
	}
	logCtx := wv.ctx
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "gdb", "-p", strconv.Itoa(pid), "-batch", "-ex", "thread apply all bt")
		out, err := cmd.CombinedOutput()
		truncated := false
		if len(out) > renderStallBacktraceCommandLimit {
			out = out[:renderStallBacktraceCommandLimit]
			truncated = true
		}
		log := logging.FromContext(logCtx).Warn().
			Str("reason", reason).
			Str("classification", classification.Category).
			Int("pid", pid).
			Bool("truncated", truncated).
			Str("backtrace", string(out))
		if ctx.Err() != nil {
			log = log.Err(ctx.Err())
		} else if err != nil {
			log = log.Err(err)
		}
		log.Msg("cef: render stall native backtrace")
	}()
}

func gpuFDInfoSummaries(pid int) []string {
	entries, err := os.ReadDir(filepath.Join("/proc", strconv.Itoa(pid), "fdinfo"))
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "fdinfo", entry.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, "drm-engine-") || strings.HasPrefix(line, "drm-client-id") || strings.HasPrefix(line, "drm-pdev") {
				out = append(out, "fd="+entry.Name()+" "+strings.TrimSpace(line))
			}
		}
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}
