package generic

import (
	"context"
	"sync"
)

// MockDatabaseOperations is a mock implementation of DatabaseOperations for testing.
// It's generic and thread-safe, suitable for testing GenericCache.
type MockDatabaseOperations[K comparable, V any] struct {
	mu sync.Mutex

	// Behavior configuration
	LoadAllFunc func(ctx context.Context) (map[K]V, error)
	PersistFunc func(ctx context.Context, key K, value V) error
	DeleteFunc  func(ctx context.Context, key K) error

	// Call tracking
	LoadAllCalls []context.Context
	PersistCalls []struct {
		Ctx   context.Context
		Key   K
		Value V
	}
	DeleteCalls []struct {
		Ctx context.Context
		Key K
	}
}

// NewMockDatabaseOperations creates a new mock with default no-op implementations.
func NewMockDatabaseOperations[K comparable, V any]() *MockDatabaseOperations[K, V] {
	return &MockDatabaseOperations[K, V]{
		LoadAllFunc: func(ctx context.Context) (map[K]V, error) {
			return make(map[K]V), nil
		},
		PersistFunc: func(ctx context.Context, key K, value V) error {
			return nil
		},
		DeleteFunc: func(ctx context.Context, key K) error {
			return nil
		},
	}
}

// LoadAll implements DatabaseOperations.LoadAll
func (m *MockDatabaseOperations[K, V]) LoadAll(ctx context.Context) (map[K]V, error) {
	m.mu.Lock()
	m.LoadAllCalls = append(m.LoadAllCalls, ctx)
	m.mu.Unlock()

	return m.LoadAllFunc(ctx)
}

// Persist implements DatabaseOperations.Persist
func (m *MockDatabaseOperations[K, V]) Persist(ctx context.Context, key K, value V) error {
	m.mu.Lock()
	m.PersistCalls = append(m.PersistCalls, struct {
		Ctx   context.Context
		Key   K
		Value V
	}{ctx, key, value})
	m.mu.Unlock()

	return m.PersistFunc(ctx, key, value)
}

// Delete implements DatabaseOperations.Delete
func (m *MockDatabaseOperations[K, V]) Delete(ctx context.Context, key K) error {
	m.mu.Lock()
	m.DeleteCalls = append(m.DeleteCalls, struct {
		Ctx context.Context
		Key K
	}{ctx, key})
	m.mu.Unlock()

	return m.DeleteFunc(ctx, key)
}

// GetLoadAllCallCount returns the number of times LoadAll was called
func (m *MockDatabaseOperations[K, V]) GetLoadAllCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.LoadAllCalls)
}

// GetPersistCallCount returns the number of times Persist was called
func (m *MockDatabaseOperations[K, V]) GetPersistCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.PersistCalls)
}

// GetDeleteCallCount returns the number of times Delete was called
func (m *MockDatabaseOperations[K, V]) GetDeleteCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.DeleteCalls)
}

// GetLastPersistCall returns the last call to Persist, or nil if none
func (m *MockDatabaseOperations[K, V]) GetLastPersistCall() *struct {
	Ctx   context.Context
	Key   K
	Value V
} {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.PersistCalls) == 0 {
		return nil
	}
	last := m.PersistCalls[len(m.PersistCalls)-1]
	return &last
}

// GetLastDeleteCall returns the last call to Delete, or nil if none
func (m *MockDatabaseOperations[K, V]) GetLastDeleteCall() *struct {
	Ctx context.Context
	Key K
} {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.DeleteCalls) == 0 {
		return nil
	}
	last := m.DeleteCalls[len(m.DeleteCalls)-1]
	return &last
}

// Reset clears all call tracking
func (m *MockDatabaseOperations[K, V]) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LoadAllCalls = nil
	m.PersistCalls = nil
	m.DeleteCalls = nil
}
