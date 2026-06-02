package filtering

import (
	"context"
	"time"
)

// Backend owns engine-specific filter activation and compiled-state storage.
// Implementations must be safe to call from Manager background workers.
type Backend interface {
	// ActivateCached loads previously compiled filters/rules.
	// It returns false with nil error when no usable cache exists.
	ActivateCached(ctx context.Context) (bool, error)

	// ActivateFiles compiles or parses downloaded filter rule files and makes the
	// result active for future engine requests/webviews.
	ActivateFiles(ctx context.Context, paths []string) error

	// HasActive reports whether this backend currently has active filters/rules.
	HasActive() bool

	// Clear removes active and cached backend filter state.
	Clear(ctx context.Context) error
}

// FilterDownloader abstracts the filter download operations.
// This interface enables testing without requiring network access.
type FilterDownloader interface {
	// GetCachedManifest reads the locally cached manifest.json.
	GetCachedManifest() (*Manifest, error)

	// FetchManifest downloads the latest manifest.json from the remote source.
	FetchManifest(ctx context.Context) (*Manifest, error)

	// DownloadFilters downloads filter JSON files from the remote source.
	DownloadFilters(ctx context.Context, onProgress func(DownloadProgress)) ([]string, error)

	// NeedsUpdate checks if filters need to be updated based on manifest version.
	NeedsUpdate(ctx context.Context) (bool, error)

	// ClearCache removes all cached filter files.
	ClearCache() error

	// HasCachedFilters checks if all filter JSON files are cached.
	HasCachedFilters() bool

	// GetCachedFilterPaths returns paths to cached filter JSON files.
	GetCachedFilterPaths() []string

	// IsCacheStale checks if the cached manifest is older than maxAge.
	IsCacheStale(maxAge time.Duration) bool
}

// Ensure concrete types implement the interfaces.
var _ FilterDownloader = (*Downloader)(nil)
