package favicon

import (
	"context"
	"fmt"
	"strings"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/dumber/internal/logging"
)

// InternalScheme is the URL scheme used for internal browser pages.
const InternalScheme = "dumb://"

// InternalDomain is the pseudo-domain used for caching internal page favicons.
const InternalDomain = "dumb"

// Service is the legacy favicon cache adapter kept for callers that have not yet
// been migrated to the application favicon usecase. It must not own refresh or
// remote-fetch policy; new code should use the usecase plus BlobStore/Fetcher.
type Service struct {
	cache *Cache
}

// NewService creates a new favicon service.
// cacheDir is the directory for disk caching; empty string disables disk caching.
func NewService(cacheDir string) *Service {
	return &Service{cache: NewCache(cacheDir)}
}

func (s *Service) Cache() *Cache {
	return s.cache
}

// Get returns favicon bytes for a domain from the legacy cache only.
func (s *Service) Get(ctx context.Context, domain string) ([]byte, error) {
	if domain == "" {
		return nil, nil
	}
	log := logging.FromContext(ctx)
	log.Debug().Str("domain", domain).Msg("favicon: Service.Get begin")

	// Internal domain returns the app logo
	if domain == InternalDomain {
		log.Debug().Str("domain", domain).Msg("favicon: Service.Get internal domain")
		return assets.LogoSVG, nil
	}

	if data, ok := s.cache.Get(ctx, domain); ok {
		log.Debug().
			Str("domain", domain).
			Int("bytes", len(data)).
			Msg("favicon: Service.Get cache hit")
		return data, nil
	}
	log.Debug().Str("domain", domain).Msg("favicon: Service.Get cache miss")
	return nil, nil
}

// IsInternalURL checks if a URL uses the internal dumb:// scheme.
func IsInternalURL(url string) bool {
	return strings.HasPrefix(url, InternalScheme)
}

// GetLogoBytes returns the raw SVG bytes of the app logo.
// Used for internal pages that need the logo as a favicon.
func GetLogoBytes() []byte {
	return assets.LogoSVG
}

// GetCached returns favicon bytes only if already cached (no external fetch).
func (s *Service) GetCached(ctx context.Context, domain string) ([]byte, bool) {
	data, ok := s.cache.Get(ctx, domain)
	logging.FromContext(ctx).Debug().
		Str("domain", domain).
		Bool("hit", ok).
		Int("bytes", len(data)).
		Msg("favicon: Service.GetCached")
	return data, ok
}

// Store saves favicon bytes for a domain to cache.
func (s *Service) Store(ctx context.Context, domain string, data []byte) error {
	s.cache.Set(ctx, domain, data)
	return nil
}

// DiskPath returns the filesystem path where a domain's favicon is cached (ICO).
func (s *Service) DiskPath(domain string) string {
	return s.cache.DiskPath(domain)
}

// DiskPathPNG returns the filesystem path for PNG favicon.
func (s *Service) DiskPathPNG(domain string) string {
	return s.cache.DiskPathPNG(domain)
}

// HasPNGOnDisk checks if a PNG favicon exists on disk for the domain.
func (s *Service) HasPNGOnDisk(domain string) bool {
	return s.cache.HasPNGOnDisk(domain)
}

// WritePNG writes raw PNG data to disk for a domain.
func (s *Service) WritePNG(domain string, pngData []byte) {
	s.cache.WritePNG(domain, pngData)
}

// Close shuts down background workers and releases resources.
func (s *Service) Close() {
	s.cache.Close()
}

// EnsureDiskCache is a legacy no-op. Disk population is handled by explicit
// texture/blob writes after EnsureCacheDir; remote refresh policy belongs to the application usecase.
func (*Service) EnsureDiskCache(ctx context.Context, domain string) {
	logging.FromContext(ctx).Debug().Str("domain", domain).Msg("favicon: legacy EnsureDiskCache skipped")
}

// DiskPathPNGSized returns the filesystem path for a sized PNG favicon.
func (s *Service) DiskPathPNGSized(domain string, size int) string {
	return s.cache.DiskPathPNGSized(domain, size)
}

// HasPNGSizedOnDisk checks if a sized PNG favicon exists on disk.
func (s *Service) HasPNGSizedOnDisk(domain string, size int) bool {
	return s.cache.HasPNGSizedOnDisk(domain, size)
}

// EnsureSizedPNG creates or overwrites a resized PNG from the original PNG.
// This is a legacy adapter method; new code should use the favicon usecase.
func (s *Service) EnsureSizedPNG(ctx context.Context, domain string, size int) error {
	if domain == "" {
		return nil
	}
	log := logging.FromContext(ctx)
	log.Debug().
		Str("domain", domain).
		Int("size", size).
		Msg("favicon: EnsureSizedPNG begin")

	srcPath := s.DiskPathPNG(domain)
	if srcPath == "" || !s.HasPNGOnDisk(domain) {
		return fmt.Errorf("source PNG not found for domain %s", domain)
	}

	dstPath := s.DiskPathPNGSized(domain, size)
	if dstPath == "" {
		return fmt.Errorf("cannot determine destination path for domain %s", domain)
	}

	log.Debug().Str("domain", domain).Int("size", size).Msg("creating sized favicon")

	err := ResizePNG(srcPath, dstPath, size)
	log.Debug().
		Err(err).
		Str("domain", domain).
		Int("size", size).
		Msg("favicon: EnsureSizedPNG end")
	return err
}

// EnsureInternalFaviconPNG ensures the internal app logo PNG exists on disk.
// Returns the path to the sized PNG file. Used by dmenu to get a filesystem
// path to the internal favicon for rofi/fuzzel display.
func (s *Service) EnsureInternalFaviconPNG(pngData []byte, size int) string {
	if s.HasPNGSizedOnDisk(InternalDomain, size) {
		return s.DiskPathPNGSized(InternalDomain, size)
	}

	s.cache.WritePNGSized(InternalDomain, pngData, size)
	return s.DiskPathPNGSized(InternalDomain, size)
}

// EnsureCacheDir ensures the favicon cache directory exists.
// Call this before using DiskPathPNG with external save functions like GTK's SaveToPng.
func (s *Service) EnsureCacheDir() error {
	return s.cache.EnsureDir()
}
