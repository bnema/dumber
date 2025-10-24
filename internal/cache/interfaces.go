package cache

import (
	"context"

	"github.com/bnema/dumber/internal/db"
)

//go:generate mockgen -source=interfaces.go -destination=mocks/mock_querier.go

// HistoryQuerier defines the interface for querying history data.
type HistoryQuerier interface {
	GetHistory(ctx context.Context, limit int64) ([]db.History, error)
}
