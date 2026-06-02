package filtering_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/infrastructure/filtering"
	"github.com/bnema/dumber/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testContext() context.Context {
	logger := logging.NewFromConfigValues("debug", "pretty")
	return logging.WithContext(context.Background(), logger)
}

type fakeBackend struct {
	mu          sync.Mutex
	cacheResult bool
	cacheErr    error
	activateErr error
	clearErr    error
	active      bool
	calls       []string
	activePaths [][]string
	clearCalls  int
}

func (b *fakeBackend) ActivateCached(_ context.Context) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, "cache")
	if b.cacheErr != nil {
		return false, b.cacheErr
	}
	if b.cacheResult {
		b.active = true
	}
	return b.cacheResult, nil
}

func (b *fakeBackend) ActivateFiles(_ context.Context, paths []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, "files")
	b.activePaths = append(b.activePaths, append([]string(nil), paths...))
	if b.activateErr != nil {
		return b.activateErr
	}
	b.active = true
	return nil
}

func (b *fakeBackend) HasActive() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.active
}

func (b *fakeBackend) Clear(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, "clear")
	b.clearCalls++
	b.active = false
	return b.clearErr
}

func (b *fakeBackend) snapshot() (calls []string, activePaths [][]string, clearCalls int, active bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	paths := make([][]string, 0, len(b.activePaths))
	for _, p := range b.activePaths {
		paths = append(paths, append([]string(nil), p...))
	}
	return append([]string(nil), b.calls...), paths, b.clearCalls, b.active
}

type fakeDownloader struct {
	mu               sync.Mutex
	manifest         *filtering.Manifest
	cachedPaths      []string
	stale            bool
	needsUpdate      bool
	needsUpdateErr   error
	downloadPaths    []string
	downloadErr      error
	clearErr         error
	downloadCalls    int
	clearCalls       int
	needsUpdateCalls int
}

func (d *fakeDownloader) GetCachedManifest() (*filtering.Manifest, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.manifest, nil
}

func (d *fakeDownloader) FetchManifest(context.Context) (*filtering.Manifest, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.manifest, nil
}

func (d *fakeDownloader) DownloadFilters(context.Context, func(filtering.DownloadProgress)) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.downloadCalls++
	if d.downloadErr != nil {
		return nil, d.downloadErr
	}
	return append([]string(nil), d.downloadPaths...), nil
}

func (d *fakeDownloader) NeedsUpdate(context.Context) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.needsUpdateCalls++
	if d.needsUpdateErr != nil {
		return false, d.needsUpdateErr
	}
	return d.needsUpdate, nil
}

func (d *fakeDownloader) ClearCache() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.clearCalls++
	return d.clearErr
}

func (d *fakeDownloader) HasCachedFilters() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.cachedPaths) > 0
}

func (d *fakeDownloader) GetCachedFilterPaths() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.cachedPaths...)
}

func (d *fakeDownloader) IsCacheStale(time.Duration) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stale
}

func (d *fakeDownloader) snapshot() (downloadCalls, clearCalls, needsUpdateCalls int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.downloadCalls, d.clearCalls, d.needsUpdateCalls
}

func validRuleFile(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "combined-part1.json")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`[{"trigger":{"url-filter":"test"},"action":{"type":"block"}}]`), 0o644))
	return path
}

func newTestManager(t *testing.T, backend *fakeBackend, downloader *fakeDownloader, enabled, autoUpdate bool) *filtering.Manager {
	t.Helper()
	mgr, err := filtering.NewManager(filtering.ManagerConfig{
		JSONDir:    filepath.Join(t.TempDir(), "json"),
		Enabled:    enabled,
		AutoUpdate: autoUpdate,
		Backend:    backend,
		Downloader: downloader,
	})
	require.NoError(t, err)
	return mgr
}

func waitForState(t *testing.T, mgr *filtering.Manager, state filtering.FilterState) filtering.FilterStatus {
	t.Helper()
	require.Eventually(t, func() bool {
		return mgr.Status().State == state
	}, 3*time.Second, 50*time.Millisecond)
	return mgr.Status()
}

func TestManager_LoadAsync_ChecksCacheFirst(t *testing.T) {
	backend := &fakeBackend{cacheResult: true}
	downloader := &fakeDownloader{manifest: &filtering.Manifest{Version: "2025.12.19"}}
	mgr := newTestManager(t, backend, downloader, true, false)

	ctx := testContext()
	require.NoError(t, mgr.Initialize(ctx))
	mgr.LoadAsync(ctx)

	status := waitForState(t, mgr, filtering.StateActive)
	assert.Equal(t, "2025.12.19", status.Version)
	calls, activePaths, _, _ := backend.snapshot()
	assert.Equal(t, []string{"cache"}, calls)
	assert.Empty(t, activePaths)
	downloadCalls, _, _ := downloader.snapshot()
	assert.Zero(t, downloadCalls)
}

