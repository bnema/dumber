package port

import (
	"context"

	"github.com/bnema/dumber/internal/application/dto"
)

// HistoryChangeSink receives persisted history change notifications.
type HistoryChangeSink interface {
	OnHistoryChanged(ctx context.Context, change dto.HistoryChange)
}

// HistoryMutationCoordinator serializes destructive history mutations with pending history writes.
type HistoryMutationCoordinator interface {
	BeginHistoryMutation(ctx context.Context) (release func(), err error)
}
