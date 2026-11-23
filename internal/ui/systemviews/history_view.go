package systemviews

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bnema.dev/dumber/internal/domain/entity"
	browserurl "bnema.dev/dumber/internal/domain/url"
)

const historyURLDisplayMaxRunes = 180

// Named history regions that live updates replace in place.
const (
	historyRegionAlerts   = "history-alerts"
	historyRegionFilters  = "history-filters"
	historyRegionDomains  = "history-domains"
	historyRegionTimeline = "history-timeline"
)

// historyLiveRegions renders every region a live search can change. The
// search form is never part of a region, so typing keeps focus and caret.
func historyLiveRegions(data historyRenderData) []DOMRegion {
	return []DOMRegion{
		{Name: historyRegionAlerts, HTML: mustRenderComponent(HistoryAlerts(data))},
		{Name: historyRegionFilters, HTML: mustRenderComponent(HistoryFilters(data))},
		{Name: historyRegionDomains, HTML: mustRenderComponent(HistoryDomains(data))},
		{Name: historyRegionTimeline, HTML: mustRenderComponent(HistoryTimeline(data))},
	}
}

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
	return "History · " + historyTitleDetail(data)
}

func historyTitleDetail(data historyRenderData) string {
	if data.Loading {
		return "Loading…"
	}
	entries, visits, days := historySummaryValues(data)
	return strings.Join([]string{
		historyCountLabel(entries, "entry", "entries"),
		historyCountLabel(visits, "visit", "visits"),
		historyCountLabel(days, "day", "days"),
	}, " · ")
}

func historyCountLabel(count int64, singular, plural string) string {
	label := plural
	if count == 1 {
		label = singular
	}
	return groupThousands(count) + " " + label
}

// groupThousands formats 40947 as "40,947" so large counts stay scannable.
func groupThousands(count int64) string {
	digits := strconv.FormatInt(count, 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return sign + b.String()
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
		return fmt.Sprintf("Showing %d visit%s", count, pluralSuffix(count))
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
		return dateKey, local.Format("Mon 2 Jan")
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
	return truncateHistoryURL(stripURLScheme(entry.URL))
}

func stripURLScheme(raw string) string {
	raw = strings.TrimSpace(raw)
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(strings.ToLower(raw), prefix) {
			raw = raw[len(prefix):]
			break
		}
	}
	return strings.TrimSuffix(raw, "/")
}

func historyItemFaviconURL(entry *entity.HistoryEntry) string {
	if entry == nil || browserurl.IsSystemviewPageURL(entry.URL) {
		// Internal pages have no site favicon; never ask the resolver.
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

// historyItemSubtitle shows where the page lives (URL without scheme) and how
// often it was visited, e.g. "github.com/bnema/dumber · 12 visits".
func historyItemSubtitle(entry *entity.HistoryEntry) string {
	if entry == nil {
		return ""
	}
	parts := []string{historyItemURL(entry)}
	if entry.VisitCount > 1 {
		parts = append(parts, fmt.Sprintf("%d visits", entry.VisitCount))
	}
	return strings.Join(nonEmptyStrings(parts), " · ")
}

// historyItemTime is the row meta: relative for today's visits ("5m ago"),
// clock time for older days, since the day heading already carries the date.
func historyItemTime(entry *entity.HistoryEntry, now time.Time) string {
	if entry == nil || entry.LastVisited.IsZero() {
		return ""
	}
	visited := entry.LastVisited.In(now.Location())
	if visited.Format("2006-01-02") != now.Format("2006-01-02") {
		return visited.Format("15:04")
	}
	elapsed := now.Sub(visited)
	switch {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm ago", int(elapsed/time.Minute))
	default:
		return fmt.Sprintf("%dh ago", int(elapsed/time.Hour))
	}
}

func historyDomainIsActive(domainKey, filter string) bool {
	filter = strings.TrimSpace(filter)
	return filter != "" && strings.EqualFold(domainKey, filter)
}

func historyDomainChipClass(domainKey, filter string) string {
	if historyDomainIsActive(domainKey, filter) {
		return "sv-domain-chip sv-domain-chip-active"
	}
	return "sv-domain-chip"
}

func historyDomainPressed(domainKey, filter string) string {
	return strconv.FormatBool(historyDomainIsActive(domainKey, filter))
}

func historyDomainTitle(domain *entity.DomainStat) string {
	if domain == nil {
		return ""
	}
	return historyCountLabel(domain.PageCount, "page", "pages") + " · " + historyCountLabel(domain.TotalVisits, "visit", "visits")
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
