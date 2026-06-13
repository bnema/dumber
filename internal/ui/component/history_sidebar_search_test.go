package component

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// applySearchResults seam tests
// =============================================================================

// newTestSidebarSearchHarness creates a minimal HistorySidebar with only the
// fields needed by applySearchResults, avoiding GTK widget construction.
func newTestSidebarSearchHarness() *HistorySidebar {
	return &HistorySidebar{
		mu:     sync.RWMutex{},
		logger: zerolog.Nop(),
	}
}

func TestApplySearchResults_NonStaleApplied(t *testing.T) {
	hs := newTestSidebarSearchHarness()
	hs.searchGen = 1

	entries := []*entity.HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example", LastVisited: time.Now()},
	}

	applied := hs.applySearchResults(entries, 1, nil)
	assert.True(t, applied, "non-stale results must be applied")
	assert.True(t, hs.searchDone)
	assert.Nil(t, hs.searchErr)
	require.NotNil(t, hs.searchResults)
	assert.Len(t, hs.searchResults, 1)
	assert.Equal(t, "https://example.com", hs.searchResults[0].URL)
	require.NotNil(t, hs.groups, "groups must be built from results")
	assert.Equal(t, "Today", hs.groups[0].Label)
}

func TestApplySearchResults_StaleGenRejected(t *testing.T) {
	hs := newTestSidebarSearchHarness()
	hs.searchGen = 2 // Gen has moved on

	entries := []*entity.HistoryEntry{
		{ID: 1, URL: "https://stale.com", Title: "Stale", LastVisited: time.Now()},
	}

	applied := hs.applySearchResults(entries, 1, nil) // gen=1 is stale
	assert.False(t, applied, "stale results must be rejected")
	assert.False(t, hs.searchDone, "search must not be marked done for stale result")
	assert.Nil(t, hs.searchResults, "searchResults must remain nil")
	assert.Nil(t, hs.groups, "groups must remain nil")
}

func TestApplySearchResults_StaleAfterIncrement(t *testing.T) {
	hs := newTestSidebarSearchHarness()
	hs.searchGen = 1

	// First result applied as gen=1
	firstEntries := []*entity.HistoryEntry{
		{ID: 1, URL: "https://first.com", Title: "First", LastVisited: time.Now()},
	}
	applied := hs.applySearchResults(firstEntries, 1, nil)
	assert.True(t, applied)

	// Second search starts, gen advances
	hs.searchGen = 2

	// Stale result from gen=1 arrives
	staleEntries := []*entity.HistoryEntry{
		{ID: 2, URL: "https://stale.com", Title: "Stale", LastVisited: time.Now()},
	}
	applied = hs.applySearchResults(staleEntries, 1, nil)
	assert.False(t, applied, "stale result must be rejected")

	// The original results must be preserved
	require.NotNil(t, hs.searchResults)
	assert.Len(t, hs.searchResults, 1)
	assert.Equal(t, "https://first.com", hs.searchResults[0].URL)
}

func TestApplySearchResults_ErrorStored(t *testing.T) {
	hs := newTestSidebarSearchHarness()
	hs.searchGen = 1

	wantErr := errors.New("search failed")
	entries := []*entity.HistoryEntry{} // empty but not nil

	applied := hs.applySearchResults(entries, 1, wantErr)
	assert.True(t, applied)
	assert.True(t, hs.searchDone)
	assert.ErrorIs(t, hs.searchErr, wantErr)
	require.NotNil(t, hs.searchResults)
	assert.Empty(t, hs.searchResults)
}

func TestApplySearchResults_EmptyResultsApplied(t *testing.T) {
	hs := newTestSidebarSearchHarness()
	hs.searchGen = 1

	applied := hs.applySearchResults([]*entity.HistoryEntry{}, 1, nil)
	assert.True(t, applied)
	assert.True(t, hs.searchDone)
	assert.Nil(t, hs.searchErr)
	require.NotNil(t, hs.searchResults)
	assert.Empty(t, hs.searchResults)
	// Groups from empty entries should be nil or empty
	assert.Nil(t, hs.groups, "empty entries should produce nil groups")
}

// =============================================================================
// Async search seam: controllable history port fake
// =============================================================================

