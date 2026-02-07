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
	"github.com/bnema/dumber/internal/infrastructure/filtering/mocks"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func testContext() context.Context {
	logger := logging.NewFromConfigValues("debug", "pretty")
	return logging.WithContext(context.Background(), logger)
}

func TestManager_LoadAsync_ChecksCacheFirst(t *testing.T) {
	// Setup temp directories
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	jsonDir := filepath.Join(tmpDir, "json")

	// Create test JSON file for fallback download
	testJSON := `[{"trigger":{"url-filter":"test"},"action":{"type":"block"}}]`
	jsonFile := filepath.Join(jsonDir, "combined-part1.json")
	require.NoError(t, os.MkdirAll(jsonDir, 0o755))
	require.NoError(t, os.WriteFile(jsonFile, []byte(testJSON), 0o644))

	// Create mocks
	mockStore := mocks.NewMockFilterStore(t)
	mockDownloader := mocks.NewMockFilterDownloader(t)

	// Setup expectations: filter exists in store but returns nil (can't create real filter in tests)
	// This verifies the cache check path is taken
	mockStore.EXPECT().
		HasCompiledFilter(mock.Anything, filtering.FilterIdentifier).
		Return(true)

	mockStore.EXPECT().
		Load(mock.Anything, filtering.FilterIdentifier).
		Return(nil, nil) // nil filter triggers fallback to download

	// Fallback: download will be triggered since Load returned nil filter
	mockDownloader.EXPECT().
		DownloadFilters(mock.Anything, mock.Anything).
		Return([]string{jsonFile}, nil)

	mockStore.EXPECT().
		Compile(mock.Anything, filtering.FilterIdentifier, mock.Anything).
		Return(&webkit.UserContentFilter{}, nil)

	mockDownloader.EXPECT().
		GetCachedManifest().
		Return(&filtering.Manifest{Version: "2025.12.19"}, nil)

	// Create manager with mocks
	mgr, err := filtering.NewManager(filtering.ManagerConfig{
		StoreDir:   storeDir,
		JSONDir:    jsonDir,
		Enabled:    true,
		AutoUpdate: false,
		Store:      mockStore,
		Downloader: mockDownloader,
	})
	require.NoError(t, err)

	// Track status changes
	var statuses []filtering.FilterStatus
	var mu sync.Mutex
	mgr.SetStatusCallback(func(status filtering.FilterStatus) {
		mu.Lock()
		statuses = append(statuses, status)
		mu.Unlock()
	})

	// Initialize and load
	ctx := testContext()
	err = mgr.Initialize(ctx)
	require.NoError(t, err)

	// Start async load and wait for completion
	done := make(chan struct{})
	go func() {
		mgr.LoadAsync(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("LoadAsync timed out")
	}

	// Give callbacks time to fire
	time.Sleep(100 * time.Millisecond)

	// Verify status progression
	mu.Lock()
	defer mu.Unlock()

	// Should have loading states followed by active
	require.GreaterOrEqual(t, len(statuses), 2, "Expected at least 2 status updates")

	// Final status should be active
	finalStatus := statuses[len(statuses)-1]
	assert.Equal(t, filtering.StateActive, finalStatus.State)
	assert.Equal(t, "2025.12.19", finalStatus.Version)
}

func TestManager_LoadAsync_DownloadsWhenCacheMiss(t *testing.T) {
	// Setup temp directories
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	jsonDir := filepath.Join(tmpDir, "json")

	// Create test JSON files
	testJSON := `[{"trigger":{"url-filter":"test"},"action":{"type":"block"}}]`
	jsonFile := filepath.Join(jsonDir, "combined-part1.json")
	require.NoError(t, os.MkdirAll(jsonDir, 0o755))
	require.NoError(t, os.WriteFile(jsonFile, []byte(testJSON), 0o644))

	// Create mocks
	mockStore := mocks.NewMockFilterStore(t)
	mockDownloader := mocks.NewMockFilterDownloader(t)

	// Setup expectations: no filter in store
	mockStore.EXPECT().
		HasCompiledFilter(mock.Anything, filtering.FilterIdentifier).
		Return(false)

	// Download should be triggered
	mockDownloader.EXPECT().
		DownloadFilters(mock.Anything, mock.Anything).
		Return([]string{jsonFile}, nil)

	// Compile should be called
	mockStore.EXPECT().
		Compile(mock.Anything, filtering.FilterIdentifier, mock.Anything).
		Return(&webkit.UserContentFilter{}, nil)

	// GetCachedManifest for version
	mockDownloader.EXPECT().
		GetCachedManifest().
		Return(&filtering.Manifest{Version: "2025.12.19"}, nil)

	// Create manager with mocks
	mgr, err := filtering.NewManager(filtering.ManagerConfig{
		StoreDir:   storeDir,
		JSONDir:    jsonDir,
		Enabled:    true,
		AutoUpdate: false,
		Store:      mockStore,
		Downloader: mockDownloader,
	})
	require.NoError(t, err)

	// Track status changes
	var statuses []filtering.FilterStatus
	var mu sync.Mutex
	mgr.SetStatusCallback(func(status filtering.FilterStatus) {
		mu.Lock()
		statuses = append(statuses, status)
		mu.Unlock()
	})

	// Initialize and load
	ctx := testContext()
	err = mgr.Initialize(ctx)
	require.NoError(t, err)

	// Start async load and wait for completion
	done := make(chan struct{})
	go func() {
		mgr.LoadAsync(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("LoadAsync timed out")
	}

	// Give callbacks time to fire
	time.Sleep(100 * time.Millisecond)

	// Verify status progression
	mu.Lock()
	defer mu.Unlock()

	// Should have loading states followed by active
	foundLoading := false
	foundActive := false
	for _, s := range statuses {
		if s.State == filtering.StateLoading {
			foundLoading = true
		}
		if s.State == filtering.StateActive {
			foundActive = true
		}
	}
	assert.True(t, foundLoading, "Expected loading state")
	assert.True(t, foundActive, "Expected active state")
}

func TestManager_CheckForUpdates_DownloadsWhenNewVersionAvailable(t *testing.T) {
	// Setup temp directories
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	jsonDir := filepath.Join(tmpDir, "json")

	// Create test JSON files
	testJSON := `[{"trigger":{"url-filter":"test"},"action":{"type":"block"}}]`
	jsonFile := filepath.Join(jsonDir, "combined-part1.json")
	require.NoError(t, os.MkdirAll(jsonDir, 0o755))
	require.NoError(t, os.WriteFile(jsonFile, []byte(testJSON), 0o644))

	// Create mocks
	mockStore := mocks.NewMockFilterStore(t)
	mockDownloader := mocks.NewMockFilterDownloader(t)

	// NeedsUpdate returns true (new version available)
	mockDownloader.EXPECT().
		NeedsUpdate(mock.Anything).
		Return(true, nil)

	// Download triggered
	mockDownloader.EXPECT().
		DownloadFilters(mock.Anything, mock.Anything).
		Return([]string{jsonFile}, nil)

	// Compile called
	mockStore.EXPECT().
		Compile(mock.Anything, filtering.FilterIdentifier, mock.Anything).
		Return(&webkit.UserContentFilter{}, nil)

	// GetCachedManifest for version
	mockDownloader.EXPECT().
		GetCachedManifest().
		Return(&filtering.Manifest{Version: "2025.12.20"}, nil)

	// Create manager with mocks
	mgr, err := filtering.NewManager(filtering.ManagerConfig{
		StoreDir:   storeDir,
		JSONDir:    jsonDir,
		Enabled:    true,
		AutoUpdate: true,
		Store:      mockStore,
		Downloader: mockDownloader,
	})
	require.NoError(t, err)

	// Track status changes
	var statuses []filtering.FilterStatus
	var mu sync.Mutex
	mgr.SetStatusCallback(func(s filtering.FilterStatus) {
		mu.Lock()
		statuses = append(statuses, s)
		mu.Unlock()
	})

	// Call CheckForUpdates
	ctx := testContext()
	err = mgr.CheckForUpdates(ctx)
	require.NoError(t, err)

	// Verify final status is active with new version
	mu.Lock()
	defer mu.Unlock()

	if len(statuses) > 0 {
		finalStatus := statuses[len(statuses)-1]
		assert.Equal(t, filtering.StateActive, finalStatus.State)
		assert.Equal(t, "2025.12.20", finalStatus.Version)
	}
}

func TestManager_CheckForUpdates_SkipsWhenUpToDate(t *testing.T) {
	// Setup temp directories
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	jsonDir := filepath.Join(tmpDir, "json")

	// Create mocks
	mockStore := mocks.NewMockFilterStore(t)
	mockDownloader := mocks.NewMockFilterDownloader(t)

	// NeedsUpdate returns false (already up to date)
	mockDownloader.EXPECT().
		NeedsUpdate(mock.Anything).
		Return(false, nil)

	// Download should NOT be called (we verify this via no expectation)

	// Create manager with mocks
	mgr, err := filtering.NewManager(filtering.ManagerConfig{
		StoreDir:   storeDir,
		JSONDir:    jsonDir,
		Enabled:    true,
		AutoUpdate: true,
		Store:      mockStore,
		Downloader: mockDownloader,
	})
	require.NoError(t, err)

	// Call CheckForUpdates
	ctx := testContext()
	err = mgr.CheckForUpdates(ctx)
	require.NoError(t, err)

	// Mock expectations will verify DownloadFilters was not called
}

func TestManager_CheckForUpdates_SkipsWhenDisabled(t *testing.T) {
	// Setup temp directories
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	jsonDir := filepath.Join(tmpDir, "json")

	// Create mocks
	mockStore := mocks.NewMockFilterStore(t)
	mockDownloader := mocks.NewMockFilterDownloader(t)

	// No expectations - nothing should be called when disabled

	// Create manager with mocks - autoUpdate disabled
	mgr, err := filtering.NewManager(filtering.ManagerConfig{
		StoreDir:   storeDir,
		JSONDir:    jsonDir,
		Enabled:    true,
		AutoUpdate: false, // Disabled
		Store:      mockStore,
		Downloader: mockDownloader,
	})
	require.NoError(t, err)

	// Call CheckForUpdates
	ctx := testContext()
	err = mgr.CheckForUpdates(ctx)
	require.NoError(t, err)

	// Mock expectations will verify nothing was called
}

func TestManager_Clear_RemovesFilterAndCache(t *testing.T) {
	// Setup temp directories
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	jsonDir := filepath.Join(tmpDir, "json")

	// Create mocks
	mockStore := mocks.NewMockFilterStore(t)
	mockDownloader := mocks.NewMockFilterDownloader(t)

	// Remove should be called
	mockStore.EXPECT().
		Remove(mock.Anything, filtering.FilterIdentifier).
		Return(nil)

	// ClearCache should be called
	mockDownloader.EXPECT().
		ClearCache().
		Return(nil)

	// Create manager with mocks
	mgr, err := filtering.NewManager(filtering.ManagerConfig{
		StoreDir:   storeDir,
		JSONDir:    jsonDir,
		Enabled:    true,
		AutoUpdate: false,
		Store:      mockStore,
		Downloader: mockDownloader,
	})
	require.NoError(t, err)

	// Clear filters
	ctx := testContext()
	err = mgr.Clear(ctx)
	require.NoError(t, err)

	// Verify status is uninitialized
	status := mgr.Status()
	assert.Equal(t, filtering.StateUninitialized, status.State)
}

func TestManager_Initialize_DisabledByConfig(t *testing.T) {
	// Setup temp directories
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	jsonDir := filepath.Join(tmpDir, "json")

	// Create mocks (no expectations - nothing should be called)
	mockStore := mocks.NewMockFilterStore(t)
	mockDownloader := mocks.NewMockFilterDownloader(t)

	// Create manager with filtering disabled
	mgr, err := filtering.NewManager(filtering.ManagerConfig{
		StoreDir:   storeDir,
		JSONDir:    jsonDir,
		Enabled:    false, // Disabled
		AutoUpdate: false,
		Store:      mockStore,
		Downloader: mockDownloader,
	})
	require.NoError(t, err)

	// Initialize
	ctx := testContext()
	err = mgr.Initialize(ctx)
	require.NoError(t, err)

	// Verify status is disabled
	status := mgr.Status()
	assert.Equal(t, filtering.StateDisabled, status.State)
}

func TestManager_StatusCallback_CalledOnStateChange(t *testing.T) {
	// Setup temp directories
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	jsonDir := filepath.Join(tmpDir, "json")

	// Create test JSON file
	testJSON := `[{"trigger":{"url-filter":"test"},"action":{"type":"block"}}]`
	jsonFile := filepath.Join(jsonDir, "combined-part1.json")
	require.NoError(t, os.MkdirAll(jsonDir, 0o755))
	require.NoError(t, os.WriteFile(jsonFile, []byte(testJSON), 0o644))

	// Create mocks
	mockStore := mocks.NewMockFilterStore(t)
	mockDownloader := mocks.NewMockFilterDownloader(t)

	// Setup for a load with fallback to download (since we can't return real filter)
	mockStore.EXPECT().
		HasCompiledFilter(mock.Anything, filtering.FilterIdentifier).
		Return(true)
	mockStore.EXPECT().
		Load(mock.Anything, filtering.FilterIdentifier).
		Return(nil, nil) // nil triggers download fallback
	mockDownloader.EXPECT().
		DownloadFilters(mock.Anything, mock.Anything).
		Return([]string{jsonFile}, nil)
	mockStore.EXPECT().
		Compile(mock.Anything, filtering.FilterIdentifier, mock.Anything).
		Return(&webkit.UserContentFilter{}, nil)
	mockDownloader.EXPECT().
		GetCachedManifest().
		Return(&filtering.Manifest{Version: "test"}, nil)

	// Create manager
	mgr, err := filtering.NewManager(filtering.ManagerConfig{
		StoreDir:   storeDir,
		JSONDir:    jsonDir,
		Enabled:    true,
		AutoUpdate: false,
		Store:      mockStore,
		Downloader: mockDownloader,
	})
	require.NoError(t, err)

	// Track callback invocations
	callCount := 0
	var mu sync.Mutex
	mgr.SetStatusCallback(func(_ filtering.FilterStatus) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})

	// Initialize and load
	ctx := testContext()
	_ = mgr.Initialize(ctx)

	done := make(chan struct{})
	go func() {
		mgr.LoadAsync(ctx)
		close(done)
	}()
	<-done

	time.Sleep(100 * time.Millisecond)

	// Verify callback was called multiple times
	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, callCount, 2, "Expected callback to be called at least twice")
}

