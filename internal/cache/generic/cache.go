package generic

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/logging"
)

// Cache provides a generic interface for caching any type of data.
// K is the key type (must be comparable), V is the value type.
// All operations are thread-safe via sync.Map.
//
// Design principles:
// - RAM-first: All reads from memory (no DB queries)
// - Async writes: Updates happen immediately in cache, persisted asynchronously
// - Bulk load: Load all data from DB at startup
// - Flush on shutdown: Ensure all pending writes complete gracefully
type Cache[K comparable, V any] interface {
	// Load bulk-loads all data from storage into memory at startup
	Load(ctx context.Context) error

	// Get retrieves a value from the cache (RAM only, never queries DB)
	Get(key K) (V, bool)

	// Set updates the cache immediately and persists to DB asynchronously
	Set(key K, value V) error

	// Delete removes from cache immediately and persists deletion asynchronously
	Delete(key K) error

	// List returns all cached values as a slice
	List() []V

	// Flush waits for all pending async writes to complete (call on shutdown)
	Flush() error
}

// DatabaseOperations defines the interface for database operations required by the cache.
// This interface allows for easy mocking in tests using gomock.
type DatabaseOperations[K comparable, V any] interface {
	// LoadAll loads all entries from the database
	LoadAll(ctx context.Context) (map[K]V, error)

	// Persist saves a single entry to the database
	Persist(ctx context.Context, key K, value V) error

	// Delete removes a single entry from the database
	Delete(ctx context.Context, key K) error
}

// GenericCache implements Cache[K, V] using sync.Map for thread-safe storage
// and a DatabaseOperations interface for database operations.
type GenericCache[K comparable, V any] struct {
	cache sync.Map
	dbOps DatabaseOperations[K, V]

	// Track pending async operations for graceful shutdown
	pendingWrites sync.WaitGroup
}

// NewGenericCache creates a new cache with the provided database operations.
//
// Parameters:
//   - dbOps: Implementation of DatabaseOperations for loading, persisting, and deleting data
func NewGenericCache[K comparable, V any](
	dbOps DatabaseOperations[K, V],
) *GenericCache[K, V] {
	return &GenericCache[K, V]{
		dbOps: dbOps,
	}
}

// Load bulk-loads all data from the database into memory.
// Should be called once at application startup.
func (c *GenericCache[K, V]) Load(ctx context.Context) error {
	data, err := c.dbOps.LoadAll(ctx)
	if err != nil {
		return err
	}

	for k, v := range data {
		c.cache.Store(k, v)
	}

	return nil
}

// Get retrieves a value from the cache. Returns (value, true) if found,
// or (zero value, false) if not found. Never queries the database.
func (c *GenericCache[K, V]) Get(key K) (V, bool) {
	val, ok := c.cache.Load(key)
	if !ok {
		var zero V
		return zero, false
	}
	return val.(V), true
}

// Set updates the cache immediately (synchronous) and persists to the database
// asynchronously (non-blocking). Returns immediately without waiting for DB write.
func (c *GenericCache[K, V]) Set(key K, value V) error {
	// Update cache immediately (sync, fast)
	c.cache.Store(key, value)

	// Persist to DB asynchronously (non-blocking)
	c.pendingWrites.Add(1)
	go func() {
		defer c.pendingWrites.Done()

		ctx := context.Background()
		if err := c.dbOps.Persist(ctx, key, value); err != nil {
			logging.Warn(fmt.Sprintf("Warning: async persist failed for key %v: %v", key, err))
		}
	}()

	return nil
}

// Delete removes from cache immediately (synchronous) and persists deletion
// to the database asynchronously (non-blocking).
func (c *GenericCache[K, V]) Delete(key K) error {
	// Remove from cache immediately (sync, fast)
	c.cache.Delete(key)

	// Persist deletion to DB asynchronously (non-blocking)
	c.pendingWrites.Add(1)
	go func() {
		defer c.pendingWrites.Done()

		ctx := context.Background()
		if err := c.dbOps.Delete(ctx, key); err != nil {
			logging.Warn(fmt.Sprintf("Warning: async delete failed for key %v: %v", key, err))
		}
	}()

	return nil
}

// List returns all values currently in the cache as a slice.
// The order is not guaranteed.
func (c *GenericCache[K, V]) List() []V {
	var values []V

	c.cache.Range(func(key, value interface{}) bool {
		values = append(values, value.(V))
		return true
	})

	return values
}

// Flush blocks until all pending async writes complete.
// Should be called during graceful shutdown to ensure no data loss.
func (c *GenericCache[K, V]) Flush() error {
	c.pendingWrites.Wait()
	return nil
}
