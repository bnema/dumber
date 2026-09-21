package desktop

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

type prepareTestCloser struct{}

func (*prepareTestCloser) Close() error { return nil }

func TestBrowserLaunchRelayPrepareReservesUntilListen(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	relay := NewBrowserLaunchRelay(ipc)
	reservation := &prepareTestCloser{}
	err := relay.(port.BrowserLaunchRelayPreparer).Prepare(reservation)
	require.NoError(t, err)
	defer reservation.Close()

	// A competing process must not steal the socket before GTK is ready.
	competitor := NewBrowserLaunchRelay(ipc)
	err = competitor.(port.BrowserLaunchRelayPreparer).Prepare(&prepareTestCloser{})
	require.Error(t, err)

	// Bound is not ready: requests must not be acknowledged yet.
	conn, err := net.Dial("unix", ipc.BrowserLaunchSocket)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Write([]byte("{\"url\":\"https://example.com\"}\n"))
	require.NoError(t, err)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(30*time.Millisecond)))
	var response [1]byte
	_, err = conn.Read(response[:])
	require.Error(t, err)
	var timeout net.Error
	require.ErrorAs(t, err, &timeout)
	require.True(t, timeout.Timeout())

	received := make(chan string, 2)
	closer, err := relay.Listen(t.Context(), browserWindowOpenerFunc(func(_ context.Context, url string) error {
		received <- url
		return nil
	}))
	require.NoError(t, err)
	require.NotSame(t, reservation, closer)
	select {
	case url := <-received:
		require.Equal(t, "https://example.com", url)
	case <-time.After(time.Second):
		t.Fatal("reserved socket was not activated")
	}
	require.NoError(t, closer.Close())
	require.NoError(t, reservation.Close())
	require.NoFileExists(t, ipc.BrowserLaunchSocket)
}

func TestBrowserLaunchRelayPrepareRecoversStalePathWhileLeaseHeld(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	require.NoError(t, os.MkdirAll(filepath.Dir(ipc.BrowserLaunchSocket), 0o700))
	require.NoError(t, os.WriteFile(ipc.BrowserLaunchSocket, []byte("stale"), 0o600))
	relay := NewBrowserLaunchRelay(ipc)
	err := relay.(port.BrowserLaunchRelayPreparer).Prepare(&prepareTestCloser{})
	require.NoError(t, err)
	info, err := os.Stat(ipc.BrowserLaunchSocket)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSocket)
}

func TestBrowserLaunchRelayListenerStopsIndependentlyOfLease(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	relay := NewBrowserLaunchRelay(ipc)
	lease := &prepareTestCloser{}
	require.NoError(t, relay.(port.BrowserLaunchRelayPreparer).Prepare(lease))
	listener, err := relay.Listen(t.Context(), browserWindowOpenerFunc(func(context.Context, string) error { return nil }))
	require.NoError(t, err)

	// Closing the listener unlinks its socket but does not own or release lease.
	require.NoError(t, listener.Close())
	require.NoFileExists(t, ipc.BrowserLaunchSocket)
	require.NoError(t, lease.Close())

	replacement := NewBrowserLaunchRelay(ipc)
	require.NoError(t, replacement.(port.BrowserLaunchRelayPreparer).Prepare(&prepareTestCloser{}))
	require.FileExists(t, ipc.BrowserLaunchSocket)
	// Idempotent old close must not unlink the successor socket.
	require.NoError(t, listener.Close())
	require.FileExists(t, ipc.BrowserLaunchSocket)
}
