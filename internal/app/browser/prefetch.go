// Package browser provides the main browser application components.
package browser

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/pkg/webkit"
)

// PrefetchManager handles DNS prefetching and resource preloading.
// Improves page load times by resolving DNS before navigation.
type PrefetchManager struct {
	mu sync.RWMutex

	// Configuration
	enabled       int32         // atomic: 1 = enabled, 0 = disabled
	maxPrefetches int           // Maximum concurrent prefetches
	throttleDelay time.Duration // Minimum delay between prefetches for same domain
	maxCacheSize  int           // Maximum entries in recentPrefetches before cleanup

	// State tracking
	recentPrefetches map[string]time.Time // domain -> last prefetch time
	prefetchCount    int64

	// Database access for history-based prefetching
	queries db.DatabaseQuerier
}

// PrefetchConfig contains configuration for the prefetch manager
type PrefetchConfig struct {
	Enabled       bool
	MaxPrefetches int
	ThrottleDelay time.Duration
}

// DefaultPrefetchConfig returns sensible default configuration
func DefaultPrefetchConfig() PrefetchConfig {
	return PrefetchConfig{
		Enabled:       true,
		MaxPrefetches: 10,
		ThrottleDelay: 100 * time.Millisecond,
	}
}

// NewPrefetchManager creates a new prefetch manager
func NewPrefetchManager(cfg PrefetchConfig, queries db.DatabaseQuerier) *PrefetchManager {
	var enabled int32
	if cfg.Enabled {
		enabled = 1
	}
	return &PrefetchManager{
		enabled:          enabled,
		maxPrefetches:    cfg.MaxPrefetches,
		throttleDelay:    cfg.ThrottleDelay,
		maxCacheSize:     500, // Limit cache to prevent unbounded growth
		recentPrefetches: make(map[string]time.Time),
		queries:          queries,
	}
}

// isEnabled returns true if prefetching is enabled (atomic read)
func (pm *PrefetchManager) isEnabled() bool {
	return atomic.LoadInt32(&pm.enabled) == 1
}

// cleanupOldEntries removes old entries from the cache to prevent unbounded growth
func (pm *PrefetchManager) cleanupOldEntries() {
	// Only cleanup if we exceed max size
	if len(pm.recentPrefetches) <= pm.maxCacheSize {
		return
	}

	cutoff := time.Now().Add(-10 * pm.throttleDelay)
	for hostname, lastTime := range pm.recentPrefetches {
		if lastTime.Before(cutoff) {
			delete(pm.recentPrefetches, hostname)
		}
	}
}

// PrefetchDNS prefetches DNS for the given URL.
// Throttles requests to avoid excessive DNS queries.
func (pm *PrefetchManager) PrefetchDNS(rawURL string) {
	if !pm.isEnabled() || rawURL == "" {
		return
	}

	// Extract hostname from URL
	hostname := pm.extractHostname(rawURL)
	if hostname == "" {
		return
	}

	// Check throttle
	pm.mu.Lock()
	if lastPrefetch, exists := pm.recentPrefetches[hostname]; exists {
		if time.Since(lastPrefetch) < pm.throttleDelay {
			pm.mu.Unlock()
			return
		}
	}
	pm.recentPrefetches[hostname] = time.Now()
	pm.prefetchCount++
	pm.cleanupOldEntries() // Prevent unbounded growth
	pm.mu.Unlock()

	// Perform DNS prefetch
	webkit.PrefetchDNS(hostname)
	logging.Debug(fmt.Sprintf("[prefetch] DNS prefetch for %s", hostname))
}

// PrefetchDNSBatch prefetches DNS for multiple URLs
func (pm *PrefetchManager) PrefetchDNSBatch(urls []string) {
	if !pm.isEnabled() {
		return
	}

	seen := make(map[string]bool)
	count := 0

	for _, rawURL := range urls {
		if count >= pm.maxPrefetches {
			break
		}

		hostname := pm.extractHostname(rawURL)
		if hostname == "" || seen[hostname] {
			continue
		}
		seen[hostname] = true

		pm.PrefetchDNS(rawURL)
		count++
	}

	if count > 0 {
		logging.Debug(fmt.Sprintf("[prefetch] Batch prefetched DNS for %d domains", count))
	}
}

