package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type browserWindowOpenerFunc func(context.Context, string) error

func (f browserWindowOpenerFunc) OpenFreshWindow(ctx context.Context, url string) error {
	return f(ctx, url)
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()

	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, time.Second, 10*time.Millisecond)
}

func waitForSocketGone(t *testing.T, path string) {
	t.Helper()

	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return os.IsNotExist(err)
	}, time.Second, 10*time.Millisecond)
}

func shortTempDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "dbr-")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	return dir
}

func testIPC(root string) runtimeprofile.IPCPaths {
	runtimeDir := filepath.Join(root, "runtime")
	return runtimeprofile.IPCPaths{
		RuntimeDir:          runtimeDir,
		BrowserLaunchSocket: filepath.Join(runtimeDir, browserLaunchSocketName),
	}
}

func testNamespacedIPC(root, engine string) runtimeprofile.IPCPaths {
	runtimeDir := filepath.Join(root, "runtime", engine)
	return runtimeprofile.IPCPaths{
		RuntimeDir:          runtimeDir,
		BrowserLaunchSocket: filepath.Join(runtimeDir, browserLaunchSocketName),
	}
}

func overrideBrowserLaunchRequestID(t *testing.T, id string) func() {
	t.Helper()
	previous := newBrowserLaunchRequestID
	newBrowserLaunchRequestID = func() string { return id }
	return func() { newBrowserLaunchRequestID = previous }
}

func TestBrowserLaunchRelay_DeliverOpenFreshWindow_MissingListenerReturnsFalseNil(t *testing.T) {
	relay := NewBrowserLaunchRelay(testIPC(shortTempDir(t)))

	delivered, err := relay.DeliverOpenFreshWindow(context.Background(), "https://example.com")

	require.NoError(t, err)
	assert.False(t, delivered)
}

func TestBrowserLaunchRelay_SameProfileAndEngine_UsesSameSocket(t *testing.T) {
	ipc := testNamespacedIPC(shortTempDir(t), "cef")
	listenerRelay := NewBrowserLaunchRelay(ipc)
	clientRelay := NewBrowserLaunchRelay(ipc)

	received := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := listenerRelay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, url string) error {
		received <- url
		return nil
	}))
	require.NoError(t, err)
	defer closer.Close()

	waitForSocket(t, ipc.BrowserLaunchSocket)

	delivered, err := clientRelay.DeliverOpenFreshWindow(context.Background(), "https://example.com/same-namespace")
	require.NoError(t, err)
	require.True(t, delivered)
	require.Equal(t, "https://example.com/same-namespace", <-received)
}

func TestBrowserLaunchRelay_DevCEFAndDevWebKit_UseDifferentSockets(t *testing.T) {
	root := t.TempDir()
	cefIPC := testNamespacedIPC(root, "cef")
	wkIPC := testNamespacedIPC(root, "webkit")
	require.NotEqual(t, cefIPC.BrowserLaunchSocket, wkIPC.BrowserLaunchSocket)
}

func TestBrowserLaunchRelay_DeliverOpenFreshWindow_SendsRequestIDAndAcceptsMatchingResponse(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	relay := NewBrowserLaunchRelay(ipc)
	restore := overrideBrowserLaunchRequestID(t, "request-test-1")
	defer restore()

	receivedRequest := make(chan browserLaunchRequest, 1)
	require.NoError(t, os.MkdirAll(ipc.RuntimeDir, 0o700))
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: ipc.BrowserLaunchSocket, Net: "unix"})
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
		_ = os.Remove(ipc.BrowserLaunchSocket)
	}()

	go func() {
		conn, acceptErr := listener.AcceptUnix()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		var request browserLaunchRequest
		if decodeErr := json.NewDecoder(conn).Decode(&request); decodeErr != nil {
			return
		}
		receivedRequest <- request
		_ = json.NewEncoder(conn).Encode(browserLaunchResponse{RequestID: request.RequestID, Accepted: true})
	}()

	delivered, err := relay.DeliverOpenFreshWindow(context.Background(), "https://example.com/request-id")

	require.NoError(t, err)
	require.True(t, delivered)
	request := <-receivedRequest
	require.Equal(t, "request-test-1", request.RequestID)
	require.Equal(t, "https://example.com/request-id", request.URL)
}

func TestBrowserLaunchRelay_DeliverOpenFreshWindow_RejectsMismatchedResponseRequestID(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	relay := NewBrowserLaunchRelay(ipc)
	restore := overrideBrowserLaunchRequestID(t, "request-test-2")
	defer restore()

	require.NoError(t, os.MkdirAll(ipc.RuntimeDir, 0o700))
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: ipc.BrowserLaunchSocket, Net: "unix"})
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
		_ = os.Remove(ipc.BrowserLaunchSocket)
	}()

	go func() {
		conn, acceptErr := listener.AcceptUnix()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		var request browserLaunchRequest
		if decodeErr := json.NewDecoder(conn).Decode(&request); decodeErr != nil {
			return
		}
		_ = json.NewEncoder(conn).Encode(browserLaunchResponse{RequestID: "different-request", Accepted: true})
	}()

	delivered, err := relay.DeliverOpenFreshWindow(context.Background(), "https://example.com/request-id-mismatch")

	require.Error(t, err)
	require.True(t, delivered)
	require.Contains(t, err.Error(), "mismatched browser launch relay response")
}

