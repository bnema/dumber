package entity

import "time"

// HistoryEntry represents a visited URL in browsing history.
type HistoryEntry struct {
	ID          int64
	URL         string
	Title       string
	FaviconURL  string
	VisitCount  int64
	LastVisited time.Time
	CreatedAt   time.Time
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
