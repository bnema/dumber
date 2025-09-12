// Package cache provides binary format serialization for fuzzy cache data structures.
package cache

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"syscall"
	"unsafe"

	"github.com/bnema/dumber/internal/logging"
)

// Binary cache format layout:
// Header:    [magic:4][version:4][entryCount:4][indexOffset:8][lastModified:8]
// Entries:   [url_len:2][url:N][title_len:2][title:N][visit:2][lastVisit:4][score:2]
// Index:     [trigramCount:4][trigram:3][count:4][ids:4*count]...

// Binary format constants for cache file structure
const (
	CacheMagic   = 0x44554d42 // "DUMB" in little-endian
	CacheVersion = 1
	HeaderSize   = 28 // Size of binary header
)

// BinaryHeader represents the cache file header.
type BinaryHeader struct {
	Magic        uint32 // Magic number for format identification
	Version      uint32 // Cache format version
	EntryCount   uint32 // Number of entries in cache
	IndexOffset  uint64 // Offset to trigram index section
	LastModified int64  // Unix timestamp of last modification
}

// SaveToBinary writes the cache to a binary file using memory mapping for performance.
func (c *DmenuFuzzyCache) SaveToBinary(filename string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Calculate total file size
	totalSize := c.calculateFileSize()

	// Create the file with the exact size needed
	file, err := os.Create(filename) //nolint:gosec // G304: filename is controlled by cache configuration, not user input
	if err != nil {
		return fmt.Errorf("failed to create cache file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logging.Warn(fmt.Sprintf("failed to close cache file: %v", closeErr))
		}
	}()

	// Pre-allocate file space
	if err := file.Truncate(int64(totalSize)); err != nil {
		return fmt.Errorf("failed to allocate file space: %w", err)
	}

	// Memory map the file for fast writing
	data, err := syscall.Mmap(int(file.Fd()), 0, totalSize, syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("failed to mmap file: %w", err)
	}
	defer func() {
		if unmapErr := syscall.Munmap(data); unmapErr != nil {
			logging.Warn(fmt.Sprintf("failed to unmap cache file: %v", unmapErr))
		}
	}()

	// Write data using unsafe pointer operations for maximum speed
	offset := 0

	// Write header
	offset = c.writeHeader(data, offset, totalSize)

	// Write entries
	entriesEndOffset := c.writeEntries(data, offset)

	// Write trigram index
	trigramEndOffset := c.writeTrigramIndex(data, entriesEndOffset)

	// Write sorted index
	c.writeSortedIndex(data, trigramEndOffset)

	// Force sync to disk (Linux-specific, skip on error)
	// syscall.Msync may not be available on all platforms
	// _ = syscall.Msync(data, syscall.MS_SYNC)

	return nil
}

