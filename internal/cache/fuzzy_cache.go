package cache

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/logging"
)

// Cache update constants
const (
	updateWaitIntervalMs = 10   // Milliseconds to wait during concurrent cache build
	dbHashTimeoutSec     = 5    // Seconds timeout for database hash calculation
	recentHistoryLimit   = 20   // Number of recent entries to check for changes
	exactMatchBonus      = 0.95 // Score bonus for exact matches
	dirPerm              = 0755 // Directory permissions
)

// CacheManager handles the lifecycle of the fuzzy cache with smart invalidation.
type CacheManager struct {
	cache      *DmenuFuzzyCache
	config     *CacheConfig
	queries    HistoryQuerier
	mu         sync.RWMutex
	lastDBHash string // Hash of database content to detect changes
	updating   int32  // Atomic flag to prevent concurrent updates
}

// NewCacheManager creates a new cache manager.
func NewCacheManager(queries HistoryQuerier, config *CacheConfig) *CacheManager {
	if config == nil {
		config = DefaultCacheConfig()
	}

	// Set default cache file path if not specified
	if config.CacheFile == "" {
		stateDir, err := config.GetStateDir()
		if err != nil {
			// Fallback to temp directory if state dir fails
			config.CacheFile = filepath.Join(os.TempDir(), "dumber_fuzzy_cache.bin")
		} else {
			config.CacheFile = filepath.Join(stateDir, "dmenu_fuzzy_cache.bin")
		}
	}

	cm := &CacheManager{
		cache:   &DmenuFuzzyCache{},
		config:  config,
		queries: queries,
	}

	return cm
}

// GetCache returns the current cache, loading from filesystem or DB as needed.
func (cm *CacheManager) GetCache(ctx context.Context) (*DmenuFuzzyCache, error) {
	cm.mu.RLock()

	// Check if we have a valid cache
	if cm.cache.entryCount > 0 {
		defer cm.mu.RUnlock()
		return cm.cache, nil
	}
	cm.mu.RUnlock()

	// Need to load cache
	return cm.loadOrBuildCache(ctx)
}

// loadOrBuildCache loads cache from filesystem or builds from database.
func (cm *CacheManager) loadOrBuildCache(ctx context.Context) (*DmenuFuzzyCache, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Double-check after acquiring lock
	if cm.cache.entryCount > 0 {
		return cm.cache, nil
	}

	// Try loading from filesystem first
	if cm.shouldLoadFromFile() {
		if err := cm.cache.LoadFromBinary(cm.config.CacheFile); err == nil {
			// Verify cache is still valid
			if cm.isCacheValid(ctx) {
				return cm.cache, nil
			}
		}
	}

	// Build fresh cache from database
	return cm.buildCacheFromDB(ctx)
}

// shouldLoadFromFile determines if we should try loading from filesystem.
func (cm *CacheManager) shouldLoadFromFile() bool {
	if _, err := os.Stat(cm.config.CacheFile); os.IsNotExist(err) {
		return false
	}

	// Check if file is valid and not too old
	if !IsValidCacheFile(cm.config.CacheFile) {
		return false
	}

	// Check file age
	info, err := os.Stat(cm.config.CacheFile)
	if err != nil {
		return false
	}

	return time.Since(info.ModTime()) < cm.config.TTL
}

// isCacheValid checks if the cache is still valid by comparing DB hash.
func (cm *CacheManager) isCacheValid(ctx context.Context) bool {
	currentHash, err := cm.calculateDBHash(ctx)
	if err != nil {
		return false
	}

	return currentHash == cm.lastDBHash
}

// buildCacheFromDB builds a fresh cache from the database.
func (cm *CacheManager) buildCacheFromDB(ctx context.Context) (*DmenuFuzzyCache, error) {
	// Prevent concurrent builds
	if !atomic.CompareAndSwapInt32(&cm.updating, 0, 1) {
		// Another goroutine is building, wait for it
		for atomic.LoadInt32(&cm.updating) == 1 {
			time.Sleep(updateWaitIntervalMs * time.Millisecond)
		}
		return cm.cache, nil
	}
	defer atomic.StoreInt32(&cm.updating, 0)

	startTime := time.Now()

	// Get history from database
	history, err := cm.queries.GetHistory(ctx, int64(cm.config.MaxEntries))
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	// Build new cache
	newCache := &DmenuFuzzyCache{
		version:      CacheVersion,
		lastModified: time.Now().Unix(),
		entryCount:   uint32(len(history)),
	}

	// Convert database entries to compact format
	newCache.entries = make([]CompactEntry, len(history))
	for i, entry := range history {
		title := ""
		if entry.Title.Valid {
			title = entry.Title.String
		}

		visitCount := int64(0)
		if entry.VisitCount.Valid {
			visitCount = entry.VisitCount.Int64
		}

		lastVisited := time.Now()
		if entry.LastVisited.Valid {
			lastVisited = entry.LastVisited.Time
		}

		faviconURL := ""
		if entry.FaviconUrl.Valid {
			faviconURL = entry.FaviconUrl.String
		}

		newCache.entries[i] = NewCompactEntry(
			entry.Url,
			title,
			faviconURL,
			visitCount,
			lastVisited,
		)
	}

	// Build search indices
	newCache.buildTrigramIndex()
	newCache.buildPrefixTrie()
	newCache.buildSortedIndex() // Pre-sort for O(1) getTopEntries

	// Update current cache
	cm.cache = newCache

	// Update DB hash
	cm.lastDBHash, _ = cm.calculateDBHash(ctx)

	// Save to filesystem in background
	go cm.saveToFilesystemAsync()

	// Log cache timing to file only (not stdout) to avoid interfering with dmenu
	if logger := logging.GetLogger(); logger != nil {
		logger.WriteFileOnly(logging.LogLevelInfo(), fmt.Sprintf("built in %v with %d entries", time.Since(startTime), len(history)), "CACHE")
	}
	return cm.cache, nil
}

