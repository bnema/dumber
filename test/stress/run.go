//go:build stress

package stress

import (
	"bufio"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	defaultDuration = 10 * time.Minute
	sessionTTLExtra = 10 * time.Minute
	injectAfter     = 20 * time.Second
	injectExitWait  = 5 * time.Second
	metricsWait     = 30 * time.Second
	pauseMinMS      = 200
	pauseSpanMS     = 1300
	maxPanes        = 4

	// Cumulative action weights out of weightTotal: navigate 40%, new-tab,
	// close-tab, split and close-pane 15% each.
	weightTotal    = 100
	weightNavigate = 40
	weightNewTab   = weightNavigate + 15
	weightCloseTab = weightNewTab + 15
	weightSplit    = weightCloseTab + 15
	logScanMax     = 16 * 1024 * 1024
	logScanInit    = 64 * 1024
)

var stressPaths = []string{"/page", "/redirect", "/popup", "/iframe", "/alloc"}

// stressRun holds the state of one stress run, used for assertions and artifacts.
type stressRun struct {
	t        *testing.T
	sb       *sandbox
	ps       *pageServer
	rng      *rand.Rand
	seed     uint64
	duration time.Duration
	start    time.Time
	proc     *dumberProc
	logDir   string
	// tabs and panes are lower bounds of the real counts. panes is for the
	// active tab only: closing the last pane of a tab closes the tab, and
	// closing the last tab quits Dumber, so the loop never relies on more
	// panes or tabs than it knows exist.
	tabs     int
	panes    int
	actions  int
	last     string
	lastShot string
	// firstPaint is the time from window-mapped to the first good startup shot.
	firstPaint time.Duration
	// died is set once a specific "dumber died ..." failure is recorded.
	died     bool
	failures []string
}

func (r *stressRun) failf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	r.failures = append(r.failures, msg)
	r.t.Error(msg)
}

func (r *stressRun) pid() int {
	if r.proc == nil {
		return 0
	}
	return r.proc.pid
}

func (r *stressRun) loop() {
	start := time.Now()
	deadline := start.Add(r.duration)
	inject := os.Getenv("DUMBER_STRESS_INJECT") == "abort"
	injected := false
	for time.Now().Before(deadline) {
		if inject && !injected && time.Since(start) >= injectAfter {
			injected = true
			r.t.Logf("injecting SIGABRT into pid %d", r.pid())
			if err := r.proc.signal(syscall.SIGABRT); err != nil {
				r.t.Logf("SIGABRT: %v", err)
			}
			r.proc.waitGone(injectExitWait)
		}
		if !r.proc.alive() {
			// A relay open with no running instance would start a new browser.
			r.died = true
			r.failf("dumber died before action %d (last %q)", r.actions+1, r.last)
			return
		}
		name := r.step()
		r.actions++
		r.last = name
		if !r.proc.alive() {
			r.died = true
			r.failf("dumber died after action %d (%s)", r.actions, name)
			return
		}
		time.Sleep(time.Duration(pauseMinMS+r.rng.IntN(pauseSpanMS)) * time.Millisecond)
	}
}

// step runs one weighted random action and returns its name. Tab and pane
// counters change only when every key press succeeded.
func (r *stressRun) step() string {
	n := r.rng.IntN(weightTotal)
	switch {
	case n < weightNavigate:
		host := hostIP
		if r.rng.IntN(2) == 1 {
			host = hostLocal
		}
		path := stressPaths[r.rng.IntN(len(stressPaths))]
		r.navigate(host, path)
		return "navigate " + host + path
	case n < weightNewTab:
		if !r.keys("Ctrl+t", "n") {
			return "new-tab (key failed)"
		}
		r.tabs++
		r.panes = 1
		return "new-tab"
	case n < weightCloseTab:
		if r.tabs <= 1 {
			return "close-tab (skipped)"
		}
		if !r.keys("Ctrl+t", "x") {
			return "close-tab (key failed)"
		}
		r.tabs--
		// The newly active tab's pane count is unknown: assume one.
		r.panes = 1
		return "close-tab"
	case n < weightSplit:
		if r.panes >= maxPanes {
			return "split (skipped)"
		}
		if !r.keys("Ctrl+p", "r") {
			return "split (key failed)"
		}
		r.panes++
		return "split"
	default:
		if r.panes <= 1 {
			return "close-pane (skipped)"
		}
		if !r.keys("Ctrl+p", "x") {
			return "close-pane (key failed)"
		}
		r.panes--
		return "close-pane"
	}
}

