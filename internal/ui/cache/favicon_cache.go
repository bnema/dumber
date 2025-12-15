// Package cache provides in-memory caching utilities for the UI layer.
package cache

import (
	"context"
	"net/url"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
)

// FaviconCache provides a two-tier favicon caching system:
// 1. In-memory cache keyed by domain for fast lookups
// 2. WebKit FaviconDatabase as persistent backing store
type FaviconCache struct {
	mu        sync.RWMutex
	cache     map[string]*gdk.Texture // key: domain (e.g., "github.com")
	faviconDB *webkit.FaviconDatabase
}

// NewFaviconCache creates a new FaviconCache with the given FaviconDatabase.
// The faviconDB can be nil, in which case only the in-memory cache is used.
func NewFaviconCache(faviconDB *webkit.FaviconDatabase) *FaviconCache {
	return &FaviconCache{
		cache:     make(map[string]*gdk.Texture),
		faviconDB: faviconDB,
	}
}

// extractDomain extracts the normalized domain (host) from a URL string.
// Normalizes by stripping "www." prefix so youtube.com and www.youtube.com
// resolve to the same cache key.
func extractDomain(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := parsed.Host
	// Normalize: strip www. prefix
	host = strings.TrimPrefix(host, "www.")
	return host
}

// Set stores a favicon texture for the given domain.
func (fc *FaviconCache) Set(domain string, texture *gdk.Texture) {
	if domain == "" || texture == nil {
		return
	}
	fc.mu.Lock()
	fc.cache[domain] = texture
	fc.mu.Unlock()
}

// SetByURL stores a favicon texture for the domain extracted from the URL.
func (fc *FaviconCache) SetByURL(pageURL string, texture *gdk.Texture) {
	domain := extractDomain(pageURL)
	fc.Set(domain, texture)
}

// Get retrieves a favicon texture by domain from the in-memory cache.
// Returns nil if not found.
func (fc *FaviconCache) Get(domain string) *gdk.Texture {
	if domain == "" {
		return nil
	}
	fc.mu.RLock()
	texture := fc.cache[domain]
	fc.mu.RUnlock()
	return texture
}

// GetByURL retrieves a favicon texture by extracting the domain from the URL.
// Returns nil if not found.
func (fc *FaviconCache) GetByURL(pageURL string) *gdk.Texture {
	domain := extractDomain(pageURL)
	return fc.Get(domain)
}

// getURLVariants returns URL variants to try for favicon lookup.
// WebKit stores favicons under exact URLs (e.g., https://www.youtube.com/)
// so we need to try multiple variants: with/without www, with/without trailing slash.
func getURLVariants(pageURL string) []string {
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}

	variants := make([]string, 0, 4)

	// Generate host variants (with and without www.)
	hosts := []string{parsed.Host}
	if strings.HasPrefix(parsed.Host, "www.") {
		hosts = append(hosts, strings.TrimPrefix(parsed.Host, "www."))
	} else {
		hosts = append(hosts, "www."+parsed.Host)
	}

	// Generate path variants (with and without trailing slash)
	paths := []string{parsed.Path}
	if parsed.Path == "" || parsed.Path == "/" {
		paths = []string{"", "/"}
	} else if strings.HasSuffix(parsed.Path, "/") {
		paths = append(paths, strings.TrimSuffix(parsed.Path, "/"))
	} else {
		paths = append(paths, parsed.Path+"/")
	}

	// Generate all combinations (skip the original URL)
	original := pageURL
	for _, host := range hosts {
		for _, path := range paths {
			p := *parsed
			p.Host = host
			p.Path = path
			variant := p.String()
			if variant != original {
				variants = append(variants, variant)
			}
		}
	}

	return variants
}

// GetOrFetch checks the in-memory cache first, then queries the FaviconDatabase
// asynchronously on cache miss. The callback is invoked with the texture (or nil).
// This is safe to call from any goroutine; the callback runs on the GLib main loop.
func (fc *FaviconCache) GetOrFetch(ctx context.Context, pageURL string, callback func(*gdk.Texture)) {
	if callback == nil {
		return
	}

	log := logging.FromContext(ctx)
	domain := extractDomain(pageURL)

	// Check in-memory cache first
	if texture := fc.Get(domain); texture != nil {
		callback(texture)
		return
	}

	// No FaviconDatabase, return nil
	if fc.faviconDB == nil {
		callback(nil)
		return
	}

	// Helper to invoke callback on main thread
	invokeCallback := func(texture *gdk.Texture) {
		cb := glib.SourceFunc(func(uintptr) bool {
			callback(texture)
			return false
		})
		glib.IdleAdd(&cb, 0)
	}

	// Get all URL variants to try (www/non-www, with/without trailing slash)
	variants := getURLVariants(pageURL)

	// Recursive function to try variants one by one
	var tryVariant func(index int)
	tryVariant = func(index int) {
		if index >= len(variants) {
			// All variants exhausted
			invokeCallback(nil)
			return
		}

		variantURL := variants[index]
		variantCb := gio.AsyncReadyCallback(func(_ uintptr, resultPtr uintptr, _ uintptr) {
			if resultPtr == 0 {
				tryVariant(index + 1)
				return
			}

			result := &gio.AsyncResultBase{Ptr: resultPtr}
			texture, err := fc.faviconDB.GetFaviconFinish(result)
			if err != nil {
				log.Debug().Err(err).Str("url", variantURL).Msg("favicon variant fetch failed")
				tryVariant(index + 1)
				return
			}

			if texture != nil && domain != "" {
				fc.Set(domain, texture)
			}
			invokeCallback(texture)
		})

		fc.faviconDB.GetFavicon(variantURL, nil, &variantCb, 0)
	}

	// Query FaviconDatabase with original URL first
	asyncCb := gio.AsyncReadyCallback(func(_ uintptr, resultPtr uintptr, _ uintptr) {
		if resultPtr == 0 {
			log.Debug().Str("url", pageURL).Msg("favicon fetch: nil result, trying variants")
			tryVariant(0)
			return
		}

		result := &gio.AsyncResultBase{Ptr: resultPtr}
		texture, err := fc.faviconDB.GetFaviconFinish(result)
		if err != nil {
			log.Debug().Err(err).Str("url", pageURL).Msg("favicon fetch failed, trying variants")
			tryVariant(0)
			return
		}

		// Cache the result
		if texture != nil && domain != "" {
			fc.Set(domain, texture)
		}

		invokeCallback(texture)
	})

	// Start async fetch (nil cancellable, 0 userData)
	fc.faviconDB.GetFavicon(pageURL, nil, &asyncCb, 0)
}

// Clear removes all entries from the in-memory cache.
func (fc *FaviconCache) Clear() {
	fc.mu.Lock()
	fc.cache = make(map[string]*gdk.Texture)
	fc.mu.Unlock()
}

// Size returns the number of entries in the in-memory cache.
func (fc *FaviconCache) Size() int {
	fc.mu.RLock()
	size := len(fc.cache)
	fc.mu.RUnlock()
	return size
}
