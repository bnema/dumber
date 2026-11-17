package systemviews

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// regionDOM records region updates on top of full mounts.
type regionDOM struct {
	recordingDOM
	regions     [][]DOMRegion
	transitions []bool
}

func (d *regionDOM) UpdateRegions(update RegionUpdate) error {
	d.regions = append(d.regions, update.Regions)
	d.transitions = append(d.transitions, update.Transition)
	return nil
}

func loadedHistoryApp(t *testing.T, dom DOM, history *recordingHistoryService) *App {
	t.Helper()
	app := NewApp(Dependencies{DOM: dom, History: history, LocationURI: "dumb://history"})
	require.NoError(t, app.LoadInitial(context.Background()))
	history.statsCalled = false
	history.called = false
	return app
}

func TestEnqueueCoalescesLiveSearchToLatestQuery(t *testing.T) {
	t.Parallel()

	app := NewApp(Dependencies{})
	queue := make(chan DOMAction, 8)
	app.actionQueue = queue
	app.actionCtx = t.Context()

	for _, query := range []string{"g", "gi", "git"} {
		require.True(t, app.enqueueDOMAction(DOMAction{Action: historyActionSearchLive, Data: map[string]string{"query": query}}))
	}

	require.Len(t, queue, 1, "typing queues a single live search")
	queued := <-queue
	assert.Equal(t, "git", app.takeLiveSearchQuery(queued.Data["query"]), "the queued action runs the latest query")

	require.True(t, app.enqueueDOMAction(DOMAction{Action: historyActionSearchLive, Data: map[string]string{"query": "gith"}}))
	assert.Len(t, queue, 1, "a new keystroke after the run queues again")
}

func TestLiveSearchUpdatesRegionsWithoutReloadingStats(t *testing.T) {
	t.Parallel()

	dom := &regionDOM{}
	history := &recordingHistoryService{
		entries:       []*entity.HistoryEntry{{ID: 1, URL: "https://example.com", Title: "Example"}},
		searchEntries: []*entity.HistoryEntry{{ID: 7, URL: "https://go.dev", Title: "Go result"}},
		stats:         &entity.HistoryStats{TotalEntries: 3},
	}
	app := loadedHistoryApp(t, dom, history)
	mountedBefore := dom.HTML()

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionSearchLive,
		Data:   map[string]string{"query": " go "},
	}))

	assert.Equal(t, "go", history.query)
	assert.False(t, history.statsCalled, "live search must not reload stats")
	assert.Equal(t, mountedBefore, dom.HTML(), "live search must not remount the page")
	require.Len(t, dom.regions, 1)
	assert.False(t, dom.transitions[0], "typing never animates")

	byName := map[string]string{}
	for _, region := range dom.regions[0] {
		byName[region.Name] = region.HTML
	}
	assert.Contains(t, byName[historyRegionTimeline], "Go result")
	assert.Contains(t, byName[historyRegionFilters], "Showing 1 matching item")
	assert.Contains(t, byName, historyRegionAlerts)
	assert.Contains(t, byName, historyRegionDomains)
	assert.Contains(t, app.renderedHTML, "Go result", "full markup stays in sync for later mounts")
}

func TestLiveSearchEmptyQueryRestoresTimeline(t *testing.T) {
	t.Parallel()

	dom := &regionDOM{}
	history := &recordingHistoryService{
		entries:       []*entity.HistoryEntry{{ID: 1, URL: "https://example.com", Title: "Timeline entry"}},
		searchEntries: []*entity.HistoryEntry{{ID: 7, URL: "https://go.dev", Title: "Go result"}},
	}
	app := loadedHistoryApp(t, dom, history)

	ctx := context.Background()
	require.NoError(t, app.HandleDOMAction(ctx, DOMAction{Action: historyActionSearchLive, Data: map[string]string{"query": "go"}}))
	require.NoError(t, app.HandleDOMAction(ctx, DOMAction{Action: historyActionSearchLive, Data: map[string]string{"query": ""}}))

	require.Len(t, dom.regions, 2)
	assert.True(t, history.called, "clearing the query reloads the timeline window")
	assert.Contains(t, dom.regions[1][3].HTML, "Timeline entry")
	assert.NotContains(t, dom.regions[1][1].HTML, "matching")
}