func (r *stressRun) navigate(host, path string) {
	if err := relayOpen(r.sb, r.ps.url(host, path)); err != nil {
		r.t.Logf("relay open %s%s: %v", host, path, err)
	}
}

// keys presses each combo then Escape, and reports whether all succeeded.
func (r *stressRun) keys(combos ...string) bool {
	ok := true
	for _, c := range append(combos, "Escape") {
		if out, err := exec.Command("neferwl", "-s", r.sb.session, "key", c).CombinedOutput(); err != nil {
			r.t.Logf("key %s: %v %s", c, err, out)
			ok = false
		}
	}
	return ok
}

func (r *stressRun) logsDir() string { return filepath.Join(r.sb.root, "state", "dumber", "logs") }

func (r *stressRun) fatalPath() string {
	return filepath.Join(r.logsDir(), "crashes", fmt.Sprintf("pid_%d.fatal.txt", r.pid()))
}

// assertLogs checks the newest session log and the crash dir.
func (r *stressRun) assertLogs() {
	logs, _ := filepath.Glob(filepath.Join(r.logsDir(), "session_*.log"))
	if len(logs) == 0 {
		r.failf("no session log under %s", r.logsDir())
	} else {
		newest := newestFile(logs)
		f, err := os.Open(newest)
		if err != nil {
			r.failf("open session log: %v", err)
		} else {
			for _, msg := range checkSessionLog(f) {
				r.failf("%s: %s", filepath.Base(newest), msg)
			}
			_ = f.Close()
		}
	}
	if st, err := os.Stat(r.fatalPath()); err == nil && st.Size() > 0 {
		r.failf("fatal output present: %s (%d bytes)", r.fatalPath(), st.Size())
	}
}

// checkSessionLog returns one message per broken log invariant.
func checkSessionLog(rd io.Reader) []string {
	var watch, started bool
	var repairs, growth int
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 0, logScanInit), logScanMax)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.Contains(line, "runtime watch enabled"):
			watch = true
		case strings.Contains(line, "starting GTK main loop"):
			started = watch
		case strings.Contains(line, "cef: repaired signal handler missing SA_ONSTACK"):
			// CEF init installs SIGHUP/SIGINT/SIGTERM/SIGCHLD handlers without
			// SA_ONSTACK; Dumber repairs them once during startup and logs it.
			// Those repairs are expected. Only repairs after the GTK main loop
			// starts (heartbeat audits) mean Chromium reinstalled a handler.
			if started {
				repairs++
			}
		case strings.Contains(line, "runtime: goroutine count keeps growing"):
			growth++
		}
	}
	var msgs []string
	if err := sc.Err(); err != nil {
		msgs = append(msgs, fmt.Sprintf("read session log: %v (checks below cover only the lines before it)", err))
	}
	if !watch {
		msgs = append(msgs, "no \"runtime watch enabled\" line")
	} else if !started {
		msgs = append(msgs, "no \"starting GTK main loop\" line after runtime watch start; cannot tell startup from runtime SA_ONSTACK repairs")
	}
	if repairs > 0 {
		msgs = append(msgs, fmt.Sprintf("%d SA_ONSTACK repairs after startup", repairs))
	}
	if growth > 0 {
		msgs = append(msgs, fmt.Sprintf("%d goroutine growth warnings", growth))
	}
	return msgs
}

func newestFile(paths []string) string {
	sort.Slice(paths, func(i, j int) bool { return modTime(paths[i]).After(modTime(paths[j])) })
	return paths[0]
}

func modTime(p string) time.Time {
	st, err := os.Stat(p)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}
