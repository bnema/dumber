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
	Entries   []*entity.HistoryEntry
	Analytics *entity.HistoryAnalytics
	Domains   []*entity.DomainStat
}

func historyHTML(data historyRenderData) string {
	sections := []string{
		sectionHTML("sv-history-summary", "History", historySummaryHTML(data)),
		sectionHTML("sv-history-domains", "Top domains", historyDomainsHTML(data.Domains)),
		sectionHTML("sv-history-timeline", "Recent visits", historyTimelineHTML(data.Entries)),
	}
	return strings.Join(sections, "")
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
		chips.WriteString(fmt.Sprintf(
			`<span class="sv-domain-chip"><span>%s</span><span class="sv-domain-meta">%d pages · %d visits</span></span>`,
			html.EscapeString(strings.TrimPrefix(domain.Domain, "www.")),
			domain.PageCount,
			domain.TotalVisits,
		))
	}
	if chips.Len() == 0 {
		return emptyStateHTML("No domain statistics yet")
	}
	return `<div class="sv-domain-list">` + chips.String() + `</div>`
}

func historyTimelineHTML(entries []*entity.HistoryEntry) string {
	groups := groupHistoryEntries(entries)
	if len(groups) == 0 {
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

	return fmt.Sprintf(`<article class="sv-history-item"><div class="sv-history-main">%s<p class="sv-history-url">%s</p></div><p class="sv-history-meta">%s</p></article>`,
		linkHTML(entry.URL, label),
		html.EscapeString(entry.URL),
		html.EscapeString(strings.Join(nonEmptyStrings(meta), " · ")),
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
