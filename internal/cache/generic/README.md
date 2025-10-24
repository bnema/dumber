# Generic Cache Framework

A type-safe, high-performance caching layer for Dumber Browser that provides RAM-first access with asynchronous database persistence.

## Overview

The generic cache framework provides a consistent caching pattern across all Dumber Browser caches:
- **RAM-First**: All reads from memory (no DB queries)
- **Async Writes**: Updates happen immediately in cache, persisted asynchronously
- **Bulk Load**: Load all data from DB at startup
- **Graceful Shutdown**: Flush all pending writes on shutdown

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Application Code                        │
│                    (BrowserService, etc.)                    │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                  Cache[K, V] Interface                       │
│                                                               │
│  Load(ctx)        - Bulk load at startup                     │
│  Get(key)         - RAM lookup only (never DB)               │
│  Set(key, value)  - Immediate cache + async DB               │
│  Delete(key)      - Immediate cache + async DB               │
│  List()           - All cached values                        │
│  Flush()          - Wait for pending writes                  │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│              GenericCache[K, V] Implementation               │
│                                                               │
│  ┌──────────────┐     ┌──────────────────────────────────┐ │
│  │  sync.Map    │     │   DatabaseOperations[K, V]       │ │
│  │  (RAM cache) │     │                                  │ │
│  │              │     │  - LoadAll(ctx) → map[K]V       │ │
│  │              │     │  - Persist(ctx, K, V) → error   │ │
│  │              │     │  - Delete(ctx, K) → error       │ │
│  └──────────────┘     └──────────────────────────────────┘ │
│                                                               │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  pendingWrites sync.WaitGroup                          │ │
│  │  (tracks async operations for graceful shutdown)       │ │
│  └────────────────────────────────────────────────────────┘ │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                  Database (SQLite)                           │
│                                                               │
│  zoom_levels, shortcuts, certificate_validations, etc.       │
└─────────────────────────────────────────────────────────────┘
```

## Usage Example

### 1. Implement DatabaseOperations

First, create a struct that implements the `DatabaseOperations[K, V]` interface:

```go
package cache

import (
    "context"
    "github.com/bnema/dumber/internal/cache/generic"
    "github.com/bnema/dumber/internal/db"
)

// ZoomDBOperations implements DatabaseOperations for zoom levels
type ZoomDBOperations struct {
    queries db.DatabaseQuerier
}

func NewZoomDBOperations(queries db.DatabaseQuerier) *ZoomDBOperations {
    return &ZoomDBOperations{queries: queries}
}

func (z *ZoomDBOperations) LoadAll(ctx context.Context) (map[string]float64, error) {
    levels, err := z.queries.ListZoomLevels(ctx)
    if err != nil {
        return nil, err
    }

    result := make(map[string]float64, len(levels))
    for _, level := range levels {
        result[level.Domain] = level.ZoomFactor
    }
    return result, nil
}

func (z *ZoomDBOperations) Persist(ctx context.Context, key string, value float64) error {
    return z.queries.SetZoomLevel(ctx, key, value)
}

func (z *ZoomDBOperations) Delete(ctx context.Context, key string) error {
    return z.queries.DeleteZoomLevel(ctx, key)
}
```

### 2. Create a Cache Instance

```go
package cache

import "github.com/bnema/dumber/internal/cache/generic"

type ZoomCache struct {
    *generic.GenericCache[string, float64]
}

func NewZoomCache(queries db.DatabaseQuerier) *ZoomCache {
    dbOps := NewZoomDBOperations(queries)
    return &ZoomCache{
        GenericCache: generic.NewGenericCache(dbOps),
    }
}
```

### 3. Use in Application Code

```go
// At startup
zoomCache := cache.NewZoomCache(dbQueries)
if err := zoomCache.Load(ctx); err != nil {
    log.Fatalf("Failed to load zoom cache: %v", err)
}

// Get (always from RAM, never hits DB)
if zoomLevel, ok := zoomCache.Get("example.com"); ok {
    fmt.Printf("Zoom level: %.2f\n", zoomLevel)
}

// Set (immediate in cache, async to DB)
if err := zoomCache.Set("example.com", 1.5); err != nil {
    log.Printf("Failed to set zoom level: %v", err)
}

// On shutdown
if err := zoomCache.Flush(); err != nil {
    log.Printf("Failed to flush cache: %v", err)
}
```

## Performance Characteristics

| Operation | Latency     | Notes                                      |
|-----------|-------------|--------------------------------------------|
| Load()    | ~2-50ms     | One-time at startup, bulk loads all data   |
| Get()     | <1µs        | sync.Map lookup, never touches DB          |
| Set()     | <1µs        | Returns immediately, DB write async        |
| Delete()  | <1µs        | Returns immediately, DB delete async       |
| List()    | ~10-100µs   | Iterates sync.Map, no DB access            |
| Flush()   | Varies      | Blocks until all async writes complete     |

## Thread Safety

All operations are thread-safe:
- `sync.Map` provides lock-free concurrent reads and writes
- `sync.WaitGroup` coordinates graceful shutdown
- Database operations are serialized per key (last write wins)

## Testing

The framework includes comprehensive unit tests with a mock implementation:

```go
import "github.com/bnema/dumber/internal/cache/generic"

func TestMyCache(t *testing.T) {
    mock := generic.NewMockDatabaseOperations[string, int]()

    // Configure mock behavior
    mock.LoadAllFunc = func(ctx context.Context) (map[string]int, error) {
        return map[string]int{"key": 42}, nil
    }

    cache := generic.NewGenericCache(mock)

    // Test cache operations
    if err := cache.Load(context.Background()); err != nil {
        t.Fatal(err)
    }

    val, ok := cache.Get("key")
    if !ok || val != 42 {
        t.Errorf("Expected key=42, got %v, %v", val, ok)
    }

    // Verify mock was called
    if count := mock.GetLoadAllCallCount(); count != 1 {
        t.Errorf("Expected LoadAll called once, got %d", count)
    }
}
```

## Design Decisions

### Why Generic?

Type safety and code reuse. The same cache implementation works for:
- `Cache[string, float64]` (zoom levels)
- `Cache[string, Shortcut]` (search shortcuts)
- `Cache[string, CertValidation]` (TLS certificates)

### Why Async Persistence?

- **Startup Performance**: <500ms startup time requirement
- **UI Responsiveness**: Never block user actions on DB writes
- **Write Coalescing**: Multiple rapid updates to same key only persist latest value

### Why Bulk Load?

- Small datasets (10-100 entries typical)
- Faster than incremental loading with query overhead
- Simpler code: no cache miss logic needed

### Why sync.Map Instead of map + RWMutex?

- Lock-free reads in common case
- Better performance under concurrent load
- Standard library, battle-tested

## Limitations

- **Not for large datasets**: Loads entire dataset into RAM
- **No TTL/expiration**: All entries cached indefinitely
- **No size limits**: Assumes datasets fit comfortably in memory
- **Last write wins**: No conflict resolution for concurrent updates

For Dumber Browser's use case (small configuration tables), these tradeoffs are acceptable.

## Future Enhancements

Potential improvements if needed:
- [ ] Add metrics (cache hits, miss rate, persist latency)
- [ ] Add TTL support for time-sensitive data
- [ ] Add size-based eviction for large caches
- [ ] Add batch persistence for write-heavy workloads
- [ ] Add versioning/CAS for concurrent update detection
