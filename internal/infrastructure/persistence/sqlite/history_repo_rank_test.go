package sqlite

import (
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
)

func TestSortRankedHistoryMatches(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	older := now.Add(-24 * time.Hour)

	items := []rankedHistoryMatch{
		{
			match:         entity.HistoryMatch{Entry: &entity.HistoryEntry{ID: 1, URL: "z-url", VisitCount: 5, LastVisited: now}, Score: 10},
			score:         10,
			exactURLMatch: false,
			prefixURL:     false,
			urlMatch:      false,
			prefixTitle:   false,
		},
		{
			match:         entity.HistoryMatch{Entry: &entity.HistoryEntry{ID: 2, URL: "a-url", VisitCount: 5, LastVisited: now}, Score: 20},
			score:         20,
			exactURLMatch: false,
			prefixURL:     false,
			urlMatch:      false,
			prefixTitle:   false,
		},
		{
			match:         entity.HistoryMatch{Entry: &entity.HistoryEntry{ID: 3, URL: "b-url", VisitCount: 5, LastVisited: now}, Score: 10},
			score:         10,
			exactURLMatch: true,
			prefixURL:     false,
			urlMatch:      false,
			prefixTitle:   false,
		},
		{
			match:         entity.HistoryMatch{Entry: &entity.HistoryEntry{ID: 4, URL: "c-url", VisitCount: 5, LastVisited: now}, Score: 10},
			score:         10,
			exactURLMatch: false,
			prefixURL:     true,
			urlMatch:      false,
			prefixTitle:   false,
		},
		{
			match:         entity.HistoryMatch{Entry: &entity.HistoryEntry{ID: 5, URL: "d-url", VisitCount: 5, LastVisited: now}, Score: 10},
			score:         10,
			exactURLMatch: false,
			prefixURL:     false,
			urlMatch:      true,
			prefixTitle:   false,
		},
		{
			match:         entity.HistoryMatch{Entry: &entity.HistoryEntry{ID: 6, URL: "e-url", VisitCount: 5, LastVisited: now}, Score: 10},
			score:         10,
			exactURLMatch: false,
			prefixURL:     false,
			urlMatch:      false,
			prefixTitle:   true,
		},
		{
			match:         entity.HistoryMatch{Entry: &entity.HistoryEntry{ID: 7, URL: "f-url", VisitCount: 10, LastVisited: now}, Score: 10},
			score:         10,
			exactURLMatch: false,
			prefixURL:     false,
			urlMatch:      false,
			prefixTitle:   false,
		},
		{
			match:         entity.HistoryMatch{Entry: &entity.HistoryEntry{ID: 8, URL: "g-url", VisitCount: 5, LastVisited: now}, Score: 10},
			score:         10,
			exactURLMatch: false,
			prefixURL:     false,
			urlMatch:      false,
			prefixTitle:   false,
		},
		{
			match:         entity.HistoryMatch{Entry: &entity.HistoryEntry{ID: 9, URL: "h-url", VisitCount: 5, LastVisited: older}, Score: 10},
			score:         10,
			exactURLMatch: false,
			prefixURL:     false,
			urlMatch:      false,
			prefixTitle:   false,
		},
	}

	sortRankedHistoryMatches(items)

	wantOrder := []int64{2, 3, 4, 5, 6, 7, 8, 1, 9}
	for i, want := range wantOrder {
		got := items[i].match.Entry.ID
		if got != want {
			t.Errorf("position %d: want ID %d, got ID %d", i, want, got)
		}
	}
}
