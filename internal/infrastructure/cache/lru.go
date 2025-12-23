// Package cache provides cache implementations for the application layer.
package cache

import (
	"container/list"
	"sync"
)

// LRU is a thread-safe LRU (Least Recently Used) cache with a fixed capacity.
// It implements port.Cache[K, V].
//
// When the cache reaches capacity, the least recently accessed entry is evicted
// to make room for new entries. Both Get and Set operations mark an entry as
// recently used.
type LRU[K comparable, V any] struct {
	capacity int
	mu       sync.RWMutex
	items    map[K]*list.Element
	order    *list.List // Front = most recent, Back = least recent
}

// entry holds a key-value pair in the LRU cache.
type entry[K comparable, V any] struct {
	key   K
	value V
}

// NewLRU creates a new LRU cache with the given capacity.
// Capacity must be positive; if zero or negative, a capacity of 1 is used.
func NewLRU[K comparable, V any](capacity int) *LRU[K, V] {
	if capacity <= 0 {
		capacity = 1
	}
	return &LRU[K, V]{
		capacity: capacity,
		items:    make(map[K]*list.Element),
		order:    list.New(),
	}
}

// Get retrieves a value by key and marks it as recently used.
// Returns the value and true if found, or the zero value and false if not found.
func (c *LRU[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		return elem.Value.(*entry[K, V]).value, true
	}
	var zero V
	return zero, false
}

// Set adds or updates a value in the cache.
// If the key already exists, its value is updated and it's marked as recently used.
// If the cache is at capacity, the least recently used entry is evicted.
func (c *LRU[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry
	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		elem.Value.(*entry[K, V]).value = value
		return
	}

	// Evict LRU entry if at capacity
	if c.order.Len() >= c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*entry[K, V]).key)
		}
	}

	// Add new entry at front (most recent)
	elem := c.order.PushFront(&entry[K, V]{key: key, value: value})
	c.items[key] = elem
}

// Remove deletes a key from the cache.
// If the key doesn't exist, this is a no-op.
func (c *LRU[K, V]) Remove(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.order.Remove(elem)
		delete(c.items, key)
	}
}

// Len returns the number of items currently in the cache.
func (c *LRU[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.order.Len()
}

// Clear removes all items from the cache.
func (c *LRU[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[K]*list.Element)
	c.order.Init()
}
