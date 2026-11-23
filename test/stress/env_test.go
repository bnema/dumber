//go:build stress

package stress

import (
	"image"
	"image/color"
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
		"GITHUB_TOKEN=secret", "AWS_SECRET_ACCESS_KEY=secret", "SSH_AUTH_SOCK=/run/ssh",
		"DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1/bus", "DBUS_SYSTEM_BUS_ADDRESS=unix:path=/run/dbus",
		"WAYLAND_SOCKET=3", "LANG=C.UTF-8", "LC_ALL=C", "TZ=UTC", "VK_ICD_FILENAMES=/x.json",
		"MESA_DEBUG=1", "LIBVA_DRIVER_NAME=iHD", "__GLX_VENDOR_LIBRARY_NAME=nvidia", "DRI_PRIME=1",
	}
	env := sandboxEnv(base, "/tmp/dst-1", "wayland-9")
	got := map[string][]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		got[k] = append(got[k], v)
	}
	for _, k := range []string{
		"DISPLAY", "ENV", "GODEBUG", "DUMBER_FOO", "GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY", "SSH_AUTH_SOCK",
		"DBUS_SESSION_BUS_ADDRESS", "DBUS_SYSTEM_BUS_ADDRESS", "WAYLAND_SOCKET",
	} {
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
		"LANG": "C.UTF-8", "LC_ALL": "C", "TZ": "UTC", "VK_ICD_FILENAMES": "/x.json", "MESA_DEBUG": "1",
		"LIBVA_DRIVER_NAME": "iHD", "__GLX_VENDOR_LIBRARY_NAME": "nvidia", "DRI_PRIME": "1",
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
	startNeferwl(t, sb, sessionID(t), 5*time.Minute)
	t.Cleanup(func() { stopAll(t, sb.bin) })
	p := launchDumber(t, sb, ps.url(hostIP, "/page"), filepath.Join(sb.root, "out"))
	if err := relayOpen(sb, ps.url(hostLocal, "/page")); err != nil {
		t.Fatalf("relay open: %v", err)
	}
	if !p.alive() {
		t.Fatalf("dumber pid %d died", p.pid)
	}
	if n := len(findBrowserProcs(sb.bin)); n != 1 {
		t.Fatalf("relay open left %d browser processes, want 1", n)
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
	if err := os.Mkdir(filepath.Join(root, "keep-me"), dirPerm); err != nil {
		t.Fatal(err)
	}
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
	if strings.Join(got, ",") != "20260103-000000,20260104-000000,20260105-000000,keep-me" {
		t.Fatalf("kept %v", got)
	}
}

func TestValidShellName(t *testing.T) {
	for name, want := range map[string]bool{
		"FOO_1": true, "_X": true, "A-B": false, "1A": false, "a b": false, "": false, "BASH_FUNC_x%%": false,
	} {
		if got := validShellName(name); got != want {
			t.Errorf("validShellName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestMatchRatio(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := range 10 {
		for x := range 10 {
			c := color.RGBA{A: 0xff}
			if x < 3 {
				c = color.RGBA{R: 0x2e + 5, G: 0x8b - 5, B: 0x57, A: 0xff}
			}
			img.Set(x, y, c)
		}
	}
	if got := matchRatio(img, pageColor, shotTolerance); got != 0.3 {
		t.Fatalf("matchRatio = %v, want 0.3", got)
	}
	if got := matchRatio(img, pageColor, 4); got != 0 {
		t.Fatalf("matchRatio tol 4 = %v, want 0", got)
	}
}

func TestStatState(t *testing.T) {
	for in, want := range map[string]byte{
		"123 (dumber) S 1 2":  'S',
		"123 (we) ird) Z 1 2": 'Z',
		"123 (a b)) R 1":      'R',
	} {
		if got, ok := statState([]byte(in)); !ok || got != want {
			t.Errorf("statState(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := statState([]byte("123 (x")); ok {
		t.Error("want !ok for truncated stat")
	}
}

func TestSessionID(t *testing.T) {
	id := sessionID(t)
	if len(id) > sessionIDMax || !strings.HasPrefix(id, "stress-") {
		t.Fatalf("bad id %q", id)
	}
	for _, c := range id {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			t.Fatalf("bad char %q in %q", c, id)
		}
	}
}
