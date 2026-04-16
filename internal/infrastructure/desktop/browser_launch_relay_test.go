package desktop

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testXDGPaths struct {
	runtimeDir string
	stateDir   string
}

var _ port.XDGPaths = testXDGPaths{}

func (testXDGPaths) ConfigDir() (string, error)      { return "", nil }
func (testXDGPaths) DataDir() (string, error)        { return "", nil }
func (x testXDGPaths) StateDir() (string, error)     { return x.stateDir, nil }
func (x testXDGPaths) RuntimeDir() (string, error)   { return x.runtimeDir, nil }
func (testXDGPaths) CacheDir() (string, error)       { return "", nil }
func (testXDGPaths) FilterJSONDir() (string, error)  { return "", nil }
func (testXDGPaths) FilterStoreDir() (string, error) { return "", nil }
func (testXDGPaths) FilterCacheDir() (string, error) { return "", nil }
func (testXDGPaths) ManDir() (string, error)         { return "", nil }
func (testXDGPaths) DownloadDir() (string, error)    { return "", nil }

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

func TestBrowserLaunchRelay_DeliverOpenFreshWindow_MissingListenerReturnsFalseNil(t *testing.T) {
	relay := NewBrowserLaunchRelay(testXDGPaths{runtimeDir: shortTempDir(t)})

	delivered, err := relay.DeliverOpenFreshWindow(context.Background(), "https://example.com")

	require.NoError(t, err)
	assert.False(t, delivered)
}

func TestBrowserLaunchRelay_DeliverOpenFreshWindow_RoundTrip(t *testing.T) {
	runtimeDir := filepath.Join(shortTempDir(t), "runtime")
	relay := NewBrowserLaunchRelay(testXDGPaths{runtimeDir: runtimeDir})

	received := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, url string) error {
		received <- url
		return nil
	}))
	require.NoError(t, err)
	defer closer.Close()

	waitForSocket(t, filepath.Join(runtimeDir, "browser-launch.sock"))

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

func TestBrowserLaunchRelay_UsesStateFallbackWhenRuntimeDirMissing(t *testing.T) {
	stateDir := shortTempDir(t)
	relay := NewBrowserLaunchRelay(testXDGPaths{stateDir: stateDir})

	received := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, url string) error {
		received <- url
		return nil
	}))
	require.NoError(t, err)
	defer closer.Close()

	socketPath := filepath.Join(stateDir, "runtime", "browser-launch.sock")
	waitForSocket(t, socketPath)

	delivered, err := relay.DeliverOpenFreshWindow(context.Background(), "https://example.com/fallback")

	require.NoError(t, err)
	assert.True(t, delivered)

	select {
	case got := <-received:
		assert.Equal(t, "https://example.com/fallback", got)
	case <-time.After(time.Second):
		t.Fatal("expected opener to receive the URL")
	}
}

func TestBrowserLaunchRelay_SecondListenWhileLiveFailsWithoutReplacingListener(t *testing.T) {
	runtimeDir := filepath.Join(shortTempDir(t), "runtime")
	relay := NewBrowserLaunchRelay(testXDGPaths{runtimeDir: runtimeDir})

	firstReceived := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, url string) error {
		firstReceived <- url
		return nil
	}))
	require.NoError(t, err)
	defer closer.Close()

	waitForSocket(t, filepath.Join(runtimeDir, "browser-launch.sock"))

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
	runtimeDir := filepath.Join(shortTempDir(t), "runtime")
	relay := NewBrowserLaunchRelay(testXDGPaths{runtimeDir: runtimeDir})

	socketPath := filepath.Join(runtimeDir, "browser-launch.sock")
	require.NoError(t, os.MkdirAll(runtimeDir, 0o700))
	require.NoError(t, os.WriteFile(socketPath, []byte("stale"), 0o600))

	received := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, url string) error {
		received <- url
		return nil
	}))
	require.NoError(t, err)
	defer closer.Close()

	waitForSocket(t, socketPath)

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
	runtimeDir := filepath.Join(shortTempDir(t), "runtime")
	relay := NewBrowserLaunchRelay(testXDGPaths{runtimeDir: runtimeDir})

	received := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, url string) error {
		received <- url
		return nil
	}))
	require.NoError(t, err)
	defer closer.Close()

	socketPath := filepath.Join(runtimeDir, "browser-launch.sock")
	waitForSocket(t, socketPath)

	conn, err := net.Dial("unix", socketPath)
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
	runtimeDir := filepath.Join(shortTempDir(t), "runtime")
	relay := NewBrowserLaunchRelay(testXDGPaths{runtimeDir: runtimeDir})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(context.Context, string) error { return nil }))
	require.NoError(t, err)

	socketPath := filepath.Join(runtimeDir, "browser-launch.sock")
	waitForSocket(t, socketPath)

	require.NoError(t, closer.Close())
	waitForSocketGone(t, socketPath)
}

