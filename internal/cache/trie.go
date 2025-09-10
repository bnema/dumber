package cache

import (
	"sort"
	"strings"
	"unicode"
)

// NewTrie creates a new empty trie.
func NewTrie() *Trie {
	return &Trie{
		Root: &TrieNode{
			Children: make(map[rune]*TrieNode),
			Entries:  make([]uint32, 0),
		},
	}
}

// Insert adds an entry to the trie for all its prefixes.
func (t *Trie) Insert(text string, entryID uint32) {
	text = normalizeText(text)

	// Insert all meaningful prefixes
	words := strings.Fields(text)
	for _, word := range words {
		if len(word) < 2 { // Skip single characters
			continue
		}
		t.insertWord(word, entryID)

		// Also insert domain parts for URLs
		if strings.Contains(word, ".") {
			parts := strings.Split(word, ".")
			for _, part := range parts {
				if len(part) >= 2 {
					t.insertWord(part, entryID)
				}
			}
		}
	}
}

// insertWord inserts a single word and its prefixes into the trie.
func (t *Trie) insertWord(word string, entryID uint32) {
	node := t.Root

	// Insert the full word
	for _, char := range word {
		if node.Children[char] == nil {
			node.Children[char] = &TrieNode{
				Children: make(map[rune]*TrieNode),
				Entries:  make([]uint32, 0),
			}
		}
		node = node.Children[char]
	}

	// Add the entry ID to all nodes in the path
	current := t.Root
	for _, char := range word {
		current = current.Children[char]
		if !containsUint32(current.Entries, entryID) {
			current.Entries = append(current.Entries, entryID)
		}
	}
	node.IsEnd = true
}

// Search finds all entries that match the given prefix.
func (t *Trie) Search(prefix string) []uint32 {
	if len(prefix) == 0 {
		return nil
	}

	prefix = normalizeText(prefix)
	node := t.Root

	// Navigate to the prefix node
	for _, char := range prefix {
		if node.Children[char] == nil {
			return nil // Prefix not found
		}
		node = node.Children[char]
	}

	// Return all entries at this node
	result := make([]uint32, len(node.Entries))
	copy(result, node.Entries)
	return result
}

// PrefixSearch finds all entries that start with any word in the query.
func (t *Trie) PrefixSearch(query string) []uint32 {
	query = normalizeText(query)
	words := strings.Fields(query)

	entrySet := make(map[uint32]struct{})

	for _, word := range words {
		if len(word) < 2 {
			continue
		}

		entries := t.Search(word)
		for _, entryID := range entries {
			entrySet[entryID] = struct{}{}
		}
	}

	// Convert set back to slice
	result := make([]uint32, 0, len(entrySet))
	for entryID := range entrySet {
		result = append(result, entryID)
	}

	return result
}

// buildTrigramIndex creates a trigram index from the cache entries.
func (c *DmenuFuzzyCache) buildTrigramIndex() {
	c.trigramIndex = make(map[string][]uint32)

	for i, entry := range c.entries {
		entryID := uint32(i)

		// Build trigrams from URL and title
		text := normalizeText(entry.URL + " " + entry.Title)
		trigrams := extractTrigrams(text)

		for _, trigram := range trigrams {
			if !containsUint32(c.trigramIndex[trigram], entryID) {
				c.trigramIndex[trigram] = append(c.trigramIndex[trigram], entryID)
			}
		}
	}
}

// buildPrefixTrie creates a prefix trie from the cache entries.
func (c *DmenuFuzzyCache) buildPrefixTrie() {
	c.prefixTrie = NewTrie()

	for i, entry := range c.entries {
		entryID := uint32(i)

		// Add URL and title to trie
		c.prefixTrie.Insert(entry.URL, entryID)
		if entry.Title != "" {
			c.prefixTrie.Insert(entry.Title, entryID)
		}
	}
}

// extractTrigrams extracts all 3-character sequences from text.
func extractTrigrams(text string) []string {
	if len(text) < 3 {
		return nil
	}

	trigrams := make([]string, 0, len(text)-2)
	runes := []rune(text)

	for i := 0; i <= len(runes)-3; i++ {
		trigram := string(runes[i : i+3])
		trigrams = append(trigrams, trigram)
	}

	return trigrams
}