func TestLiveSearchSameQueryIsNoop(t *testing.T) {
	t.Parallel()

	dom := &regionDOM{}
	history := &recordingHistoryService{entries: []*entity.HistoryEntry{{ID: 1, URL: "https://example.com"}}}
	app := loadedHistoryApp(t, dom, history)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: historyActionSearchLive, Data: map[string]string{"query": "  "}}))

	assert.Empty(t, dom.regions)
	assert.False(t, history.searchCalled)
}

func TestLiveSearchFallsBackToMountWithoutRegionSupport(t *testing.T) {
	t.Parallel()

	dom := &recordingDOM{}
	history := &recordingHistoryService{
		entries:       []*entity.HistoryEntry{{ID: 1, URL: "https://example.com"}},
		searchEntries: []*entity.HistoryEntry{{ID: 7, URL: "https://go.dev", Title: "Go result"}},
	}
	app := loadedHistoryApp(t, dom, history)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: historyActionSearchLive, Data: map[string]string{"query": "go"}}))

	assert.Contains(t, dom.HTML(), "Go result")
}

func TestHistoryFilterActionsSwapRegionsWithTransition(t *testing.T) {
	t.Parallel()

	for _, action := range []DOMAction{
		{Action: historyActionFilterDomain, Data: map[string]string{"domain": "example.com"}},
		{Action: historyActionClearDomain},
		{Action: historyActionClear},
		{Action: historyActionSearch, Data: map[string]string{"query": "example"}},
	} {
		t.Run(action.Action, func(t *testing.T) {
			t.Parallel()
			dom := &regionDOM{}
			history := &recordingHistoryService{entries: []*entity.HistoryEntry{{ID: 1, URL: "https://example.com", Title: "Example"}}}
			app := loadedHistoryApp(t, dom, history)
			mountedBefore := dom.HTML()

			require.NoError(t, app.HandleDOMAction(context.Background(), action))

			require.Len(t, dom.regions, 1)
			assert.True(t, dom.transitions[0], "filter changes animate")
			assert.Equal(t, mountedBefore, dom.HTML(), "filter changes keep the page mounted")
		})
	}
}

func TestHistoryDeleteKeepsFullRemount(t *testing.T) {
	t.Parallel()

	dom := &regionDOM{}
	history := &recordingHistoryService{entries: []*entity.HistoryEntry{{ID: 42, URL: "https://example.com", Title: "Example"}}}
	app := loadedHistoryApp(t, dom, history)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionDeleteRange,
		Data:   map[string]string{"range": "hour"},
	}))

	assert.Empty(t, dom.regions, "deletions change totals, so the page remounts")
	assert.Contains(t, dom.HTML(), "Deleted history from the last hour")
}

func TestLiveSearchBeforeInitialLoadRendersFullRoute(t *testing.T) {
	t.Parallel()

	dom := &regionDOM{}
	history := &recordingHistoryService{
		entries:       []*entity.HistoryEntry{{ID: 1, URL: "https://example.com", Title: "Example"}},
		searchEntries: []*entity.HistoryEntry{{ID: 7, URL: "https://go.dev", Title: "Go result"}},
		stats:         &entity.HistoryStats{TotalEntries: 3},
		domainStats:   []*entity.DomainStat{{Domain: "go.dev", PageCount: 2}},
	}
	app := NewApp(Dependencies{DOM: dom, History: history, LocationURI: "dumb://history"})
	app.LoadInitialShell(context.Background())
	require.Contains(t, app.renderedHTML, "Loading…")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: historyActionSearchLive,
		Data:   map[string]string{"query": "go"},
	}))

	assert.True(t, history.statsCalled, "first live search loads stats")
	assert.Empty(t, dom.regions, "full route is mounted, not region-swapped")
	require.NotNil(t, app.historyStats)
	assert.NotEmpty(t, app.historyDomainStats)
	assert.True(t, app.historyLoaded)
	assert.NotContains(t, dom.HTML(), "Loading…")
	assert.Contains(t, dom.HTML(), "Go result")
	assert.Contains(t, dom.HTML(), "go.dev")
}
