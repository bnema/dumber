package port

import (
	"context"

	"bnema.dev/dumber/internal/domain/favicon"
)

// FaviconRefreshScheduler deduplicates and runs asynchronous favicon refresh work.
// Schedule returns true when work is queued and false for duplicate or rejected work.
type FaviconRefreshScheduler interface {
	Schedule(ctx context.Context, key favicon.Key, work func(context.Context)) bool
}
