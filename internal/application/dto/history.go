package dto

import "github.com/bnema/dumber/internal/domain/entity"

// HistorySearchInput holds search parameters for history search use cases.
type HistorySearchInput struct {
	Query string
	Limit int
	// MaxAgeDays restricts results to entries last visited within the last N
	// days. Zero or negative means no age restriction (all stored history).
	MaxAgeDays int
}

// HistorySearchOutput holds search results for history search use cases.
type HistorySearchOutput struct {
	Matches []entity.HistoryMatch
}
