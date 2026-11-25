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
	w := NewWatcher(dir)
	var userCSS, home int
	paths, onChange, err := w.dispatcher(port.SystemViewFileHandlers{
		OnUserCSS:    func() { userCSS++ },
		OnHomeConfig: func() { home++ },
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{filepath.Join(dir, "user.css"), filepath.Join(dir, "home.toml")}, paths)

	onChange(filepath.Join(dir, "home.toml"))
	assert.Equal(t, 1, home)
	assert.Zero(t, userCSS)

	onChange(filepath.Join(dir, "user.css"))
	onChange(filepath.Join(dir, "other.toml"))
	assert.Equal(t, 1, userCSS)
	assert.Equal(t, 1, home)
}

// TestWatcher_SmokeDispatch runs the real fsnotify path once.
func TestWatcher_SmokeDispatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := &Watcher{configDir: dir, watcher: filewatch.New(20 * time.Millisecond)}
	home := make(chan struct{}, 4)
	require.NoError(t, w.Start(context.Background(), port.SystemViewFileHandlers{
		OnHomeConfig: func() { home <- struct{}{} },
	}))
	t.Cleanup(func() { _ = w.Stop() })

	require.NoError(t, os.WriteFile(filepath.Join(dir, "home.toml"), []byte("x"), 0o600))
	select {
	case <-home:
	case <-time.After(5 * time.Second):
		t.Fatal("home.toml change not dispatched")
	}
}

func TestWatcher_StartWithoutHandlersFails(t *testing.T) {
	t.Parallel()
	w := NewWatcher(t.TempDir())
	require.Error(t, w.Start(context.Background(), port.SystemViewFileHandlers{}))
}
