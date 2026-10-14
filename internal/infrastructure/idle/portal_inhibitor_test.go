package idle

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnema/zerobus"
)

const testRequestPath = "/org/freedesktop/portal/desktop/request/dumber/test"

// portalCall is a method call received by the fake portal.
type portalCall struct {
	member string
	path   string
	flags  uint32
	reason string
}

// fakePortal serves the Inhibit portal on a private bus and reports every
// call it answers on calls, before replying.
type fakePortal struct {
	calls chan portalCall
	// endRequest makes the portal answer Close with an error, like a
	// portal that already ended the request with a Response signal.
	endRequest bool
	// failInhibit makes the portal reject Inhibit calls.
	failInhibit bool
}

func (f *fakePortal) serve(t *testing.T, conn *zerobus.Conn) {
	t.Helper()
	code, err := conn.RequestName(portalDest, zerobus.NameFlagDoNotQueue)
	if err != nil || code != zerobus.NamePrimaryOwner {
		t.Fatalf("RequestName = %d, %v", code, err)
	}
	go func() {
		for {
			m, err := conn.ReadMessage()
			if err != nil {
				return // connection closed at cleanup
			}
			if m.Type != zerobus.TypeMethodCall {
				continue
			}
			f.answer(conn, m)
		}
	}()
}

func (f *fakePortal) answer(conn *zerobus.Conn, m *zerobus.Message) {
	call := portalCall{member: strings.Clone(m.Member), path: strings.Clone(m.Path)}
	switch m.Member {
	case "Get":
		e := conn.NewReply(m, "v")
		e.Variant("u")
		e.Uint32(3)
	case "Inhibit":
		if f.failInhibit {
			conn.NewError(m, "org.freedesktop.DBus.Error.AccessDenied", "s").Str("denied")
			break
		}
		body := m.Body()
		body.Str() // window
		call.flags = body.Uint32()
		end := body.Array('{')
		for body.More(end) {
			body.Struct()
			key := strings.Clone(body.Str())
			sig := body.Variant()
			if key == "reason" && sig == "s" {
				call.reason = strings.Clone(body.Str())
				continue
			}
			body.Skip(sig)
		}
		conn.NewReply(m, "o").ObjectPath(testRequestPath)
	case "Close":
		if f.endRequest {
			conn.NewError(m, "org.freedesktop.DBus.Error.UnknownObject", "s").Str("request ended")
		} else {
			conn.NewReply(m, "")
		}
	default:
		conn.NewError(m, "org.freedesktop.DBus.Error.UnknownMethod", "")
	}
	// Record before replying so the call is visible once the caller returns.
	f.calls <- call
	_, _ = conn.Send()
}

func (f *fakePortal) next(t *testing.T) portalCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	default:
		t.Fatal("expected a portal call")
		return portalCall{}
	}
}

func (f *fakePortal) none(t *testing.T) {
	t.Helper()
	select {
	case c := <-f.calls:
		t.Fatalf("unexpected portal call %s on %s", c.member, c.path)
	default:
	}
}

// newTestInhibitor starts a private bus with a fake portal and returns an
// inhibitor connected to it.
func newTestInhibitor(t *testing.T, portal *fakePortal) *PortalInhibitor {
	t.Helper()
	address := startPrivateSessionBus(t)
	portal.calls = make(chan portalCall, 8)
	portal.serve(t, dial(t, address))

	p := newPortalInhibitor(context.Background(), dial(t, address))
	t.Cleanup(func() { _ = p.Close() })
	if c := portal.next(t); c.member != "Get" {
		t.Fatalf("first call = %s, want Get", c.member)
	}
	return p
}

func TestPortalInhibitorRefcountsOneRequest(t *testing.T) {
	portal := &fakePortal{}
	p := newTestInhibitor(t, portal)
	ctx := context.Background()

	if err := p.Inhibit(ctx, "video playing"); err != nil {
		t.Fatal(err)
	}
	c := portal.next(t)
	if c.member != "Inhibit" || c.flags != flagIdle|flagSuspend || c.reason != "video playing" {
		t.Fatalf("Inhibit call = %+v", c)
	}

	if err := p.Inhibit(ctx, "second video"); err != nil {
		t.Fatal(err)
	}
	if err := p.Uninhibit(ctx); err != nil {
		t.Fatal(err)
	}
	portal.none(t)
	if !p.IsInhibited() {
		t.Fatal("released while one inhibit is still held")
	}

	if err := p.Uninhibit(ctx); err != nil {
		t.Fatal(err)
	}
	if c := portal.next(t); c.member != "Close" || c.path != testRequestPath {
		t.Fatalf("release call = %+v", c)
	}
	if p.IsInhibited() {
		t.Fatal("still inhibited after the last Uninhibit")
	}
}