// LoadFromBinary loads cache from a binary file using memory mapping.
func (c *DmenuFuzzyCache) LoadFromBinary(filename string) error {
	file, err := os.Open(filename) //nolint:gosec // G304: filename is controlled by cache configuration, not user input
	if err != nil {
		return fmt.Errorf("failed to open cache file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logging.Warn(fmt.Sprintf("failed to close cache file: %v", closeErr))
		}
	}()

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat cache file: %w", err)
	}
	fileSize := int(stat.Size())

	if fileSize < HeaderSize {
		return fmt.Errorf("cache file too small: %d bytes", fileSize)
	}

	// Memory map the file for fast reading
	data, err := syscall.Mmap(int(file.Fd()), 0, fileSize, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("failed to mmap cache file: %w", err)
	}
	// Note: We don't defer Munmap here as we want to keep the mapping alive
	// The mapping will be cleaned up when the process exits

	// Parse header
	header, err := c.parseHeader(data)
	if err != nil {
		if unmapErr := syscall.Munmap(data); unmapErr != nil {
			logging.Warn(fmt.Sprintf("failed to unmap cache file: %v", unmapErr))
		}
		return fmt.Errorf("failed to parse header: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Parse entries
	entries, err := c.parseEntries(data, HeaderSize, header.EntryCount)
	if err != nil {
		if unmapErr := syscall.Munmap(data); unmapErr != nil {
			logging.Warn(fmt.Sprintf("failed to unmap cache file: %v", unmapErr))
		}
		return fmt.Errorf("failed to parse entries: %w", err)
	}

	// Parse trigram index
	if header.IndexOffset > uint64(^uint(0)>>1) { // Check if it exceeds int max
		return fmt.Errorf("index offset too large: %d", header.IndexOffset)
	}
	trigramIndex, trigramEndOffset, err := c.parseTrigramIndex(data, int(header.IndexOffset)) //nolint:gosec // G115: bounds checked above
	if err != nil {
		if unmapErr := syscall.Munmap(data); unmapErr != nil {
			logging.Warn(fmt.Sprintf("failed to unmap cache file: %v", unmapErr))
		}
		return fmt.Errorf("failed to parse trigram index: %w", err)
	}

	// Parse sorted index
	sortedIndex, err := c.parseSortedIndex(data, trigramEndOffset)
	if err != nil {
		if unmapErr := syscall.Munmap(data); unmapErr != nil {
			logging.Warn(fmt.Sprintf("failed to unmap cache file: %v", unmapErr))
		}
		return fmt.Errorf("failed to parse sorted index: %w", err)
	}

	// Update cache state
	c.entries = entries
	c.trigramIndex = trigramIndex
	c.sortedIndex = sortedIndex
	c.entryCount = header.EntryCount
	c.lastModified = header.LastModified
	c.version = header.Version

	return nil
}

// calculateFileSize estimates the total file size needed.
func (c *DmenuFuzzyCache) calculateFileSize() int {
	size := HeaderSize

	// Calculate entries section size
	for _, entry := range c.entries {
		size += 2 + len(entry.URL)   // url_len + url
		size += 2 + len(entry.Title) // title_len + title
		size += 2 + 4 + 2            // visit + lastVisit + score
	}

	// Calculate trigram index size
	size += 4 // trigram count
	for trigram, ids := range c.trigramIndex {
		size += len(trigram) + 4 + len(ids)*4 // trigram + count + ids
	}

	// Calculate sorted index size
	size += 4 + len(c.sortedIndex)*4 // count + indices

	return size
}

// writeHeader writes the binary header.
func (c *DmenuFuzzyCache) writeHeader(data []byte, offset int, totalSize int) int {
	// Log header info to file only (not stdout) to avoid interfering with dmenu
	if logger := logging.GetLogger(); logger != nil {
		logger.WriteFileOnly(logging.LogLevelInfo(), fmt.Sprintf("writing header: totalSize=%d bytes", totalSize), "CACHE")
	}
	// Calculate index offset (header + all entries)
	indexOffset := HeaderSize
	for _, entry := range c.entries {
		indexOffset += 2 + len(entry.URL) + 2 + len(entry.Title) + 8
	}

	// Check bounds for integer conversions
	if len(c.entries) > 0xFFFFFFFF { // uint32 max
		logging.Warn(fmt.Sprintf("too many entries for binary format: %d, truncating", len(c.entries)))
	}
	if indexOffset < 0 {
		log.Printf("Warning: negative index offset: %d, using 0", indexOffset)
		indexOffset = 0
	}

	header := BinaryHeader{
		Magic:        CacheMagic,
		Version:      CacheVersion,
		EntryCount:   uint32(len(c.entries)),   //nolint:gosec // G115: bounds checked above
		IndexOffset:  uint64(indexOffset),      //nolint:gosec // G115: bounds checked above
		LastModified: c.lastModified,
	}

	// Write header using unsafe for maximum performance
	*(*BinaryHeader)(unsafe.Pointer(&data[offset])) = header
	return offset + HeaderSize
}

// writeEntries writes all cache entries in binary format.
func (c *DmenuFuzzyCache) writeEntries(data []byte, offset int) int {
	for _, entry := range c.entries {
		// Write URL
		urlLen := uint16(len(entry.URL))
		*(*uint16)(unsafe.Pointer(&data[offset])) = urlLen
		offset += 2
		copy(data[offset:], entry.URL)
		offset += len(entry.URL)

		// Write Title
		titleLen := uint16(len(entry.Title))
		*(*uint16)(unsafe.Pointer(&data[offset])) = titleLen
		offset += 2
		copy(data[offset:], entry.Title)
		offset += len(entry.Title)

		// Write metadata
		*(*uint16)(unsafe.Pointer(&data[offset])) = entry.VisitCount
		offset += 2
		*(*uint32)(unsafe.Pointer(&data[offset])) = entry.LastVisit
		offset += 4
		*(*uint16)(unsafe.Pointer(&data[offset])) = entry.Score
		offset += 2
	}
	return offset
}

// writeTrigramIndex writes the trigram index in binary format.
func (c *DmenuFuzzyCache) writeTrigramIndex(data []byte, offset int) int {
	// Write trigram count
	trigramCount := uint32(len(c.trigramIndex))
	*(*uint32)(unsafe.Pointer(&data[offset])) = trigramCount
	offset += 4

	// Write trigrams and their entry lists
	for trigram, ids := range c.trigramIndex {
		// Write trigram (exactly 3 bytes)
		copy(data[offset:offset+3], trigram)
		offset += 3

		// Write entry count
		idCount := uint32(len(ids))
		*(*uint32)(unsafe.Pointer(&data[offset])) = idCount
		offset += 4

		// Write entry IDs
		for _, id := range ids {
			*(*uint32)(unsafe.Pointer(&data[offset])) = id
			offset += 4
		}
	}
	return offset
}

// writeSortedIndex writes the sorted index in binary format.
func (c *DmenuFuzzyCache) writeSortedIndex(data []byte, offset int) int {
	// Write sorted index count
	indexCount := uint32(len(c.sortedIndex))
	*(*uint32)(unsafe.Pointer(&data[offset])) = indexCount
	offset += 4

	// Write sorted indices
	for _, id := range c.sortedIndex {
		*(*uint32)(unsafe.Pointer(&data[offset])) = id
		offset += 4
	}
	return offset
}

// parseHeader parses the binary header from mapped data.
func (c *DmenuFuzzyCache) parseHeader(data []byte) (*BinaryHeader, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("insufficient data for header")
	}

	header := (*BinaryHeader)(unsafe.Pointer(&data[0]))

	if header.Magic != CacheMagic {
		return nil, fmt.Errorf("invalid magic number: 0x%x", header.Magic)
	}

	if header.Version != CacheVersion {
		return nil, fmt.Errorf("unsupported cache version: %d", header.Version)
	}

	return header, nil
}

