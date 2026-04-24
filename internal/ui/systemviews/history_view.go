package systemviews

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
)

type historyRenderData struct {
	Entries      []*entity.HistoryEntry
	Analytics    *entity.HistoryAnalytics
	Domains      []*entity.DomainStat
	Query        string
	DomainFilter string
	Offset       int
	Limit        int
	Notice       string
	Error        string
}

type historyTimelineGroup struct {
	Date    string
	Label   string
	Entries []*entity.HistoryEntry
}

type historyCleanupItem struct {
	Label   string
	RangeID string
	Confirm string
}

func historyHTML(data historyRenderData) string {
	return mustRenderComponent(HistoryView(data))
}

func historyCleanupItems() []historyCleanupItem {
	return []historyCleanupItem{
		{Label: "Last hour", RangeID: "hour", Confirm: "Delete history for last hour?"},
		{Label: "Today", RangeID: "day", Confirm: "Delete history for today?"},
		{Label: "Week", RangeID: "week", Confirm: "Delete history for week?"},
		{Label: "Month", RangeID: "month", Confirm: "Delete history for month?"},
		{Label: "All", RangeID: "all", Confirm: "Delete all browsing history?"},
	}
}

func historyLimit(data historyRenderData) int {
	if data.Limit <= 0 {
		return historyTimelineLimit
	}
	return data.Limit
}

func previousHistoryOffset(data historyRenderData) int {
	offset := data.Offset - historyLimit(data)
	if offset < 0 {
		return 0
	}
	return offset
}

func nextHistoryOffset(data historyRenderData) int {
	return max(data.Offset, 0) + historyLimit(data)
}

func disableHistoryPrev(data historyRenderData) bool {
	return data.Offset <= 0 || strings.TrimSpace(data.Query) != ""
}

func disableHistoryNext(data historyRenderData) bool {
	return strings.TrimSpace(data.Query) != "" || countHistoryEntries(data.Entries) < historyLimit(data)
}

func showHistoryFilters(data historyRenderData) bool {
	return strings.TrimSpace(data.Query) != "" || strings.TrimSpace(data.DomainFilter) != ""
}

func historyOffsetLabel(offset int, query string) string {
	if strings.TrimSpace(query) != "" {
		return " matching search"
	}
	if offset <= 0 {
		return ""
	}
	return fmt.Sprintf(" from offset %d", offset)
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func historySummaryValues(data historyRenderData) (entries, visits, uniqueDays int64) {
	entries = int64(countHistoryEntries(data.Entries))
	visits = sumHistoryVisits(data.Entries)
	uniqueDays = countUniqueHistoryDays(data.Entries)

	if data.Analytics != nil {
		entries = data.Analytics.TotalEntries
		visits = data.Analytics.TotalVisits
		uniqueDays = data.Analytics.UniqueDays
	}
	return entries, visits, uniqueDays
}

func historyTimelineEmptyMessage(data historyRenderData) string {
	if strings.TrimSpace(data.Query) != "" || strings.TrimSpace(data.DomainFilter) != "" {
		return "No history entries match the current filters"
	}
	return "No history entries"
}

func groupHistoryEntries(entries []*entity.HistoryEntry) []historyTimelineGroup {
	groups := make([]historyTimelineGroup, 0)
	indexByDate := map[string]int{}
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		dateKey, label := historyDateKeyAndLabel(entry.LastVisited)
		idx, ok := indexByDate[dateKey]
		if !ok {
			idx = len(groups)
			indexByDate[dateKey] = idx
			groups = append(groups, historyTimelineGroup{Date: dateKey, Label: label})
		}
		groups[idx].Entries = append(groups[idx].Entries, entry)
	}
	return groups
}

func historyDateKeyAndLabel(ts time.Time) (string, string) {
	if ts.IsZero() {
		return "unknown", "Unknown date"
	}

	local := ts.Local()
	dateKey := local.Format("2006-01-02")
	today := time.Now().Local()
	todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	entryStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())

	switch {
	case entryStart.Equal(todayStart):
		return dateKey, "Today"
	case entryStart.Equal(yesterdayStart):
		return dateKey, "Yesterday"
	default:
		return dateKey, local.Format("Mon Jan 2")
	}
}

func countHistoryEntries(entries []*entity.HistoryEntry) int {
	count := 0
	for _, entry := range entries {
		if entry != nil {
			count++
		}
	}
	return count
}

func sumHistoryVisits(entries []*entity.HistoryEntry) int64 {
	var visits int64
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.VisitCount <= 0 {
			visits++
			continue
		}
		visits += entry.VisitCount
	}
	return visits
}

func countUniqueHistoryDays(entries []*entity.HistoryEntry) int64 {
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if entry == nil || entry.LastVisited.IsZero() {
			continue
		}
		seen[entry.LastVisited.Local().Format("2006-01-02")] = struct{}{}
	}
	return int64(len(seen))
}

func filterHistoryEntriesByDomain(entries []*entity.HistoryEntry, domain string) []*entity.HistoryEntry {
	domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "www.")
	if domain == "" {
		return entries
	}
	filtered := make([]*entity.HistoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		entryDomain := strings.TrimPrefix(strings.ToLower(displayHistoryDomain(entry.URL)), "www.")
		if entryDomain == domain {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func displayHistoryDomain(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return strings.TrimSpace(raw)
	}
	return strings.TrimPrefix(parsed.Hostname(), "www.")
}

func formatHistoryTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Local().Format("15:04")
}

func historyItemLabel(entry *entity.HistoryEntry) string {
	if entry == nil {
		return ""
	}
	label := strings.TrimSpace(entry.Title)
	if label == "" {
		label = displayHistoryDomain(entry.URL)
	}
	if label == "" {
		label = entry.URL
	}
	return label
}

func historyItemMeta(entry *entity.HistoryEntry) string {
	if entry == nil {
		return ""
	}
	meta := []string{displayHistoryDomain(entry.URL)}
	if entry.VisitCount > 1 {
		meta = append(meta, fmt.Sprintf("%dx", entry.VisitCount))
	}
	if !entry.LastVisited.IsZero() {
		meta = append(meta, formatHistoryTime(entry.LastVisited))
	}
	return strings.Join(nonEmptyStrings(meta), " · ")
}

func nonEmptyStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
