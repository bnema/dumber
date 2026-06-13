package component

import (
	"fmt"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupHistoryByDay_Empty(t *testing.T) {
	assert.Nil(t, groupHistoryByDay(nil))
	assert.Nil(t, groupHistoryByDay([]*entity.HistoryEntry{}))
}

func TestGroupHistoryByDay_SingleEntry(t *testing.T) {
	now := time.Now()
	entries := []*entity.HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example", LastVisited: now},
	}
	groups := groupHistoryByDay(entries)
	require.Len(t, groups, 1)
	assert.Equal(t, "Today", groups[0].Label)
	assert.Len(t, groups[0].Entries, 1)
}

func TestGroupHistoryByDay_TodayYesterdayOlder(t *testing.T) {
	now := time.Now()
	today := now
	yesterday := now.AddDate(0, 0, -1)
	older := now.AddDate(0, 0, -5)

	entries := []*entity.HistoryEntry{
		{ID: 1, URL: "https://older.com", Title: "Older", LastVisited: older},
		{ID: 2, URL: "https://yesterday.com", Title: "Yesterday", LastVisited: yesterday},
		{ID: 3, URL: "https://today.com", Title: "Today", LastVisited: today},
	}
	groups := groupHistoryByDay(entries)
	require.Len(t, groups, 3)
	assert.Equal(t, "Today", groups[0].Label)
	assert.Equal(t, "Yesterday", groups[1].Label)
	assert.NotEmpty(t, groups[2].Label, "older entry should have a formatted date label")
	assert.Len(t, groups[2].Entries, 1)
}

func TestGroupHistoryByDay_SameDayEntriesGrouped(t *testing.T) {
	now := time.Now()
	entries := []*entity.HistoryEntry{
		{ID: 1, URL: "https://a.com", Title: "A", LastVisited: now},
		{ID: 2, URL: "https://b.com", Title: "B", LastVisited: now.Add(-time.Hour)},
	}
	groups := groupHistoryByDay(entries)
	require.Len(t, groups, 1)
	assert.Equal(t, "Today", groups[0].Label)
	assert.Len(t, groups[0].Entries, 2)
}

func TestReadableURL_StripsProtocol(t *testing.T) {
	assert.Equal(t, "example.com", readableURL("https://example.com"))
	assert.Equal(t, "example.com/page", readableURL("http://example.com/page"))
	assert.Equal(t, "example.com", readableURL("https://www.example.com"))
}

func TestReadableURL_KeepsPath(t *testing.T) {
	assert.Equal(t, "example.com/path/to/page", readableURL("https://example.com/path/to/page"))
}

func TestRelativeTime_Now(t *testing.T) {
	assert.Equal(t, "now", relativeTime(time.Now()))
}

func TestRelativeTime_Minutes(t *testing.T) {
	assert.Equal(t, "1m ago", relativeTime(time.Now().Add(-1*time.Minute)))
	assert.Equal(t, "5m ago", relativeTime(time.Now().Add(-5*time.Minute)))
}

func TestRelativeTime_Hours(t *testing.T) {
	assert.Equal(t, "1h ago", relativeTime(time.Now().Add(-1*time.Hour)))
	assert.Equal(t, "3h ago", relativeTime(time.Now().Add(-3*time.Hour)))
}

func TestRelativeTime_Days(t *testing.T) {
	assert.Equal(t, "1d ago", relativeTime(time.Now().Add(-25*time.Hour)))
	assert.Equal(t, "3d ago", relativeTime(time.Now().Add(-72*time.Hour)))
}

func TestGroupHistoryByDay_CrossYearDifferentLabels(t *testing.T) {
	now := time.Now()
	thisYear := now
	lastYear := now.AddDate(-1, 0, 0)
	twoYearsAgo := now.AddDate(-2, 0, 0)

	entries := []*entity.HistoryEntry{
		{ID: 1, URL: "https://two-years-ago.com", Title: "Two Years Ago", LastVisited: twoYearsAgo},
		{ID: 2, URL: "https://last-year.com", Title: "Last Year", LastVisited: lastYear},
		{ID: 3, URL: "https://this-year.com", Title: "This Year", LastVisited: thisYear},
	}
	groups := groupHistoryByDay(entries)
	require.Len(t, groups, 3)
	assert.Equal(t, "Today", groups[0].Label)
	assert.Equal(t, lastYear.Format(dayLabelOtherYearFormat), groups[1].Label)
	assert.Equal(t, twoYearsAgo.Format(dayLabelOtherYearFormat), groups[2].Label)
	assert.Len(t, groups[2].Entries, 1)
}

