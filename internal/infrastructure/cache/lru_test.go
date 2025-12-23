package cache

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLRU_BasicOperations(t *testing.T) {
	cache := NewLRU[string, int](3)

	// Test Set and Get
	cache.Set("a", 1)
	cache.Set("b", 2)
	cache.Set("c", 3)

	val, ok := cache.Get("a")
	assert.True(t, ok)
	assert.Equal(t, 1, val)

	val, ok = cache.Get("b")
	assert.True(t, ok)
	assert.Equal(t, 2, val)

	// Test not found
	val, ok = cache.Get("notfound")
	assert.False(t, ok)
	assert.Equal(t, 0, val)

	assert.Equal(t, 3, cache.Len())
}

func TestLRU_Eviction(t *testing.T) {
	cache := NewLRU[string, int](2)

	cache.Set("a", 1)
	cache.Set("b", 2)
	// Cache is now at capacity: [b, a] (b is most recent)

	// Adding "c" should evict "a" (least recently used)
	cache.Set("c", 3)

	_, ok := cache.Get("a")
	assert.False(t, ok, "a should have been evicted")

	val, ok := cache.Get("b")
	assert.True(t, ok)
	assert.Equal(t, 2, val)

	val, ok = cache.Get("c")
	assert.True(t, ok)
	assert.Equal(t, 3, val)
}

func TestLRU_GetUpdatesRecency(t *testing.T) {
	cache := NewLRU[string, int](2)

	cache.Set("a", 1)
	cache.Set("b", 2)
	// Order: [b, a]

	// Access "a" to make it most recent
	cache.Get("a")
	// Order: [a, b]

	// Adding "c" should now evict "b" (least recently used)
	cache.Set("c", 3)

	val, ok := cache.Get("a")
	assert.True(t, ok, "a should still exist")
	assert.Equal(t, 1, val)

	_, ok = cache.Get("b")
	assert.False(t, ok, "b should have been evicted")
}

func TestLRU_UpdateExisting(t *testing.T) {
	cache := NewLRU[string, int](2)

	cache.Set("a", 1)
	cache.Set("b", 2)

	// Update "a" with new value
	cache.Set("a", 100)

	val, ok := cache.Get("a")
	assert.True(t, ok)
	assert.Equal(t, 100, val)

	// Cache should still have 2 items (no duplicate)
	assert.Equal(t, 2, cache.Len())
}

func TestLRU_Remove(t *testing.T) {
	cache := NewLRU[string, int](3)

	cache.Set("a", 1)
	cache.Set("b", 2)
	cache.Set("c", 3)

	cache.Remove("b")

	_, ok := cache.Get("b")
	assert.False(t, ok)
	assert.Equal(t, 2, cache.Len())

	// Remove non-existent key should not panic
	cache.Remove("notfound")
	assert.Equal(t, 2, cache.Len())
}

func TestLRU_Clear(t *testing.T) {
	cache := NewLRU[string, int](3)

	cache.Set("a", 1)
	cache.Set("b", 2)
	cache.Set("c", 3)

	cache.Clear()

	assert.Equal(t, 0, cache.Len())
	_, ok := cache.Get("a")
	assert.False(t, ok)
}

func TestLRU_ZeroCapacity(t *testing.T) {
	// Should default to capacity of 1
	cache := NewLRU[string, int](0)

	cache.Set("a", 1)
	val, ok := cache.Get("a")
	assert.True(t, ok)
	assert.Equal(t, 1, val)

	// Adding another should evict first
	cache.Set("b", 2)
	_, ok = cache.Get("a")
	assert.False(t, ok)
}

func TestLRU_ConcurrentAccess(t *testing.T) {
	cache := NewLRU[int, int](100)
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cache.Set(i, i*10)
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cache.Get(i)
		}(i)
	}
	wg.Wait()

	// Concurrent mixed operations
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			cache.Set(i+100, i)
		}(i)
		go func(i int) {
			defer wg.Done()
			cache.Get(i)
		}(i)
		go func(i int) {
			defer wg.Done()
			cache.Remove(i + 50)
		}(i)
	}
	wg.Wait()

	// Should not panic and cache should be in valid state
	require.LessOrEqual(t, cache.Len(), 100)
}

func TestLRU_PointerValues(t *testing.T) {
	type Data struct {
		Name  string
		Value float64
	}

	cache := NewLRU[string, *Data](2)

	cache.Set("key1", &Data{Name: "test1", Value: 1.5})
	cache.Set("key2", &Data{Name: "test2", Value: 2.5})

	val, ok := cache.Get("key1")
	require.True(t, ok)
	assert.Equal(t, "test1", val.Name)
	assert.InDelta(t, 1.5, val.Value, 0.001)

	// Update pointer value
	cache.Set("key1", &Data{Name: "updated", Value: 99.9})

	val, ok = cache.Get("key1")
	require.True(t, ok)
	assert.Equal(t, "updated", val.Name)
	assert.InDelta(t, 99.9, val.Value, 0.001)
}
