// Package cache provides in-memory caching utilities for the UI layer.
package cache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
)

const (
	// DuckDuckGo favicon API URL template (better transparency support)
	duckduckgoFaviconURL = "https://icons.duckduckgo.com/ip3/%s.ico"
	// HTTP client timeout for favicon fetch
	faviconFetchTimeout = 5 * time.Second
)

// FaviconCache provides a two-tier favicon caching system:
// 1. In-memory cache keyed by domain for fast lookups
// 2. WebKit FaviconDatabase as persistent backing store
// 3. Google favicon API as fallback
type FaviconCache struct {
	mu        sync.RWMutex
	cache     map[string]*gdk.Texture // key: domain (e.g., "github.com")
	faviconDB *webkit.FaviconDatabase
	client    *http.Client
}

// NewFaviconCache creates a new FaviconCache with the given FaviconDatabase.
// The faviconDB can be nil, in which case only the in-memory cache is used.
func NewFaviconCache(faviconDB *webkit.FaviconDatabase) *FaviconCache {
	return &FaviconCache{
		cache:     make(map[string]*gdk.Texture),
		faviconDB: faviconDB,
		client: &http.Client{
			Timeout: faviconFetchTimeout,
		},
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

// GetOrFetch checks the in-memory cache first, then queries the FaviconDatabase,
// and falls back to Google's favicon API if not found.
// The callback is invoked with the texture (or nil).
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

	// Helper to invoke callback on main thread
	invokeCallback := func(texture *gdk.Texture) {
		cb := glib.SourceFunc(func(uintptr) bool {
			callback(texture)
			return false
		})
		glib.IdleAdd(&cb, 0)
	}

	// Helper to fetch from external API and invoke callback
	fetchExternal := func() {
		go func() {
			texture := fc.fetchFromExternal(ctx, domain)
			if texture != nil {
				fc.Set(domain, texture)
			}
			invokeCallback(texture)
		}()
	}

	// No FaviconDatabase, fetch from external API directly
	if fc.faviconDB == nil {
		fetchExternal()
		return
	}

	// Try FaviconDatabase first (just once, no variant attempts)
	asyncCb := gio.AsyncReadyCallback(func(_ uintptr, resultPtr uintptr, _ uintptr) {
		if resultPtr == 0 {
			log.Debug().Str("domain", domain).Msg("favicon not in database, fetching from DuckDuckGo")
			fetchExternal()
			return
		}

		result := &gio.AsyncResultBase{Ptr: resultPtr}
		texture, err := fc.faviconDB.GetFaviconFinish(result)
		if err != nil || texture == nil {
			log.Debug().Str("domain", domain).Msg("favicon not in database, fetching from DuckDuckGo")
			fetchExternal()
			return
		}

		// Found in database, cache and return
		fc.Set(domain, texture)
		invokeCallback(texture)
	})

	// Try with the original URL
	fc.faviconDB.GetFavicon(pageURL, nil, &asyncCb, 0)
}

// fetchFromExternal fetches a favicon from DuckDuckGo's favicon API.
// Returns nil if fetch fails.
func (fc *FaviconCache) fetchFromExternal(ctx context.Context, domain string) *gdk.Texture {
	log := logging.FromContext(ctx)

	if domain == "" {
		return nil
	}

	faviconURL := fmt.Sprintf(duckduckgoFaviconURL, url.QueryEscape(domain))
	log.Debug().Str("url", faviconURL).Msg("fetching favicon from DuckDuckGo")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, faviconURL, nil)
	if err != nil {
		log.Debug().Err(err).Msg("failed to create favicon request")
		return nil
	}

	resp, err := fc.client.Do(req)
	if err != nil {
		log.Debug().Err(err).Str("domain", domain).Msg("failed to fetch favicon from DuckDuckGo")
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Debug().Int("status", resp.StatusCode).Str("domain", domain).Msg("DuckDuckGo favicon API returned non-OK status")
		return nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Debug().Err(err).Str("domain", domain).Msg("failed to read favicon response")
		return nil
	}

	if len(data) == 0 {
		log.Debug().Str("domain", domain).Msg("empty favicon response from DuckDuckGo")
		return nil
	}

	// Convert bytes to GdkTexture
	bytes := glib.NewBytes(data, uint(len(data)))
	if bytes == nil {
		log.Debug().Str("domain", domain).Msg("failed to create GBytes from favicon data")
		return nil
	}

	texture, err := gdk.NewTextureFromBytes(bytes)
	if err != nil {
		log.Debug().Err(err).Str("domain", domain).Msg("failed to create texture from favicon bytes")
		return nil
	}

	log.Debug().Str("domain", domain).Msg("favicon fetched from DuckDuckGo and cached")
	return texture
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
