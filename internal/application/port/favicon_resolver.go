package port

import (
	"context"

	"github.com/bnema/dumber/internal/domain/favicon"
)

// FaviconResolvePurpose identifies the context requesting favicon resolution.
type FaviconResolvePurpose string

const (
	// FaviconResolvePurposeUI resolves favicons for end-user UI display.
	FaviconResolvePurposeUI FaviconResolvePurpose = "ui"
	// FaviconResolvePurposeSystemview resolves favicons for internal views without remote loads.
	FaviconResolvePurposeSystemview FaviconResolvePurpose = "systemview"
	// FaviconResolvePurposeRefresh resolves favicons for background refresh work.
	FaviconResolvePurposeRefresh FaviconResolvePurpose = "refresh"
)

// FaviconResolveOptions controls cache lookup, blocking refresh, background refresh, and caller intent.
type FaviconResolveOptions struct {
	AllowBlockingRefresh      bool
	ScheduleBackgroundRefresh bool
	Purpose                   FaviconResolvePurpose
}

// ResolvedFavicon is a fully resolved favicon payload with metadata and refresh state.
// Key identifies the cache entry; Bytes and ContentType hold image data; Metadata and BackgroundRefreshScheduled describe state.
type ResolvedFavicon struct {
	Key                        favicon.Key
	Bytes                      []byte
	ContentType                string
	Metadata                   *favicon.Metadata
	BackgroundRefreshScheduled bool
}

// FaviconResolver orchestrates favicon lookup, observation, refresh, sizing, and invalidation.
// It extends FaviconSystemviewResolver for boundary-safe systemview reads.
type FaviconResolver interface {
	FaviconSystemviewResolver
	Resolve(ctx context.Context, rawURLOrDomain string, size int, options FaviconResolveOptions) (*ResolvedFavicon, error)
	Observe(ctx context.Context, pageURL, iconURL string, bytes []byte, source favicon.Source, contentType string) (*favicon.Metadata, error)
	RefreshFromIconURLs(ctx context.Context, pageURL string, iconURLs []string) error
	EnsureSized(ctx context.Context, key favicon.Key, size int) error
	Invalidate(ctx context.Context, key favicon.Key) error
}

// FaviconSystemviewResolver is the boundary-safe API used by engine scheme
// handlers. Implementations must resolve exact keys and parent fallbacks through
// the application usecase and must not trigger remote network loads.
type FaviconSystemviewResolver interface {
	ResolveSystemviewIcon(ctx context.Context, rawDomain string, size int) (*ResolvedFavicon, error)
}

// SystemviewFaviconResolverSetter is an optional engine capability that wires
// the application favicon resolver to internal systemview pages.
type SystemviewFaviconResolverSetter interface {
	SetSystemviewFaviconResolver(resolver FaviconSystemviewResolver)
}