// InvalidateAndRefresh forces cache invalidation and refresh.
// This should be called when we know the DB has changed (e.g., after adding history).
func (cm *CacheManager) InvalidateAndRefresh(ctx context.Context) {
	go func() {
		cm.mu.Lock()
		defer cm.mu.Unlock()

		// Clear current cache to force rebuild
		cm.cache = &DmenuFuzzyCache{}
		cm.lastDBHash = ""

		// Remove cache file
		if err := os.Remove(cm.config.CacheFile); err != nil {
			logging.Warn(fmt.Sprintf("failed to remove cache file %s: %v", cm.config.CacheFile, err))
		}

		// Build new cache
		_, err := cm.buildCacheFromDB(ctx)
		if err != nil {
			logging.Warn(fmt.Sprintf("failed to refresh cache: %v", err))
		}
	}()
}

// OnApplicationExit should be called when the application is about to exit.
// This triggers a background cache refresh for the next startup.
func (cm *CacheManager) OnApplicationExit(ctx context.Context) {
	// Check if cache needs updating
	if !cm.isCacheValid(ctx) {
		// Refresh cache in background for next startup
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), dbHashTimeoutSec*time.Second)
			defer cancel()

			_, err := cm.buildCacheFromDB(ctx)
			if err != nil {
				logging.Warn(fmt.Sprintf("failed to refresh cache on exit: %v", err))
			}
		}()
	}
}

// saveToFilesystemAsync saves the cache to filesystem in a background goroutine.
func (cm *CacheManager) saveToFilesystemAsync() {
	// Create cache directory if it doesn't exist
	cacheDir := filepath.Dir(cm.config.CacheFile)
	if err := os.MkdirAll(cacheDir, dirPerm); err != nil {
		logging.Warn(fmt.Sprintf("failed to create cache directory: %v", err))
		return
	}

	// Save to temporary file first, then atomic rename
	tempFile := cm.config.CacheFile + ".tmp"
	if err := cm.cache.SaveToBinary(tempFile); err != nil {
		logging.Warn(fmt.Sprintf("failed to save cache: %v", err))
		if err := os.Remove(tempFile); err != nil {
			logging.Warn(fmt.Sprintf("failed to remove temp file %s: %v", tempFile, err))
		}
		return
	}

	// Atomic rename
	if err := os.Rename(tempFile, cm.config.CacheFile); err != nil {
		logging.Warn(fmt.Sprintf("failed to rename cache file: %v", err))
		if err := os.Remove(tempFile); err != nil {
			logging.Warn(fmt.Sprintf("failed to remove temp file %s: %v", tempFile, err))
		}
	}
}

// calculateDBHash creates a hash of the current database state to detect changes.
func (cm *CacheManager) calculateDBHash(ctx context.Context) (string, error) {
	// Get a small sample of recent entries to create a fingerprint
	recentHistory, err := cm.queries.GetHistory(ctx, recentHistoryLimit) // Just check recent entries
	if err != nil {
		return "", err
	}

	// Create hash from URLs, visit counts, and last visited times
	hasher := md5.New()
	for _, entry := range recentHistory {
		hasher.Write([]byte(entry.Url))
		if entry.VisitCount.Valid {
			if _, err := fmt.Fprintf(hasher, "%d", entry.VisitCount.Int64); err != nil {
				logging.Warn(fmt.Sprintf("failed to write visit count to hasher: %v", err))
			}
		}
		if entry.LastVisited.Valid {
			hasher.Write([]byte(entry.LastVisited.Time.Format(time.RFC3339)))
		}
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// Search performs a fuzzy search using the cached data.
func (cm *CacheManager) Search(ctx context.Context, query string) (*FuzzyResult, error) {
	cache, err := cm.GetCache(ctx)
	if err != nil {
		return nil, err
	}

	return cache.Search(query, cm.config), nil
}

// GetTopEntries returns the top entries without a search query.
func (cm *CacheManager) GetTopEntries(ctx context.Context) (*FuzzyResult, error) {
	cache, err := cm.GetCache(ctx)
	if err != nil {
		return nil, err
	}

	return cache.getTopEntries(cm.config), nil
}

// Stats returns cache statistics.
func (cm *CacheManager) Stats() CacheStats {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	stats := CacheStats{
		EntryCount:   int(cm.cache.entryCount),
		TrigramCount: len(cm.cache.trigramIndex),
		LastModified: time.Unix(cm.cache.lastModified, 0),
	}

	if info, err := os.Stat(cm.config.CacheFile); err == nil {
		stats.FileSize = info.Size()
		stats.FileModTime = info.ModTime()
	}

	return stats
}

// CacheStats provides information about the cache state.
type CacheStats struct {
	EntryCount   int       // Number of cached entries
	TrigramCount int       // Number of trigrams in index
	FileSize     int64     // Size of cache file on disk
	LastModified time.Time // When cache was last built
	FileModTime  time.Time // When cache file was last modified
}

// String returns a string representation of cache stats.
func (cs CacheStats) String() string {
	return fmt.Sprintf("Entries: %d, Trigrams: %d, File: %d bytes, Modified: %v",
		cs.EntryCount, cs.TrigramCount, cs.FileSize, cs.FileModTime.Format("15:04:05"))
}
