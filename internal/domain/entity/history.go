package entity

import "time"

// HistoryEntry represents a visited URL in browsing history.
type HistoryEntry struct {
	ID          int64     `json:"id"`
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	FaviconURL  string    `json:"favicon_url"`
	VisitCount  int64     `json:"visit_count"`
	LastVisited time.Time `json:"last_visited"`
	CreatedAt   time.Time `json:"created_at"`
}

// NewHistoryEntry creates a new history entry for a URL.
func NewHistoryEntry(url, title string) *HistoryEntry {
	now := time.Now()
	return &HistoryEntry{
		URL:         url,
		Title:       title,
		VisitCount:  1,
		LastVisited: now,
		CreatedAt:   now,
	}
}

// IncrementVisit updates the entry for a new visit.
func (h *HistoryEntry) IncrementVisit() {
	h.VisitCount++
	h.LastVisited = time.Now()
}

// HistoryMatch represents a history entry that matched a search query.
// Used for fuzzy search results with scoring.
type HistoryMatch struct {
	Entry *HistoryEntry
	Score float64 // Match score (higher is better)
}

// HistoryStats contains aggregated history statistics.
type HistoryStats struct {
	TotalEntries int64 `json:"total_entries"`
	TotalVisits  int64 `json:"total_visits"`
	UniqueDays   int64 `json:"unique_days"`
}

// DomainStat contains per-domain visit statistics.
type DomainStat struct {
	Domain      string    `json:"domain"`
	PageCount   int64     `json:"page_count"`
	TotalVisits int64     `json:"total_visits"`
	LastVisit   time.Time `json:"last_visit"`
}

// HourlyDistribution contains visit counts by hour of day.
type HourlyDistribution struct {
	Hour       int   `json:"hour"`
	VisitCount int64 `json:"visit_count"`
}

// DailyVisitCount contains visit counts by day.
type DailyVisitCount struct {
	Day     string `json:"day"`
	Entries int64  `json:"entries"`
	Visits  int64  `json:"visits"`
}

// HistoryAnalytics contains all analytics data for the homepage.
type HistoryAnalytics struct {
	TotalEntries       int64                 `json:"total_entries"`
	TotalVisits        int64                 `json:"total_visits"`
	UniqueDays         int64                 `json:"unique_days"`
	TopDomains         []*DomainStat         `json:"top_domains"`
	DailyVisits        []*DailyVisitCount    `json:"daily_visits"`
	HourlyDistribution []*HourlyDistribution `json:"hourly_distribution"`
}
