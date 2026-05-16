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
	procRoot                              = "/proc"
	maxLoggedThreadsPerProcess            = 8
	maxDetailedThreadsPerProcess          = 48
	threadStatUTimeIndex                  = 11
	threadStatSTimeIndex                  = 12
	maxKernelStackLines                   = 6
	renderStallProcessDiagnosticsCooldown = 30 * time.Second
	renderStallBacktraceCooldown          = 30 * time.Second
	renderStallBacktraceCommandLimit      = 64 * 1024
)

var (
	renderStallProcessDiagnosticsLastUnixNS atomic.Int64
	renderStallBacktraceLastUnixNS          atomic.Int64
)

func (wv *WebView) logCEFProcessDiagnostics(reason string, classification renderStallClassification) {
	if wv == nil || wv.ctx == nil {
		return
	}
	if !reserveRenderStallProcessDiagnostics(time.Now()) {
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
			Strs("cmdline_flags", safeChromiumCmdlineFlags(cmdline)).
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

func reserveRenderStallProcessDiagnostics(now time.Time) bool {
	nowNS := now.UnixNano()
	for {
		lastNS := renderStallProcessDiagnosticsLastUnixNS.Load()
		last := unixNSTime(lastNS)
		if !last.IsZero() && now.Sub(last) < renderStallProcessDiagnosticsCooldown {
			return false
		}
		if renderStallProcessDiagnosticsLastUnixNS.CompareAndSwap(lastNS, nowNS) {
			return true
		}
	}
}

func safeChromiumCmdlineFlags(cmdline string) []string {
	fields := strings.Fields(cmdline)
	flags := make([]string, 0, len(fields))
	seen := make(map[string]struct{})
	for _, field := range fields {
		if !strings.HasPrefix(field, "--") {
			continue
		}
		name, value, hasValue := strings.Cut(field, "=")
		var safe string
		switch name {
		case "--type", "--lang", "--enable-features", "--disable-features", "--ozone-platform",
			"--use-gl", "--use-angle", "--enable-logging", "--log-severity",
			"--force-device-scale-factor", "--high-dpi-support", "--enable-use-zoom-for-dsf":
			if hasValue && value != "" {
				safe = name + "=" + value
			} else {
				safe = name
			}
		case "--enable-gpu-rasterization", "--disable-gpu", "--disable-software-rasterizer", "--in-process-gpu", "--no-sandbox", "--no-zygote":
			safe = name
		default:
			continue
		}
		if _, ok := seen[safe]; ok {
			continue
		}
		seen[safe] = struct{}{}
		flags = append(flags, safe)
	}
	return flags
}

func procPath(parts ...string) string {
	return filepath.Join(append([]string{procRoot}, parts...)...)
}

func childPIDs(parent int) []int {
	if out := childPIDsFromProcChildren(parent); len(out) > 0 {
		return out
	}
	entries, err := os.ReadDir(procRoot)
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
	path := procPath(strconv.Itoa(parent), "task", strconv.Itoa(parent), "children")
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
	b, err := os.ReadFile(procPath(strconv.Itoa(pid), "status"))
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
	b, err := os.ReadFile(procPath(strconv.Itoa(pid), "cmdline"))
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
	entries, err := os.ReadDir(procPath(strconv.Itoa(pid), "task"))
	if err != nil {
		return 0
	}
	return len(entries)
}

type procThreadSummary struct {
	tid   int
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
	entries, err := os.ReadDir(procPath(strconv.Itoa(pid), "task"))
	if err != nil {
		return nil
	}
	items := make([]procThreadSummary, 0, len(entries))
	for _, entry := range entries {
		tid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile(procPath(strconv.Itoa(pid), "task", strconv.Itoa(tid), "stat"))
		if err != nil {
			continue
		}
		text, ticks := parseThreadStat(tid, string(b))
		items = append(items, procThreadSummary{tid: tid, text: text, ticks: ticks})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ticks > items[j].ticks })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	if detailed {
		for i := range items {
			text := items[i].text + " wchan=" + procThreadWchan(pid, items[i].tid)
			if stack := procThreadKernelStack(pid, items[i].tid); stack != "" {
				text = text + " kernel_stack=" + stack
			}
			items[i].text = text
		}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.text)
	}
	return out
}

func parseThreadStat(tid int, stat string) (string, uint64) {
	open := strings.IndexByte(stat, '(')
	closeIdx := strings.LastIndexByte(stat, ')')
	if open < 0 || closeIdx <= open || closeIdx+2 >= len(stat) {
		return strconv.Itoa(tid), 0
	}
	comm := stat[open+1 : closeIdx]
	rest := strings.Fields(stat[closeIdx+2:])
	state := "?"
	if len(rest) > 0 {
		state = rest[0]
	}
	var ticks uint64
	if len(rest) > threadStatSTimeIndex {
		utime, _ := strconv.ParseUint(rest[threadStatUTimeIndex], 10, 64)
		stime, _ := strconv.ParseUint(rest[threadStatSTimeIndex], 10, 64)
		ticks = utime + stime
	}
	return "tid=" + strconv.Itoa(tid) + " state=" + state + " ticks=" + strconv.FormatUint(ticks, 10) + " comm=" + comm, ticks
}

func procThreadWchan(pid, tid int) string {
	b, err := os.ReadFile(procPath(strconv.Itoa(pid), "task", strconv.Itoa(tid), "wchan"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func procThreadKernelStack(pid, tid int) string {
	b, err := os.ReadFile(procPath(strconv.Itoa(pid), "task", strconv.Itoa(tid), "stack"))
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > maxKernelStackLines {
		lines = lines[:maxKernelStackLines]
	}
	return strings.Join(lines, " | ")
}

func (wv *WebView) maybeCaptureRenderStallBacktrace(reason string, classification renderStallClassification) {
	if wv == nil || wv.ctx == nil || wv.destroyed.Load() || !renderStallBacktraceEnabled() {
		return
	}
	now := time.Now()
	nowNS := now.UnixNano()
	for {
		lastNS := renderStallBacktraceLastUnixNS.Load()
		last := unixNSTime(lastNS)
		if !last.IsZero() && now.Sub(last) < renderStallBacktraceCooldown {
			return
		}
		if renderStallBacktraceLastUnixNS.CompareAndSwap(lastNS, nowNS) {
			break
		}
	}
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
	if wv.ctx == nil || wv.destroyed.Load() || !renderStallBacktraceEnabled() {
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
	entries, err := os.ReadDir(procPath(strconv.Itoa(pid), "fdinfo"))
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		b, err := os.ReadFile(procPath(strconv.Itoa(pid), "fdinfo", entry.Name()))
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
