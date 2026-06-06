package idle

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestWatchForResponseCleanupSurvivesConnFieldCleared(t *testing.T) {
	// clearConnOnDoneContext clears inhibitor.conn after watchForResponse has
	// installed its cleanup defer; the test fails if cleanup dereferences the
	// mutable field instead of the D-Bus connection snapshot.
	conn := connectPrivateSessionBus(t)
	defer conn.Close()

	inhibitor := &PortalInhibitor{conn: conn}
	ctx := &clearConnOnDoneContext{
		Context: context.Background(),
		done:    closedDoneChannel(),
		clear: func() {
			inhibitor.mu.Lock()
			inhibitor.conn = nil
			inhibitor.mu.Unlock()
		},
	}

	inhibitor.watchForResponse(ctx, "/org/freedesktop/portal/desktop/request/dumber/test")

	inhibitor.mu.Lock()
	connCleared := inhibitor.conn == nil
	inhibitor.mu.Unlock()
	if !connCleared {
		t.Fatal("watchForResponse returned before observing context cancellation")
	}
}

func connectPrivateSessionBus(t *testing.T) *dbus.Conn {
	t.Helper()

	address := startPrivateSessionBus(t)
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatalf("connect to private D-Bus session bus: %v", err)
	}
	return conn
}

func startPrivateSessionBus(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skipf("dbus-daemon unavailable: %v", err)
	}

	var stderr bytes.Buffer
	cmd := exec.Command("dbus-daemon", "--session", "--nofork", "--print-address=1")
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

type clearConnOnDoneContext struct {
	context.Context
	done  <-chan struct{}
	once  sync.Once
	clear func()
}

func (c *clearConnOnDoneContext) Done() <-chan struct{} {
	c.once.Do(c.clear)
	return c.done
}

func closedDoneChannel() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}
