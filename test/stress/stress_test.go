//go:build stress

package stress

import (
	"bufio"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	defaultDuration = 10 * time.Minute
	sessionTTLExtra = 10 * time.Minute
	shotRetries     = 10
	startupRetries  = 40
	slowFirstPaint  = 10 * time.Second
	keepArtifacts   = 3
	shotRetryDelay  = time.Second
	shotMinRatio    = 0.20
	shotTolerance   = 8
	injectAfter     = 20 * time.Second
	injectExitWait  = 5 * time.Second
	metricsWait     = 30 * time.Second
	pauseMinMS      = 200
	pauseSpanMS     = 1300
	maxPanes        = 4
)

// pageColor is #2e8b57, the /page background.
var pageColor = [3]int{0x2e, 0x8b, 0x57}

var stressPaths = []string{"/page", "/redirect", "/popup", "/iframe", "/alloc"}

// stressRun holds the state of one stress run, used for assertions and artifacts.
type stressRun struct {
	t        *testing.T
	sb       *sandbox
	ps       *pageServer
	rng      *rand.Rand
	seed     uint64
	duration time.Duration
	pid      int
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
	died       bool
	failures   []string
}

func (r *stressRun) failf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	r.failures = append(r.failures, msg)
	r.t.Error(msg)
}

func TestStressLoop(t *testing.T) {
	requireTools(t)
	duration := defaultDuration
	if v := os.Getenv("DUMBER_STRESS_DURATION"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("DUMBER_STRESS_DURATION: %v", err)
		}
		duration = d
	}
	seed := uint64(time.Now().UnixNano())
	if v := os.Getenv("DUMBER_STRESS_SEED"); v != "" {
		s, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			t.Fatalf("DUMBER_STRESS_SEED: %v", err)
		}
		seed = s
	}
	t.Logf("seed=%d duration=%s", seed, duration)
	artifacts := filepath.Join("artifacts", time.Now().UTC().Format("20060102-150405"))

	r := &stressRun{
		t: t, seed: seed, duration: duration,
		rng: rand.New(rand.NewPCG(seed, seed>>1)), tabs: 1, panes: 1,
	}
	r.ps = newPageServer(t)
	r.sb = newSandbox(t)
	r.sb.bin = buildDumber(t, r.sb.root)
	startNeferwl(t, r.sb, sessionID(), duration+sessionTTLExtra)
	r.logDir = filepath.Join(r.sb.root, "out")
	r.pid = launchDumber(t, r.sb, r.ps.url(hostIP, "/page"), r.logDir)
	t.Logf("dumber pid=%d sandbox=%s", r.pid, r.sb.root)
	t.Cleanup(func() {
		if alive(r.pid) {
			stopDumber(r.pid, stopTimeout)
		}
		if t.Failed() {
			r.collect(artifacts)
		}
	})

	if elapsed, ok := r.shotCheck("startup", startupRetries); ok {
		r.firstPaint = elapsed
		t.Logf("first_paint_s=%.1f", elapsed.Seconds())
		if elapsed > slowFirstPaint {
			t.Logf("WARN slow first paint: %.1fs > %s (product issue, not a failure)", elapsed.Seconds(), slowFirstPaint)
		}
	}
	if r.died {
		return
	}
	r.loop()
	if r.died {
		r.assertLogs()
		return
	}
	r.navigate(hostIP, "/page")
	r.shotCheck("end", shotRetries)
	time.Sleep(metricsWait)
	if !alive(r.pid) {
		r.died = true
		r.failf("dumber died before SIGTERM (after %d actions, last %q)", r.actions, r.last)
	} else if stopDumber(r.pid, stopTimeout) {
		r.failf("shutdown hang: dumber still alive %s after SIGTERM", stopTimeout)
	}
	r.assertLogs()
	t.Logf("actions=%d", r.actions)
}

