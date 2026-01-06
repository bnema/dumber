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

// Service implements the domain FaviconService interface.
// It coordinates between the cache and fetcher components.
type Service struct {
	cache   *Cache
	fetcher *Fetcher
}

// NewService creates a new favicon service.
// cacheDir is the directory for disk caching; empty string disables disk caching.
func NewService(cacheDir string) *Service {
	return &Service{
		cache:   NewCache(cacheDir),
		fetcher: NewFetcher(),
	}
}

// Get returns favicon bytes for a domain.
// Checks memory cache, then disk cache, then fetches from external API.
func (s *Service) Get(ctx context.Context, domain string) ([]byte, error) {
	if domain == "" {
		return nil, nil
	}

	// Internal domain returns the app logo
	if domain == InternalDomain {
		return assets.LogoSVG, nil
	}

	// Check cache first (memory + disk)
	if data, ok := s.cache.Get(domain); ok {
		return data, nil
	}

	// Fetch from external API
	data, err := s.fetcher.Fetch(ctx, domain)
	if err != nil {
		return nil, err
	}

	// Store in cache if we got data
	if len(data) > 0 {
		s.cache.Set(domain, data)
	}

	return data, nil
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
func (s *Service) GetCached(domain string) ([]byte, bool) {
	return s.cache.Get(domain)
}

// Store saves favicon bytes for a domain to cache.
func (s *Service) Store(domain string, data []byte) error {
	s.cache.Set(domain, data)
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

// EnsureDiskCache fetches favicon from external API and saves to disk if not already cached.
// This is used when we receive a texture from WebKit but want to ensure disk persistence.
func (s *Service) EnsureDiskCache(ctx context.Context, domain string) {
	if domain == "" {
		return
	}

	// Already on disk?
	if s.cache.HasOnDisk(domain) {
		return
	}

	log := logging.FromContext(ctx)

	// Fetch and store
	data, err := s.fetcher.Fetch(ctx, domain)
	if err != nil {
		log.Debug().Err(err).Str("domain", domain).Msg("failed to fetch favicon for disk cache")
		return
	}

	if len(data) > 0 {
		s.cache.Set(domain, data)
	}
}

// DiskPathPNGSized returns the filesystem path for a sized PNG favicon.
func (s *Service) DiskPathPNGSized(domain string, size int) string {
	return s.cache.DiskPathPNGSized(domain, size)
}

// HasPNGSizedOnDisk checks if a sized PNG favicon exists on disk.
func (s *Service) HasPNGSizedOnDisk(domain string, size int) bool {
	return s.cache.HasPNGSizedOnDisk(domain, size)
}

// EnsureSizedPNG creates a resized PNG from the original if it doesn't exist.
// This is used to generate normalized icons for dmenu/fuzzel.
func (s *Service) EnsureSizedPNG(ctx context.Context, domain string, size int) error {
	if domain == "" {
		return nil
	}

	// Already exists?
	if s.HasPNGSizedOnDisk(domain, size) {
		return nil
	}

	// Check source PNG exists
	srcPath := s.DiskPathPNG(domain)
	if srcPath == "" || !s.HasPNGOnDisk(domain) {
		return fmt.Errorf("source PNG not found for domain %s", domain)
	}

	dstPath := s.DiskPathPNGSized(domain, size)
	if dstPath == "" {
		return fmt.Errorf("cannot determine destination path for domain %s", domain)
	}

	log := logging.FromContext(ctx)
	log.Debug().Str("domain", domain).Int("size", size).Msg("creating sized favicon")

	return ResizePNG(srcPath, dstPath, size)
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
