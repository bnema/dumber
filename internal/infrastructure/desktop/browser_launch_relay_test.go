package desktop

import (
	"context"
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