func TestManager_LoadAsync_DownloadsWhenCacheMiss(t *testing.T) {
	tmpDir := t.TempDir()
	jsonFile := validRuleFile(t, filepath.Join(tmpDir, "json"))
	backend := &fakeBackend{cacheResult: false}
	downloader := &fakeDownloader{
		manifest:      &filtering.Manifest{Version: "2025.12.19"},
		downloadPaths: []string{jsonFile},
	}
	mgr := newTestManager(t, backend, downloader, true, false)

	var statuses []filtering.FilterStatus
	var mu sync.Mutex
	mgr.SetStatusCallback(func(status filtering.FilterStatus) {
		mu.Lock()
		statuses = append(statuses, status)
		mu.Unlock()
	})

	ctx := testContext()
	require.NoError(t, mgr.Initialize(ctx))
	mgr.LoadAsync(ctx)

	waitForState(t, mgr, filtering.StateActive)
	calls, activePaths, _, _ := backend.snapshot()
	assert.Equal(t, []string{"cache", "files"}, calls)
	require.Len(t, activePaths, 1)
	assert.Equal(t, []string{jsonFile}, activePaths[0])
	downloadCalls, _, _ := downloader.snapshot()
	assert.Equal(t, 1, downloadCalls)

	mu.Lock()
	defer mu.Unlock()
	assert.NotEmpty(t, statuses)
	assert.Contains(t, states(statuses), filtering.StateLoading)
	assert.Contains(t, states(statuses), filtering.StateActive)
}

func TestManager_LoadAsync_UsesLocalCachedFilesBeforeDownload(t *testing.T) {
	jsonFile := validRuleFile(t, filepath.Join(t.TempDir(), "json"))
	backend := &fakeBackend{cacheResult: false}
	downloader := &fakeDownloader{
		manifest:    &filtering.Manifest{Version: "local"},
		cachedPaths: []string{jsonFile},
	}
	mgr := newTestManager(t, backend, downloader, true, false)

	ctx := testContext()
	require.NoError(t, mgr.Initialize(ctx))
	mgr.LoadAsync(ctx)

	waitForState(t, mgr, filtering.StateActive)
	calls, activePaths, _, _ := backend.snapshot()
	assert.Equal(t, []string{"cache", "files"}, calls)
	require.Len(t, activePaths, 1)
	assert.Equal(t, []string{jsonFile}, activePaths[0])
	downloadCalls, _, _ := downloader.snapshot()
	assert.Zero(t, downloadCalls)
}

func TestManager_CheckForUpdates_DownloadsWhenNewVersionAvailable(t *testing.T) {
	jsonFile := validRuleFile(t, filepath.Join(t.TempDir(), "json"))
	backend := &fakeBackend{}
	downloader := &fakeDownloader{
		manifest:      &filtering.Manifest{Version: "2025.12.20"},
		needsUpdate:   true,
		downloadPaths: []string{jsonFile},
	}
	mgr := newTestManager(t, backend, downloader, true, true)

	err := mgr.CheckForUpdates(testContext())
	require.NoError(t, err)

	status := mgr.Status()
	assert.Equal(t, filtering.StateActive, status.State)
	assert.Equal(t, "2025.12.20", status.Version)
	_, activePaths, _, _ := backend.snapshot()
	require.Len(t, activePaths, 1)
	assert.Equal(t, []string{jsonFile}, activePaths[0])
}

func TestManager_CheckForUpdates_SkipsWhenUpToDate(t *testing.T) {
	backend := &fakeBackend{}
	downloader := &fakeDownloader{needsUpdate: false}
	mgr := newTestManager(t, backend, downloader, true, true)

	err := mgr.CheckForUpdates(testContext())
	require.NoError(t, err)

	downloadCalls, _, needsUpdateCalls := downloader.snapshot()
	assert.Equal(t, 1, needsUpdateCalls)
	assert.Zero(t, downloadCalls)
}

func TestManager_CheckForUpdates_SkipsWhenDisabled(t *testing.T) {
	backend := &fakeBackend{}
	downloader := &fakeDownloader{needsUpdate: true}
	mgr := newTestManager(t, backend, downloader, true, false)

	err := mgr.CheckForUpdates(testContext())
	require.NoError(t, err)

	downloadCalls, _, needsUpdateCalls := downloader.snapshot()
	assert.Zero(t, needsUpdateCalls)
	assert.Zero(t, downloadCalls)
}