func (r *stressRun) loop() {
	start := time.Now()
	deadline := start.Add(r.duration)
	inject := os.Getenv("DUMBER_STRESS_INJECT") == "abort"
	injected := false
	for time.Now().Before(deadline) {
		if inject && !injected && time.Since(start) >= injectAfter {
			injected = true
			r.t.Logf("injecting SIGABRT into pid %d", r.pid)
			_ = syscall.Kill(r.pid, syscall.SIGABRT)
			r.waitExit(injectExitWait)
		}
		if !alive(r.pid) {
			// A relay open with no running instance would start a new browser.
			r.died = true
			r.failf("dumber died before action %d (last %q)", r.actions+1, r.last)
			return
		}
		name := r.step()
		r.actions++
		r.last = name
		if !alive(r.pid) {
			r.died = true
			r.failf("dumber died after action %d (%s)", r.actions, name)
			return
		}
		time.Sleep(time.Duration(pauseMinMS+r.rng.IntN(pauseSpanMS)) * time.Millisecond)
	}
}

// step runs one weighted random action and returns its name.
func (r *stressRun) step() string {
	n := r.rng.IntN(100)
	switch {
	case n < 40:
		host := hostIP
		if r.rng.IntN(2) == 1 {
			host = hostLocal
		}
		path := stressPaths[r.rng.IntN(len(stressPaths))]
		r.navigate(host, path)
		return "navigate " + host + path
	case n < 55:
		r.keys("Ctrl+t", "n")
		r.tabs++
		r.panes = 1
		return "new-tab"
	case n < 70:
		if r.tabs <= 1 {
			return "close-tab (skipped)"
		}
		r.keys("Ctrl+t", "x")
		r.tabs--
		// The newly active tab's pane count is unknown: assume one.
		r.panes = 1
		return "close-tab"
	case n < 85:
		if r.panes >= maxPanes {
			return "split (skipped)"
		}
		r.keys("Ctrl+p", "r")
		r.panes++
		return "split"
	default:
		if r.panes <= 1 {
			return "close-pane (skipped)"
		}
		r.keys("Ctrl+p", "x")
		r.panes--
		return "close-pane"
	}
}

// waitExit polls until pid exits or timeout elapses.
func (r *stressRun) waitExit(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for alive(r.pid) && time.Now().Before(deadline) {
		time.Sleep(stopPoll)
	}
}

func (r *stressRun) navigate(host, path string) {
	if err := relayOpen(r.sb, r.ps.url(host, path)); err != nil {
		r.t.Logf("relay open %s%s: %v", host, path, err)
	}
}

func (r *stressRun) keys(combos ...string) {
	for _, c := range append(combos, "Escape") {
		if out, err := exec.Command("neferwl", "-s", r.sb.session, "key", c).CombinedOutput(); err != nil {
			r.t.Logf("key %s: %v %s", c, err, out)
		}
	}
}

// shotCheck requires >= 20% of the screenshot to be the /page color within
// retries attempts. It returns the time until the first good shot.
func (r *stressRun) shotCheck(stage string, retries int) (time.Duration, bool) {
	start := time.Now()
	var ratio float64
	for i := range retries {
		if i > 0 {
			time.Sleep(shotRetryDelay)
		}
		if !alive(r.pid) {
			r.died = true
			r.failf("dumber died during %s screenshot check", stage)
			return 0, false
		}
		path, err := r.shot()
		if err != nil {
			r.t.Logf("shot %s: %v", stage, err)
			continue
		}
		ratio, err = colorRatio(path)
		if err != nil {
			r.t.Logf("shot %s: %v", stage, err)
			continue
		}
		if ratio >= shotMinRatio {
			return time.Since(start), true
		}
	}
	r.failf("%s screenshot: page color covers %.1f%%, want >= %.0f%%", stage, ratio*100, shotMinRatio*100)
	return 0, false
}

func (r *stressRun) shot() (string, error) {
	out, err := exec.Command("neferwl", "-s", r.sb.session, "shot").Output()
	if err != nil {
		return "", fmt.Errorf("neferwl shot: %w", err)
	}
	path, err := parseShotPath(out)
	if err != nil {
		return "", err
	}
	r.lastShot = path
	return path, nil
}