type fakeHistorySidebarHistory struct {
	getRecentFn func(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error)
	searchFn    func(ctx context.Context, input dto.HistorySearchInput) (*dto.HistorySearchOutput, error)
	deleteFn    func(ctx context.Context, id int64) error
}

func (f *fakeHistorySidebarHistory) GetRecent(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error) {
	if f.getRecentFn != nil {
		return f.getRecentFn(ctx, limit, offset)
	}
	<-make(chan struct{})
	return nil, nil
}

func (f *fakeHistorySidebarHistory) Search(ctx context.Context, input dto.HistorySearchInput) (*dto.HistorySearchOutput, error) {
	if f.searchFn != nil {
		return f.searchFn(ctx, input)
	}
	return &dto.HistorySearchOutput{Matches: []entity.HistoryMatch{}}, nil
}

func (f *fakeHistorySidebarHistory) Delete(ctx context.Context, id int64) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, id)
	}
	return nil
}

func TestApplySearchResults_StaleGenerationDropsResultsAfterSearch(t *testing.T) {
	searchCalled := make(chan struct{}, 1)
	idleCalled := make(chan glib.SourceFunc, 1)

	history := &fakeHistorySidebarHistory{
		searchFn: func(context.Context, dto.HistorySearchInput) (*dto.HistorySearchOutput, error) {
			searchCalled <- struct{}{}
			return &dto.HistorySearchOutput{Matches: []entity.HistoryMatch{
				{Entry: &entity.HistoryEntry{ID: 1, URL: "https://result.com", Title: "Result", LastVisited: time.Now()}},
			}}, nil
		},
	}

	hs := newTestSidebarSearchHarness()
	hs.ctx = t.Context()
	hs.historyUC = history
	hs.searchGen = 1
	hs.idleScheduler = func(cb glib.SourceFunc) {
		idleCalled <- cb
	}

	// Start search with gen=1.
	hs.doFTSearch("test", 1)
	select {
	case <-searchCalled:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for search use case to be invoked")
	}

	// Advance gen before the queued UI callback applies results.
	hs.mu.Lock()
	hs.searchGen = 2
	hs.mu.Unlock()

	select {
	case cb := <-idleCalled:
		cb(0)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduled idle callback")
	}

	hs.mu.RLock()
	defer hs.mu.RUnlock()
	assert.Nil(t, hs.searchResults)
	assert.False(t, hs.searchDone)
	assert.Nil(t, hs.groups)
}

func TestApplySearchResults_CurrentGenerationAppliedAfterSearch(t *testing.T) {
	searchCalled := make(chan struct{}, 1)
	idleCalled := make(chan glib.SourceFunc, 1)

	history := &fakeHistorySidebarHistory{
		searchFn: func(context.Context, dto.HistorySearchInput) (*dto.HistorySearchOutput, error) {
			searchCalled <- struct{}{}
			return &dto.HistorySearchOutput{Matches: []entity.HistoryMatch{
				{Entry: &entity.HistoryEntry{ID: 1, URL: "https://live.com", Title: "Live", LastVisited: time.Now()}},
			}}, nil
		},
	}

	hs := newTestSidebarSearchHarness()
	hs.ctx = t.Context()
	hs.historyUC = history
	hs.searchGen = 1
	hs.idleScheduler = func(cb glib.SourceFunc) {
		idleCalled <- cb
	}

	// Start the search and wait for the use case to be invoked.
	hs.doFTSearch("live", 1)
	select {
	case <-searchCalled:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for search use case to be invoked")
	}

	select {
	case cb := <-idleCalled:
		cb(0)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduled idle callback")
	}

	hs.mu.RLock()
	assert.NotNil(t, hs.searchResults)
	assert.True(t, hs.searchDone)
	assert.Len(t, hs.searchResults, 1)
	assert.Equal(t, "https://live.com", hs.searchResults[0].URL)
	hs.mu.RUnlock()
}

// =============================================================================
// Reload with query preservation
// =============================================================================

