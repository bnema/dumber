package systemviews

import (
	"fmt"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	browserurl "github.com/bnema/dumber/internal/domain/url"
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
	Notice  string
}

func historyHTML(data historyRenderData) string {
	return mustRenderComponent(HistoryView(data))
}

func historyDocumentTitle(data historyRenderData) string {
	if query := strings.TrimSpace(data.Query); query != "" {
		return "History — search: " + truncateTitle(query, 48)
	}
	if domain := strings.TrimSpace(data.DomainFilter); domain != "" {
		label := browserurl.DisplayDomain(domain)
		if label == "" {
			label = domain
		}
		return "History — " + label
	}

	entries := int64(countHistoryEntries(data.Entries))
	if data.Analytics != nil {
		entries = data.Analytics.TotalEntries
	}
	if entries == 1 {
		return "History — 1 entry"
	}
	return fmt.Sprintf("History — %d entries", entries)
}

func truncateTitle(value string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(value))
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "…"
}

var historyCleanupRanges = []historyCleanupItem{
	{Label: "Last hour", RangeID: "hour", Confirm: "Delete visits from the last hour?", Notice: "Deleted history from the last hour"},
	{Label: "Today", RangeID: "day", Confirm: "Delete visits from today?", Notice: "Deleted history from today"},
	{Label: "Week", RangeID: "week", Confirm: "Delete visits from the last week?", Notice: "Deleted history from this week"},
	{Label: "Month", RangeID: "month", Confirm: "Delete visits from the last month?", Notice: "Deleted history from this month"},
	{Label: "All", RangeID: "all", Confirm: "Delete all browsing history?", Notice: "Deleted all history"},
}

func historyCleanupItems() []historyCleanupItem {
	return historyCleanupRanges
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
	now := time.Now().Local()
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		dateKey, label := historyDateKeyAndLabel(entry.LastVisited, now)
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

func historyDateKeyAndLabel(ts, now time.Time) (string, string) {
	if ts.IsZero() {
		return "unknown", "Unknown date"
	}

	local := ts.Local()
	dateKey := local.Format("2006-01-02")
	today := now.Local()
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
		// Each HistoryEntry represents at least one visit, even when VisitCount
		// is unset or non-positive in older persisted rows.
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

func displayHistoryDomain(raw string) string {
	return browserurl.DisplayDomain(raw)
}

func historyDomainActionKey(domain *entity.DomainStat) string {
	if domain == nil {
		return ""
	}
	return browserurl.CanonicalDomain(domain.Domain)
}

func historyDomainDisplayLabel(domain *entity.DomainStat) string {
	if domain == nil {
		return ""
	}
	return displayHistoryDomain(domain.Domain)
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
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
