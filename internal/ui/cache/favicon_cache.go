// Package cache provides in-memory caching utilities for the UI layer.
package cache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/infrastructure/config"
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
	// diskWriteBufferSize defines the capacity of the favicon write channel
	diskWriteBufferSize = 100
	// File permissions for favicon cache
	diskCacheDirPerm  = 0750
	diskCacheFilePerm = 0600
)

// diskWrite represents a favicon to be written to disk asynchronously.
type diskWrite struct {
	domain string
	data   []byte
}

// FaviconCache provides a multi-tier favicon caching system:
// 1. In-memory cache keyed by domain for fast lookups
// 2. Disk cache for persistence across restarts
// 3. WebKit FaviconDatabase as backing store
// 4. DuckDuckGo favicon API as fallback
type FaviconCache struct {
	mu           sync.RWMutex
	cache        map[string]*gdk.Texture // key: domain (e.g., "github.com")
	faviconDB    *webkit.FaviconDatabase
	client       *http.Client
	diskCacheDir string         // path to favicon cache directory
	writeChan    chan diskWrite // channel for async writes
	closeOnce    sync.Once      // ensures Close is called only once
}

// NewFaviconCache creates a new FaviconCache with the given FaviconDatabase.
// The faviconDB can be nil, in which case only the in-memory cache is used.
func NewFaviconCache(faviconDB *webkit.FaviconDatabase) *FaviconCache {
	// Get disk cache directory (fail silently if error)
	diskCacheDir, _ := config.GetFaviconCacheDir()

	fc := &FaviconCache{
		cache:        make(map[string]*gdk.Texture),
		faviconDB:    faviconDB,
		diskCacheDir: diskCacheDir,
		writeChan:    make(chan diskWrite, diskWriteBufferSize),
		client: &http.Client{
			Timeout: faviconFetchTimeout,
		},
	}

	// Start background writer goroutine
	if diskCacheDir != "" {
		go fc.diskWriter()
	}

	return fc
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
// Returns nil if not found in memory cache.
func (fc *FaviconCache) GetByURL(pageURL string) *gdk.Texture {
	domain := extractDomain(pageURL)
	return fc.Get(domain)
}

// GetFromCacheByURL checks both memory and disk cache for a favicon.
// Returns nil if not found. Does not fetch from external sources.
// If found on disk, also populates memory cache.
func (fc *FaviconCache) GetFromCacheByURL(pageURL string) *gdk.Texture {
	domain := extractDomain(pageURL)
	if domain == "" {
		return nil
	}

	// Check memory cache first
	if texture := fc.Get(domain); texture != nil {
		return texture
	}

	// Check disk cache
	if texture := fc.loadFromDisk(domain); texture != nil {
		fc.Set(domain, texture)
		return texture
	}

	return nil
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

	// Check disk cache second
	if texture := fc.loadFromDisk(domain); texture != nil {
		fc.Set(domain, texture)
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
			texture, rawData := fc.fetchFromExternal(ctx, domain)
			if texture != nil {
				fc.Set(domain, texture)
				// Queue async disk write
				if len(rawData) > 0 {
					fc.queueDiskWrite(domain, rawData)
				}
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
// Returns the texture and raw bytes (bytes used for disk cache).
func (fc *FaviconCache) fetchFromExternal(ctx context.Context, domain string) (texture *gdk.Texture, rawData []byte) {
	log := logging.FromContext(ctx)

	if domain == "" {
		return nil, nil
	}

	faviconURL := fmt.Sprintf(duckduckgoFaviconURL, url.QueryEscape(domain))
	log.Debug().Str("url", faviconURL).Msg("fetching favicon from DuckDuckGo")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, faviconURL, http.NoBody)
	if err != nil {
		log.Debug().Err(err).Msg("failed to create favicon request")
		return nil, nil
	}

	resp, err := fc.client.Do(req)
	if err != nil {
		log.Debug().Err(err).Str("domain", domain).Msg("failed to fetch favicon from DuckDuckGo")
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Debug().Int("status", resp.StatusCode).Str("domain", domain).Msg("DuckDuckGo favicon API returned non-OK status")
		return nil, nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Debug().Err(err).Str("domain", domain).Msg("failed to read favicon response")
		return nil, nil
	}

	if len(data) == 0 {
		log.Debug().Str("domain", domain).Msg("empty favicon response from DuckDuckGo")
		return nil, nil
	}

	// Convert bytes to GdkTexture
	bytes := glib.NewBytes(data, uint(len(data)))
	if bytes == nil {
		log.Debug().Str("domain", domain).Msg("failed to create GBytes from favicon data")
		return nil, nil
	}

	texture, err = gdk.NewTextureFromBytes(bytes)
	if err != nil {
		log.Debug().Err(err).Str("domain", domain).Msg("failed to create texture from favicon bytes")
		return nil, nil
	}

	log.Debug().Str("domain", domain).Msg("favicon fetched from DuckDuckGo and cached")
	return texture, data
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

// Close stops the background writer goroutine.
func (fc *FaviconCache) Close() {
	fc.closeOnce.Do(func() {
		if fc.writeChan != nil {
			close(fc.writeChan)
		}
	})
}

// sanitizeDomainForFilename converts a domain to a safe filename.
func sanitizeDomainForFilename(domain string) string {
	replacer := strings.NewReplacer(
		":", "_",
		"/", "_",
		"\\", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(domain) + ".ico"
}

// loadFromDisk attempts to load a favicon from disk cache.
// Returns nil if not found or on error.
func (fc *FaviconCache) loadFromDisk(domain string) *gdk.Texture {
	if fc.diskCacheDir == "" {
		return nil
	}

	filename := sanitizeDomainForFilename(domain)
	path := filepath.Join(fc.diskCacheDir, filename)

	//nolint:gosec // path is constructed from sanitized domain, not user input
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	if len(data) == 0 {
		return nil
	}

	bytes := glib.NewBytes(data, uint(len(data)))
	if bytes == nil {
		return nil
	}

	texture, err := gdk.NewTextureFromBytes(bytes)
	if err != nil {
		return nil
	}

	return texture
}

// writeToDisk atomically writes favicon data to disk.
func (fc *FaviconCache) writeToDisk(domain string, data []byte) {
	if fc.diskCacheDir == "" || len(data) == 0 {
		return
	}

	// Ensure directory exists (lazy initialization)
	if err := os.MkdirAll(fc.diskCacheDir, diskCacheDirPerm); err != nil {
		return
	}

	filename := sanitizeDomainForFilename(domain)
	finalPath := filepath.Join(fc.diskCacheDir, filename)
	tempPath := finalPath + ".tmp"

	// Write to temp file
	if err := os.WriteFile(tempPath, data, diskCacheFilePerm); err != nil {
		return
	}

	// Atomic rename
	if err := os.Rename(tempPath, finalPath); err != nil {
		_ = os.Remove(tempPath)
	}
}

// queueDiskWrite sends favicon data to be written asynchronously.
func (fc *FaviconCache) queueDiskWrite(domain string, data []byte) {
	select {
	case fc.writeChan <- diskWrite{domain: domain, data: data}:
		// queued successfully
	default:
		// channel full, skip write
	}
}

// diskWriter processes async write requests.
func (fc *FaviconCache) diskWriter() {
	for write := range fc.writeChan {
		fc.writeToDisk(write.domain, write.data)
	}
}
