# Quickstart: WebKit Memory Optimization

**Feature**: 005-webkit-memory-optimization
**Purpose**: Validate that WebKit memory optimization reduces memory usage by 40-60% while maintaining browser functionality
**Estimated Time**: 15 minutes

## Prerequisites
- dumb-browser built with webkit_cgo tags
- WebKit2GTK 6.0+ installed
- Linux system with /proc filesystem
- At least 2GB available RAM for testing

## Quick Validation Test

### Step 1: Baseline Memory Measurement
```bash
# Start browser with default settings
./dumber --url=https://example.com

# In another terminal, measure baseline memory
ps aux | grep dumber | grep -v grep
# Note the RSS memory usage (column 6) - should be ~400MB
```

### Step 2: Enable Memory Optimization
```bash
# Stop the browser and restart with memory optimization
./dumber --memory-limit=256 --cache-model=document_viewer --url=https://example.com

# Measure optimized memory usage
ps aux | grep dumber | grep -v grep
# Note the RSS memory usage - should be 150-250MB (40-60% reduction)
```

### Step 3: Test Browser Functionality
Navigate to several websites to ensure functionality is preserved:
```bash
# Test different website types
https://example.com          # Simple static site
https://news.ycombinator.com # Content-heavy site
https://github.com          # Interactive web app
```

**Expected**: All sites should load correctly with no functionality loss

### Step 4: Test Memory Monitoring
```bash
# Enable detailed memory monitoring
./dumber --memory-monitoring=true --url=https://example.com

# Check logs for memory events
tail -f ~/.config/dumber/logs/memory.log
```

**Expected**: Log entries showing memory usage, GC events, and optimization activities

### Step 5: Test Memory Pressure Response
```bash
# Simulate high memory usage by opening many tabs or heavy websites
# Browser should automatically trigger cleanup when approaching limits

# Monitor memory stats via API endpoint (if implemented)
curl http://localhost:8080/api/memory/stats
```

**Expected**: Memory usage should stabilize, cleanup events should be logged

## Integration Test Scenarios

### Scenario 1: Memory-Optimized Configuration
**Given** browser started with memory-optimized preset
**When** I navigate to a typical website
**Then** memory usage should be ≤250MB

```bash
# Test memory-optimized preset
./dumber --preset=memory-optimized --url=https://example.com
ps aux | grep dumber | awk '{print $6}' # Should be ≤256MB
```

### Scenario 2: Process Recycling
**Given** browser with recycling threshold set to 10 pages
**When** I navigate to 15 different pages
**Then** browser should recommend process recycling

```bash
# Test process recycling
./dumber --recycle-threshold=10 --memory-monitoring=true
# Navigate to 15 different URLs
# Check logs for recycling recommendation
```

### Scenario 3: Memory Stability
**Given** browser with memory optimization enabled
**When** I browse 50 different web pages
**Then** memory usage should remain stable (no significant growth)

```bash
# Test memory stability over time
for i in {1..50}; do
  echo "Loading page $i"
  # Navigate to different pages
  sleep 2
  ps aux | grep dumber | awk '{print $6}' >> memory_usage.log
done
# Analyze memory_usage.log for growth trends
```

## Acceptance Criteria Validation

### ✅ FR-001: Memory Usage Reduction
- [ ] Baseline measurement: ~400MB
- [ ] Optimized measurement: 150-250MB
- [ ] Reduction percentage: 40-60%

### ✅ FR-002: Configurable Memory Limits
- [ ] Memory limit configuration working
- [ ] Different presets produce different memory usage
- [ ] Custom thresholds respected

### ✅ FR-003: Optimization Presets
- [ ] Memory-optimized preset: lowest memory usage
- [ ] Balanced preset: moderate memory usage
- [ ] Performance preset: highest memory usage

### ✅ FR-004: Automatic Memory Cleanup
- [ ] Memory pressure triggers cleanup
- [ ] Cleanup completes without crashing
- [ ] Memory usage decreases after cleanup

### ✅ FR-005: Memory Statistics
- [ ] Memory stats API accessible
- [ ] Statistics updated regularly
- [ ] Historical data tracked

### ✅ FR-006: Browser Stability
- [ ] No crashes during optimization
- [ ] All websites load correctly
- [ ] Navigation remains smooth

### ✅ FR-007: Essential Functionality Preserved
- [ ] Page navigation works
- [ ] JavaScript execution works
- [ ] Form submission works
- [ ] Media playback works (if applicable)

### ✅ FR-008: Manual Memory Cleanup
- [ ] Manual cleanup trigger available
- [ ] Cleanup executes successfully
- [ ] Memory usage decreases after manual trigger

### ✅ FR-009: Process Recycling
- [ ] Page load counting accurate
- [ ] Recycling recommendations logged
- [ ] New process has reset counters

### ✅ FR-010: Memory Event Logging
- [ ] Memory optimization events logged
- [ ] Log format is structured and readable
- [ ] Log verbosity controllable

## Performance Validation

### Memory Usage Benchmarks
```bash
# Run benchmark script to measure memory over time
./scripts/memory-benchmark.sh

Expected results:
- Startup memory: <100MB
- Single page memory: 150-250MB (optimized) vs 350-450MB (default)
- 10 page memory: <300MB (optimized) vs >500MB (default)
- Memory stability: <10% growth over 50 page loads
```

### Performance Impact Assessment
```bash
# Measure page load times with/without optimization
./scripts/performance-benchmark.sh

Expected results:
- Page load time increase: <20%
- JavaScript performance impact: <15%
- Navigation responsiveness: maintained
```

## Troubleshooting

### Common Issues

**Memory optimization not working**: 
- Check WebKit2GTK version (requires 6.0+)
- Verify webkit_cgo build tags enabled
- Check configuration values in logs

**Browser crashes with memory limits**:
- Increase memory limit (try 512MB)
- Disable kill threshold (set to 0)
- Check for website-specific memory requirements

**Memory monitoring not working**:
- Verify /proc filesystem accessible
- Check process permissions
- Enable debug logging

### Validation Commands
```bash
# Check WebKit version
pkg-config --modversion webkitgtk-6.0

# Verify build configuration
./dumber --version | grep webkit_cgo

# Test memory monitoring
./dumber --memory-monitoring=true --verbose
```

## Success Criteria
- [ ] Memory usage reduced by 40-60% compared to baseline
- [ ] All browser functionality preserved
- [ ] Memory monitoring and statistics working
- [ ] Process recycling recommendations functional
- [ ] No stability regressions
- [ ] Configuration presets working correctly
- [ ] Performance impact within acceptable limits (<20%)

**Expected Completion Time**: 15-30 minutes depending on thorough testing
**Pass Criteria**: All acceptance criteria validated, no critical functionality broken