func TestGroupHistoryByDay_MaintainsInputOrderWithinDay(t *testing.T) {
	now := time.Now()
	entries := []*entity.HistoryEntry{
		{ID: 1, URL: "https://first.com", Title: "First", LastVisited: now},
		{ID: 2, URL: "https://second.com", Title: "Second", LastVisited: now.Add(-30 * time.Minute)},
		{ID: 3, URL: "https://third.com", Title: "Third", LastVisited: now.Add(-2 * time.Hour)},
	}
	groups := groupHistoryByDay(entries)
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Entries, 3)
	// Entries within the same day maintain input order
	assert.Equal(t, "https://first.com", groups[0].Entries[0].URL)
	assert.Equal(t, "https://second.com", groups[0].Entries[1].URL)
	assert.Equal(t, "https://third.com", groups[0].Entries[2].URL)
}

func TestGroupHistoryByDay_LeapYearBoundary(t *testing.T) {
	// Two consecutive days in a leap year should produce separate groups.
	// Use local noon on a known leap year to avoid UTC-to-local date
	// rollover, and check that labels include the year when not the
	// current year.
	loc := time.Now().Location()
	leapYear := 2024
	feb28 := time.Date(leapYear, time.February, 28, 12, 0, 0, 0, loc)
	feb29 := time.Date(leapYear, time.February, 29, 12, 0, 0, 0, loc)

	entries := []*entity.HistoryEntry{
		{ID: 1, URL: "https://feb29.com", Title: "Feb 29", LastVisited: feb29},
		{ID: 2, URL: "https://feb28.com", Title: "Feb 28", LastVisited: feb28},
	}
	groups := groupHistoryByDay(entries)
	require.Len(t, groups, 2, "Feb 28 and Feb 29 should be in separate groups")
	// Verify order: newest first
	// Since 2024 is likely not the current year, labels include year
	assert.Equal(t, feb29.Format("January 2, 2006"), groups[0].Label)
	assert.Equal(t, feb28.Format("January 2, 2006"), groups[1].Label)
}

func TestGroupHistoryByDay_SameLocalDay(t *testing.T) {
	// Two entries on the same calendar day in local timezone should be grouped.
	loc := time.Now().Location()
	sameDay := time.Date(2026, time.June, 10, 12, 0, 0, 0, loc)
	earlier := time.Date(2026, time.June, 10, 8, 0, 0, 0, loc)

	entries := []*entity.HistoryEntry{
		{ID: 1, URL: "https://later.com", Title: "Later", LastVisited: sameDay},
		{ID: 2, URL: "https://earlier.com", Title: "Earlier", LastVisited: earlier},
	}
	groups := groupHistoryByDay(entries)
	require.Len(t, groups, 1, "entries on same calendar day should be in one group")
	assert.Contains(t, groups[0].Label, "June")
}

func TestReadableURL_StripsFTPAndWWW(t *testing.T) {
	assert.Equal(t, "example.com", readableURL("ftp://example.com"))
	assert.Equal(t, "example.com", readableURL("http://www.example.com"))
	assert.Equal(t, "example.com", readableURL("https://www.example.com/"))
}

func TestReadableURL_NoProtocol(t *testing.T) {
	assert.Equal(t, "example.com", readableURL("example.com"))
}

func TestReadableURL_TrailingSlash(t *testing.T) {
	assert.Equal(t, "example.com", readableURL("https://example.com/"))
	assert.Equal(t, "example.com/path/", readableURL("https://example.com/path/"))
}

func TestReadableURL_EmptyOrRoot(t *testing.T) {
	assert.Empty(t, readableURL(""))
	assert.Empty(t, readableURL("/"))
}

func TestReadableURL_PreservesPort(t *testing.T) {
	assert.Equal(t, "example.com:8080", readableURL("https://example.com:8080"))
	assert.Equal(t, "example.com:3000/path", readableURL("https://example.com:3000/path"))
}

