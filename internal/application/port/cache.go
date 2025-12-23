package port

// Cache is a generic cache interface for storing key-value pairs.
// Implementations should be thread-safe.
type Cache[K comparable, V any] interface {
	// Get retrieves a value by key. Returns the value and true if found,
	// or the zero value and false if not found.
	Get(key K) (V, bool)

	// Set stores a value for the given key. If the cache is at capacity,
	// the least recently used entry may be evicted.
	Set(key K, value V)

	// Remove deletes a key from the cache.
	Remove(key K)

	// Len returns the number of items currently in the cache.
	Len() int
}