func parseShotPath(out []byte) (string, error) {
	var res struct {
		Shots []struct {
			Path string `json:"path"`
		} `json:"shots"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return "", fmt.Errorf("parse shot json: %w", err)
	}
	if len(res.Shots) == 0 || res.Shots[0].Path == "" {
		return "", fmt.Errorf("shot json has no path: %s", out)
	}
	return res.Shots[0].Path, nil
}

func colorRatio(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return 0, fmt.Errorf("decode %s: %w", path, err)
	}
	return matchRatio(img, pageColor, shotTolerance), nil
}

func matchRatio(img image.Image, want [3]int, tol int) float64 {
	b := img.Bounds()
	total, match := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			total++
			if near(int(cr>>8), want[0], tol) && near(int(cg>>8), want[1], tol) && near(int(cb>>8), want[2], tol) {
				match++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(match) / float64(total)
}

func near(a, b, tol int) bool { return a-b <= tol && b-a <= tol }

func (r *stressRun) logsDir() string { return filepath.Join(r.sb.root, "state", "dumber", "logs") }

func (r *stressRun) fatalPath() string {
	return filepath.Join(r.logsDir(), "crashes", fmt.Sprintf("pid_%d.fatal.txt", r.pid))
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
	if r.died {
		r.failf("dumber died before SIGTERM")
	}
}

// checkSessionLog returns one message per broken log invariant.
func checkSessionLog(rd io.Reader) []string {
	var watch, started bool
	var repairs, growth int
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
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

// collect copies failure evidence into dir and writes summary.txt.
func (r *stressRun) collect(dir string) {
	t := r.t
	pruneArtifacts(t, filepath.Dir(dir), keepArtifacts)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		t.Logf("artifacts dir: %v", err)
		return
	}
	copyLog := func(src, dst string) {
		if err := copyPath(src, filepath.Join(dir, dst)); err != nil {
			t.Logf("artifact %s: %v", dst, err)
		}
	}
	copyLog(r.sb.bin, "dumber")
	copyLog(r.logsDir(), "logs")
	if _, err := os.Stat(r.fatalPath()); err == nil {
		copyLog(r.fatalPath(), "fatal.txt")
	}
	if r.lastShot != "" {
		copyLog(r.lastShot, "last-shot.png")
	}
	copyLog(filepath.Join(r.logDir, "stdout.txt"), "stdout.txt")
	copyLog(filepath.Join(r.logDir, "stderr.txt"), "stderr.txt")
	core := filepath.Join(dir, "core")
	if out, err := exec.Command("coredumpctl", "dump", strconv.Itoa(r.pid), "-o", core).CombinedOutput(); err != nil {
		t.Logf("coredumpctl dump %d: %v %s", r.pid, err, strings.TrimSpace(string(out)))
	}
	var sum strings.Builder
	fmt.Fprintf(&sum, "seed=%d\nduration=%s\nactions=%d\nlast_action=%s\nfirst_paint_s=%.1f\npid=%d\nsandbox=%s\nfailures:\n",
		r.seed, r.duration, r.actions, r.last, r.firstPaint.Seconds(), r.pid, r.sb.root)
	for _, f := range r.failures {
		fmt.Fprintf(&sum, "- %s\n", f)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.txt"), []byte(sum.String()), filePerm); err != nil {
		t.Logf("summary.txt: %v", err)
	}
	abs, _ := filepath.Abs(dir)
	t.Logf("artifacts: %s", abs)
}

// pruneArtifacts keeps only the keep newest run dirs under root (cores are large).
// Run dir names are UTC timestamps, so lexical order is age order.
func pruneArtifacts(t *testing.T, root string, keep int) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Strings(dirs)
	for len(dirs) > keep {
		old := filepath.Join(root, dirs[0])
		if err := os.RemoveAll(old); err != nil {
			t.Logf("prune %s: %v", old, err)
		}
		dirs = dirs[1:]
	}
}

// copyPath copies a file or a directory tree from src to dst.
func copyPath(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return copyFile(src, dst)
	}
	return filepath.WalkDir(src, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, dirPerm)
		}
		return copyFile(p, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
