package port

import "context"

// FaviconService provides favicon retrieval and caching.
type FaviconService interface {
	GetCached(ctx context.Context, domain string) ([]byte, bool)
	Get(ctx context.Context, domain string) ([]byte, error)
	DiskPathPNG(domain string) string
	HasPNGOnDisk(domain string) bool
	HasPNGSizedOnDisk(domain string, size int) bool
	EnsureSizedPNG(ctx context.Context, domain string, size int) error
	EnsureCacheDir() error
	// EnsureDiskCache ensures a favicon is written to the on-disk cache for the given
	// domain. It is intentionally fire-and-forget: any errors are logged internally by
	// the implementation and are not returned to the caller.
	EnsureDiskCache(ctx context.Context, domain string)
	Close()
}
