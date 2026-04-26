package dto

import "github.com/bnema/dumber/internal/domain/entity"

// HistorySearchInput holds search parameters for history search use cases.
type HistorySearchInput struct {
	Query string
	Limit int
}

// HistorySearchOutput holds search results for history search use cases.
type HistorySearchOutput struct {
	Matches []entity.HistoryMatch
}
