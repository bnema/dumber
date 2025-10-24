package generic

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestLoad verifies that Load correctly populates the cache from the database
func TestLoad(t *testing.T) {
	mock := NewMockDatabaseOperations[string, int]()
	mock.LoadAllFunc = func(ctx context.Context) (map[string]int, error) {
		return map[string]int{
			"one":   1,
			"two":   2,
			"three": 3,
		}, nil
	}

	cache := NewGenericCache(mock)

	err := cache.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify LoadAll was called once
	if count := mock.GetLoadAllCallCount(); count != 1 {
		t.Errorf("Expected LoadAll to be called once, got %d", count)
	}

	// Verify all entries were loaded into cache
	if val, ok := cache.Get("one"); !ok || val != 1 {
		t.Errorf("Expected one=1, got %v, %v", val, ok)
	}
	if val, ok := cache.Get("two"); !ok || val != 2 {
		t.Errorf("Expected two=2, got %v, %v", val, ok)
	}
	if val, ok := cache.Get("three"); !ok || val != 3 {
		t.Errorf("Expected three=3, got %v, %v", val, ok)
	}
}

// TestLoadError verifies that Load propagates errors from the database
func TestLoadError(t *testing.T) {
	expectedErr := errors.New("database connection failed")

	mock := NewMockDatabaseOperations[string, int]()
	mock.LoadAllFunc = func(ctx context.Context) (map[string]int, error) {
		return nil, expectedErr
	}

	cache := NewGenericCache(mock)

	err := cache.Load(context.Background())
	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	// Verify LoadAll was called
	if count := mock.GetLoadAllCallCount(); count != 1 {
		t.Errorf("Expected LoadAll to be called once, got %d", count)
	}
}

// TestGetNotFound verifies that Get returns false for non-existent keys
func TestGetNotFound(t *testing.T) {
	mock := NewMockDatabaseOperations[string, int]()
	cache := NewGenericCache(mock)

	if err := cache.Load(context.Background()); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	val, ok := cache.Get("nonexistent")
	if ok {
		t.Errorf("Expected Get to return false for nonexistent key, got %v", val)
	}
	if val != 0 {
		t.Errorf("Expected zero value, got %v", val)
	}
}

// TestSet verifies that Set immediately updates the cache and persists asynchronously
func TestSet(t *testing.T) {
	mock := NewMockDatabaseOperations[string, int]()

	cache := NewGenericCache(mock)
	if err := cache.Load(context.Background()); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Set should return immediately
	err := cache.Set("test", 42)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// Value should be in cache immediately
	val, ok := cache.Get("test")
	if !ok || val != 42 {
		t.Errorf("Expected test=42 immediately after Set, got %v, %v", val, ok)
	}

	// Wait for async persist to complete
	if err := cache.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// Verify Persist was called
	if count := mock.GetPersistCallCount(); count != 1 {
		t.Errorf("Expected Persist to be called once, got %d", count)
	}

	// Verify correct parameters
	lastCall := mock.GetLastPersistCall()
	if lastCall == nil {
		t.Fatal("Expected Persist to be called")
	}
	if lastCall.Key != "test" || lastCall.Value != 42 {
		t.Errorf("Expected Persist(test, 42), got Persist(%v, %v)", lastCall.Key, lastCall.Value)
	}
}

