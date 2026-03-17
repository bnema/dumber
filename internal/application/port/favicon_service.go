package port

import "context"

// FaviconService provides favicon retrieval and caching.
type FaviconService interface {
	GetCached(domain string) ([]byte, bool)
	Get(ctx context.Context, domain string) ([]byte, error)
	DiskPathPNG(domain string) string
	HasPNGOnDisk(domain string) bool
	HasPNGSizedOnDisk(domain string, size int) bool
	EnsureSizedPNG(ctx context.Context, domain string, size int) error
	EnsureCacheDir() error
	EnsureDiskCache(ctx context.Context, domain string)
	Close()
}