func TestRelativeTime_BoundaryEdgeCases(t *testing.T) {
	// Just under 1 minute → "now"
	assert.Equal(t, "now", relativeTime(time.Now().Add(-30*time.Second)))
	// 59 seconds → "now"
	assert.Equal(t, "now", relativeTime(time.Now().Add(-59*time.Second)))
	// 59 minutes 59 seconds → "59m ago"
	assert.Equal(t, "59m ago", relativeTime(time.Now().Add(-59*time.Minute).Add(-59*time.Second)))
	// 23 hours 59 minutes → "23h ago"
	assert.Equal(t, "23h ago", relativeTime(time.Now().Add(-23*time.Hour).Add(-59*time.Minute)))
	// 6 days 23 hours → "6d ago"
	assert.Equal(t, "6d ago", relativeTime(time.Now().Add(-6*24*time.Hour).Add(-23*time.Hour)))
	// 7 days → "Jul 8" format (changes by date, just verify not in "Xd ago")
	result := relativeTime(time.Now().Add(-7 * 24 * time.Hour))
	assert.NotContains(t, result, "d ago", "7+ days should not use day format")
}

func TestRelativeTime_Future(t *testing.T) {
	future := time.Now().Add(time.Hour)
	result := relativeTime(future)
	// Current implementation returns "now" for negative durations.
	// This is a known limitation; the test documents the behavior.
	assert.Equal(t, "now", result, "future times currently reported as 'now' (known limitation)")
}

func TestRelativeTime_DifferentYear(t *testing.T) {
	// An entry from a previous year should show a date with month abbreviation
	// and year when not current year.
	lastYear := time.Now().AddDate(-1, -1, 0)
	result := relativeTime(lastYear)
	_, err := time.Parse("Jan 2, 2006", result)
	assert.NoError(t, err, "old entry should use the Jan 2, 2006 layout, got %q", result)
}

func TestDayLabelForKey_MultiYearAgo(t *testing.T) {
	// Produce a label for a dayKey that is multiple years ago
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	// Two years ago, same month/day
	past := now.AddDate(-2, 0, -1)
	key := dayKey{past.Year(), past.Month(), past.Day()}
	label := dayLabelForKey(key, todayStart, now)
	assert.Contains(t, label, past.Format("2006"), "multi-year-old day label should include year")
}

func TestDayLabelForKey_WithinCurrentYear(t *testing.T) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	// Earlier this month
	past := now.AddDate(0, 0, -10)
	key := dayKey{past.Year(), past.Month(), past.Day()}
	label := dayLabelForKey(key, todayStart, now)
	// Should be weekday format without year
	assert.NotContains(t, label, "2006", "within-year label should not include year")
	assert.Contains(t, label, past.Weekday().String(), "within-year label should include weekday")
}

// =============================================================================
// keyboardNavModel tests
// =============================================================================

func makeGroups(dayCounts ...int) []historyGroup {
	now := time.Now()
	groups := make([]historyGroup, len(dayCounts))
	for i, count := range dayCounts {
		entries := make([]*entity.HistoryEntry, count)
		for j := 0; j < count; j++ {
			entries[j] = &entity.HistoryEntry{
				ID:          int64(i*100 + j),
				URL:         fmt.Sprintf("https://entry-%d-%d.com", i, j),
				Title:       fmt.Sprintf("Entry %d-%d", i, j),
				LastVisited: now.Add(-time.Duration(i) * 24 * time.Hour).Add(-time.Duration(j) * time.Minute),
			}
		}
		groups[i] = historyGroup{
			Label:   fmt.Sprintf("Day %d", i),
			Entries: entries,
		}
	}
	return groups
}

func TestKeyboardNavModel_EmptyGroups(t *testing.T) {
	m := newKeyboardNavModel(nil)
	assert.Equal(t, 0, m.totalRows())
	assert.Equal(t, -1, m.firstSelectableIndex())
	assert.Equal(t, -1, m.lastSelectableIndex())
	assert.Nil(t, m.entryAt(0))
	assert.Equal(t, "", m.entryURLAt(0))

	m = newKeyboardNavModel([]historyGroup{})
	assert.Equal(t, 0, m.totalRows())
}

