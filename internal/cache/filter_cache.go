package cache

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/bnema/dumber/internal/logging"
)

// FilterData represents the compiled filter data for caching
type FilterData struct {
	Version       uint32
	Created       time.Time
	NetworkRules  []NetworkRule
	CosmeticRules []CosmeticRule
}

// NetworkRule represents a compiled network filtering rule
type NetworkRule struct {
	Pattern      string
	Domain       string
	Action       uint8  // 0=block, 1=allow
	ResourceType uint32 // Bitmask of resource types
	Priority     uint8
}

// CosmeticRule represents a compiled cosmetic filtering rule
type CosmeticRule struct {
	Domain   string
	Selector string
}

const (
	// Cache format version for compatibility
	cacheFormatVersion = uint32(1)

	// Magic bytes to identify cache files
	cacheMagic = uint64(0x44554D4245524301) // "DUMBERC" + version

	// File permissions
	cacheFilePerms = 0644
)

// FilterCache provides high-performance binary filter caching with memory mapping
type FilterCache struct {
	path     string
	mmapData []byte
	size     int64
}

// CacheHeader represents the binary cache file header
type CacheHeader struct {
	Magic     uint64 // Magic bytes for file identification
	Version   uint32 // Cache format version
	DataSize  uint64 // Size of data section
	RuleCount uint32 // Number of filter rules
	Created   int64  // Unix timestamp when cache was created
	Checksum  uint64 // CRC64 checksum of data section
}

// FilterRule represents a compiled filter rule in binary format
type FilterRule struct {
	Type         uint8  // Rule type (0=network, 1=cosmetic)
	Priority     uint8  // Rule priority
	Action       uint8  // Action to take (0=block, 1=allow, etc.)
	Flags        uint8  // Additional flags
	PatternLen   uint16 // Length of pattern string
	DomainLen    uint16 // Length of domain string
	ResourceType uint32 // Bitmask of resource types
	// Followed by: Pattern (variable length), Domain (variable length)
}

// NewFilterCache creates a new filter cache instance
func NewFilterCache(path string) *FilterCache {
	return &FilterCache{
		path: path,
	}
}

// LoadMapped loads the cache file using memory mapping for zero-copy access
func (fc *FilterCache) LoadMapped() (*FilterData, error) {
	file, err := os.Open(fc.path)
	if err != nil {
		return nil, fmt.Errorf("failed to open cache file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			logging.Warn(fmt.Sprintf("failed to close cache file: %v", err))
		}
	}()

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat cache file: %w", err)
	}

	fc.size = stat.Size()
	if fc.size < int64(unsafe.Sizeof(CacheHeader{})) {
		return nil, fmt.Errorf("cache file too small")
	}

	// Memory map the file for read-only access
	fc.mmapData, err = syscall.Mmap(int(file.Fd()), 0, int(fc.size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap cache file: %w", err)
	}

	// Parse the header
	header, err := fc.parseHeader()
	if err != nil {
		if unmapErr := fc.Unmap(); unmapErr != nil {
			logging.Warn(fmt.Sprintf("[filter-cache] failed to unmap after header parse error: %v", unmapErr))
		}
		return nil, fmt.Errorf("invalid cache header: %w", err)
	}

	// Validate magic and version
	if header.Magic != cacheMagic {
		if unmapErr := fc.Unmap(); unmapErr != nil {
			logging.Warn(fmt.Sprintf("[filter-cache] failed to unmap after magic validation: %v", unmapErr))
		}
		return nil, fmt.Errorf("invalid cache magic bytes")
	}

	if header.Version != cacheFormatVersion {
		if unmapErr := fc.Unmap(); unmapErr != nil {
			logging.Warn(fmt.Sprintf("[filter-cache] failed to unmap after version validation: %v", unmapErr))
		}
		return nil, fmt.Errorf("unsupported cache version: %d", header.Version)
	}

	// Parse filter data without copying
	data, err := fc.parseInPlace(header)
	if err != nil {
		if unmapErr := fc.Unmap(); unmapErr != nil {
			logging.Warn(fmt.Sprintf("failed to unmap during error handling: %v", unmapErr))
		}
		return nil, fmt.Errorf("failed to parse cache data: %w", err)
	}

	logging.Info(fmt.Sprintf("Loaded binary cache: %d rules, %d bytes (memory-mapped)",
		header.RuleCount, fc.size))

	return data, nil
}

