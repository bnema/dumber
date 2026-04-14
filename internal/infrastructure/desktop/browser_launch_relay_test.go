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

func (x testXDGPaths) ConfigDir() (string, error)      { return "", nil }
func (x testXDGPaths) DataDir() (string, error)        { return "", nil }
func (x testXDGPaths) StateDir() (string, error)       { return x.stateDir, nil }
func (x testXDGPaths) RuntimeDir() (string, error)     { return x.runtimeDir, nil }
func (x testXDGPaths) CacheDir() (string, error)       { return "", nil }
func (x testXDGPaths) FilterJSONDir() (string, error)  { return "", nil }
func (x testXDGPaths) FilterStoreDir() (string, error) { return "", nil }
func (x testXDGPaths) FilterCacheDir() (string, error) { return "", nil }
func (x testXDGPaths) ManDir() (string, error)         { return "", nil }
func (x testXDGPaths) DownloadDir() (string, error)    { return "", nil }

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

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(ctx context.Context, url string) error {
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

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(ctx context.Context, url string) error {
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

	closer, err := relay.Listen(ctx, browserWindowOpenerFunc(func(ctx context.Context, url string) error {
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
