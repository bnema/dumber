package entity

import "time"

// HistoryWindow contains one bounded slice of browsing history plus display window metadata.
type HistoryWindow struct {
	Entries           []*HistoryEntry `json:"entries"`
	Before            time.Time       `json:"before"`
	After             time.Time       `json:"after"`
	CursorLastVisited time.Time       `json:"cursor_last_visited"`
	CursorID          int64           `json:"cursor_id,omitempty"`
	HasMore           bool            `json:"has_more"`
}