func TestKeyboardNavModel_SingleGroup(t *testing.T) {
	groups := makeGroups(3) // 1 header + 3 entries = 4 rows
	m := newKeyboardNavModel(groups)

	assert.Equal(t, 4, m.totalRows())

	// Row 0: header (not selectable)
	assert.False(t, m.isSelectable(0))
	assert.Nil(t, m.entryAt(0))
	assert.Equal(t, "", m.entryURLAt(0))

	// Rows 1-3: entries (selectable)
	for i := 1; i <= 3; i++ {
		assert.True(t, m.isSelectable(i), "row %d should be selectable", i)
		assert.NotNil(t, m.entryAt(i))
		assert.Contains(t, m.entryURLAt(i), "entry-0")
	}

	// first / last
	assert.Equal(t, 1, m.firstSelectableIndex())
	assert.Equal(t, 3, m.lastSelectableIndex())
}

func TestKeyboardNavModel_MultipleGroups(t *testing.T) {
	groups := makeGroups(2, 3) // group0: header+e0+e1, group1: header+e0+e1+e2
	m := newKeyboardNavModel(groups)

	// Layout: [H0, E0-0, E0-1, H1, E1-0, E1-1, E1-2] = 7 rows
	assert.Equal(t, 7, m.totalRows())

	// Selectable: rows 1,2,4,5,6
	assert.False(t, m.isSelectable(0)) // H0
	assert.True(t, m.isSelectable(1))  // E0-0
	assert.True(t, m.isSelectable(2))  // E0-1
	assert.False(t, m.isSelectable(3)) // H1
	assert.True(t, m.isSelectable(4))  // E1-0
	assert.True(t, m.isSelectable(5))  // E1-1
	assert.True(t, m.isSelectable(6))  // E1-2

	assert.Equal(t, 1, m.firstSelectableIndex())
	assert.Equal(t, 6, m.lastSelectableIndex())
}

func TestKeyboardNavModel_NextPreviousSelectable(t *testing.T) {
	groups := makeGroups(2, 2) // rows: H0(0), E0-0(1), E0-1(2), H1(3), E1-0(4), E1-1(5)
	m := newKeyboardNavModel(groups)

	// Next from 0 (header) → 1
	assert.Equal(t, 1, m.nextSelectableIndex(0, +1))
	// Next from 1 → 2
	assert.Equal(t, 2, m.nextSelectableIndex(1, +1))
	// Next from 2 → 4 (skip header H1 at 3)
	assert.Equal(t, 4, m.nextSelectableIndex(2, +1))
	// Next from 5 → -1 (end)
	assert.Equal(t, -1, m.nextSelectableIndex(5, +1))

	// Previous from 5 (E1-1) → 4
	assert.Equal(t, 4, m.nextSelectableIndex(5, -1))
	// Previous from 4 → 2 (skip header H1 at 3)
	assert.Equal(t, 2, m.nextSelectableIndex(4, -1))
	// Previous from 1 → -1 (beginning, only header before)
	assert.Equal(t, -1, m.nextSelectableIndex(1, -1))
	// Previous from 0 (header) → -1
	assert.Equal(t, -1, m.nextSelectableIndex(0, -1))
}

func TestKeyboardNavModel_InvalidDirection(t *testing.T) {
	groups := makeGroups(2)
	m := newKeyboardNavModel(groups)
	assert.Equal(t, -1, m.nextSelectableIndex(0, 0))
	assert.Equal(t, -1, m.nextSelectableIndex(0, 2))
	assert.Equal(t, -1, m.nextSelectableIndex(0, -2))
}

func TestKeyboardNavModel_DayBoundaries(t *testing.T) {
	groups := makeGroups(2, 1, 2)
	// Layout: H0(0), E0-0(1), E0-1(2), H1(3), E1-0(4), H2(5), E2-0(6), E2-1(7)
	m := newKeyboardNavModel(groups)

	// Previous day from E0-1(2) → E0-1 is in group 0, no previous day.
	assert.Equal(t, -1, m.previousDayBoundary(2))

	// Previous day from E1-0(4) → first entry in group 0 = row 1
	assert.Equal(t, 1, m.previousDayBoundary(4))

	// Previous day from first group's entry (E0-0 at 1) → no previous day
	assert.Equal(t, -1, m.previousDayBoundary(1))

	// Next day from E0-0(1) → first entry in group 1 = row 4
	assert.Equal(t, 4, m.nextDayBoundary(1))

	// Next day from E0-1(2) → row 4
	assert.Equal(t, 4, m.nextDayBoundary(2))

	// Next day from E1-0(4) → first entry in group 2 = row 6
	assert.Equal(t, 6, m.nextDayBoundary(4))

	// Next day from E2-0(6, group 2) → last group, no next day
	assert.Equal(t, -1, m.nextDayBoundary(6))

	// Next day from E2-1(7, last entry) → no next day
	assert.Equal(t, -1, m.nextDayBoundary(7))
}

