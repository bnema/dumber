package webkitfilter

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/filtering"
	"github.com/bnema/puregotk/v4/webkit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	identifiers []string
	loads       map[string]*webkit.UserContentFilter
	compileErr  error
	removeErr   error
	compiled    []string
	removed     []string
}

func (s *fakeStore) Compile(_ context.Context, identifier string, _ string) (*webkit.UserContentFilter, error) {
	s.compiled = append(s.compiled, identifier)
	if s.compileErr != nil {
		return nil, s.compileErr
	}
	return &webkit.UserContentFilter{}, nil
}

func (s *fakeStore) Load(_ context.Context, identifier string) (*webkit.UserContentFilter, error) {
	if s.loads == nil {
		return nil, nil
	}
	return s.loads[identifier], nil
}

func (s *fakeStore) Remove(_ context.Context, identifier string) error {
	s.removed = append(s.removed, identifier)
	return s.removeErr
}

func (s *fakeStore) FetchIdentifiers(context.Context) ([]string, error) {
	return append([]string(nil), s.identifiers...), nil
}

func (s *fakeStore) Path() string { return "" }

func TestNewBackendCreatesStoreDirBeforeConstructingStore(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "filters", "store")
	constructed := false

	backend, err := NewBackend(Config{
		StoreDir: storeDir,
		NewStore: func(path string) StoreOps {
			constructed = true
			assert.Equal(t, storeDir, path)
			_, statErr := os.Stat(storeDir)
			require.NoError(t, statErr)
			return &fakeStore{}
		},
	})

	require.NoError(t, err)
	require.NotNil(t, backend)
	assert.True(t, constructed)
}

func TestBackendActivateCachedLoadsOnlyFilterIdentifiers(t *testing.T) {
	filter := &webkit.UserContentFilter{}
	store := &fakeStore{
		identifiers: []string{"other", filtering.FilterIdentifierPrefix + "-0"},
		loads: map[string]*webkit.UserContentFilter{
			filtering.FilterIdentifierPrefix + "-0": filter,
		},
	}
	backend, err := NewBackend(Config{Store: store})
	require.NoError(t, err)

	loaded, err := backend.ActivateCached(context.Background())
	require.NoError(t, err)
	assert.True(t, loaded)
	assert.True(t, backend.HasActive())
	assert.Equal(t, []*webkit.UserContentFilter{filter}, backend.GetFilters())
}

func TestBackendActivateFilesReplacesActiveFilters(t *testing.T) {
	store := &fakeStore{}
	backend, err := NewBackend(Config{Store: store})
	require.NoError(t, err)

	err = backend.ActivateFiles(context.Background(), []string{"a.json", "b.json"})
	require.NoError(t, err)

	assert.True(t, backend.HasActive())
	assert.Equal(t, []string{filtering.FilterIdentifierPrefix + "-0", filtering.FilterIdentifierPrefix + "-1"}, store.compiled)
	assert.Len(t, backend.GetFilters(), 2)
}

func TestBackendActivateFilesFailsOnCompileErrorWithoutReplacingActiveFilters(t *testing.T) {
	store := &fakeStore{}
	backend, err := NewBackend(Config{Store: store})
	require.NoError(t, err)
	require.NoError(t, backend.ActivateFiles(context.Background(), []string{"active.json"}))
	active := backend.GetFilters()

	store.compileErr = errors.New("compile failed")
	err = backend.ActivateFiles(context.Background(), []string{"broken.json"})
	require.Error(t, err)
	assert.Equal(t, active, backend.GetFilters())
}

func TestBackendClearRemovesCompiledFilterIdentifiers(t *testing.T) {
	store := &fakeStore{identifiers: []string{filtering.FilterIdentifierPrefix + "-0", "other"}}
	backend, err := NewBackend(Config{Store: store})
	require.NoError(t, err)
	require.NoError(t, backend.ActivateFiles(context.Background(), []string{"active.json"}))

	err = backend.Clear(context.Background())
	require.NoError(t, err)

	assert.False(t, backend.HasActive())
	assert.Equal(t, []string{filtering.FilterIdentifierPrefix + "-0"}, store.removed)
}