// parseEntries parses cache entries from binary data.
func (c *DmenuFuzzyCache) parseEntries(data []byte, offset int, count uint32) ([]CompactEntry, error) {
	entries := make([]CompactEntry, 0, count)

	for i := uint32(0); i < count; i++ {
		if offset+2 > len(data) {
			return nil, fmt.Errorf("insufficient data for URL length at entry %d", i)
		}

		// Read URL
		urlLen := *(*uint16)(unsafe.Pointer(&data[offset]))
		offset += 2
		if offset+int(urlLen) > len(data) {
			return nil, fmt.Errorf("insufficient data for URL at entry %d", i)
		}
		url := string(data[offset : offset+int(urlLen)])
		offset += int(urlLen)

		// Read Title
		if offset+2 > len(data) {
			return nil, fmt.Errorf("insufficient data for title length at entry %d", i)
		}
		titleLen := *(*uint16)(unsafe.Pointer(&data[offset]))
		offset += 2
		if offset+int(titleLen) > len(data) {
			return nil, fmt.Errorf("insufficient data for title at entry %d", i)
		}
		title := string(data[offset : offset+int(titleLen)])
		offset += int(titleLen)

		// Read metadata
		if offset+8 > len(data) {
			return nil, fmt.Errorf("insufficient data for metadata at entry %d", i)
		}
		visitCount := *(*uint16)(unsafe.Pointer(&data[offset]))
		offset += 2
		lastVisit := *(*uint32)(unsafe.Pointer(&data[offset]))
		offset += 4
		score := *(*uint16)(unsafe.Pointer(&data[offset]))
		offset += 2

		entries = append(entries, CompactEntry{
			URL:        url,
			Title:      title,
			VisitCount: visitCount,
			LastVisit:  lastVisit,
			Score:      score,
		})
	}

	return entries, nil
}

// parseTrigramIndex parses the trigram index from binary data.
func (c *DmenuFuzzyCache) parseTrigramIndex(data []byte, offset int) (map[string][]uint32, int, error) {
	if offset+4 > len(data) {
		return nil, 0, fmt.Errorf("insufficient data for trigram count")
	}

	trigramCount := *(*uint32)(unsafe.Pointer(&data[offset]))
	offset += 4

	trigramIndex := make(map[string][]uint32, trigramCount)

	for i := uint32(0); i < trigramCount; i++ {
		// Read trigram
		if offset+3 > len(data) {
			return nil, 0, fmt.Errorf("insufficient data for trigram %d", i)
		}
		trigram := string(data[offset : offset+3])
		offset += 3

		// Read entry count
		if offset+4 > len(data) {
			return nil, 0, fmt.Errorf("insufficient data for entry count in trigram %d", i)
		}
		idCount := *(*uint32)(unsafe.Pointer(&data[offset]))
		offset += 4

		// Read entry IDs
		if offset+int(idCount)*4 > len(data) {
			return nil, 0, fmt.Errorf("insufficient data for entry IDs in trigram %d", i)
		}

		ids := make([]uint32, idCount)
		for j := uint32(0); j < idCount; j++ {
			ids[j] = *(*uint32)(unsafe.Pointer(&data[offset]))
			offset += 4
		}

		trigramIndex[trigram] = ids
	}

	return trigramIndex, offset, nil
}

// parseSortedIndex parses the sorted index from binary data.
func (c *DmenuFuzzyCache) parseSortedIndex(data []byte, offset int) ([]uint32, error) {
	if offset+4 > len(data) {
		return nil, fmt.Errorf("insufficient data for sorted index count")
	}

	indexCount := *(*uint32)(unsafe.Pointer(&data[offset]))
	offset += 4

	if offset+int(indexCount)*4 > len(data) {
		return nil, fmt.Errorf("insufficient data for sorted index entries")
	}

	sortedIndex := make([]uint32, indexCount)
	for i := uint32(0); i < indexCount; i++ {
		sortedIndex[i] = *(*uint32)(unsafe.Pointer(&data[offset]))
		offset += 4
	}

	return sortedIndex, nil
}

// IsValidCacheFile checks if a file is a valid cache file without fully loading it.
func IsValidCacheFile(filename string) bool {
	file, err := os.Open(filename)
	if err != nil {
		return false
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("Warning: failed to close cache validation file: %v", closeErr)
		}
	}()

	var header BinaryHeader
	err = binary.Read(file, binary.LittleEndian, &header)
	if err != nil {
		return false
	}

	return header.Magic == CacheMagic && header.Version == CacheVersion
}