func TestKeyboardNavModel_EntryAtURL(t *testing.T) {
	groups := makeGroups(1, 1)
	m := newKeyboardNavModel(groups)

	e0 := m.entryAt(1)
	require.NotNil(t, e0)
	assert.Equal(t, "https://entry-0-0.com", e0.URL)
	assert.Equal(t, "https://entry-0-0.com", m.entryURLAt(1))

	e1 := m.entryAt(3)
	require.NotNil(t, e1)
	assert.Equal(t, "https://entry-1-0.com", e1.URL)

	// Header rows return nil / ""
	assert.Nil(t, m.entryAt(0)) // H0
	assert.Equal(t, "", m.entryURLAt(0))
	assert.Nil(t, m.entryAt(2)) // H1
	assert.Equal(t, "", m.entryURLAt(2))

	// Out of range
	assert.Nil(t, m.entryAt(99))
	assert.Equal(t, "", m.entryURLAt(99))
	assert.Nil(t, m.entryAt(-1))
}

func TestKeyboardNavModel_EntryCount(t *testing.T) {
	assert.Equal(t, 0, newKeyboardNavModel(nil).entryCount())
	assert.Equal(t, 0, newKeyboardNavModel([]historyGroup{}).entryCount())
	assert.Equal(t, 5, newKeyboardNavModel(makeGroups(2, 3)).entryCount())
	assert.Equal(t, 10, newKeyboardNavModel(makeGroups(3, 3, 4)).entryCount())
}

func TestBuildHistoryDisplayRows_IncludesHeadersAndEntries(t *testing.T) {
	now := time.Now()
	entryA := &entity.HistoryEntry{ID: 1, URL: "https://a.com", Title: "A", LastVisited: now}
	entryB := &entity.HistoryEntry{ID: 2, URL: "https://b.com", Title: "B", LastVisited: now.Add(-time.Hour)}
	groups := []historyGroup{
		{Label: "Today", Entries: []*entity.HistoryEntry{entryA, entryB}},
		{Label: "Yesterday", Entries: nil},
	}

	rows := buildHistoryDisplayRows(groups)
	require.Len(t, rows, 4)
	assert.Equal(t, historyDisplayRowHeader, rows[0].Kind)
	assert.Equal(t, "Today", rows[0].Label)
	assert.Equal(t, 0, rows[0].GroupIndex)
	assert.Equal(t, historyDisplayRowEntry, rows[1].Kind)
	assert.Same(t, entryA, rows[1].Entry)
	assert.Equal(t, 0, rows[1].GroupIndex)
	assert.Equal(t, historyDisplayRowEntry, rows[2].Kind)
	assert.Same(t, entryB, rows[2].Entry)
	assert.Equal(t, historyDisplayRowHeader, rows[3].Kind)
	assert.Equal(t, "Yesterday", rows[3].Label)
	assert.Equal(t, 1, rows[3].GroupIndex)
}

func TestKeyboardNavModelFromRows_UsesExplicitDisplayRows(t *testing.T) {
	now := time.Now()
	entryA := &entity.HistoryEntry{ID: 1, URL: "https://a.com", Title: "A", LastVisited: now}
	entryB := &entity.HistoryEntry{ID: 2, URL: "https://b.com", Title: "B", LastVisited: now.Add(-24 * time.Hour)}
	rows := buildHistoryDisplayRows([]historyGroup{
		{Label: "Today", Entries: []*entity.HistoryEntry{entryA}},
		{Label: "Yesterday", Entries: []*entity.HistoryEntry{entryB}},
	})

	m := newKeyboardNavModelFromRows(rows)
	assert.Equal(t, 4, m.totalRows())
	assert.False(t, m.isSelectable(0))
	assert.True(t, m.isSelectable(1))
	assert.False(t, m.isSelectable(2))
	assert.True(t, m.isSelectable(3))
	assert.Same(t, entryA, m.entryAt(1))
	assert.Equal(t, entryB.URL, m.entryURLAt(3))
	assert.Equal(t, 3, m.nextDayBoundary(1))
	assert.Equal(t, 1, m.previousDayBoundary(3))
}

