package filtering

import (
	"context"
	"time"

	"github.com/bnema/puregotk-webkit/webkit"
)

// FilterStore abstracts the WebKit UserContentFilterStore operations.
// This interface enables testing without requiring GTK/WebKit.
type FilterStore interface {
	// Compile compiles a JSON filter file and stores it with the given identifier.
	Compile(ctx context.Context, identifier string, jsonPath string) (*webkit.UserContentFilter, error)

	// Load loads a previously compiled filter by its identifier.
	Load(ctx context.Context, identifier string) (*webkit.UserContentFilter, error)

	// Remove removes a compiled filter by its identifier.
	Remove(ctx context.Context, identifier string) error

	// HasCompiledFilter checks if a compiled filter exists for the given identifier.
	HasCompiledFilter(ctx context.Context, identifier string) bool

	// FetchIdentifiers returns all stored filter identifiers.
	FetchIdentifiers(ctx context.Context) ([]string, error)

	// Path returns the storage path for compiled filters.
	Path() string
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
var (
	_ FilterStore      = (*Store)(nil)
	_ FilterDownloader = (*Downloader)(nil)
)
