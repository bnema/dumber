package systemviewfiles

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/infrastructure/filewatch"
)

func TestUserStyleRead_MissingFileIsEmpty(t *testing.T) {
	t.Parallel()
	css, err := NewUserStyle(t.TempDir()).Read()
	require.NoError(t, err)
	assert.Empty(t, css)
}

func TestUserStyleRead_CapsSize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	style := NewUserStyle(dir)
	require.Equal(t, filepath.Join(dir, "user.css"), style.Path())
	require.NoError(t, os.WriteFile(style.Path(), []byte(strings.Repeat("a", MaxUserCSSBytes+10)), 0o600))

	css, err := style.Read()
	require.NoError(t, err)
	assert.Len(t, css, MaxUserCSSBytes)
}

func TestWatcher_DispatchesByPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := &Watcher{configDir: dir, watcher: filewatch.New(20 * time.Millisecond)}
	userCSS := make(chan struct{}, 4)
	home := make(chan struct{}, 4)
	require.NoError(t, w.Start(context.Background(), port.SystemViewFileHandlers{
		OnUserCSS:    func() { userCSS <- struct{}{} },
		OnHomeConfig: func() { home <- struct{}{} },
	}))
	t.Cleanup(func() { _ = w.Stop() })

	require.NoError(t, os.WriteFile(filepath.Join(dir, "home.toml"), []byte("x"), 0o600))
	waitFor(t, home, "home.toml")
	assertQuiet(t, userCSS, "user.css")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "user.css"), []byte("body{}"), 0o600))
	waitFor(t, userCSS, "user.css")
	assertQuiet(t, home, "home.toml")
}

func TestWatcher_StartWithoutHandlersFails(t *testing.T) {
	t.Parallel()
	w := NewWatcher(t.TempDir())
	require.Error(t, w.Start(context.Background(), port.SystemViewFileHandlers{}))
}

func waitFor(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s change not dispatched", name)
	}
}

func assertQuiet(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("unexpected %s dispatch", name)
	case <-time.After(150 * time.Millisecond):
	}
}
