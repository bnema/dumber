//go:build stress

package stress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	dumberAppID   = "com.github.bnema.dumber"
	socketPathMax = 100
	dirPerm       = 0o700
	filePerm      = 0o600
	execPerm      = 0o700
	relayTimeout  = 20 * time.Second
	procDir       = "/proc"
	stopTimeout   = 20 * time.Second
	stopPoll      = 100 * time.Millisecond
	killTimeout   = 10 * time.Second
	sessionIDMax  = 32
	deadBus       = "unix:path=/nonexistent"
)

// sandbox is an isolated XDG tree plus the NeferWL session Dumber runs in.
type sandbox struct {
	root    string
	bin     string // built dumber binary
	session string // NeferWL session id
	wayland string // WAYLAND_DISPLAY from the session JSON
}

// requireTools skips unless DUMBER_STRESS=1 and every external tool exists.
func requireTools(t *testing.T) {
	t.Helper()
	if os.Getenv("DUMBER_STRESS") != "1" {
		t.Skip("set DUMBER_STRESS=1 to run the stress harness")
	}
	for _, tool := range []string{"neferwl", "dbus-run-session", "coredumpctl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found on PATH", tool)
		}
	}
}

// buildDumber builds an unoptimized dumber binary into dir and returns its path.
func buildDumber(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		t.Fatal("go env GOMOD: not inside a module")
	}
	bin := filepath.Join(dir, "dumber")
	cmd := exec.Command("go", "build", "-gcflags", "all=-N -l",
		"-ldflags", "-X main.version=stress", "-o", bin, "./cmd/dumber")
	cmd.Dir = filepath.Dir(gomod)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dumber: %v\n%s", err, b)
	}
	return bin
}

// newSandbox creates a short /tmp root with config/data/state/cache dirs.
func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "dst-")
	if err != nil {
		t.Fatalf("sandbox root: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			_ = os.RemoveAll(root)
		}
	})
	for _, d := range []string{"config", "data", "state", "cache"} {
		if err := os.MkdirAll(filepath.Join(root, d), dirPerm); err != nil {
			t.Fatalf("sandbox %s: %v", d, err)
		}
	}
	sock := root + "/state/dumber/runtime/cef/browser-launch.sock"
	if len(sock) >= socketPathMax {
		t.Fatalf("socket path too long (%d): %s", len(sock), sock)
	}
	return &sandbox{root: root}
}

// env returns the sandbox environment: an allowlist of host variables plus
// the sandbox XDG dirs and the session Wayland display.
func (sb *sandbox) env() []string {
	return sandboxEnv(os.Environ(), sb.root, sb.wayland)
}

// envKeep lists host variables passed through as-is.
var envKeep = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true,
	"TERM": true, "LANG": true, "TZ": true, "XDG_RUNTIME_DIR": true,
}

// envKeepPrefixes lists locale and GPU driver variable prefixes passed through.
var envKeepPrefixes = []string{"LC_", "VK_", "MESA_", "LIBVA_", "__GLX_", "DRI_"}

// envNever must never reach the sandbox: a real bus or Wayland socket would
// let Dumber (or a fallback GUI from a failed relay) touch the user's desktop.
var envNever = map[string]bool{
	"DBUS_SESSION_BUS_ADDRESS": true, "DBUS_SYSTEM_BUS_ADDRESS": true, "WAYLAND_SOCKET": true,
}

func keepEnv(k string) bool {
	if envNever[k] {
		return false
	}
	if envKeep[k] {
		return true
	}
	for _, p := range envKeepPrefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

func sandboxEnv(base []string, root, wayland string) []string {
	env := make([]string, 0, len(envKeep)+9)
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if keepEnv(k) {
			env = append(env, kv)
		}
	}
	return append(env,
		"XDG_CONFIG_HOME="+filepath.Join(root, "config"),
		"XDG_DATA_HOME="+filepath.Join(root, "data"),
		"XDG_STATE_HOME="+filepath.Join(root, "state"),
		"XDG_CACHE_HOME="+filepath.Join(root, "cache"),
		"WAYLAND_DISPLAY="+wayland,
		"GDK_BACKEND=wayland",
		"GOTRACEBACK=crash",
		"GOGC=10",
		"DUMBER_RUNTIME_WATCH=1",
	)
}