// parseHeader extracts the cache header from memory-mapped data
func (fc *FilterCache) parseHeader() (*CacheHeader, error) {
	if len(fc.mmapData) < int(unsafe.Sizeof(CacheHeader{})) {
		return nil, fmt.Errorf("insufficient data for header")
	}

	header := (*CacheHeader)(unsafe.Pointer(&fc.mmapData[0]))
	return header, nil
}

// parseInPlace parses filter data directly from memory-mapped data without copying
func (fc *FilterCache) parseInPlace(header *CacheHeader) (*FilterData, error) {
	data := &FilterData{
		Version:       header.Version,
		NetworkRules:  make([]NetworkRule, 0, header.RuleCount),
		CosmeticRules: make([]CosmeticRule, 0),
	}

	// Start parsing after the header
	offset := int(unsafe.Sizeof(CacheHeader{}))
	remaining := header.RuleCount

	for remaining > 0 && offset < len(fc.mmapData) {
		// Parse rule header
		if offset+int(unsafe.Sizeof(FilterRule{})) > len(fc.mmapData) {
			break
		}

		ruleHeader := (*FilterRule)(unsafe.Pointer(&fc.mmapData[offset]))
		offset += int(unsafe.Sizeof(FilterRule{}))

		// Extract pattern string (zero-copy)
		if offset+int(ruleHeader.PatternLen) > len(fc.mmapData) {
			break
		}
		pattern := string(fc.mmapData[offset : offset+int(ruleHeader.PatternLen)])
		offset += int(ruleHeader.PatternLen)

		// Extract domain string (zero-copy)
		if offset+int(ruleHeader.DomainLen) > len(fc.mmapData) {
			break
		}
		domain := string(fc.mmapData[offset : offset+int(ruleHeader.DomainLen)])
		offset += int(ruleHeader.DomainLen)

		// Create appropriate rule type
		switch ruleHeader.Type {
		case 0: // Network rule
			data.NetworkRules = append(data.NetworkRules, NetworkRule{
				Pattern:      pattern,
				Domain:       domain,
				Action:       ruleHeader.Action,
				ResourceType: ruleHeader.ResourceType,
				Priority:     ruleHeader.Priority,
			})
		case 1: // Cosmetic rule
			data.CosmeticRules = append(data.CosmeticRules, CosmeticRule{
				Domain:   domain,
				Selector: pattern,
			})
		}

		remaining--
	}

	return data, nil
}

// Write saves filter data to cache in binary format
func (fc *FilterCache) Write(data *FilterData) error {
	// Create temporary file for atomic write
	tmpPath := fc.path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, cacheFilePerms)
	if err != nil {
		return fmt.Errorf("failed to create cache file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			logging.Warn(fmt.Sprintf("failed to close cache file: %v", err))
		}
	}()

	// Calculate total rule count
	totalRules := uint32(len(data.NetworkRules) + len(data.CosmeticRules))

	// Write header
	header := CacheHeader{
		Magic:     cacheMagic,
		Version:   data.Version,
		RuleCount: totalRules,
		Created:   data.Created.Unix(),
		DataSize:  0, // Will be calculated
		Checksum:  0, // Will be calculated
	}

	if err := binary.Write(file, binary.LittleEndian, header); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			logging.Warn(fmt.Sprintf("failed to remove temp cache file: %v", removeErr))
		}
		return fmt.Errorf("failed to write header: %w", err)
	}

	dataStart, _ := file.Seek(0, io.SeekCurrent)

	// Write network rules
	for _, rule := range data.NetworkRules {
		if err := fc.writeNetworkRule(file, rule); err != nil {
			if removeErr := os.Remove(tmpPath); removeErr != nil {
				logging.Warn(fmt.Sprintf("failed to remove temp cache file: %v", removeErr))
			}
			return fmt.Errorf("failed to write network rule: %w", err)
		}
	}

	// Write cosmetic rules
	for _, rule := range data.CosmeticRules {
		if err := fc.writeCosmeticRule(file, rule); err != nil {
			if removeErr := os.Remove(tmpPath); removeErr != nil {
				logging.Warn(fmt.Sprintf("failed to remove temp cache file: %v", removeErr))
			}
			return fmt.Errorf("failed to write cosmetic rule: %w", err)
		}
	}

	// Calculate data size
	dataEnd, _ := file.Seek(0, io.SeekCurrent)
	dataSize := uint64(dataEnd - dataStart)

	// Update header with data size
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			logging.Warn(fmt.Sprintf("failed to remove temp cache file: %v", removeErr))
		}
		return err
	}

	header.DataSize = dataSize
	// TODO: Calculate and set checksum

	if err := binary.Write(file, binary.LittleEndian, header); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			logging.Warn(fmt.Sprintf("failed to remove temp cache file: %v", removeErr))
		}
		return fmt.Errorf("failed to update header: %w", err)
	}

	if err := file.Sync(); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			logging.Warn(fmt.Sprintf("failed to remove temp cache file: %v", removeErr))
		}
		return fmt.Errorf("failed to sync cache file: %w", err)
	}

	if err := file.Close(); err != nil {
		logging.Warn(fmt.Sprintf("failed to close cache file: %v", err))
	}

	// Atomic rename
	if err := os.Rename(tmpPath, fc.path); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			logging.Warn(fmt.Sprintf("failed to remove temp cache file: %v", removeErr))
		}
		return fmt.Errorf("failed to rename cache file: %w", err)
	}

	logging.Info(fmt.Sprintf("Wrote binary cache: %d rules, %d bytes", totalRules, dataSize))
	return nil
}

