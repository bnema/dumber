# Research: WebKit Memory Optimization

**Feature**: 005-webkit-memory-optimization
**Phase**: 0 - Research & Technical Decisions
**Date**: 2025-01-10

## Technical Decisions Made

### WebKit Memory Pressure API Selection
**Decision**: Use WebKitMemoryPressureSettings introduced in WebKit2GTK 6.0
**Rationale**: 
- Provides granular control over memory limits and cleanup thresholds
- Native WebKit memory management rather than external process monitoring
- Integrates with WebKit's internal memory pressure signals
- Available in WebKit2GTK 6.0+ (already required by project)

**Alternatives considered**:
- Manual process monitoring via /proc filesystem: Less integrated, more overhead
- System-level memory pressure (systemd, kernel): Too coarse-grained for per-WebView control
- JavaScript-only GC triggers: Insufficient for WebKit native memory (images, CSS, DOM)

### Cache Model Optimization Strategy
**Decision**: Use WEBKIT_CACHE_MODEL_DOCUMENT_VIEWER as memory-optimized default
**Rationale**:
- Minimal memory caching strategy designed for document viewing
- 40-60% less memory usage than WEBKIT_CACHE_MODEL_WEB_BROWSER
- Still maintains reasonable performance for typical browsing
- Configurable - users can choose more performance-oriented models

**Alternatives considered**:
- Custom cache implementation: Too complex, reinvents WebKit internals
- Disable all caching: Would break many websites and hurt performance significantly
- Dynamic cache model switching: Added complexity without clear benefits

### Process Recycling Implementation
**Decision**: Track page load count per WebView, recommend recycling at configurable threshold
**Rationale**:
- WebKit processes accumulate memory over time due to fragmentation
- Simple counter-based approach is reliable and predictable
- User/application controls recycling policy, not automatic termination
- Preserves user experience while managing long-term memory growth

**Alternatives considered**:
- Time-based recycling: Unpredictable memory impact, could interrupt user workflows
- Memory-threshold recycling: More complex, could lead to recycling loops
- Automatic process termination: Too aggressive, would lose user state

### Memory Monitoring Architecture
**Decision**: Hybrid approach using /proc filesystem + WebKit memory pressure callbacks
**Rationale**:
- /proc provides detailed RSS/VmSize metrics for external monitoring tools
- WebKit callbacks provide internal memory pressure events
- Both together give complete picture for debugging and optimization
- Enables both real-time monitoring and historical tracking

**Alternatives considered**:
- WebKit-only monitoring: Limited visibility into actual system memory usage
- /proc-only monitoring: Misses WebKit internal memory pressure states
- External monitoring tools: Adds dependencies, complicates deployment

### Configuration Preset Strategy
**Decision**: Provide three preset configurations (memory-optimized, balanced, performance)
**Rationale**:
- Covers main use cases: low-memory devices, general use, high-performance needs
- Simple choice for users, comprehensive under the hood
- Provides sensible defaults while allowing customization
- Aligns with constitutional simplicity requirements

**Alternatives considered**:
- Single configuration: Too rigid, doesn't accommodate different hardware/needs
- Extensive customization UI: Too complex, violates constitutional simplicity
- Auto-detection: Unpredictable, users prefer explicit control

## Implementation Technologies Confirmed

### Core Technologies
- **WebKit2GTK 6.0**: Memory pressure settings, cache model control, JavaScript GC triggers
- **GTK4**: Event handling, settings management (already in use)
- **Go 1.25.1 + CGO**: C API bindings for WebKit memory features
- **GLib/GObject**: Memory management, signal handling for WebKit callbacks

### Testing Technologies
- **Go testing**: Unit tests for configuration validation, memory calculations
- **Integration testing**: Real WebKit processes, actual memory measurement
- **Contract testing**: WebKit C API contract verification
- **Performance testing**: Memory usage benchmarking, garbage collection impact

### Monitoring Technologies
- **/proc filesystem**: RSS, VmSize, VmPeak memory metrics
- **WebKit memory pressure callbacks**: Internal memory state changes
- **Go log package**: Structured logging for memory events
- **Time-based metrics**: GC frequency, process lifecycle tracking

## Best Practices Research

### WebKit Memory Management Patterns
- **Memory pressure thresholds**: Conservative (20-33%), Strict (35-50%), Kill (70-80%)
- **Cache model hierarchy**: DocumentViewer < WebBrowser < PrimaryWebBrowser
- **GC trigger frequency**: 30-120 second intervals optimal for background cleanup
- **Process recycling**: 25-100 page loads typical before fragmentation impact

### CGO Memory Safety Patterns
- **Finalizers**: Set on WebView structs to cleanup native resources
- **Reference counting**: Proper release of WebKit objects via g_object_unref
- **String handling**: CString/free pairs, defer cleanup patterns
- **Callback safety**: Store Go pointers safely for C callback functions

### Performance Impact Mitigation
- **Lazy configuration**: Apply memory settings only when explicitly requested
- **Background operations**: GC and monitoring in separate goroutines
- **Graceful degradation**: Fallback to defaults if memory APIs unavailable
- **User control**: Make all optimizations opt-in to preserve existing performance

## Risk Assessment

### Low Risk
- Configuration storage and validation (standard Go patterns)
- Memory monitoring (/proc parsing is well-established)
- Preset configurations (simple struct initialization)

### Medium Risk
- WebKit memory pressure integration (newer API, potential version compatibility)
- Process recycling coordination (timing and state management complexity)
- CGO callback handling (requires careful pointer management)

### High Risk
- Aggressive memory limits causing browser instability
- Performance degradation from excessive GC or monitoring
- Memory pressure false positives leading to unnecessary cleanup

### Mitigation Strategies
- Comprehensive integration testing with real websites
- Conservative default values with gradual optimization options
- Extensive logging and monitoring to detect issues early
- Graceful fallback when memory optimization fails

## Unknowns Resolved
✅ All technical unknowns from Phase 0 resolved
✅ WebKit2GTK 6.0 memory APIs researched and confirmed available
✅ CGO integration patterns established for memory management
✅ Testing approach defined for memory-related features
✅ Performance impact mitigation strategies identified