func TestBrowserLaunchRelay_ContextCancelStopsServing(t *testing.T) {
	runtimeDir := filepath.Join(shortTempDir(t), "runtime")
	relay := NewBrowserLaunchRelay(testXDGPaths{runtimeDir: runtimeDir})

	ctx, cancel := context.WithCancel(context.Background())
	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(context.Context, string) error { return nil }))
	require.NoError(t, err)

	socketPath := filepath.Join(runtimeDir, "browser-launch.sock")
	waitForSocket(t, socketPath)

	cancel()
	waitForSocketGone(t, socketPath)
	require.NoError(t, closer.Close())

	delivered, err := relay.DeliverOpenFreshWindow(context.Background(), "https://example.com/after-cancel")
	require.NoError(t, err)
	assert.False(t, delivered)
}

func TestBrowserLaunchRelay_DeliverOpenFreshWindow_RespectsResponseReadTimeout(t *testing.T) {
	runtimeDir := shortTempDir(t)
	socketPath := filepath.Join(runtimeDir, "browser-launch.sock")
	require.NoError(t, os.MkdirAll(runtimeDir, 0o700))

	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
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

	relay := NewBrowserLaunchRelay(testXDGPaths{runtimeDir: runtimeDir})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	delivered, err := relay.DeliverOpenFreshWindow(ctx, "https://example.com/slow")
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, delivered)
}

func TestBrowserLaunchRelay_DeliverOpenFreshWindow_ReturnsTrueOnNoDeadlineTimeout(t *testing.T) {
	runtimeDir := shortTempDir(t)
	socketPath := filepath.Join(runtimeDir, "browser-launch.sock")
	require.NoError(t, os.MkdirAll(runtimeDir, 0o700))

	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
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

	relay := NewBrowserLaunchRelay(testXDGPaths{runtimeDir: runtimeDir})

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
		require.NoError(t, got.err)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("DeliverOpenFreshWindow blocked without a caller deadline")
	}
}

func TestBrowserLaunchRelay_AcknowledgesBeforeOpenerReturns(t *testing.T) {
	runtimeDir := filepath.Join(shortTempDir(t), "runtime")
	relay := NewBrowserLaunchRelay(testXDGPaths{runtimeDir: runtimeDir})

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

	waitForSocket(t, filepath.Join(runtimeDir, "browser-launch.sock"))

	conn, err := net.Dial("unix", filepath.Join(runtimeDir, "browser-launch.sock"))
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(300*time.Millisecond)))
	require.NoError(t, json.NewEncoder(conn).Encode(browserLaunchRequest{URL: "https://example.com/slow-but-healthy"}))

	var response browserLaunchResponse
	require.NoError(t, json.NewDecoder(conn).Decode(&response))
	assert.Empty(t, response.Error)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected opener to be invoked after acknowledgement")
	}
	close(release)
}

func TestBrowserLaunchRelay_SilentClientDoesNotStallListener(t *testing.T) {
	runtimeDir := filepath.Join(shortTempDir(t), "runtime")
	relay := NewBrowserLaunchRelay(testXDGPaths{runtimeDir: runtimeDir})

	received := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(_ context.Context, url string) error {
		received <- url
		return nil
	}))
	require.NoError(t, err)
	defer closer.Close()

	socketPath := filepath.Join(runtimeDir, "browser-launch.sock")
	waitForSocket(t, socketPath)

	conn, err := net.Dial("unix", socketPath)
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
