package systemviews

import (
	"fmt"
	"html"
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

func historyHTML(data historyRenderData) string {
	sections := []string{
		historyStatusHTML(data),
		sectionHTML("sv-history-controls", "Manage history", historyControlsHTML(data)),
		sectionHTML("sv-history-summary", "History", historySummaryHTML(data)),
		sectionHTML("sv-history-domains", "Top domains", historyDomainsHTML(data.Domains)),
		sectionHTML("sv-history-timeline", "Recent visits", historyTimelineHTML(data)),
	}
	return strings.Join(sections, "")
}

func historyStatusHTML(data historyRenderData) string {
	var out strings.Builder
	if strings.TrimSpace(data.Notice) != "" {
		out.WriteString(fmt.Sprintf(`<div class="sv-alert sv-alert-success" role="status">%s</div>`, html.EscapeString(data.Notice)))
	}
	if strings.TrimSpace(data.Error) != "" {
		out.WriteString(fmt.Sprintf(`<div class="sv-alert sv-alert-error" role="alert">%s</div>`, html.EscapeString(data.Error)))
	}
	return out.String()
}

func historyControlsHTML(data historyRenderData) string {
	query := strings.TrimSpace(data.Query)
	domain := strings.TrimSpace(data.DomainFilter)
	limit := data.Limit
	if limit <= 0 {
		limit = historyTimelineLimit
	}
	offset := data.Offset
	if offset < 0 {
		offset = 0
	}

	var out strings.Builder
	out.WriteString(`<form class="sv-toolbar sv-history-search" data-sv-action="history.search">`)
	out.WriteString(fmt.Sprintf(`<label class="sv-search-label"><span>Search</span><input data-sv-history-search name="query" type="search" value="%s" placeholder="Search title or URL" autocomplete="off"></label>`, html.EscapeString(query)))
	out.WriteString(`<button class="sv-button" type="submit">Search</button>`)
	out.WriteString(`<button class="sv-button sv-button-secondary" type="button" data-sv-action="history.clear">Clear</button>`)
	out.WriteString(`</form>`)

	if query != "" || domain != "" {
		out.WriteString(`<div class="sv-active-filters">`)
		if query != "" {
			out.WriteString(fmt.Sprintf(`<span class="sv-filter-chip">Query: %s</span>`, html.EscapeString(query)))
		}
		if domain != "" {
			out.WriteString(fmt.Sprintf(`<span class="sv-filter-chip">Domain: %s <button type="button" data-sv-action="history.clearDomain" aria-label="Clear domain filter">×</button></span>`, html.EscapeString(domain)))
		}
		out.WriteString(`</div>`)
	}

	out.WriteString(`<div class="sv-history-actions-panel">`)
	out.WriteString(`<div class="sv-button-row" aria-label="History pagination">`)
	prevOffset := offset - limit
	if prevOffset < 0 {
		prevOffset = 0
	}
	out.WriteString(historyActionButton("Previous", historyActionPage, map[string]string{"offset": fmt.Sprintf("%d", prevOffset)}, false, offset == 0 || query != ""))
	out.WriteString(fmt.Sprintf(`<span class="sv-meta">Showing %d item%s%s</span>`, countHistoryEntries(data.Entries), pluralSuffix(countHistoryEntries(data.Entries)), historyOffsetLabel(offset, query)))
	out.WriteString(historyActionButton("Next", historyActionPage, map[string]string{"offset": fmt.Sprintf("%d", offset+limit)}, false, query != "" || countHistoryEntries(data.Entries) < limit))
	out.WriteString(`</div>`)

	out.WriteString(`<div class="sv-button-row" aria-label="History cleanup">`)
	out.WriteString(`<span class="sv-meta">Clean up</span>`)
	for _, item := range []struct {
		label   string
		rangeID string
	}{
		{label: "Last hour", rangeID: "hour"},
		{label: "Today", rangeID: "day"},
		{label: "Week", rangeID: "week"},
		{label: "Month", rangeID: "month"},
		{label: "All", rangeID: "all"},
	} {
		confirm := "Delete history for " + strings.ToLower(item.label) + "?"
		if item.rangeID == "all" {
			confirm = "Delete all browsing history?"
		}
		out.WriteString(historyActionButton(item.label, historyActionDeleteRange, map[string]string{"range": item.rangeID, "sv-confirm": confirm}, true, false))
	}
	out.WriteString(`</div>`)
	out.WriteString(`</div>`)

	out.WriteString(`<p class="sv-meta sv-key-hints">Keys: <kbd>/</kbd> search, <kbd>j</kbd>/<kbd>k</kbd> move, <kbd>Enter</kbd> open, <kbd>d</kbd> delete focused entry.</p>`)
	return out.String()
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

func historyActionButton(label, action string, data map[string]string, danger, disabled bool) string {
	classes := "sv-button sv-button-secondary"
	if danger {
		classes += " sv-button-danger"
	}
	attrs := []string{
		`type="button"`,
		fmt.Sprintf(`class="%s"`, html.EscapeString(classes)),
		fmt.Sprintf(`data-sv-action="%s"`, html.EscapeString(action)),
	}
	if disabled {
		attrs = append(attrs, `disabled aria-disabled="true"`)
	}
	for key, value := range data {
		attrs = append(attrs, fmt.Sprintf(`data-%s="%s"`, html.EscapeString(kebabDataKey(key)), html.EscapeString(value)))
	}
	return fmt.Sprintf(`<button %s>%s</button>`, strings.Join(attrs, " "), html.EscapeString(label))
}

func kebabDataKey(key string) string {
	return strings.ReplaceAll(key, "_", "-")
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func historySummaryHTML(data historyRenderData) string {
	entries := int64(countHistoryEntries(data.Entries))
	visits := sumHistoryVisits(data.Entries)
	uniqueDays := countUniqueHistoryDays(data.Entries)

	if data.Analytics != nil {
		entries = data.Analytics.TotalEntries
		visits = data.Analytics.TotalVisits
		uniqueDays = data.Analytics.UniqueDays
	}

	return fmt.Sprintf(`<div class="sv-stat-grid">%s%s%s</div>`,
		historyStatCardHTML("Entries", entries),
		historyStatCardHTML("Visits", visits),
		historyStatCardHTML("Days", uniqueDays),
	)
}

func historyStatCardHTML(label string, value int64) string {
	return fmt.Sprintf(`<div class="sv-stat-card"><span class="sv-stat-value">%d</span><span class="sv-stat-label">%s</span></div>`, value, html.EscapeString(label))
}

func historyDomainsHTML(domains []*entity.DomainStat) string {
	var chips strings.Builder
	for _, domain := range domains {
		if domain == nil || strings.TrimSpace(domain.Domain) == "" {
			continue
		}
		displayDomain := strings.TrimPrefix(domain.Domain, "www.")
		chips.WriteString(`<span class="sv-domain-chip">`)
		chips.WriteString(fmt.Sprintf(
			`<button type="button" class="sv-domain-filter" data-sv-action="history.filterDomain" data-domain="%s">%s</button>`,
			html.EscapeString(displayDomain),
			html.EscapeString(displayDomain),
		))
		chips.WriteString(fmt.Sprintf(`<span class="sv-domain-meta">%d pages · %d visits</span>`, domain.PageCount, domain.TotalVisits))
		chips.WriteString(fmt.Sprintf(
			`<button type="button" class="sv-icon-button sv-danger" data-sv-action="history.deleteDomain" data-domain="%s" data-sv-confirm="Delete all history for %s?" aria-label="Delete history for %s">×</button>`,
			html.EscapeString(displayDomain),
			html.EscapeString(displayDomain),
			html.EscapeString(displayDomain),
		))
		chips.WriteString(`</span>`)
	}
	if chips.Len() == 0 {
		return emptyStateHTML("No domain statistics yet")
	}
	return `<div class="sv-domain-list">` + chips.String() + `</div>`
}

func historyTimelineHTML(data historyRenderData) string {
	groups := groupHistoryEntries(data.Entries)
	if len(groups) == 0 {
		if strings.TrimSpace(data.Query) != "" || strings.TrimSpace(data.DomainFilter) != "" {
			return emptyStateHTML("No history entries match the current filters")
		}
		return emptyStateHTML("No history entries")
	}

	var body strings.Builder
	for _, group := range groups {
		body.WriteString(`<section class="sv-timeline-group">`)
		body.WriteString(`<h3>` + html.EscapeString(group.Label) + `</h3>`)
		body.WriteString(listHTML(historyItemsHTML(group.Entries), "No history entries"))
		body.WriteString(`</section>`)
	}
	return body.String()
}

func historyItemsHTML(entries []*entity.HistoryEntry) string {
	var items strings.Builder
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		items.WriteString(listRowHTML(historyItemHTML(entry)))
	}

	return items.String()
}

func historyItemHTML(entry *entity.HistoryEntry) string {
	label := strings.TrimSpace(entry.Title)
	if label == "" {
		label = displayHistoryDomain(entry.URL)
	}
	if label == "" {
		label = entry.URL
	}

	meta := []string{displayHistoryDomain(entry.URL)}
	if entry.VisitCount > 1 {
		meta = append(meta, fmt.Sprintf("%dx", entry.VisitCount))
	}
	if !entry.LastVisited.IsZero() {
		meta = append(meta, formatHistoryTime(entry.LastVisited))
	}

	deleteButton := historyActionButton("Delete", historyActionDeleteEntry, map[string]string{
		"id":         fmt.Sprintf("%d", entry.ID),
		"sv-confirm": "Delete this history entry?",
	}, true, entry.ID <= 0)

	return fmt.Sprintf(`<article class="sv-history-item" data-sv-history-row data-history-id="%d"><div class="sv-history-main"><a class="sv-link sv-history-open" href="%s">%s</a><p class="sv-history-url">%s</p><p class="sv-history-meta">%s</p></div><div class="sv-history-row-actions"><a class="sv-button sv-button-secondary" href="%s">Open</a>%s</div></article>`,
		entry.ID,
		html.EscapeString(sanitizeHref(entry.URL)),
		html.EscapeString(label),
		html.EscapeString(entry.URL),
		html.EscapeString(strings.Join(nonEmptyStrings(meta), " · ")),
		html.EscapeString(sanitizeHref(entry.URL)),
		deleteButton,
	)
}

type historyTimelineGroup struct {
	Date    string
	Label   string
	Entries []*entity.HistoryEntry
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

func nonEmptyStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