// TestSetAsyncPersist verifies that Set doesn't block on slow database writes
func TestSetAsyncPersist(t *testing.T) {
	mock := NewMockDatabaseOperations[string, int]()
	slowPersist := make(chan struct{})

	mock.PersistFunc = func(ctx context.Context, key string, value int) error {
		<-slowPersist // Block until we signal
		return nil
	}

	cache := NewGenericCache(mock)
	if err := cache.Load(context.Background()); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Set should return immediately even though persist is blocked
	done := make(chan struct{})
	go func() {
		if err := cache.Set("test", 42); err != nil {
			t.Errorf("Set() failed: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
		// Good, Set returned immediately
	case <-time.After(100 * time.Millisecond):
		t.Error("Set() blocked for too long")
	}

	// Verify value is in cache even though persist hasn't completed
	val, ok := cache.Get("test")
	if !ok || val != 42 {
		t.Errorf("Expected test=42 in cache before persist completes, got %v, %v", val, ok)
	}

	// Now allow persist to complete
	close(slowPersist)
	if err := cache.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// Verify Persist was called
	if count := mock.GetPersistCallCount(); count != 1 {
		t.Errorf("Expected Persist to be called once, got %d", count)
	}
}

// TestDelete verifies that Delete immediately removes from cache and persists async
func TestDelete(t *testing.T) {
	mock := NewMockDatabaseOperations[string, int]()
	mock.LoadAllFunc = func(ctx context.Context) (map[string]int, error) {
		return map[string]int{"test": 42}, nil
	}

	cache := NewGenericCache(mock)
	if err := cache.Load(context.Background()); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify entry exists
	if _, ok := cache.Get("test"); !ok {
		t.Fatal("Expected test to exist before Delete")
	}

	// Delete should return immediately
	err := cache.Delete("test")
	if err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	// Value should be removed from cache immediately
	if val, ok := cache.Get("test"); ok {
		t.Errorf("Expected test to be deleted immediately, got %v", val)
	}

	// Wait for async delete to complete
	if err := cache.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// Verify Delete was called
	if count := mock.GetDeleteCallCount(); count != 1 {
		t.Errorf("Expected Delete to be called once, got %d", count)
	}

	// Verify correct parameters
	lastCall := mock.GetLastDeleteCall()
	if lastCall == nil {
		t.Fatal("Expected Delete to be called")
	}
	if lastCall.Key != "test" {
		t.Errorf("Expected Delete(test), got Delete(%v)", lastCall.Key)
	}
}

// TestList verifies that List returns all cached values
func TestList(t *testing.T) {
	mock := NewMockDatabaseOperations[string, int]()
	mock.LoadAllFunc = func(ctx context.Context) (map[string]int, error) {
		return map[string]int{
			"one":   1,
			"two":   2,
			"three": 3,
		}, nil
	}

	cache := NewGenericCache(mock)
	if err := cache.Load(context.Background()); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	values := cache.List()
	if len(values) != 3 {
		t.Errorf("Expected 3 values, got %d", len(values))
	}

	// Verify all values are present (order not guaranteed)
	found := make(map[int]bool)
	for _, v := range values {
		found[v] = true
	}

	for _, expected := range []int{1, 2, 3} {
		if !found[expected] {
			t.Errorf("Expected value %d in List(), not found", expected)
		}
	}
}

// TestFlush verifies that Flush waits for all pending operations
func TestFlush(t *testing.T) {
	mock := NewMockDatabaseOperations[string, int]()
	persistDelay := make(chan struct{})

	mock.PersistFunc = func(ctx context.Context, key string, value int) error {
		<-persistDelay // Wait for signal
		return nil
	}

	cache := NewGenericCache(mock)
	if err := cache.Load(context.Background()); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Start multiple async operations
	for i := 0; i < 5; i++ {
		if err := cache.Set("test", i); err != nil {
			t.Fatalf("Set() failed: %v", err)
		}
	}

	// Verify operations haven't completed yet
	if count := mock.GetPersistCallCount(); count != 0 {
		t.Logf("Note: %d persists completed before signal", count)
	}

	// Allow all operations to proceed
	close(persistDelay)

	// Flush should wait for all to complete
	if err := cache.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// All persists should have completed
	if count := mock.GetPersistCallCount(); count != 5 {
		t.Errorf("Expected 5 persists after Flush, got %d", count)
	}
}

// TestConcurrentAccess verifies thread-safety of cache operations
func TestConcurrentAccess(t *testing.T) {
	mock := NewMockDatabaseOperations[int, int]()

	cache := NewGenericCache(mock)
	if err := cache.Load(context.Background()); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := id*numOperations + j
				if err := cache.Set(key, key*2); err != nil {
					t.Errorf("Set() failed: %v", err)
				}
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				cache.Get(j) // Result doesn't matter, just testing for races
			}
		}()
	}

	wg.Wait()
	if err := cache.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// Verify expected number of entries
	values := cache.List()
	expectedEntries := numGoroutines * numOperations
	if len(values) != expectedEntries {
		t.Errorf("Expected %d entries after concurrent writes, got %d", expectedEntries, len(values))
	}

	// Verify all Persists were called
	if count := mock.GetPersistCallCount(); count != expectedEntries {
		t.Errorf("Expected %d Persist calls, got %d", expectedEntries, count)
	}
}

// TestPersistError verifies that persist errors are logged but don't break the cache
func TestPersistError(t *testing.T) {
	mock := NewMockDatabaseOperations[string, int]()
	expectedErr := errors.New("database write failed")

	mock.PersistFunc = func(ctx context.Context, key string, value int) error {
		return expectedErr
	}

	cache := NewGenericCache(mock)
	if err := cache.Load(context.Background()); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Set should succeed even if persist fails
	err := cache.Set("test", 42)
	if err != nil {
		t.Errorf("Set() should not return error even if persist fails, got %v", err)
	}

	// Value should still be in cache
	val, ok := cache.Get("test")
	if !ok || val != 42 {
		t.Errorf("Expected test=42 in cache despite persist error, got %v, %v", val, ok)
	}

	// Flush should complete (errors are logged, not returned)
	if err := cache.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// Verify Persist was called despite error
	if count := mock.GetPersistCallCount(); count != 1 {
		t.Errorf("Expected Persist to be called once, got %d", count)
	}
}

// TestDeleteError verifies that delete errors are logged but don't break the cache
func TestDeleteError(t *testing.T) {
	mock := NewMockDatabaseOperations[string, int]()
	expectedErr := errors.New("database delete failed")

	mock.LoadAllFunc = func(ctx context.Context) (map[string]int, error) {
		return map[string]int{"test": 42}, nil
	}

	mock.DeleteFunc = func(ctx context.Context, key string) error {
		return expectedErr
	}

	cache := NewGenericCache(mock)
	if err := cache.Load(context.Background()); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Delete should succeed even if DB delete fails
	err := cache.Delete("test")
	if err != nil {
		t.Errorf("Delete() should not return error even if DB delete fails, got %v", err)
	}

	// Value should still be removed from cache
	if val, ok := cache.Get("test"); ok {
		t.Errorf("Expected test to be deleted from cache despite DB error, got %v", val)
	}

	if err := cache.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// Verify Delete was called despite error
	if count := mock.GetDeleteCallCount(); count != 1 {
		t.Errorf("Expected Delete to be called once, got %d", count)
	}
}

// TestMultipleSetsAndDeletes verifies correct handling of multiple operations
func TestMultipleSetsAndDeletes(t *testing.T) {
	mock := NewMockDatabaseOperations[string, int]()

	cache := NewGenericCache(mock)
	if err := cache.Load(context.Background()); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Perform multiple sets
	for i := 0; i < 10; i++ {
		if err := cache.Set("key", i); err != nil {
			t.Fatalf("Set() failed: %v", err)
		}
	}

	// Latest value should be in cache
	val, ok := cache.Get("key")
	if !ok || val != 9 {
		t.Errorf("Expected key=9, got %v, %v", val, ok)
	}

	// Flush and verify all persists were called
	if err := cache.Flush(); err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	if count := mock.GetPersistCallCount(); count != 10 {
		t.Errorf("Expected 10 Persist calls, got %d", count)
	}
}
