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

func TestWatcher_DispatchesUserCSSChangesOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := &Watcher{configDir: dir, watcher: filewatch.New(20 * time.Millisecond)}
	calls := make(chan struct{}, 4)
	require.NoError(t, w.Start(context.Background(), func() { calls <- struct{}{} }))
	t.Cleanup(func() { _ = w.Stop() })

	require.NoError(t, os.WriteFile(filepath.Join(dir, "home.toml"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "user.css"), []byte("body{}"), 0o600))

	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("user.css change not dispatched")
	}
	select {
	case <-calls:
		t.Fatal("unexpected extra dispatch (home.toml must be a no-op)")
	case <-time.After(200 * time.Millisecond):
	}
}