func TestPortalInhibitorToleratesEndedRequest(t *testing.T) {
	portal := &fakePortal{endRequest: true}
	p := newTestInhibitor(t, portal)
	ctx := context.Background()

	if err := p.Inhibit(ctx, "video playing"); err != nil {
		t.Fatal(err)
	}
	portal.next(t)
	if err := p.Uninhibit(ctx); err != nil {
		t.Fatalf("Uninhibit after the portal ended the request: %v", err)
	}
	portal.next(t)

	// The connection stays usable for the next inhibition.
	if err := p.Inhibit(ctx, "next video"); err != nil {
		t.Fatal(err)
	}
	if c := portal.next(t); c.member != "Inhibit" {
		t.Fatalf("call = %s, want Inhibit", c.member)
	}
}

func TestPortalInhibitorRollsBackFailedInhibit(t *testing.T) {
	portal := &fakePortal{failInhibit: true}
	p := newTestInhibitor(t, portal)

	if err := p.Inhibit(context.Background(), "video playing"); err == nil {
		t.Fatal("Inhibit succeeded although the portal rejected it")
	}
	portal.next(t)
	if p.IsInhibited() {
		t.Fatal("refcount kept after a failed Inhibit")
	}

	// Nothing to release: Uninhibit must not call Close.
	if err := p.Uninhibit(context.Background()); err != nil {
		t.Fatal(err)
	}
	portal.none(t)
}

func TestPortalInhibitorCloseReleasesActiveRequest(t *testing.T) {
	portal := &fakePortal{}
	p := newTestInhibitor(t, portal)

	if err := p.Inhibit(context.Background(), "video playing"); err != nil {
		t.Fatal(err)
	}
	portal.next(t)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if c := portal.next(t); c.member != "Close" || c.path != testRequestPath {
		t.Fatalf("Close call = %+v", c)
	}
	if p.IsInhibited() {
		t.Fatal("still inhibited after Close")
	}
}

func TestPortalInhibitorWithoutPortalIsNoop(t *testing.T) {
	p := newPortalInhibitor(context.Background(), dial(t, startPrivateSessionBus(t)))
	t.Cleanup(func() { _ = p.Close() })
	if p.conn != nil {
		t.Fatal("kept the connection although no portal answered")
	}

	ctx := context.Background()
	if err := p.Inhibit(ctx, "video playing"); err != nil {
		t.Fatal(err)
	}
	if !p.IsInhibited() {
		t.Fatal("refcount not tracked without a portal")
	}
	if err := p.Uninhibit(ctx); err != nil {
		t.Fatal(err)
	}
	if p.IsInhibited() {
		t.Fatal("still inhibited after Uninhibit")
	}
}

func dial(t *testing.T, address string) *zerobus.Conn {
	t.Helper()
	conn, err := zerobus.Dial(address)
	if err != nil {
		t.Fatalf("connect to private D-Bus session bus: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// privateBusConfig is a session bus without service directories, so it never
// auto-starts the real portal installed on the host.
const privateBusConfig = `<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>unix:tmpdir=/tmp</listen>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
`

func startPrivateSessionBus(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skipf("dbus-daemon unavailable: %v", err)
	}

	config := filepath.Join(t.TempDir(), "bus.conf")
	if err := os.WriteFile(config, []byte(privateBusConfig), 0o600); err != nil {
		t.Fatalf("write dbus-daemon config: %v", err)
	}

	var stderr bytes.Buffer
	cmd := exec.Command("dbus-daemon", "--config-file="+config, "--nofork", "--print-address=1")
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("open dbus-daemon stdout: %v", err)
	}
	err = cmd.Start()
	if err != nil {
		t.Fatalf("start dbus-daemon: %v", err)
	}

	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})

	address, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read dbus-daemon address: %v; stderr: %s", err, stderr.String())
	}
	return strings.TrimSpace(address)
}