func TestBrowserLaunchRelay_DeliverOpenFreshWindow_RoundTrip(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	relay := NewBrowserLaunchRelay(ipc)

	received := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, url string) error {
		received <- url
		return nil
	}))
	require.NoError(t, err)
	defer closer.Close()

	waitForSocket(t, ipc.BrowserLaunchSocket)

	delivered, err := relay.DeliverOpenFreshWindow(context.Background(), "https://example.com/new")

	require.NoError(t, err)
	assert.True(t, delivered)

	select {
	case got := <-received:
		assert.Equal(t, "https://example.com/new", got)
	case <-time.After(time.Second):
		t.Fatal("expected opener to receive the URL")
	}
}

func TestBrowserLaunchRelay_RequiresBrowserLaunchSocketPath(t *testing.T) {
	relay := NewBrowserLaunchRelay(runtimeprofile.IPCPaths{})

	_, err := relay.Listen(context.Background(), browserWindowOpenerFunc(func(context.Context, string) error { return nil }))
	require.Error(t, err)
	require.Contains(t, err.Error(), "browser launch socket path")
}

func TestBrowserLaunchRelay_SecondListenWhileLiveFailsWithoutReplacingListener(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	relay := NewBrowserLaunchRelay(ipc)

	firstReceived := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, url string) error {
		firstReceived <- url
		return nil
	}))
	require.NoError(t, err)
	defer closer.Close()

	waitForSocket(t, ipc.BrowserLaunchSocket)

	_, err = relay.Listen(ctx, browserWindowOpenerFunc(func(context.Context, string) error { return nil }))
	require.Error(t, err)

	delivered, err := relay.DeliverOpenFreshWindow(context.Background(), "https://example.com/live")
	require.NoError(t, err)
	require.True(t, delivered)

	select {
	case got := <-firstReceived:
		assert.Equal(t, "https://example.com/live", got)
	case <-time.After(time.Second):
		t.Fatal("expected the original listener to remain active")
	}
}

func TestBrowserLaunchRelay_RebindsStaleSocketPath(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	relay := NewBrowserLaunchRelay(ipc)

	require.NoError(t, os.MkdirAll(filepath.Dir(ipc.BrowserLaunchSocket), 0o700))
	require.NoError(t, os.WriteFile(ipc.BrowserLaunchSocket, []byte("stale"), 0o600))

	received := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, url string) error {
		received <- url
		return nil
	}))
	require.NoError(t, err)
	defer closer.Close()

	waitForSocket(t, ipc.BrowserLaunchSocket)

	delivered, err := relay.DeliverOpenFreshWindow(context.Background(), "https://example.com/stale")
	require.NoError(t, err)
	require.True(t, delivered)

	select {
	case got := <-received:
		assert.Equal(t, "https://example.com/stale", got)
	case <-time.After(time.Second):
		t.Fatal("expected stale socket path to be rebound")
	}
}

func TestBrowserLaunchRelay_RejectsMalformedPayloads(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	relay := NewBrowserLaunchRelay(ipc)

	received := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, url string) error {
		received <- url
		return nil
	}))
	require.NoError(t, err)
	defer closer.Close()

	waitForSocket(t, ipc.BrowserLaunchSocket)

	conn, err := net.Dial("unix", ipc.BrowserLaunchSocket)
	require.NoError(t, err)

	_, err = conn.Write([]byte("not-json"))
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	delivered, err := relay.DeliverOpenFreshWindow(context.Background(), "https://example.com/still-works")

	require.NoError(t, err)
	assert.True(t, delivered)

	select {
	case got := <-received:
		assert.Equal(t, "https://example.com/still-works", got)
	case <-time.After(time.Second):
		t.Fatal("expected opener to receive the URL after malformed payload")
	}
}

func TestBrowserLaunchRelay_CloseRemovesSocketPath(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	relay := NewBrowserLaunchRelay(ipc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(context.Context, string) error { return nil }))
	require.NoError(t, err)

	waitForSocket(t, ipc.BrowserLaunchSocket)

	require.NoError(t, closer.Close())
	waitForSocketGone(t, ipc.BrowserLaunchSocket)
}

func TestBrowserLaunchRelay_ContextCancelStopsServing(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	relay := NewBrowserLaunchRelay(ipc)

	ctx, cancel := context.WithCancel(context.Background())
	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(context.Context, string) error { return nil }))
	require.NoError(t, err)

	waitForSocket(t, ipc.BrowserLaunchSocket)

	cancel()
	waitForSocketGone(t, ipc.BrowserLaunchSocket)
	require.NoError(t, closer.Close())

	delivered, err := relay.DeliverOpenFreshWindow(context.Background(), "https://example.com/after-cancel")
	require.NoError(t, err)
	assert.False(t, delivered)
}