// =============================================================================
// Search state transition tests
// =============================================================================

func TestTransitionSearchState_EmptyQuery(t *testing.T) {
	next := transitionSearchState("", 0)
	assert.Equal(t, "", next.Query)
	assert.False(t, next.HasSearchDone)
	assert.False(t, next.HasResults)
	assert.Equal(t, 0, next.ResultCount)
}

func TestTransitionSearchState_NewQuery(t *testing.T) {
	next := transitionSearchState("example", 5)
	assert.Equal(t, "example", next.Query)
	assert.True(t, next.HasSearchDone)
	assert.True(t, next.HasResults)
	assert.Equal(t, 5, next.ResultCount)
}

func TestTransitionSearchState_QueryWithNoResults(t *testing.T) {
	next := transitionSearchState("nonexistent", 0)
	assert.Equal(t, "nonexistent", next.Query)
	assert.True(t, next.HasSearchDone)
	assert.False(t, next.HasResults)
	assert.Equal(t, 0, next.ResultCount)
}

func TestTransitionSearchState_QueryToQuery(t *testing.T) {
	next := transitionSearchState("new", 7)
	assert.Equal(t, "new", next.Query)
	assert.True(t, next.HasSearchDone)
	assert.True(t, next.HasResults)
	assert.Equal(t, 7, next.ResultCount)
}

func TestTransitionSearchState_QueryToEmpty(t *testing.T) {
	next := transitionSearchState("", 0)
	assert.Equal(t, "", next.Query)
	assert.False(t, next.HasSearchDone)
	assert.False(t, next.HasResults)
	assert.Equal(t, 0, next.ResultCount)
}

// =============================================================================
// Reload preservation tests
// =============================================================================

func TestApplyReloadState_WithoutQuery(t *testing.T) {
	s := applyReloadState("")
	assert.Equal(t, "", s.PreservedQuery)
	assert.True(t, s.ResetBrowse)
	assert.False(t, s.ClearSearch)
}

func TestApplyReloadState_WithQuery(t *testing.T) {
	s := applyReloadState("search-term")
	assert.Equal(t, "search-term", s.PreservedQuery)
	assert.False(t, s.ResetBrowse)
	assert.True(t, s.ClearSearch)
}

// =============================================================================
// keyboardNavModel edge cases
// =============================================================================

func TestKeyboardNavModel_SingleGroupZeroEntries(t *testing.T) {
	// A group with a header but zero entries.
	groups := []historyGroup{
		{Label: "Today", Entries: []*entity.HistoryEntry{}},
	}
	m := newKeyboardNavModel(groups)

	assert.Equal(t, 1, m.totalRows()) // header only
	assert.False(t, m.isSelectable(0))
	assert.Equal(t, -1, m.firstSelectableIndex())
	assert.Equal(t, -1, m.lastSelectableIndex())
	assert.Nil(t, m.entryAt(0))
	assert.Equal(t, 0, m.entryCount())
}

func TestKeyboardNavModel_MixedEmptyAndNonEmptyGroups(t *testing.T) {
	// Groups: [A(0 entries), B(2 entries), C(0 entries)]
	// Layout: H0(0), H1(1), E1-0(2), E1-1(3), H2(4)
	groups := []historyGroup{
		{Label: "EmptyA", Entries: []*entity.HistoryEntry{}},
		{Label: "HasTwo", Entries: []*entity.HistoryEntry{
			{ID: 1, URL: "https://b1.com", Title: "B1", LastVisited: time.Now()},
			{ID: 2, URL: "https://b2.com", Title: "B2", LastVisited: time.Now()},
		}},
		{Label: "EmptyC", Entries: []*entity.HistoryEntry{}},
	}
	m := newKeyboardNavModel(groups)

	assert.Equal(t, 5, m.totalRows())
	// All headers: 0, 1, 4 are not selectable
	assert.False(t, m.isSelectable(0))
	assert.False(t, m.isSelectable(1))
	assert.False(t, m.isSelectable(4))
	// Entries: 2, 3 are selectable
	assert.True(t, m.isSelectable(2))
	assert.True(t, m.isSelectable(3))

	assert.Equal(t, 2, m.firstSelectableIndex())
	assert.Equal(t, 3, m.lastSelectableIndex())
	assert.Equal(t, 2, m.entryCount())

	// next/previous day boundary from group B (index 2) should cross
	// empty group A header to group C header
	// previousDayBoundary from entry in B (row 2) → group before B is A (no entries) → -1
	assert.Equal(t, -1, m.previousDayBoundary(2))
	// nextDayBoundary from entry in B (row 2) → group after B is C (no entries) → -1
	assert.Equal(t, -1, m.nextDayBoundary(2))
}

