package homeconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/domain/entity"
)

const validHome = `[settings]
title = "Lab"
columns = 2

[[groups]]
name = "Homelab"
[[groups.services]]
id = "jellyfin"
name = "Jellyfin"
`

func write(t *testing.T, l *Loader, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(l.Path(), []byte(content), 0o600))
}

func TestLoad_MissingFileReturnsDefault(t *testing.T) {
	l := New(t.TempDir())
	snap, err := l.Load(context.Background())
	require.NoError(t, err)
	assert.False(t, snap.Exists)
	assert.Equal(t, entity.DefaultHomeLayout(), snap.Layout)
	assert.Equal(t, l.Path(), snap.Path)
}

func TestLoad_Valid(t *testing.T) {
	l := New(t.TempDir())
	write(t, l, validHome)
	snap, err := l.Load(context.Background())
	require.NoError(t, err)
	assert.True(t, snap.Exists)
	assert.Nil(t, snap.ParseError)
	assert.Empty(t, snap.ValidationErrors)
	assert.Equal(t, "Lab", snap.Layout.Settings.Title)
	_, ok := snap.Layout.FindService("jellyfin")
	assert.True(t, ok)
}

func TestLoad_SyntaxErrorHasLine(t *testing.T) {
	l := New(t.TempDir())
	write(t, l, "[settings]\ntitle = \"x\"\ncolumns = = 3\n")
	snap, err := l.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, snap.ParseError)
	assert.Equal(t, 3, snap.ParseError.Line)
	assert.Positive(t, snap.ParseError.Column)
	assert.Equal(t, l.Path(), snap.ParseError.File)
	assert.Equal(t, entity.DefaultHomeLayout(), snap.Layout)
}

func TestLoad_UnknownKeyHasLine(t *testing.T) {
	l := New(t.TempDir())
	write(t, l, "[settings]\ntitle = \"x\"\ncolumnz = 3\n")
	snap, err := l.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, snap.ParseError)
	assert.Equal(t, 3, snap.ParseError.Line)
	assert.Contains(t, snap.ParseError.Message, "settings.columnz")
}

func TestLoad_ValidationErrors(t *testing.T) {
	l := New(t.TempDir())
	write(t, l, "[settings]\ncolumns = 9\n[[widgets]]\ntype = \"weather\"\n")
	snap, err := l.Load(context.Background())
	require.NoError(t, err)
	assert.Nil(t, snap.ParseError)
	require.Len(t, snap.ValidationErrors, 2)
	assert.Equal(t, entity.DefaultHomeLayout(), snap.Layout)
}

func TestLoad_KeepsLastValidAfterBreakingEdit(t *testing.T) {
	l := New(t.TempDir())
	write(t, l, validHome)
	first, err := l.Load(context.Background())
	require.NoError(t, err)

	write(t, l, "[settings\n")
	broken, err := l.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, broken.ParseError)
	assert.Equal(t, first.Layout, broken.Layout)

	write(t, l, "[settings]\ncolumns = 0\ncolumns = 1\n")
	broken, err = l.Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, first.Layout, broken.Layout)
}

func TestEnsureExists_WritesValidStarterOnce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	l := New(dir)

	created, err := l.EnsureExists()
	require.NoError(t, err)
	assert.True(t, created)

	snap, err := l.Load(context.Background())
	require.NoError(t, err)
	assert.Nil(t, snap.ParseError)
	assert.Empty(t, snap.ValidationErrors)
	assert.Equal(t, entity.DefaultHomeLayout(), snap.Layout)

	write(t, l, validHome)
	created, err = l.EnsureExists()
	require.NoError(t, err)
	assert.False(t, created)
	data, err := os.ReadFile(l.Path())
	require.NoError(t, err)
	assert.Equal(t, validHome, string(data))
}

func TestLoad_CachesUntilFileChanges(t *testing.T) {
	ctx := context.Background()
	l := New(t.TempDir())
	write(t, l, validHome)
	first, err := l.Load(ctx)
	require.NoError(t, err)
	again, err := l.Load(ctx)
	require.NoError(t, err)
	assert.Same(t, &first.Layout.Groups[0], &again.Layout.Groups[0], "unchanged file is not decoded again")

	// Touch only: stat differs, the content hash matches.
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(l.Path(), later, later))
	touched, err := l.Load(ctx)
	require.NoError(t, err)
	assert.Same(t, &first.Layout.Groups[0], &touched.Layout.Groups[0])

	// Same size, new content and mtime: decoded again.
	edited := strings.Replace(validHome, "Lab", "Bal", 1)
	write(t, l, edited)
	require.NoError(t, os.Chtimes(l.Path(), later.Add(time.Second), later.Add(time.Second)))
	changed, err := l.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Bal", changed.Layout.Settings.Title)
}

func TestLoad_CachedParseErrorKeepsLastValid(t *testing.T) {
	ctx := context.Background()
	l := New(t.TempDir())
	write(t, l, validHome)
	first, err := l.Load(ctx)
	require.NoError(t, err)
	write(t, l, "[settings\n")
	for range 2 {
		broken, loadErr := l.Load(ctx)
		require.NoError(t, loadErr)
		require.NotNil(t, broken.ParseError)
		assert.Equal(t, first.Layout, broken.Layout)
	}
	require.NoError(t, os.Remove(l.Path()))
	gone, err := l.Load(ctx)
	require.NoError(t, err)
	assert.False(t, gone.Exists)
}

func TestLoad_SetsContentHash(t *testing.T) {
	ctx := context.Background()
	l := New(t.TempDir())
	missing, err := l.Load(ctx)
	require.NoError(t, err)
	assert.Zero(t, missing.ContentHash)
	write(t, l, validHome)
	first, err := l.Load(ctx)
	require.NoError(t, err)
	assert.NotZero(t, first.ContentHash)
	cached, err := l.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, first.ContentHash, cached.ContentHash)
	write(t, l, validHome+"\n")
	edited, err := l.Load(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, first.ContentHash, edited.ContentHash)
}

func TestLoad_StatusKeys(t *testing.T) {
	l := New(t.TempDir())
	write(t, l, `[settings]
status = true

[[groups]]
name = "Homelab"
[[groups.services]]
id = "jellyfin"
name = "Jellyfin"
url = "https://jelly.local"
status = false
[[groups.services]]
id = "grafana"
name = "Grafana"
url = "https://grafana.local"
`)
	snap, err := l.Load(context.Background())
	require.NoError(t, err)
	require.Nil(t, snap.ParseError)
	require.Empty(t, snap.ValidationErrors)
	assert.True(t, snap.Layout.Settings.Status)
	jelly, _ := snap.Layout.FindService("jellyfin")
	require.NotNil(t, jelly.Status)
	assert.False(t, jelly.StatusEnabled(snap.Layout.Settings))
	graf, _ := snap.Layout.FindService("grafana")
	assert.Nil(t, graf.Status)
	assert.True(t, graf.StatusEnabled(snap.Layout.Settings))
}