// shellQuote wraps s in single quotes for POSIX sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// parseSessionWayland extracts the wayland display from `session new` JSON.
func parseSessionWayland(out []byte) (string, error) {
	var s struct {
		ID      string `json:"id"`
		Wayland string `json:"wayland"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &s); err != nil {
		return "", fmt.Errorf("parse session json: %w", err)
	}
	if s.Wayland == "" {
		return "", fmt.Errorf("session json has no wayland display: %s", out)
	}
	return s.Wayland, nil
}

// startNeferwl starts a headless NeferWL session and records its display in sb.
func startNeferwl(t *testing.T, sb *sandbox, id string, ttl time.Duration) {
	t.Helper()
	conf := filepath.Join(sb.root, "nwl.conf")
	if err := os.WriteFile(conf, []byte("animations = off\n"), filePerm); err != nil {
		t.Fatalf("write nwl.conf: %v", err)
	}
	out, err := exec.Command("neferwl", "session", "new", "--id", id, "--config", conf, //nolint:gosec // fixed binary, test-built args
		"--size", "1600x1000", "--ttl", ttl.String(), "--", "--no-xwayland", "--no-terminal").Output()
	if err != nil {
		t.Fatalf("neferwl session new: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("neferwl", "session", "rm", id).Run() })
	wl, err := parseSessionWayland(out)
	if err != nil {
		t.Fatal(err)
	}
	sb.session, sb.wayland = id, wl
}

// sessionID is a NeferWL session id unique to this process and test:
// lowercase a-z0-9 and '-', at most sessionIDMax characters.
func sessionID(t *testing.T) string {
	id := "stress-" + strconv.Itoa(os.Getpid()) + "-"
	var name strings.Builder
	for _, c := range strings.ToLower(strings.TrimPrefix(t.Name(), "Test")) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			name.WriteRune(c)
		} else {
			name.WriteByte('-')
		}
	}
	id += name.String()
	if len(id) > sessionIDMax {
		id = id[:sessionIDMax]
	}
	return strings.TrimRight(id, "-")
}

// launchDumber spawns dumber on url in the session, waits for its window,
// and returns the verified browser process. stdout/stderr go to logDir.
// On failure it stops every browser process started from sb.bin.
func launchDumber(t *testing.T, sb *sandbox, url, logDir string) *dumberProc {
	t.Helper()
	if err := os.MkdirAll(logDir, dirPerm); err != nil {
		t.Fatalf("log dir: %v", err)
	}
	var sh strings.Builder
	sh.WriteString("#!/bin/sh\n")
	for _, kv := range sb.env() {
		k, v, _ := strings.Cut(kv, "=")
		if !validShellName(k) {
			continue
		}
		fmt.Fprintf(&sh, "export %s=%s\n", k, shellQuote(v))
	}
	fmt.Fprintf(&sh, "exec dbus-run-session -- %s browse \"$1\" >%s 2>%s\n",
		shellQuote(sb.bin), shellQuote(filepath.Join(logDir, "stdout.txt")),
		shellQuote(filepath.Join(logDir, "stderr.txt")))
	script := filepath.Join(sb.root, "run.sh")
	if err := os.WriteFile(script, []byte(sh.String()), execPerm); err != nil {
		t.Fatalf("write run.sh: %v", err)
	}
	out, err := exec.Command("neferwl", "-s", sb.session, "do", "spawn",
		"--until", "window-mapped app_id="+dumberAppID, "--timeout", "90s",
		"--", script, url).CombinedOutput()
	if err != nil {
		stopAll(t, sb.bin)
		t.Fatalf("neferwl spawn dumber: %v\n%s", err, out)
	}
	procs := findBrowserProcs(sb.bin)
	if len(procs) != 1 {
		pids := make([]int, 0, len(procs))
		for _, p := range procs {
			pids = append(pids, p.pid)
		}
		stopAll(t, sb.bin)
		t.Fatalf("want exactly one dumber browser process, got %v", pids)
	}
	return procs[0]
}

func validShellName(k string) bool {
	if k == "" {
		return false
	}
	for i, c := range k {
		if c != '_' && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// dumberProc is a browser process found by findBrowserProcs. proc is
// pidfd-backed on Linux, so signals never reach a reused PID.
type dumberProc struct {
	pid  int
	bin  string
	proc *os.Process
}

// findBrowserProcs walks /proc for dumber browser processes (not CEF
// subprocesses) started from bin.
func findBrowserProcs(bin string) []*dumberProc {
	entries, _ := os.ReadDir(procDir)
	var procs []*dumberProc
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || !exeIs(pid, bin) {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(procDir, e.Name(), "cmdline"))
		if rerr != nil || !isBrowserCmdline(raw) {
			continue
		}
		p, ferr := os.FindProcess(pid)
		if ferr != nil {
			continue
		}
		// Re-check after taking the handle: the PID could have been reused
		// between the /proc read and FindProcess.
		if !exeIs(pid, bin) {
			_ = p.Release()
			continue
		}
		procs = append(procs, &dumberProc{pid: pid, bin: bin, proc: p})
	}
	return procs
}

func exeIs(pid int, bin string) bool {
	exe, err := os.Readlink(filepath.Join(procDir, strconv.Itoa(pid), "exe"))
	return err == nil && exe == bin
}

func isBrowserCmdline(raw []byte) bool {
	browse, sub := false, false
	for _, a := range strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00") {
		browse = browse || a == "browse"
		sub = sub || strings.HasPrefix(a, "--type=")
	}
	return browse && !sub
}

// signal sends sig through the process handle after checking that the PID
// still runs sb.bin. It returns os.ErrProcessDone when the process is gone.
func (p *dumberProc) signal(sig os.Signal) error {
	if !exeIs(p.pid, p.bin) {
		return os.ErrProcessDone
	}
	return p.proc.Signal(sig)
}

// alive reports whether the process exists, is not a zombie, and still runs bin.
func (p *dumberProc) alive() bool {
	if p == nil || p.proc.Signal(syscall.Signal(0)) != nil {
		return false
	}
	return pidAlive(p.pid) && exeIs(p.pid, p.bin)
}

// waitGone polls until the process exits or timeout elapses; it reports
// whether the process is gone.
func (p *dumberProc) waitGone(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for p.alive() {
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(stopPoll)
	}
	return true
}

// stopDumber sends SIGTERM, waits up to timeout, then SIGKILLs. It reports
// whether SIGKILL was needed, and an error if the process survived SIGKILL.
// Waiting keeps the sandbox removable: CEF writes its profile on shutdown.
func stopDumber(p *dumberProc, timeout time.Duration) (killed bool, err error) {
	if err := p.signal(syscall.SIGTERM); errors.Is(err, os.ErrProcessDone) {
		return false, nil
	}
	if p.waitGone(timeout) {
		return false, nil
	}
	_ = p.signal(syscall.SIGKILL)
	if !p.waitGone(killTimeout) {
		return true, fmt.Errorf("pid %d still alive %s after SIGKILL", p.pid, killTimeout)
	}
	return true, nil
}

// stopAll stops every surviving browser process started from bin.
func stopAll(t *testing.T, bin string) {
	for _, p := range findBrowserProcs(bin) {
		if _, err := stopDumber(p, stopTimeout); err != nil {
			t.Errorf("stop leftover dumber: %v", err)
		}
	}
}

// relayOpen runs `dumber browse url` in the sandbox env with a 20s timeout.
// The session bus points nowhere, so if the relay is unreachable and dumber
// falls back to starting a GUI, that GUI cannot reach any real bus.
func relayOpen(sb *sandbox, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), relayTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, sb.bin, "browse", url)
	cmd.Env = append(sb.env(), "DBUS_SESSION_BUS_ADDRESS="+deadBus)
	return cmd.Run()
}

// pidAlive reports whether pid exists and is not a zombie.
func pidAlive(pid int) bool {
	b, err := os.ReadFile(filepath.Join(procDir, strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	state, ok := statState(b)
	return ok && state != 'Z'
}

// statState returns the state field of /proc/<pid>/stat. The comm field can
// hold ')' itself, so the state follows the last ')'.
func statState(b []byte) (byte, bool) {
	i := bytes.LastIndexByte(b, ')')
	if i < 0 || i+2 >= len(b) {
		return 0, false
	}
	return b[i+2], true
}