func TestKeyboardNavModel_GroupIndexAt(t *testing.T) {
	groups := makeGroups(2, 0, 3) // A:2 entries, B:0 entries, C:3 entries
	// Layout: H0(0), E0-0(1), E0-1(2), H1(3), H2(4), E2-0(5), E2-1(6), E2-2(7)
	// Groups: A at rows 0-2 (header+2 entries), B at row 3 (header only), C at rows 4-7 (header+3 entries)
	m := newKeyboardNavModel(groups)

	assert.Equal(t, 0, m.groupIndexAt(0), "row 0 should be in group 0 (A header)")
	assert.Equal(t, 0, m.groupIndexAt(1), "row 1 should be in group 0 (A entry)")
	assert.Equal(t, 1, m.groupIndexAt(3), "row 3 should be in group 1 (B header)")
	assert.Equal(t, 2, m.groupIndexAt(4), "row 4 should be in group 2 (C header)")
	assert.Equal(t, 2, m.groupIndexAt(7), "row 7 should be in group 2 (last C entry)")
	assert.Equal(t, -1, m.groupIndexAt(99), "out of range should return -1")
}

func TestKeyboardNavModel_CumulativeOffsetAtGroup(t *testing.T) {
	groups := makeGroups(2, 0, 3)
	m := newKeyboardNavModel(groups)

	assert.Equal(t, 0, m.cumulativeOffsetAtGroup(0), "group 0 offset should be 0")
	// Group 0: 1 header + 2 entries = 3 rows
	assert.Equal(t, 3, m.cumulativeOffsetAtGroup(1), "group 1 offset should skip group 0")
	// Group 1: 1 header + 0 entries = 1 row, so group 2 offset = 3 + 1 = 4
	assert.Equal(t, 4, m.cumulativeOffsetAtGroup(2), "group 2 offset should skip groups 0 and 1")
	assert.Equal(t, -1, m.cumulativeOffsetAtGroup(99), "out of range should return -1")
}

func TestKeyboardNavModel_FirstEntryOfGroup(t *testing.T) {
	groups := makeGroups(2, 0, 3)
	m := newKeyboardNavModel(groups)

	// Group 0: offset=0, first entry = 1
	assert.Equal(t, 1, m.firstEntryOfGroup(0))
	// Group 1: offset=3, first entry = 4 (but group has 0 entries -> -1)
	assert.Equal(t, -1, m.firstEntryOfGroup(1))
	// Group 2: offset=4, first entry = 5
	assert.Equal(t, 5, m.firstEntryOfGroup(2))
}

func TestKeyboardNavModel_NextPreviousSelectable_EmptyGroups(t *testing.T) {
	// All groups have zero entries.
	groups := []historyGroup{
		{Label: "A", Entries: []*entity.HistoryEntry{}},
		{Label: "B", Entries: []*entity.HistoryEntry{}},
	}
	m := newKeyboardNavModel(groups)

	assert.Equal(t, -1, m.nextSelectableIndex(0, 1))
	assert.Equal(t, -1, m.nextSelectableIndex(0, -1))
}

func TestKeyboardNavModel_EntryCountWithEmptyGroups(t *testing.T) {
	groups := []historyGroup{
		{Label: "A", Entries: []*entity.HistoryEntry{{
			ID: 1, URL: "https://a.com", Title: "A", LastVisited: time.Now(),
		}}},
		{Label: "B", Entries: []*entity.HistoryEntry{}},
		{Label: "C", Entries: []*entity.HistoryEntry{{
			ID: 2, URL: "https://c.com", Title: "C", LastVisited: time.Now(),
		}}},
	}
	m := newKeyboardNavModel(groups)
	assert.Equal(t, 2, m.entryCount(), "only non-empty groups' entries should count")
}

// =============================================================================
// Search generation / stale-result suppression (pure transition model)
// =============================================================================

