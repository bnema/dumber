//go:build stress

package stress

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSandboxEnv(t *testing.T) {
	base := []string{
		"PATH=/usr/bin", "HOME=/home/x", "DISPLAY=:0", "ENV=/etc/sh", "GODEBUG=x=1",
		"WAYLAND_DISPLAY=wayland-1", "DUMBER_FOO=1", "XDG_CONFIG_HOME=/c", "XDG_DATA_HOME=/d",
		"XDG_STATE_HOME=/s", "XDG_CACHE_HOME=/k", "XDG_RUNTIME_DIR=/run/user/1",
	}
	env := sandboxEnv(base, "/tmp/dst-1", "wayland-9")
	got := map[string][]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		got[k] = append(got[k], v)
	}
	for _, k := range []string{"DISPLAY", "ENV", "GODEBUG", "DUMBER_FOO"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s not removed", k)
		}
	}
	want := map[string]string{
		"PATH": "/usr/bin", "HOME": "/home/x", "XDG_RUNTIME_DIR": "/run/user/1",
		"XDG_CONFIG_HOME": "/tmp/dst-1/config", "XDG_DATA_HOME": "/tmp/dst-1/data",
		"XDG_STATE_HOME": "/tmp/dst-1/state", "XDG_CACHE_HOME": "/tmp/dst-1/cache",
		"WAYLAND_DISPLAY": "wayland-9", "GDK_BACKEND": "wayland", "GOTRACEBACK": "crash",
		"GOGC": "10", "DUMBER_RUNTIME_WATCH": "1",
	}
	for k, v := range want {
		if vs := got[k]; len(vs) != 1 || vs[0] != v {
			t.Errorf("%s = %v, want [%s]", k, vs, v)
		}
	}
}

func TestShellQuote(t *testing.T) {
	v := "it's a $HOME `x`"
	q := shellQuote(v)
	if q != `'it'\''s a $HOME `+"`x`"+`'` {
		t.Fatalf("quote = %s", q)
	}
	out, err := exec.Command("sh", "-c", "printf %s "+q).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != v {
		t.Fatalf("sh round trip = %q, want %q", out, v)
	}
}

func TestParseSessionWayland(t *testing.T) {
	wl, err := parseSessionWayland([]byte(`{"id":"stress-1","wayland":"wayland-7","control":"/run/c","display":""}` + "\n"))
	if err != nil || wl != "wayland-7" {
		t.Fatalf("got %q, %v", wl, err)
	}
	if _, err := parseSessionWayland([]byte(`{"id":"x"}`)); err == nil {
		t.Fatal("want error for missing wayland")
	}
	if _, err := parseSessionWayland([]byte(`not json`)); err == nil {
		t.Fatal("want error for bad json")
	}
}

// TestLaunchSmoke builds Dumber, launches it on a headless session, checks it
// stays alive, and relays a second URL to it. Requires DUMBER_STRESS=1.
func TestLaunchSmoke(t *testing.T) {
	requireTools(t)
	ps := newPageServer(t)
	sb := newSandbox(t)
	sb.bin = buildDumber(t, sb.root)
	startNeferwl(t, sb, sessionID(), 5*time.Minute)
	pid := launchDumber(t, sb, ps.url(hostIP, "/page"), filepath.Join(sb.root, "out"))
	t.Cleanup(func() { stopDumber(pid, stopTimeout) })
	if err := relayOpen(sb, ps.url(hostLocal, "/page")); err != nil {
		t.Fatalf("relay open: %v", err)
	}
	if !alive(pid) {
		t.Fatalf("dumber pid %d died", pid)
	}
}

func TestCheckSessionLog(t *testing.T) {
	ok := "{\"message\":\"runtime watch enabled\"}\n{\"message\":\"cef: repaired signal handler missing SA_ONSTACK\"}\n" +
		"{\"message\":\"starting GTK main loop\"}\n"
	if msgs := checkSessionLog(strings.NewReader(ok)); len(msgs) != 0 {
		t.Fatalf("pre-watch repair flagged: %v", msgs)
	}
	bad := "{\"message\":\"runtime watch enabled\"}\n{\"message\":\"starting GTK main loop\"}\n{\"message\":\"cef: repaired signal handler missing SA_ONSTACK\"}\n{\"message\":\"runtime: goroutine count keeps growing\"}\n"
	if msgs := checkSessionLog(strings.NewReader(bad)); len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %v", msgs)
	}
	if msgs := checkSessionLog(strings.NewReader("")); len(msgs) != 1 {
		t.Fatalf("want missing watch line, got %v", msgs)
	}
	noLoop := "{\"message\":\"runtime watch enabled\"}\n"
	if msgs := checkSessionLog(strings.NewReader(noLoop)); len(msgs) != 1 || !strings.Contains(msgs[0], "GTK main loop") {
		t.Fatalf("want missing main loop marker, got %v", msgs)
	}
}

func TestParseShotPath(t *testing.T) {
	p, err := parseShotPath([]byte(`{"shots":[{"output":"HEADLESS-1","path":"/tmp/a.png"}]}`))
	if err != nil || p != "/tmp/a.png" {
		t.Fatalf("got %q, %v", p, err)
	}
	if _, err := parseShotPath([]byte(`{"shots":[]}`)); err == nil {
		t.Fatal("want error for empty shots")
	}
}

func TestPruneArtifacts(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"20260101-000000", "20260102-000000", "20260103-000000", "20260104-000000", "20260105-000000"} {
		if err := os.Mkdir(filepath.Join(root, n), dirPerm); err != nil {
			t.Fatal(err)
		}
	}
	pruneArtifacts(t, root, 3)
	entries, _ := os.ReadDir(root)
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != "20260103-000000,20260104-000000,20260105-000000" {
		t.Fatalf("kept %v", got)
	}
}
