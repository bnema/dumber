package favicon

import (
	"os"
	"path/filepath"
	"sync"

	domainurl "github.com/bnema/dumber/internal/domain/url"
)

const (
	// diskWriteBufferSize defines the capacity of the favicon write channel.
	diskWriteBufferSize = 100
	// File permissions for favicon cache.
	diskCacheDirPerm  = 0750
	diskCacheFilePerm = 0600
)

// diskWrite represents a favicon to be written to disk asynchronously.
type diskWrite struct {
	domain string
	data   []byte
}

// Cache provides multi-tier favicon caching (memory + disk).
type Cache struct {
	memCache  map[string][]byte
	diskDir   string
	writeChan chan diskWrite
	closeOnce sync.Once
	mu        sync.RWMutex
}

// NewCache creates a new favicon cache.
// If diskDir is empty, only in-memory caching is used.
func NewCache(diskDir string) *Cache {
	c := &Cache{
		memCache:  make(map[string][]byte),
		diskDir:   diskDir,
		writeChan: make(chan diskWrite, diskWriteBufferSize),
	}

	// Start background writer goroutine if disk caching is enabled
	if diskDir != "" {
		go c.diskWriter()
	}

	return c
}

// Get retrieves favicon bytes for a domain.
// Checks memory cache first, then disk cache.
// Returns the bytes and true if found, nil and false otherwise.
func (c *Cache) Get(domain string) ([]byte, bool) {
	if domain == "" {
		return nil, false
	}

	// Check memory cache first
	c.mu.RLock()
	data, ok := c.memCache[domain]
	c.mu.RUnlock()
	if ok {
		return data, true
	}

	// Check disk cache
	data = c.loadFromDisk(domain)
	if data != nil {
		// Populate memory cache
		c.mu.Lock()
		c.memCache[domain] = data
		c.mu.Unlock()
		return data, true
	}

	return nil, false
}

// Set stores favicon bytes for a domain.
// Writes to memory cache immediately and queues async disk write.
func (c *Cache) Set(domain string, data []byte) {
	if domain == "" || len(data) == 0 {
		return
	}

	// Write to memory cache
	c.mu.Lock()
	c.memCache[domain] = data
	c.mu.Unlock()

	// Queue async disk write
	c.queueDiskWrite(domain, data)
}

// DiskPath returns the filesystem path for a domain's cached favicon (ICO format).
// Returns empty string if disk caching is disabled or domain is empty.
func (c *Cache) DiskPath(domain string) string {
	if c.diskDir == "" || domain == "" {
		return ""
	}
	filename := domainurl.SanitizeDomainForFilename(domain)
	return filepath.Join(c.diskDir, filename)
}

// DiskPathPNG returns the filesystem path for a domain's PNG favicon.
// PNG format is required by external tools like rofi/fuzzel.
// Returns empty string if disk caching is disabled or domain is empty.
func (c *Cache) DiskPathPNG(domain string) string {
	if c.diskDir == "" || domain == "" {
		return ""
	}
	filename := domainurl.SanitizeDomainForPNG(domain)
	return filepath.Join(c.diskDir, filename)
}

// HasPNGOnDisk checks if a PNG favicon exists on disk for the given domain.
func (c *Cache) HasPNGOnDisk(domain string) bool {
	path := c.DiskPathPNG(domain)
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// DiskPathPNGSized returns the filesystem path for a sized PNG favicon.
// Used for normalized favicon export (e.g., 32x32) for CLI tools like rofi/fuzzel.
// Returns empty string if disk caching is disabled or domain is empty.
func (c *Cache) DiskPathPNGSized(domain string, size int) string {
	if c.diskDir == "" || domain == "" {
		return ""
	}
	filename := domainurl.SanitizeDomainForPNGSized(domain, size)
	return filepath.Join(c.diskDir, filename)
}

// HasPNGSizedOnDisk checks if a sized PNG favicon exists on disk for the given domain.
func (c *Cache) HasPNGSizedOnDisk(domain string, size int) bool {
	path := c.DiskPathPNGSized(domain, size)
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// WritePNG writes raw PNG data to disk for a domain.
// Used by UI layer to export WebKit textures for CLI tools.
func (c *Cache) WritePNG(domain string, pngData []byte) {
	if c.diskDir == "" || len(pngData) == 0 || domain == "" {
		return
	}

	// Ensure directory exists
	if err := os.MkdirAll(c.diskDir, diskCacheDirPerm); err != nil {
		return
	}

	filename := domainurl.SanitizeDomainForPNG(domain)
	finalPath := filepath.Join(c.diskDir, filename)
	tempPath := finalPath + ".tmp"

	// Write to temp file
	if err := os.WriteFile(tempPath, pngData, diskCacheFilePerm); err != nil {
		return
	}

	// Atomic rename
	if err := os.Rename(tempPath, finalPath); err != nil {
		_ = os.Remove(tempPath)
	}
}

// HasOnDisk checks if a favicon exists on disk for the given domain.
func (c *Cache) HasOnDisk(domain string) bool {
	path := c.DiskPath(domain)
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// Close shuts down the background writer goroutine.
func (c *Cache) Close() {
	c.closeOnce.Do(func() {
		if c.writeChan != nil {
			close(c.writeChan)
		}
	})
}

// Clear removes all entries from the in-memory cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	c.memCache = make(map[string][]byte)
	c.mu.Unlock()
}

// Size returns the number of entries in the in-memory cache.
func (c *Cache) Size() int {
	c.mu.RLock()
	size := len(c.memCache)
	c.mu.RUnlock()
	return size
}

// loadFromDisk attempts to load favicon bytes from disk cache.
func (c *Cache) loadFromDisk(domain string) []byte {
	path := c.DiskPath(domain)
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	if len(data) == 0 {
		return nil
	}

	return data
}

// writeToDisk atomically writes favicon data to disk.
func (c *Cache) writeToDisk(domain string, data []byte) {
	if c.diskDir == "" || len(data) == 0 {
		return
	}

	// Ensure directory exists
	if err := os.MkdirAll(c.diskDir, diskCacheDirPerm); err != nil {
		return
	}

	filename := domainurl.SanitizeDomainForFilename(domain)
	finalPath := filepath.Join(c.diskDir, filename)
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
func (c *Cache) queueDiskWrite(domain string, data []byte) {
	if c.diskDir == "" {
		return
	}
	select {
	case c.writeChan <- diskWrite{domain: domain, data: data}:
		// queued successfully
	default:
		// channel full, skip write
	}
}

// diskWriter processes async write requests.
func (c *Cache) diskWriter() {
	for write := range c.writeChan {
		c.writeToDisk(write.domain, write.data)
	}
}