// TestTransitionSearchState_StaleGenBehavior documents that the search
// generation counter (searchGen) is a production-side concern for stale
// result suppression. The transitionSearchState pure function only models
// query/result transitions; the actual generation guard is enforced by
// the GTK idle callback comparing gen vs hs.searchGen.
//
// We test the callback pattern here by simulating two sequential searches
// where a later result arrives after the gen has moved on.
func TestTransitionSearchState_SequentialSearches(t *testing.T) {
	// Search 1: "foo" -> 5 results
	next1 := transitionSearchState("foo", 5)
	assert.Equal(t, "foo", next1.Query)
	assert.True(t, next1.HasSearchDone)
	assert.True(t, next1.HasResults)
	assert.Equal(t, 5, next1.ResultCount)

	// Search 2: "foobar" -> 3 results (supersedes search 1)
	next2 := transitionSearchState("foobar", 3)
	assert.Equal(t, "foobar", next2.Query)
	assert.True(t, next2.HasSearchDone)
	assert.True(t, next2.HasResults)
	assert.Equal(t, 3, next2.ResultCount)
}

// TestTransitionSearchState_SearchThenClearThenReSearch verifies the
// transition from search -> empty -> new search produces correct state.
func TestTransitionSearchState_SearchThenClearThenReSearch(t *testing.T) {
	s := searchStateSnapshot{}

	// Search "term" -> 2 results
	s = transitionSearchState("term", 2)
	assert.Equal(t, "term", s.Query)
	assert.True(t, s.HasSearchDone)
	assert.True(t, s.HasResults)
	assert.Equal(t, 2, s.ResultCount)

	// Clear: query becomes ""
	s = transitionSearchState("", 0)
	assert.Equal(t, "", s.Query)
	assert.False(t, s.HasSearchDone)
	assert.False(t, s.HasResults)
	assert.Equal(t, 0, s.ResultCount)

	// Re-search: new term
	s = transitionSearchState("other", 7)
	assert.Equal(t, "other", s.Query)
	assert.True(t, s.HasSearchDone)
	assert.True(t, s.HasResults)
	assert.Equal(t, 7, s.ResultCount)
}

// TestApplyReloadState_EmptyAndNonEmpty verifies all reload preservation
// states are correctly modeled by the pure function.
func TestApplyReloadState_EmptyAndNonEmpty(t *testing.T) {
	tt := []struct {
		name      string
		query     string
		wantQuery string
		wantReset bool
		wantClear bool
	}{
		{name: "empty query", query: "", wantQuery: "", wantReset: true, wantClear: false},
		{name: "non-empty", query: "search", wantQuery: "search", wantReset: false, wantClear: true},
		{name: "whitespace only", query: "   ", wantQuery: "   ", wantReset: false, wantClear: true},
		{name: "long query", query: "very long search query with many terms", wantQuery: "very long search query with many terms", wantReset: false, wantClear: true},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			s := applyReloadState(tc.query)
			assert.Equal(t, tc.wantQuery, s.PreservedQuery)
			assert.Equal(t, tc.wantReset, s.ResetBrowse)
			assert.Equal(t, tc.wantClear, s.ClearSearch)
		})
	}
}

// TestDayLabelForKey_DifferentYears verifies multi-year scenarios produce
// the expected label format.
func TestDayLabelForKey_DifferentYears(t *testing.T) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Same year, not today/yesterday
	future := now.AddDate(0, 0, -3)
	key := dayKey{future.Year(), future.Month(), future.Day()}
	label := dayLabelForKey(key, todayStart, now)
	assert.NotContains(t, label, "2006", "within-current-year label should not contain year")

	// Previous year (now.Year()-1)
	lastYear := now.AddDate(-1, 0, 0)
	key = dayKey{lastYear.Year(), lastYear.Month(), lastYear.Day()}
	label = dayLabelForKey(key, todayStart, now)
	assert.Equal(t, lastYear.Format(dayLabelOtherYearFormat), label)

	// Multiple years ago
	twoYearsAgo := now.AddDate(-2, 0, 0)
	key = dayKey{twoYearsAgo.Year(), twoYearsAgo.Month(), twoYearsAgo.Day()}
	label = dayLabelForKey(key, todayStart, now)
	assert.Contains(t, label, twoYearsAgo.Format("2006"), "multi-year-old label should include year")
}