func TestBrowserLaunchRelay_DeliverOpenFreshWindow_RespectsResponseReadTimeout(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	require.NoError(t, os.MkdirAll(ipc.RuntimeDir, 0o700))

	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: ipc.BrowserLaunchSocket, Net: "unix"})
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
		_ = os.Remove(ipc.BrowserLaunchSocket)
	}()

	ready := make(chan struct{})
	go func() {
		close(ready)
		conn, acceptErr := listener.AcceptUnix()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		<-time.After(time.Second)
	}()
	<-ready

	relay := NewBrowserLaunchRelay(ipc)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	delivered, err := relay.DeliverOpenFreshWindow(ctx, "https://example.com/slow")
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, delivered)
}

func TestBrowserLaunchRelay_DeliverOpenFreshWindow_ReturnsUnconfirmedErrorOnNoDeadlineTimeout(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	require.NoError(t, os.MkdirAll(ipc.RuntimeDir, 0o700))

	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: ipc.BrowserLaunchSocket, Net: "unix"})
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
		_ = os.Remove(ipc.BrowserLaunchSocket)
	}()

	ready := make(chan struct{})
	go func() {
		close(ready)
		conn, acceptErr := listener.AcceptUnix()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		<-time.After(time.Second)
	}()
	<-ready

	relay := NewBrowserLaunchRelay(ipc)

	started := make(chan struct{})

	type deliverResult struct {
		delivered bool
		err       error
	}
	result := make(chan deliverResult, 1)
	go func() {
		close(started)
		delivered, err := relay.DeliverOpenFreshWindow(context.Background(), "https://example.com/slow")
		result <- deliverResult{delivered: delivered, err: err}
	}()
	<-started

	select {
	case got := <-result:
		assert.True(t, got.delivered)
		require.Error(t, got.err)
		require.ErrorIs(t, got.err, ErrBrowserLaunchRelayUnconfirmed)
	case <-time.After(browserLaunchIOTimeout * 3):
		t.Fatal("DeliverOpenFreshWindow blocked without a caller deadline")
	}
}

func TestBrowserLaunchRelay_AcknowledgesBeforeOpenerReturns(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	relay := NewBrowserLaunchRelay(ipc)

	started := make(chan struct{})
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, _ string) error {
		close(started)
		<-release
		return nil
	}))
	require.NoError(t, err)
	defer closer.Close()

	waitForSocket(t, ipc.BrowserLaunchSocket)

	conn, err := net.Dial("unix", ipc.BrowserLaunchSocket)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(300*time.Millisecond)))
	require.NoError(t, json.NewEncoder(conn).Encode(browserLaunchRequest{RequestID: "raw-request-1", URL: "https://example.com/slow-but-healthy"}))

	var response browserLaunchResponse
	require.NoError(t, json.NewDecoder(conn).Decode(&response))
	assert.Empty(t, response.Error)
	assert.True(t, response.Accepted)
	assert.Equal(t, "raw-request-1", response.RequestID)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected opener to be invoked after acknowledgement")
	}
	close(release)
}

func TestBrowserLaunchRelay_AcknowledgesAcceptedEvenWhenOpenerReturnsLateError(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	relay := NewBrowserLaunchRelay(ipc)

	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, _ string) error {
		close(started)
		return errors.New("late open failure")
	}))
	require.NoError(t, err)
	defer closer.Close()

	waitForSocket(t, ipc.BrowserLaunchSocket)

	conn, err := net.Dial("unix", ipc.BrowserLaunchSocket)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(300*time.Millisecond)))
	require.NoError(t, json.NewEncoder(conn).Encode(browserLaunchRequest{RequestID: "raw-request-2", URL: "https://example.com/fails-later"}))

	var response browserLaunchResponse
	require.NoError(t, json.NewDecoder(conn).Decode(&response))
	assert.Empty(t, response.Error)
	assert.True(t, response.Accepted)
	assert.Equal(t, "raw-request-2", response.RequestID)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected opener to be invoked after accepted acknowledgement")
	}
}

func TestBrowserLaunchRelay_SilentClientDoesNotStallListener(t *testing.T) {
	ipc := testIPC(shortTempDir(t))
	relay := NewBrowserLaunchRelay(ipc)

	received := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, url string) error {
		received <- url
		return nil
	}))
	require.NoError(t, err)
	defer closer.Close()

	waitForSocket(t, ipc.BrowserLaunchSocket)

	conn, err := net.Dial("unix", ipc.BrowserLaunchSocket)
	require.NoError(t, err)
	defer conn.Close()
	time.Sleep(20 * time.Millisecond)

	type deliverResult struct {
		delivered bool
		err       error
	}
	result := make(chan deliverResult, 1)
	go func() {
		delivered, err := relay.DeliverOpenFreshWindow(context.Background(), "https://example.com/recovered")
		result <- deliverResult{delivered: delivered, err: err}
	}()

	select {
	case got := <-result:
		require.NoError(t, got.err)
		require.True(t, got.delivered)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("silent client stalled the relay listener")
	}

	select {
	case got := <-received:
		assert.Equal(t, "https://example.com/recovered", got)
	case <-time.After(time.Second):
		t.Fatal("expected opener to receive the URL")
	}
}
