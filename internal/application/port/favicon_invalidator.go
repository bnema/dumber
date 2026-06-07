package port

import (
	"context"

	"github.com/bnema/dumber/internal/domain/favicon"
)

// FaviconInvalidator clears cached or derived data for a single favicon key.
type FaviconInvalidator interface {
	Invalidate(ctx context.Context, key favicon.Key) error
}

// FaviconInvalidators coordinates invalidation across registered invalidators.
type FaviconInvalidators interface {
	InvalidateAll(ctx context.Context, key favicon.Key) error
}

// FaviconInvalidatorRegistry registers invalidators during application initialization.
type FaviconInvalidatorRegistry interface {
	RegisterFaviconInvalidator(invalidator FaviconInvalidator)
}
