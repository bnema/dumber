package homeconfig

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
