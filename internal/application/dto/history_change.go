package dto

import "time"

// HistoryChangeReason describes the category of a persisted history mutation.
type HistoryChangeReason string

const (
	// HistoryChangeReasonVisit indicates one or more history visits were persisted.
	HistoryChangeReasonVisit HistoryChangeReason = "visit"
	// HistoryChangeReasonTitle indicates one or more history titles were updated.
	HistoryChangeReasonTitle HistoryChangeReason = "title"
	// HistoryChangeReasonDelete indicates one or more history entries were deleted.
	HistoryChangeReasonDelete HistoryChangeReason = "delete"
	// HistoryChangeReasonClear indicates a history clear operation was persisted.
	HistoryChangeReasonClear HistoryChangeReason = "clear"
)

// HistoryChange reports aggregate facts about persisted history mutations.
// It intentionally omits URLs so UI adapters only know that history changed.
type HistoryChange struct {
	Reasons          []HistoryChangeReason
	VisitCount       int
	TitleCount       int
	DeleteCount      int
	DeleteCountKnown bool
	ChangedAt        time.Time
}
