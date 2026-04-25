package entity

import "time"

// HistoryWindow contains one fixed-duration slice of browsing history.
type HistoryWindow struct {
	Entries []*HistoryEntry `json:"entries"`
	Before  time.Time       `json:"before"`
	After   time.Time       `json:"after"`
	HasMore bool            `json:"has_more"`
}