// PrefetchFromHistory prefetches DNS for frequently visited domains.
// Should be called after startup to warm DNS cache.
func (pm *PrefetchManager) PrefetchFromHistory(ctx context.Context) {
	if !pm.isEnabled() || pm.queries == nil {
		return
	}

	// Get most visited URLs from history
	entries, err := pm.queries.GetMostVisited(ctx, int64(pm.maxPrefetches*2))
	if err != nil {
		logging.Warn(fmt.Sprintf("[prefetch] Failed to get history for prefetch: %v", err))
		return
	}

	// Extract unique hostnames
	seen := make(map[string]bool)
	hostnames := make([]string, 0, pm.maxPrefetches)

	for _, entry := range entries {
		hostname := pm.extractHostname(entry.Url)
		if hostname == "" || seen[hostname] {
			continue
		}
		seen[hostname] = true
		hostnames = append(hostnames, hostname)

		if len(hostnames) >= pm.maxPrefetches {
			break
		}
	}

	// Prefetch in background
	if len(hostnames) > 0 {
		go func() {
			for _, hostname := range hostnames {
				webkit.PrefetchDNS(hostname)
				pm.mu.Lock()
				pm.recentPrefetches[hostname] = time.Now()
				pm.prefetchCount++
				pm.mu.Unlock()
			}
			logging.Info(fmt.Sprintf("[prefetch] Prefetched DNS for %d domains from history", len(hostnames)))
		}()
	}
}

// PrefetchCommonDomains prefetches DNS for commonly used CDNs and services.
// Call this at startup for faster loading of common resources.
func (pm *PrefetchManager) PrefetchCommonDomains() {
	if !pm.isEnabled() {
		return
	}

	commonDomains := []string{
		// CDNs
		"cdnjs.cloudflare.com",
		"cdn.jsdelivr.net",
		"unpkg.com",
		"code.jquery.com",
		// Fonts
		"fonts.googleapis.com",
		"fonts.gstatic.com",
		// Common services
		"www.google.com",
		"www.googleapis.com",
		"api.github.com",
		"raw.githubusercontent.com",
	}

	go func() {
		for _, domain := range commonDomains {
			webkit.PrefetchDNS(domain)
			pm.mu.Lock()
			pm.recentPrefetches[domain] = time.Now()
			pm.prefetchCount++
			pm.mu.Unlock()
		}
		logging.Info(fmt.Sprintf("[prefetch] Prefetched DNS for %d common domains", len(commonDomains)))
	}()
}

// OnLinkHover should be called when user hovers over a link.
// Prefetches DNS for the link target.
func (pm *PrefetchManager) OnLinkHover(linkURL string) {
	pm.PrefetchDNS(linkURL)
}

// extractHostname extracts the hostname from a URL string
func (pm *PrefetchManager) extractHostname(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	// Handle URLs without scheme
	if rawURL[0] != 'h' && rawURL[0] != 'H' {
		// Check if it looks like a hostname
		if !containsScheme(rawURL) {
			rawURL = "https://" + rawURL
		}
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	hostname := parsed.Hostname()
	if hostname == "" || hostname == "localhost" {
		return ""
	}

	// Skip internal URLs
	if hostname == "127.0.0.1" || hostname == "::1" {
		return ""
	}

	return hostname
}

// containsScheme checks if URL contains a scheme
func containsScheme(rawURL string) bool {
	for i := 0; i < len(rawURL) && i < 10; i++ {
		if rawURL[i] == ':' {
			return true
		}
		if rawURL[i] == '/' || rawURL[i] == '?' || rawURL[i] == '#' {
			return false
		}
	}
	return false
}

// Stats returns prefetch statistics
func (pm *PrefetchManager) Stats() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return map[string]interface{}{
		"enabled":        pm.isEnabled(),
		"prefetch_count": pm.prefetchCount,
		"cached_domains": len(pm.recentPrefetches),
	}
}

// SetEnabled enables or disables prefetching
func (pm *PrefetchManager) SetEnabled(enabled bool) {
	var val int32
	if enabled {
		val = 1
	}
	atomic.StoreInt32(&pm.enabled, val)
}

// ClearCache clears the prefetch throttle cache
func (pm *PrefetchManager) ClearCache() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.recentPrefetches = make(map[string]time.Time)
}