// TestHistorySidebar_ReloadPreservesQuery verifies that Reload preserves the
// active query while resetting browse/search state before the refreshed load.
func TestHistorySidebar_ReloadPreservesQuery(t *testing.T) {
	searchCalled := make(chan struct{}, 1)
	history := &fakeHistorySidebarHistory{
		searchFn: func(context.Context, dto.HistorySearchInput) (*dto.HistorySearchOutput, error) {
			searchCalled <- struct{}{}
			return &dto.HistorySearchOutput{Matches: []entity.HistoryMatch{}}, nil
		},
	}

	hs := newTestSidebarSearchHarness()
	hs.currentQuery = "preserved"
	hs.historyUC = history
	hs.ctx = t.Context()

	hs.mu.Lock()
	oldGen := hs.searchGen
	hs.loadDone = true
	hs.allEntries = []*entity.HistoryEntry{
		{ID: 1, URL: "https://old.com", Title: "Old", LastVisited: time.Now()},
	}
	hs.groups = groupHistoryByDay(hs.allEntries)
	hs.searchResults = []*entity.HistoryEntry{{ID: 2, URL: "https://stale.com", Title: "Stale", LastVisited: time.Now()}}
	hs.searchDone = true
	hs.mu.Unlock()

	hs.Reload()

	select {
	case <-searchCalled:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reload search to be invoked")
	}

	hs.mu.RLock()
	assert.Equal(t, "preserved", hs.currentQuery, "query must be preserved after Reload")
	assert.False(t, hs.loadDone, "loadDone must be reset")
	assert.False(t, hs.loadStarted, "loadStarted must be reset")
	assert.Nil(t, hs.allEntries, "entries must be cleared")
	assert.Nil(t, hs.groups, "groups must be cleared")
	assert.Nil(t, hs.searchResults, "searchResults must be cleared before refreshed search applies")
	assert.False(t, hs.searchDone, "searchDone must be reset")
	assert.Equal(t, oldGen+1, hs.searchGen, "searchGen must be incremented")
	hs.mu.RUnlock()
}

// =============================================================================
// fetchPage generation guard: stale browse results must not clear loading state
// =============================================================================

func TestFetchPage_StaleGenerationDoesNotMutateLoadingState(t *testing.T) {
	t.Parallel()

	// Repo blocks GetRecent until test signals proceed, so we can control
	// when a stale fetch completes relative to generation changes.
	getRecentCalled := make(chan struct{})
	proceed := make(chan struct{})

	history := &fakeHistorySidebarHistory{
		getRecentFn: func(context.Context, int, int) ([]*entity.HistoryEntry, error) {
			close(getRecentCalled)
			<-proceed
			return []*entity.HistoryEntry{}, nil
		},
	}

	hs := newTestSidebarSearchHarness()
	hs.ctx = t.Context()
	hs.historyUC = history

	// Simulate: gen 1 started, then gen 2 started (e.g. by Reload/Show).
	// The second call incremented loadGen and set isLoading/loadStarted.
	hs.mu.Lock()
	hs.loadGen = 2
	hs.isLoading = true
	hs.loadStarted = true
	hs.totalLoaded = 0
	hs.hasMore = true
	hs.mu.Unlock()

	// Start stale gen 1 fetch in background
	fetchDone := make(chan struct{})
	go func() {
		hs.fetchPage(0, 1) // gen=1 is stale — loadGen is 2
		close(fetchDone)
	}()

	// Wait until GetRecent is entered (fetchPage is blocked inside the repo call)
	<-getRecentCalled

	// Let GetRecent return — this simulates the gen 1 fetch completing
	// after gen 2 has already taken over.
	close(proceed)

	// Wait for fetchPage to finish processing the stale return
	<-fetchDone

	// Verify: stale completion must NOT clear the new generation's loading state
	hs.mu.RLock()
	assert.True(t, hs.isLoading, "isLoading must remain true when stale gen completes")
	assert.True(t, hs.loadStarted, "loadStarted must remain true when stale gen completes")
	assert.Equal(t, uint64(2), hs.loadGen, "loadGen must remain unchanged")
	assert.False(t, hs.loadDone, "loadDone must remain false; gen 2 hasn't completed")
	assert.Equal(t, 0, hs.totalLoaded, "stale results must not update totalLoaded")
	hs.mu.RUnlock()
}
