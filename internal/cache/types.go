package cache

import (
	"sync"
	"time"
	"unsafe"

	"github.com/bnema/dumber/internal/config"
)

// DmenuFuzzyCache provides high-performance fuzzy search for dmenu history.
type DmenuFuzzyCache struct {
	// Pre-computed search indices
	trigramIndex map[string][]uint32 // 3-char sequences -> entry IDs
	prefixTrie   *Trie               // Compressed trie for prefix matching
	sortedIndex  []uint32            // Pre-sorted entry indices by score (highest first)

	// Compact history storage
	entries []CompactEntry // Minimal memory footprint

	// Metadata
	version      uint32 // Cache format version
	lastModified int64  // Unix timestamp
	entryCount   uint32 // Number of entries

	// Concurrency control
	mu   sync.RWMutex
	data unsafe.Pointer // Atomic pointer to cache data
}

// CompactEntry represents a history entry optimized for memory usage.
type CompactEntry struct {
	URL        string // Original URL
	Title      string // Page title
	VisitCount uint16 // Max 65535 visits (sufficient for most use cases)
	LastVisit  uint32 // Days since Unix epoch (saves 4 bytes vs time.Time)
	Score      uint16 // Pre-computed base score (0-65535)
}

// FuzzyResult contains the results of a fuzzy search query.
type FuzzyResult struct {
	Matches   []FuzzyMatch  // Matching entries
	QueryTime time.Duration // Time taken to execute query
}

// FuzzyMatch represents a single fuzzy search match.
type FuzzyMatch struct {
	Entry        *CompactEntry // Matched entry
	Score        float64       // Final fuzzy match score (0.0-1.0)
	URLScore     float64       // URL similarity score
	TitleScore   float64       // Title similarity score
	RecencyScore float64       // Recency boost score
	VisitScore   float64       // Visit count boost score
	MatchType    MatchType     // Type of match found
}

// MatchType indicates how the query matched the entry.
type MatchType uint8

const (
	MatchTypeExact   MatchType = iota // Exact substring match
	MatchTypePrefix                   // Prefix match
	MatchTypeFuzzy                    // Fuzzy similarity match
	MatchTypeTrigram                  // Trigram-based match
)

// CacheConfig holds configuration for the fuzzy cache.
type CacheConfig struct {
	// File paths
	CacheFile string // Path to binary cache file

	// Performance tuning
	MaxEntries       uint32  // Maximum entries to cache
	TrigramThreshold uint32  // Minimum trigram matches required
	ScoreThreshold   float64 // Minimum score for results
	MaxResults       int     // Maximum results to return

	// Scoring weights
	URLWeight     float64 // Weight for URL matches
	TitleWeight   float64 // Weight for title matches
	RecencyWeight float64 // Weight for recency
	VisitWeight   float64 // Weight for visit count

	// Cache behavior
	TTL           time.Duration // Cache time-to-live
	WarmupEnabled bool          // Enable background cache warming
}

// DefaultCacheConfig returns sensible default configuration.
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		MaxEntries:       10000,
		TrigramThreshold: 2,
		ScoreThreshold:   0.2,
		MaxResults:       50,
		URLWeight:        0.4,
		TitleWeight:      0.3,
		RecencyWeight:    0.2,
		VisitWeight:      0.1,
		TTL:              30 * time.Minute,
		WarmupEnabled:    true,
	}
}

// GetStateDir returns the state directory using the config package.
func (c *CacheConfig) GetStateDir() (string, error) {
	return config.GetStateDir()
}

// Trie represents a compressed prefix tree for fast prefix matching.
type Trie struct {
	Root *TrieNode
}

// TrieNode represents a node in the prefix trie.
type TrieNode struct {
	Children map[rune]*TrieNode // Child nodes
	Entries  []uint32           // Entry IDs that end at this node
	IsEnd    bool               // Marks end of a prefix
}

// Reset clears the FuzzyResult for reuse.
func (r *FuzzyResult) Reset() {
	r.Matches = r.Matches[:0]
	r.QueryTime = 0
}

// String returns a string representation of the match type.
func (mt MatchType) String() string {
	switch mt {
	case MatchTypeExact:
		return "exact"
	case MatchTypePrefix:
		return "prefix"
	case MatchTypeFuzzy:
		return "fuzzy"
	case MatchTypeTrigram:
		return "trigram"
	default:
		return "unknown"
	}
}

// DaysFromTime converts a time.Time to days since Unix epoch.
func DaysFromTime(t time.Time) uint32 {
	return uint32(t.Unix() / 86400) // 86400 seconds per day
}

// TimeFromDays converts days since Unix epoch back to time.Time.
func TimeFromDays(days uint32) time.Time {
	return time.Unix(int64(days)*86400, 0)
}

// NewCompactEntry creates a CompactEntry from database values.
func NewCompactEntry(url, title string, visitCount int64, lastVisit time.Time) CompactEntry {
	entry := CompactEntry{
		URL:   url,
		Title: title,
	}

	// Clamp visit count to uint16 max
	if visitCount > 65535 {
		entry.VisitCount = 65535
	} else if visitCount < 0 {
		entry.VisitCount = 0
	} else {
		entry.VisitCount = uint16(visitCount)
	}

	entry.LastVisit = DaysFromTime(lastVisit)

	// Pre-compute a base score for sorting
	entry.Score = entry.calculateBaseScore()

	return entry
}

// calculateBaseScore computes a base score for the entry based on visits and recency.
func (e *CompactEntry) calculateBaseScore() uint16 {
	visitScore := float64(e.VisitCount) / 65535.0 // Normalize to 0-1

	now := time.Now()
	lastVisit := TimeFromDays(e.LastVisit)
	daysSince := now.Sub(lastVisit).Hours() / 24
	recencyScore := 1.0 / (1.0 + daysSince/30.0) // Decay over 30 days

	// Weighted combination
	baseScore := (visitScore*0.3 + recencyScore*0.7) * 65535.0

	if baseScore > 65535 {
		return 65535
	}
	return uint16(baseScore)
}

// Display returns a formatted string for dmenu display.
func (e *CompactEntry) Display() string {
	if e.Title != "" && e.Title != e.URL {
		return e.Title
	}
	return e.URL
}
