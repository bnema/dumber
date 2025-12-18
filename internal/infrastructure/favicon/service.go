package favicon

import (
	"context"

	"github.com/bnema/dumber/internal/logging"
)

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

// GetCached returns favicon bytes only if already cached (no external fetch).
func (s *Service) GetCached(domain string) ([]byte, bool) {
	return s.cache.Get(domain)
}

// Store saves favicon bytes for a domain to cache.
func (s *Service) Store(domain string, data []byte) error {
	s.cache.Set(domain, data)
	return nil
}

// DiskPath returns the filesystem path where a domain's favicon is cached.
func (s *Service) DiskPath(domain string) string {
	return s.cache.DiskPath(domain)
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
