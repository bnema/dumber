package systemviews

import (
	"strings"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
)

func TestHistoryDateKeyAndLabelUsesProvidedNow(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("test", 2*60*60)
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, loc)

	todayKey, todayLabel := historyDateKeyAndLabel(time.Date(2026, 4, 24, 8, 30, 0, 0, loc), now)
	yesterdayKey, yesterdayLabel := historyDateKeyAndLabel(time.Date(2026, 4, 23, 22, 30, 0, 0, loc), now)
	olderKey, olderLabel := historyDateKeyAndLabel(time.Date(2026, 4, 20, 9, 0, 0, 0, loc), now)

	assert.Equal(t, "2026-04-24", todayKey)
	assert.Equal(t, "2026-04-23", yesterdayKey)
	assert.Equal(t, "2026-04-20", olderKey)
	assert.Equal(t, "Today", todayLabel)
	assert.Equal(t, "Yesterday", yesterdayLabel)
	assert.Equal(t, "Mon Apr 20", olderLabel)
}

func TestHistorySummaryValuesUsesAllProvidedEntries(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("test", 2*60*60)
	data := historyRenderData{Entries: []*entity.HistoryEntry{
		{VisitCount: 2, LastVisited: time.Date(2026, 4, 23, 8, 0, 0, 0, loc)},
		{VisitCount: 3, LastVisited: time.Date(2026, 4, 24, 8, 0, 0, 0, loc)},
	}}

	entries, visits, days := historySummaryValues(data)

	assert.Equal(t, int64(2), entries)
	assert.Equal(t, int64(5), visits)
	assert.Equal(t, int64(2), days)
}

func TestHistoryShowingLabelDescribesUnpaginatedDefault(t *testing.T) {
	t.Parallel()

	data := historyRenderData{Entries: []*entity.HistoryEntry{{}, {}}, Limit: 0}

	assert.Equal(t, "Loaded 2 items", historyShowingLabel(data))
	assert.True(t, disableHistoryNext(data))
}

func TestHistoryTimelineAppendMergesDuplicateFirstDayGroup(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("test", 2*60*60)
	data := historyRenderData{
		Entries: []*entity.HistoryEntry{
			{URL: "https://same.example", Title: "Same day", LastVisited: time.Date(2026, 4, 23, 8, 0, 0, 0, loc)},
			{URL: "https://older.example", Title: "Older day", LastVisited: time.Date(2026, 4, 22, 8, 0, 0, 0, loc)},
		},
		AppendSkipFirstDate: "2026-04-23",
	}

	html := historyTimelineAppendHTML(data)

	assert.NotContains(t, html, ">Thu Apr 23</h3>")
	assert.NotContains(t, html, `<section class="sv-timeline-group"><ul class="sv-list">`)
	assert.Contains(t, html, `data-sv-history-merge-date="2026-04-23"`)
	assert.Contains(t, html, "Same day")
	assert.Contains(t, html, `data-sv-history-date="2026-04-22"`)
	assert.Contains(t, html, ">Wed Apr 22</h3>")
}

func TestHistoryItemURLTruncatesLongURLs(t *testing.T) {
	t.Parallel()

	longURL := "https://example.com/" + strings.Repeat("a", historyURLDisplayMaxRunes)
	entry := &entity.HistoryEntry{URL: longURL}

	displayURL := historyItemURL(entry)

	assert.LessOrEqual(t, len([]rune(displayURL)), historyURLDisplayMaxRunes)
	assert.Contains(t, displayURL, "…")
	assert.NotContains(t, displayURL, strings.Repeat("a", historyURLDisplayMaxRunes))
}

func TestHistoryItemFaviconURLUsesLocalCachedFaviconAPI(t *testing.T) {
	t.Parallel()

	entry := &entity.HistoryEntry{URL: "https://example.com/docs?token=secret#section"}

	assert.Equal(t, "/api/favicon?domain=example.com&size=32", historyItemFaviconURL(entry))
}

func TestHistoryItemFaviconURLKeepsPortsForCacheKeys(t *testing.T) {
	t.Parallel()

	entry := &entity.HistoryEntry{URL: "http://localhost:1455/success?token=secret"}

	assert.Equal(t, "/api/favicon?domain=localhost%3A1455&size=32", historyItemFaviconURL(entry))
}

func TestHistoryItemFaviconURLIgnoresStoredRemoteFaviconURL(t *testing.T) {
	t.Parallel()

	entry := &entity.HistoryEntry{
		URL:        "https://example.com/docs",
		FaviconURL: "https://tracker.example/favicon.png",
	}

	assert.Equal(t, "/api/favicon?domain=example.com&size=32", historyItemFaviconURL(entry))
}
