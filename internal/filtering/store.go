package filtering

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/logging"
)

const (
	cacheVersion    = 1    // Cache format version for compatibility
	dirPermissions  = 0755 // Directory permissions
	filePermissions = 0644 // File permissions
)

// FileFilterStore implements FilterStore using filesystem storage
type FileFilterStore struct {
	cacheFile string
	metaFile  string
}

// CacheMetadata stores information about the cache
type CacheMetadata struct {
	Version        int               `json:"version"`
	CreatedAt      time.Time         `json:"created_at"`
	LastUsed       time.Time         `json:"last_used"`
	DataHash       string            `json:"data_hash"`
	FilterHashes   []string          `json:"filter_hashes"`   // Hashes of source filter lists
	SourceVersions map[string]string `json:"source_versions"` // URL -> filter list version
	LastCheckTime  time.Time         `json:"last_check_time"` // Last version check time
}

// NewFileFilterStore creates a new file-based filter store
func NewFileFilterStore() (*FileFilterStore, error) {
	cacheFile, err := config.GetFilterCacheFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache file path: %w", err)
	}

	cacheDir := filepath.Dir(cacheFile)
	metaFile := filepath.Join(cacheDir, "metadata.json")

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, dirPermissions); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &FileFilterStore{
		cacheFile: cacheFile,
		metaFile:  metaFile,
	}, nil
}

// LoadCached loads compiled filters from cache
func (fs *FileFilterStore) LoadCached() (*CompiledFilters, error) {
	// Check if cache file exists
	if _, err := os.Stat(fs.cacheFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("cache file does not exist")
	}

	// Load metadata
	metadata, err := fs.loadMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to load cache metadata: %w", err)
	}

	// Verify cache version compatibility
	if metadata.Version != cacheVersion {
		return nil, fmt.Errorf("cache version mismatch: got %d, expected %d", metadata.Version, cacheVersion)
	}

	// Load cache data
	data, err := os.ReadFile(fs.cacheFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	// Verify data integrity
	dataHash := fmt.Sprintf("%x", sha256.Sum256(data))
	if dataHash != metadata.DataHash {
		return nil, fmt.Errorf("cache data corrupted: hash mismatch")
	}

	// Deserialize compiled filters
	var compiled CompiledFilters
	if err := json.Unmarshal(data, &compiled); err != nil {
		return nil, fmt.Errorf("failed to deserialize cache data: %w", err)
	}

	// Update last used time
	metadata.LastUsed = time.Now()
	if err := fs.saveMetadata(metadata); err != nil {
		logging.Error(fmt.Sprintf("Failed to update cache metadata: %v", err))
	}

	logging.Info(fmt.Sprintf("Loaded filter cache from %s", fs.cacheFile))
	return &compiled, nil
}

// SaveCache saves compiled filters to cache
func (fs *FileFilterStore) SaveCache(filters *CompiledFilters) error {
	// Serialize filters
	data, err := json.Marshal(filters)
	if err != nil {
		return fmt.Errorf("failed to serialize filters: %w", err)
	}

	// Load existing metadata to preserve SourceVersions
	existingMeta, _ := fs.loadMetadata()
	var sourceVersions map[string]string
	var lastCheckTime time.Time
	if existingMeta != nil {
		if existingMeta.SourceVersions != nil {
			sourceVersions = existingMeta.SourceVersions
		}
		lastCheckTime = existingMeta.LastCheckTime
	}

	// Create metadata, preserving existing SourceVersions
	metadata := &CacheMetadata{
		Version:        cacheVersion,
		CreatedAt:      time.Now(),
		LastUsed:       time.Now(),
		DataHash:       fmt.Sprintf("%x", sha256.Sum256(data)),
		FilterHashes:   []string{},
		SourceVersions: sourceVersions,
		LastCheckTime:  lastCheckTime,
	}

	// Write cache data atomically
	tempFile := fs.cacheFile + ".tmp"
	if err := os.WriteFile(tempFile, data, filePermissions); err != nil {
		return fmt.Errorf("failed to write cache data: %w", err)
	}

	if err := os.Rename(tempFile, fs.cacheFile); err != nil {
		if removeErr := os.Remove(tempFile); removeErr != nil {
			logging.Error(fmt.Sprintf("Failed to cleanup temp file %s: %v", tempFile, removeErr))
		}
		return fmt.Errorf("failed to move cache file: %w", err)
	}

	// Save metadata
	if err := fs.saveMetadata(metadata); err != nil {
		return fmt.Errorf("failed to save cache metadata: %w", err)
	}

	logging.Info(fmt.Sprintf("Saved filter cache to %s", fs.cacheFile))
	return nil
}

// GetCacheInfo returns cache existence and modification time
func (fs *FileFilterStore) GetCacheInfo() (bool, time.Time, error) {
	info, err := os.Stat(fs.cacheFile)
	if os.IsNotExist(err) {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, err
	}

	return true, info.ModTime(), nil
}

// InvalidateCache removes the cache file
func (fs *FileFilterStore) InvalidateCache() error {
	// Remove cache file
	if err := os.Remove(fs.cacheFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove cache file: %w", err)
	}

	// Remove metadata file
	if err := os.Remove(fs.metaFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove metadata file: %w", err)
	}

	logging.Info("Filter cache invalidated")
	return nil
}

// loadMetadata loads cache metadata from file
func (fs *FileFilterStore) loadMetadata() (*CacheMetadata, error) {
	data, err := os.ReadFile(fs.metaFile)
	if err != nil {
		return nil, err
	}

	var metadata CacheMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// saveMetadata saves cache metadata to file
func (fs *FileFilterStore) saveMetadata(metadata *CacheMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	// Write metadata atomically
	tempFile := fs.metaFile + ".tmp"
	if err := os.WriteFile(tempFile, data, filePermissions); err != nil {
		return err
	}

	if err := os.Rename(tempFile, fs.metaFile); err != nil {
		if removeErr := os.Remove(tempFile); removeErr != nil {
			logging.Error(fmt.Sprintf("Failed to cleanup temp metadata file %s: %v", tempFile, removeErr))
		}
		return err
	}

	return nil
}

// GetSourceVersion returns the stored version for a filter list URL
func (fs *FileFilterStore) GetSourceVersion(url string) string {
	metadata, err := fs.loadMetadata()
	if err != nil {
		return ""
	}
	if metadata.SourceVersions == nil {
		return ""
	}
	return metadata.SourceVersions[url]
}

// SetSourceVersion stores the version for a filter list URL
func (fs *FileFilterStore) SetSourceVersion(url string, version string) error {
	metadata, err := fs.loadMetadata()
	if err != nil {
		// Create new metadata if none exists
		metadata = &CacheMetadata{
			Version:        cacheVersion,
			CreatedAt:      time.Now(),
			SourceVersions: make(map[string]string),
		}
	}
	if metadata.SourceVersions == nil {
		metadata.SourceVersions = make(map[string]string)
	}
	metadata.SourceVersions[url] = version
	metadata.LastCheckTime = time.Now()
	return fs.saveMetadata(metadata)
}

// GetLastCheckTime returns when versions were last checked
func (fs *FileFilterStore) GetLastCheckTime() time.Time {
	metadata, err := fs.loadMetadata()
	if err != nil {
		return time.Time{}
	}
	return metadata.LastCheckTime
}
