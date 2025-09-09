# Data Model: WebKit Memory Optimization

**Feature**: 005-webkit-memory-optimization
**Phase**: 1 - Design & Data Model
**Date**: 2025-01-10

## Core Entities

### MemoryConfig
**Purpose**: Configuration settings that control WebKit memory optimization behavior

**Fields**:
- `MemoryLimitMB` (int): Maximum memory limit per WebView in MB (0 = unlimited)
- `ConservativeThreshold` (float64): Early memory cleanup threshold (0.0-1.0, default 0.33)
- `StrictThreshold` (float64): Aggressive memory cleanup threshold (0.0-1.0, default 0.5)
- `KillThreshold` (float64): Process termination threshold (0.0-1.0, 0 = disabled)
- `PollIntervalSeconds` (float64): Memory usage polling interval (default 2.0)
- `CacheModel` (CacheModel): WebKit caching strategy (document_viewer/web_browser/primary_web_browser)
- `EnablePageCache` (bool): Enable/disable page cache for back/forward navigation
- `EnableOfflineAppCache` (bool): Enable/disable offline application caching
- `ProcessRecycleThreshold` (int): Recycle WebView after N page loads (0 = disabled)
- `EnableGCInterval` (int): JavaScript garbage collection interval in seconds (0 = disabled)
- `EnableMemoryMonitoring` (bool): Enable detailed memory logging and monitoring

**Validation Rules**:
- ConservativeThreshold must be between 0.0 and 1.0
- StrictThreshold must be between 0.0 and 1.0
- StrictThreshold must be greater than ConservativeThreshold
- KillThreshold must be between 0.0 and 1.0
- PollIntervalSeconds must be non-negative
- MemoryLimitMB must be non-negative

**State Transitions**: Immutable configuration object (changes require new instance)

### MemoryStats
**Purpose**: Real-time and historical memory usage statistics for WebView instances

**Fields**:
- `pageLoadCount` (int): Number of pages loaded in this WebView instance
- `lastGCTime` (time.Time): Timestamp of last JavaScript garbage collection
- `memoryPressureSettings` (*C.WebKitMemoryPressureSettings): Native WebKit memory settings

**Relationships**: 
- Owned by WebView instance (1:1 relationship)
- References native WebKit memory pressure settings
- Tracked by WebViewMemoryManager (many:1 relationship)

**Validation Rules**: Read-only statistics, no validation required

**State Transitions**:
- `pageLoadCount` increments on each page load
- `lastGCTime` updates when garbage collection is triggered
- `memoryPressureSettings` set once during WebView creation

### ProcessMemoryInfo
**Purpose**: System-level memory information for WebKit processes

**Fields**:
- `PID` (int): Process identifier
- `VmRSS` (int64): Resident Set Size in KB (actual RAM usage)
- `VmSize` (int64): Virtual Memory Size in KB (total virtual memory)
- `VmPeak` (int64): Peak Virtual Memory in KB (maximum ever used)
- `ProcessName` (string): Name of the process (extracted from cmdline)

**Relationships**: 
- Collected by WebViewMemoryManager for all WebKit processes
- Associated with specific WebView instances (many:1 for multi-process WebKit)

**Validation Rules**:
- All memory values must be non-negative
- PID must be positive integer

**State Transitions**: Immutable snapshot of process memory state

### OptimizationPreset
**Purpose**: Pre-configured memory optimization settings for common use cases

**Fields**: Embeds MemoryConfig with preset values

**Preset Types**:
1. **MemoryOptimized**: Maximum memory reduction (256MB limit, aggressive thresholds)
2. **Balanced**: Moderate optimization (512MB limit, balanced thresholds)  
3. **HighPerformance**: Maximum performance (no limits, minimal optimization)

**Validation Rules**: Inherits MemoryConfig validation rules

**State Transitions**: Immutable preset definitions

### WebViewMemoryManager
**Purpose**: Global memory monitoring and lifecycle management for multiple WebViews

**Fields**:
- `views` (map[uintptr]*WebView): Registry of active WebViews by ID
- `enableMonitoring` (bool): Enable background memory monitoring
- `recycleThreshold` (int): Global recycling threshold
- `monitoringInterval` (time.Duration): Monitoring check frequency
- `stopMonitoring` (chan struct{}): Channel for stopping monitoring goroutine

**Relationships**:
- Manages multiple WebView instances (1:many)
- Monitors ProcessMemoryInfo for all WebKit processes
- Singleton pattern for global memory management

**Validation Rules**:
- monitoringInterval must be positive
- recycleThreshold must be non-negative

**State Transitions**:
- WebViews registered on creation, unregistered on destruction
- Monitoring starts when first WebView registered
- Monitoring stops when last WebView unregistered

## Entity Relationships

```
WebView (1) ----owns----> (1) MemoryStats
   |                           |
   |                           |
   v                           v
WebViewMemoryManager <-tracks- ProcessMemoryInfo
   |
   |
   v (configures)
MemoryConfig <-preset- OptimizationPreset
```

## Data Flow

### Memory Configuration Flow
1. User selects optimization preset or custom settings
2. MemoryConfig validated and applied to WebView creation
3. WebKit memory pressure settings configured via C API
4. Settings persisted for process lifecycle

### Memory Monitoring Flow
1. WebViewMemoryManager polls /proc filesystem for process memory
2. WebKit memory pressure callbacks trigger on internal events
3. MemoryStats updated with current usage and GC events
4. Memory events logged if monitoring enabled

### Process Recycling Flow
1. MemoryStats.pageLoadCount incremented on navigation
2. WebViewMemoryManager checks recycling threshold
3. Recycling recommendation logged (user/application decision)
4. New WebView creation triggers fresh memory allocation

## Memory Layout Considerations

### Go Memory Management
- WebView structs include finalizers for cleanup
- CGO memory requires explicit free() calls
- Channel-based coordination for goroutine cleanup
- Careful pointer management for C callbacks

### WebKit Native Memory
- WebKitMemoryPressureSettings allocated via C API
- GObject reference counting for WebKit objects
- Memory pressure thresholds enforced by WebKit internally
- Process memory isolated per WebView instance

### Monitoring Memory Overhead
- /proc parsing: ~1KB per process check
- Memory statistics storage: ~100 bytes per WebView
- Monitoring goroutine: ~8KB stack space
- Logging overhead: configurable via EnableMemoryMonitoring flag

## Testing Considerations

### Unit Tests
- MemoryConfig validation rules
- OptimizationPreset value correctness
- ProcessMemoryInfo parsing accuracy
- WebViewMemoryManager lifecycle management

### Integration Tests
- WebKit memory pressure settings application
- Real memory usage reduction measurement
- Process recycling effectiveness
- Memory monitoring accuracy vs /proc ground truth

### Contract Tests
- WebKit C API memory functions available
- Memory pressure callbacks functional
- GObject reference counting correct
- CGO memory safety verified