func TestManager_LoadAsync_QuarantinesInvalidDownloadedPayload(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	jsonDir := filepath.Join(tmpDir, "json")

	invalidJSONFile := filepath.Join(jsonDir, "combined-part1.json")
	require.NoError(t, os.MkdirAll(jsonDir, 0o755))
	require.NoError(t, os.WriteFile(invalidJSONFile, []byte(`{"invalid":`), 0o644))

	mockStore := mocks.NewMockFilterStore(t)
	mockDownloader := mocks.NewMockFilterDownloader(t)

	mockStore.EXPECT().
		HasCompiledFilter(mock.Anything, filtering.FilterIdentifier).
		Return(false)

	mockDownloader.EXPECT().
		DownloadFilters(mock.Anything, mock.Anything).
		Return([]string{invalidJSONFile}, nil)

	mgr, err := filtering.NewManager(filtering.ManagerConfig{
		StoreDir:   storeDir,
		JSONDir:    jsonDir,
		Enabled:    true,
		AutoUpdate: false,
		Store:      mockStore,
		Downloader: mockDownloader,
	})
	require.NoError(t, err)

	ctx := testContext()
	require.NoError(t, mgr.Initialize(ctx))

	mgr.LoadAsync(ctx)

	require.Eventually(t, func() bool {
		return mgr.Status().State == filtering.StateError
	}, 3*time.Second, 50*time.Millisecond)
	assert.Equal(t, "Invalid downloaded filters", mgr.Status().Message)

	_, statErr := os.Stat(invalidJSONFile)
	require.ErrorIs(t, statErr, os.ErrNotExist)

	quarantineDir := filepath.Join(jsonDir, "quarantine")
	entries, err := os.ReadDir(quarantineDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestManager_CheckForUpdates_CompileFailureKeepsActiveFilter(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	jsonDir := filepath.Join(tmpDir, "json")

	validJSON := `[{"trigger":{"url-filter":"test"},"action":{"type":"block"}}]`
	jsonFile := filepath.Join(jsonDir, "combined-part1.json")
	require.NoError(t, os.MkdirAll(jsonDir, 0o755))
	require.NoError(t, os.WriteFile(jsonFile, []byte(validJSON), 0o644))

	mockStore := mocks.NewMockFilterStore(t)
	mockDownloader := mocks.NewMockFilterDownloader(t)

	activeFilter := &webkit.UserContentFilter{}

	mockStore.EXPECT().
		HasCompiledFilter(mock.Anything, filtering.FilterIdentifier).
		Return(true).Once()
	mockStore.EXPECT().
		Load(mock.Anything, filtering.FilterIdentifier).
		Return(activeFilter, nil).Once()
	mockDownloader.EXPECT().
		GetCachedManifest().
		Return(&filtering.Manifest{Version: "2025.12.19"}, nil).Once()
	mockDownloader.EXPECT().
		IsCacheStale(filtering.CacheMaxAge).
		Return(false).Once()

	mockDownloader.EXPECT().
		NeedsUpdate(mock.Anything).
		Return(true, nil).Once()
	mockDownloader.EXPECT().
		DownloadFilters(mock.Anything, mock.Anything).
		Return([]string{jsonFile}, nil).Once()
	mockStore.EXPECT().
		Compile(mock.Anything, filtering.FilterIdentifier, mock.Anything).
		Return(nil, errors.New("compile failed")).Once()

	mgr, err := filtering.NewManager(filtering.ManagerConfig{
		StoreDir:   storeDir,
		JSONDir:    jsonDir,
		Enabled:    true,
		AutoUpdate: true,
		Store:      mockStore,
		Downloader: mockDownloader,
	})
	require.NoError(t, err)

	ctx := testContext()
	require.NoError(t, mgr.Initialize(ctx))
	mgr.LoadAsync(ctx)

	require.Eventually(t, func() bool {
		return mgr.GetFilter() != nil && mgr.Status().State == filtering.StateActive
	}, 3*time.Second, 50*time.Millisecond)

	err = mgr.CheckForUpdates(ctx)
	require.ErrorIs(t, err, filtering.ErrUpdateSkipped)
	assert.Same(t, activeFilter, mgr.GetFilter())
	assert.Equal(t, filtering.StateActive, mgr.Status().State)
}
