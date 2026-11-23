//go:build stress

package stress

import (
	"bytes"
	"context"
	"encoding/json"
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

// env returns the host environment without display/XDG/debug leaks, plus
// the sandbox XDG dirs and the session Wayland display.
func (sb *sandbox) env() []string {
	return sandboxEnv(os.Environ(), sb.root, sb.wayland)
}

func sandboxEnv(base []string, root, wayland string) []string {
	drop := map[string]bool{
		"DISPLAY": true, "ENV": true, "WAYLAND_DISPLAY": true, "GODEBUG": true,
		"XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true,
		"XDG_STATE_HOME": true, "XDG_CACHE_HOME": true,
	}
	env := make([]string, 0, len(base)+9)
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if drop[k] || strings.HasPrefix(k, "DUMBER_") {
			continue
		}
		env = append(env, kv)
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

// sessionID is the NeferWL session id for this test process.
func sessionID() string { return "stress-" + strconv.Itoa(os.Getpid()) }

// launchDumber spawns dumber on url in the session, waits for its window,
// and returns the browser PID. stdout/stderr go to logDir.
func launchDumber(t *testing.T, sb *sandbox, url, logDir string) int {
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
		t.Fatalf("neferwl spawn dumber: %v\n%s", err, out)
	}
	pids := findBrowserPIDs(sb.bin)
	if len(pids) != 1 {
		t.Fatalf("want exactly one dumber browser process, got %v", pids)
	}
	return pids[0]
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

// findBrowserPIDs walks /proc for the dumber browser process (not CEF subprocesses).
func findBrowserPIDs(bin string) []int {
	entries, _ := os.ReadDir(procDir)
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		dir := filepath.Join(procDir, e.Name())
		if exe, lerr := os.Readlink(filepath.Join(dir, "exe")); lerr != nil || exe != bin {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(dir, "cmdline"))
		if rerr != nil {
			continue
		}
		args := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		browse, sub := false, false
		for _, a := range args {
			browse = browse || a == "browse"
			sub = sub || strings.HasPrefix(a, "--type=")
		}
		if browse && !sub {
			pids = append(pids, pid)
		}
	}
	return pids
}

// relayOpen runs `dumber browse url` in the sandbox env with a 20s timeout.
func relayOpen(sb *sandbox, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), relayTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, sb.bin, "browse", url)
	cmd.Env = sb.env()
	return cmd.Run()
}

// alive reports whether pid exists and is not a zombie.
func alive(pid int) bool {
	b, err := os.ReadFile(filepath.Join(procDir, strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	// state follows the ")" closing the comm field.
	i := bytes.LastIndexByte(b, ')')
	if i < 0 || i+2 >= len(b) {
		return false
	}
	return b[i+2] != 'Z'
}

// stopDumber sends SIGTERM to pid, waits up to timeout for it to exit, then
// SIGKILLs it. It reports whether SIGKILL was needed. Waiting keeps the
// sandbox removable: CEF writes its profile on shutdown.
func stopDumber(pid int, timeout time.Duration) (killed bool) {
	_ = syscall.Kill(pid, syscall.SIGTERM)
	deadline := time.Now().Add(timeout)
	for alive(pid) && time.Now().Before(deadline) {
		time.Sleep(stopPoll)
	}
	if !alive(pid) {
		return false
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	for alive(pid) {
		time.Sleep(stopPoll)
	}
	return true
}