// normalizeText normalizes text for consistent matching.
func normalizeText(text string) string {
	// Convert to lowercase and remove extra whitespace
	text = strings.ToLower(text)

	// Remove protocol prefixes for URLs
	text = strings.TrimPrefix(text, "https://")
	text = strings.TrimPrefix(text, "http://")
	text = strings.TrimPrefix(text, "www.")

	// Replace non-alphanumeric characters with spaces
	var result strings.Builder
	result.Grow(len(text))

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result.WriteRune(r)
		} else {
			result.WriteRune(' ')
		}
	}

	// Normalize whitespace
	return strings.Join(strings.Fields(result.String()), " ")
}

// containsUint32 checks if a slice contains a uint32 value.
func containsUint32(slice []uint32, value uint32) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

// intersectUint32 finds the intersection of two uint32 slices.
func intersectUint32(a, b []uint32) []uint32 {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}

	// Use the smaller slice for the map to save memory
	if len(a) > len(b) {
		a, b = b, a
	}

	// Create a set from the smaller slice
	set := make(map[uint32]struct{}, len(a))
	for _, v := range a {
		set[v] = struct{}{}
	}

	// Find intersection
	result := make([]uint32, 0, len(a))
	for _, v := range b {
		if _, exists := set[v]; exists {
			result = append(result, v)
			delete(set, v) // Avoid duplicates
		}
	}

	return result
}

// unionUint32 finds the union of multiple uint32 slices.
func unionUint32(slices ...[]uint32) []uint32 {
	if len(slices) == 0 {
		return nil
	}

	// Estimate capacity
	capacity := 0
	for _, slice := range slices {
		capacity += len(slice)
	}

	set := make(map[uint32]struct{}, capacity)
	for _, slice := range slices {
		for _, v := range slice {
			set[v] = struct{}{}
		}
	}

	result := make([]uint32, 0, len(set))
	for v := range set {
		result = append(result, v)
	}

	return result
}

// getTrigramCandidates finds entry candidates using trigram matching.
func (c *DmenuFuzzyCache) getTrigramCandidates(query string) []uint32 {
	query = normalizeText(query)
	trigrams := extractTrigrams(query)

	if len(trigrams) == 0 {
		return nil
	}

	// Get candidates from first trigram
	candidates := c.trigramIndex[trigrams[0]]
	if len(candidates) == 0 {
		return nil
	}

	// Intersect with other trigrams for more accuracy
	for i := 1; i < len(trigrams) && len(candidates) > 0; i++ {
		nextCandidates := c.trigramIndex[trigrams[i]]
		if len(nextCandidates) == 0 {
			continue // Skip empty trigrams
		}
		candidates = intersectUint32(candidates, nextCandidates)
	}

	return candidates
}

// getPrefixCandidates finds entry candidates using prefix matching.
func (c *DmenuFuzzyCache) getPrefixCandidates(query string) []uint32 {
	if c.prefixTrie == nil {
		return nil
	}

	return c.prefixTrie.PrefixSearch(query)
}

// getAllCandidates combines trigram and prefix matching for comprehensive candidate selection.
func (c *DmenuFuzzyCache) getAllCandidates(query string) []uint32 {
	trigramCandidates := c.getTrigramCandidates(query)
	prefixCandidates := c.getPrefixCandidates(query)

	if len(trigramCandidates) == 0 && len(prefixCandidates) == 0 {
		return nil
	}

	if len(trigramCandidates) == 0 {
		return prefixCandidates
	}

	if len(prefixCandidates) == 0 {
		return trigramCandidates
	}

	// Combine both candidate sets
	return unionUint32(trigramCandidates, prefixCandidates)
}

// buildSortedIndex creates a pre-sorted index of entries by score for O(1) top entries retrieval.
func (c *DmenuFuzzyCache) buildSortedIndex() {
	// Create index array
	c.sortedIndex = make([]uint32, len(c.entries))
	for i := range c.sortedIndex {
		c.sortedIndex[i] = uint32(i)
	}

	// Sort indices by entry scores (highest first)
	sort.Slice(c.sortedIndex, func(i, j int) bool {
		return c.entries[c.sortedIndex[i]].Score > c.entries[c.sortedIndex[j]].Score
	})
}
