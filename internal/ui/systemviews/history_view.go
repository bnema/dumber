package systemviews

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	browserurl "github.com/bnema/dumber/internal/domain/url"
)

const historyURLDisplayMaxRunes = 180

type historyRenderData struct {
	Entries             []*entity.HistoryEntry
	Stats               *entity.HistoryStats
	Analytics           *entity.HistoryAnalytics
	Domains             []*entity.DomainStat
	Query               string
	DomainFilter        string
	Offset              int
	Limit               int
	WindowBefore        time.Time
	WindowAfter         time.Time
	HasMore             bool
	AppendSkipFirstDate string
	Notice              string
	Error               string
	Loading             bool
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

func historyTimelineAppendHTML(data historyRenderData) string {
	return mustRenderComponent(HistoryTimelineAppend(data))
}

func historyDocumentTitle(data historyRenderData) string {
	if data.Loading {
		return "History - Loading"
	}
	entries, visits, days := historySummaryValues(data)
	return fmt.Sprintf(
		"History - %s, %s, %s",
		historyCountLabel(entries, "entry", "entries"),
		historyCountLabel(visits, "visit", "visits"),
		historyCountLabel(days, "day", "days"),
	)
}

func historyCountLabel(count int64, singular, plural string) string {
	label := plural
	if count == 1 {
		label = singular
	}
	return fmt.Sprintf("%d %s", count, label)
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
		return 0
	}
	return data.Limit
}

func historyIsPaginated(data historyRenderData) bool {
	return historyLimit(data) > 0
}

func disableHistoryNext(data historyRenderData) bool {
	return !historyIsPaginated(data) || strings.TrimSpace(data.Query) != "" || countHistoryEntries(data.Entries) < historyLimit(data)
}

func showHistoryFilters(data historyRenderData) bool {
	return strings.TrimSpace(data.Query) != "" || strings.TrimSpace(data.DomainFilter) != ""
}

func historyShowingLabel(data historyRenderData) string {
	if data.Loading {
		return "Loading history…"
	}
	count := countHistoryEntries(data.Entries)
	if strings.TrimSpace(data.Query) != "" {
		return fmt.Sprintf("Showing %d matching item%s", count, pluralSuffix(count))
	}
	if strings.TrimSpace(data.DomainFilter) != "" {
		return fmt.Sprintf("Loaded %d domain item%s", count, pluralSuffix(count))
	}
	return fmt.Sprintf("Loaded %d item%s", count, pluralSuffix(count))
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func historyLoadMoreCursor(data historyRenderData) string {
	if data.WindowAfter.IsZero() {
		return ""
	}
	return data.WindowAfter.Format(time.RFC3339Nano)
}

func showHistoryLoadMore(data historyRenderData) bool {
	return !data.Loading && strings.TrimSpace(data.Query) == "" && data.HasMore
}

func historySummaryLoading(data historyRenderData) bool {
	return data.Loading
}

func historySummaryValues(data historyRenderData) (entries, visits, uniqueDays int64) {
	if data.Stats != nil {
		return data.Stats.TotalEntries, data.Stats.TotalVisits, data.Stats.UniqueDays
	}
	if data.Analytics != nil {
		return data.Analytics.TotalEntries, data.Analytics.TotalVisits, data.Analytics.UniqueDays
	}
	entries = int64(countHistoryEntries(data.Entries))
	visits = sumHistoryVisits(data.Entries)
	uniqueDays = countUniqueHistoryDays(data.Entries)

	return entries, visits, uniqueDays
}

func historyTimelineEmptyMessage(data historyRenderData) string {
	if data.Loading {
		return "Loading history…"
	}
	if strings.TrimSpace(data.Query) != "" || strings.TrimSpace(data.DomainFilter) != "" {
		return "No history entries match the current filters"
	}
	return "No history entries"
}

func lastHistoryDateKey(entries []*entity.HistoryEntry) string {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i] == nil {
			continue
		}
		dateKey, _ := historyDateKeyAndLabel(entries[i].LastVisited, time.Now().Local())
		return dateKey
	}
	return ""
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

func historyItemURL(entry *entity.HistoryEntry) string {
	if entry == nil {
		return ""
	}
	return truncateHistoryURL(entry.URL)
}

func historyItemFaviconURL(entry *entity.HistoryEntry) string {
	if entry == nil {
		return ""
	}
	domain := browserurl.CanonicalDomain(entry.URL)
	if domain == "" {
		return ""
	}
	query := url.Values{}
	query.Set("domain", domain)
	query.Set("size", "32")
	return "/api/favicon?" + query.Encode()
}

func historyFaviconClass(faviconURL string) string {
	if strings.TrimSpace(faviconURL) == "" {
		return "sv-history-favicon"
	}
	return "sv-history-favicon sv-history-favicon-has-image"
}

func historyFaviconFallback(entry *entity.HistoryEntry) string {
	label := ""
	if entry != nil {
		label = displayHistoryDomain(entry.URL)
	}
	if label == "" {
		return "•"
	}
	for _, r := range label {
		return strings.ToUpper(string(r))
	}
	return "•"
}

func truncateHistoryURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	runes := []rune(raw)
	if len(runes) <= historyURLDisplayMaxRunes {
		return raw
	}
	return string(runes[:historyURLDisplayMaxRunes-1]) + "…"
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