func TestManager_Clear_RemovesFilterAndCache(t *testing.T) {
	backend := &fakeBackend{active: true}
	downloader := &fakeDownloader{}
	mgr := newTestManager(t, backend, downloader, true, false)

	err := mgr.Clear(testContext())
	require.NoError(t, err)

	status := mgr.Status()
	assert.Equal(t, filtering.StateUninitialized, status.State)
	_, _, clearCalls, active := backend.snapshot()
	assert.Equal(t, 1, clearCalls)
	assert.False(t, active)
	_, clearCacheCalls, _ := downloader.snapshot()
	assert.Equal(t, 1, clearCacheCalls)
}

func TestManager_Initialize_DisabledByConfig(t *testing.T) {
	backend := &fakeBackend{}
	downloader := &fakeDownloader{}
	mgr := newTestManager(t, backend, downloader, false, false)

	err := mgr.Initialize(testContext())
	require.NoError(t, err)

	status := mgr.Status()
	assert.Equal(t, filtering.StateDisabled, status.State)
}

func TestManager_StatusCallback_CalledOnStateChange(t *testing.T) {
	jsonFile := validRuleFile(t, filepath.Join(t.TempDir(), "json"))
	backend := &fakeBackend{cacheResult: false}
	downloader := &fakeDownloader{downloadPaths: []string{jsonFile}}
	mgr := newTestManager(t, backend, downloader, true, false)

	callCount := 0
	var mu sync.Mutex
	mgr.SetStatusCallback(func(filtering.FilterStatus) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})

	ctx := testContext()
	require.NoError(t, mgr.Initialize(ctx))
	mgr.LoadAsync(ctx)
	waitForState(t, mgr, filtering.StateActive)

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, callCount, 2, "Expected callback to be called at least twice")
}

func TestManager_LoadAsync_QuarantinesInvalidDownloadedPayload(t *testing.T) {
	jsonDir := filepath.Join(t.TempDir(), "json")
	invalidJSONFile := filepath.Join(jsonDir, "combined-part1.json")
	require.NoError(t, os.MkdirAll(jsonDir, 0o755))
	require.NoError(t, os.WriteFile(invalidJSONFile, []byte(`{"invalid":`), 0o644))

	backend := &fakeBackend{cacheResult: false}
	downloader := &fakeDownloader{downloadPaths: []string{invalidJSONFile}}
	mgr, err := filtering.NewManager(filtering.ManagerConfig{
		JSONDir:    jsonDir,
		Enabled:    true,
		AutoUpdate: false,
		Backend:    backend,
		Downloader: downloader,
	})
	require.NoError(t, err)

	ctx := testContext()
	require.NoError(t, mgr.Initialize(ctx))
	mgr.LoadAsync(ctx)

	status := waitForState(t, mgr, filtering.StateError)
	assert.Equal(t, "Invalid downloaded filters", status.Message)

	_, statErr := os.Stat(invalidJSONFile)
	require.ErrorIs(t, statErr, os.ErrNotExist)

	quarantineDir := filepath.Join(jsonDir, "quarantine")
	entries, err := os.ReadDir(quarantineDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestManager_CheckForUpdates_ActivationFailureWithoutActiveFilterSetsError(t *testing.T) {
	jsonFile := validRuleFile(t, filepath.Join(t.TempDir(), "json"))
	backend := &fakeBackend{activateErr: errors.New("compile failed")}
	downloader := &fakeDownloader{
		needsUpdate:   true,
		downloadPaths: []string{jsonFile},
	}
	mgr := newTestManager(t, backend, downloader, true, true)

	err := mgr.CheckForUpdates(testContext())
	require.ErrorIs(t, err, filtering.ErrUpdateSkipped)
	assert.Equal(t, filtering.StateError, mgr.Status().State)
	assert.Equal(t, "Compilation failed", mgr.Status().Message)
}

func TestManager_CheckForUpdates_ActivationFailureKeepsActiveFilter(t *testing.T) {
	jsonFile := validRuleFile(t, filepath.Join(t.TempDir(), "json"))
	backend := &fakeBackend{active: true, activateErr: errors.New("compile failed")}
	downloader := &fakeDownloader{
		manifest:      &filtering.Manifest{Version: "2025.12.19"},
		needsUpdate:   true,
		downloadPaths: []string{jsonFile},
	}
	mgr := newTestManager(t, backend, downloader, true, true)

	err := mgr.CheckForUpdates(testContext())
	require.ErrorIs(t, err, filtering.ErrUpdateSkipped)
	assert.Equal(t, filtering.StateActive, mgr.Status().State)
	assert.Equal(t, "Filters active (update skipped)", mgr.Status().Message)
	_, activePaths, _, active := backend.snapshot()
	assert.True(t, active)
	require.Len(t, activePaths, 1)
}

func states(statuses []filtering.FilterStatus) []filtering.FilterState {
	out := make([]filtering.FilterState, 0, len(statuses))
	for _, status := range statuses {
		out = append(out, status.State)
	}
	return out
}