// writeNetworkRule writes a network rule in binary format
func (fc *FilterCache) writeNetworkRule(file *os.File, rule NetworkRule) error {
	patternBytes := []byte(rule.Pattern)
	domainBytes := []byte(rule.Domain)

	ruleHeader := FilterRule{
		Type:         0, // Network rule
		Priority:     rule.Priority,
		Action:       rule.Action,
		Flags:        0,
		PatternLen:   uint16(len(patternBytes)),
		DomainLen:    uint16(len(domainBytes)),
		ResourceType: rule.ResourceType,
	}

	// Write rule header
	if err := binary.Write(file, binary.LittleEndian, ruleHeader); err != nil {
		return err
	}

	// Write pattern
	if _, err := file.Write(patternBytes); err != nil {
		return err
	}

	// Write domain
	if _, err := file.Write(domainBytes); err != nil {
		return err
	}

	return nil
}

// writeCosmeticRule writes a cosmetic rule in binary format
func (fc *FilterCache) writeCosmeticRule(file *os.File, rule CosmeticRule) error {
	selectorBytes := []byte(rule.Selector)
	domainBytes := []byte(rule.Domain)

	ruleHeader := FilterRule{
		Type:         1, // Cosmetic rule
		Priority:     0,
		Action:       0, // Hide
		Flags:        0,
		PatternLen:   uint16(len(selectorBytes)),
		DomainLen:    uint16(len(domainBytes)),
		ResourceType: 0,
	}

	// Write rule header
	if err := binary.Write(file, binary.LittleEndian, ruleHeader); err != nil {
		return err
	}

	// Write selector
	if _, err := file.Write(selectorBytes); err != nil {
		return err
	}

	// Write domain
	if _, err := file.Write(domainBytes); err != nil {
		return err
	}

	return nil
}

// Unmap releases the memory-mapped data
func (fc *FilterCache) Unmap() error {
	if fc.mmapData != nil {
		if err := syscall.Munmap(fc.mmapData); err != nil {
			return fmt.Errorf("failed to unmap cache data: %w", err)
		}
		fc.mmapData = nil
	}
	return nil
}

// Size returns the size of the cache file in bytes
func (fc *FilterCache) Size() int64 {
	return fc.size
}

// IsValid checks if the cache file exists and has valid format
func (fc *FilterCache) IsValid() bool {
	file, err := os.Open(fc.path)
	if err != nil {
		return false
	}
	defer func() {
		if err := file.Close(); err != nil {
			logging.Warn(fmt.Sprintf("failed to close file during validation: %v", err))
		}
	}()

	var magic uint64
	if err := binary.Read(file, binary.LittleEndian, &magic); err != nil {
		return false
	}

	return magic == cacheMagic
}

// Remove deletes the cache file
func (fc *FilterCache) Remove() error {
	if err := fc.Unmap(); err != nil {
		logging.Warn(fmt.Sprintf("failed to unmap file during removal: %v", err))
	}
	return os.Remove(fc.path)
}
