# Quickstart: WebKit2GTK Native Browser Backend

**Feature**: Native WebKit Browser Backend  
**Date**: 2025-01-21  
**Status**: Ready for implementation

## Prerequisites

### System Dependencies
```bash
# Install WebKit2GTK development libraries
sudo pacman -S webkit2gtk-4.0 gtk3

# Clone WebKit2GTK reference repository
mkdir -p /home/brice/dev/clone
cd /home/brice/dev/clone
git clone https://github.com/WebKit/webkit.git webkit2gtk-reference

# Verify CGO is available
go env CGO_ENABLED  # Should output: 1
```

### Development Environment
```bash
# Ensure Go version compatibility
go version  # Should be Go 1.21+

# Install WebKit2GTK pkg-config
pkg-config --cflags webkit2gtk-4.0  # Should output compiler flags
pkg-config --libs webkit2gtk-4.0    # Should output linker flags
```

## Quick Validation

### Step 1: Verify Current State
```bash
# Check current browser works with Wails
./dumber-browser
# Expected: Browser opens with homepage, keyboard shortcuts work

# Note current performance baseline
time ./dumber-browser &
# Expected: Browser starts within 500ms
```

### Step 2: Basic WebKit Integration Test
```bash
# Create minimal WebKit test (after pkg/webkit implementation)
cd tests/integration
go test -v ./webkit_basic_test.go
# Expected: WebView creates, loads google.fr, closes cleanly
```

### Step 3: Feature Parity Validation  
```bash
# Test keyboard shortcuts
# Expected: Alt+Left/Right = navigation, Ctrl+Shift+C = copy URL

# Test zoom functionality  
# Expected: Ctrl++/Ctrl+-/Ctrl+0 zoom in/out/reset

# Test script injection
# Expected: Global keyboard shortcuts work on external sites
```

### Step 4: Performance Validation
```bash
# Startup time test
time ./dumber-browser &
# Target: <500ms (same as Wails baseline)

# Memory usage test  
ps aux | grep dumber-browser
# Target: <100MB baseline + WebKit overhead

# Navigation performance
# Target: Page loads equivalent to current Wails performance
```

### Step 5: Data Preservation Test
```bash
# Backup current database
cp ~/.config/dumber/browser.db ~/.config/dumber/browser.db.backup

# Run WebKit version, browse some sites, adjust zoom
# Close browser, restart

# Verify data preserved
sqlite3 ~/.config/dumber/browser.db "SELECT COUNT(*) FROM history;"
# Expected: Same or increased count, no data loss

# Verify zoom settings preserved  
# Expected: Previously set zoom levels maintained
```

## Success Criteria Checklist

### Basic Functionality ✓
- [ ] Browser window opens and displays correctly
- [ ] Can navigate to google.fr successfully  
- [ ] Page renders with full HTML5/CSS3/JavaScript support
- [ ] Browser window closes cleanly without errors

### Keyboard Shortcuts ✓
- [ ] Alt+Left arrow navigates back
- [ ] Alt+Right arrow navigates forward  
- [ ] Ctrl+Shift+C copies current URL to clipboard
- [ ] Ctrl+Plus/Equals zooms in
- [ ] Ctrl+Minus zooms out
- [ ] Ctrl+0 resets zoom to 100%

### Data Preservation ✓
- [ ] Existing browser history maintained
- [ ] Zoom settings per domain preserved
- [ ] No database schema changes required
- [ ] All SQLC queries work without modification

### Performance Standards ✓  
- [ ] Startup time <500ms (constitution requirement)
- [ ] Memory usage <100MB baseline (constitution requirement)
- [ ] Navigation speed equivalent to Wails baseline
- [ ] Keyboard shortcut response time <100ms

### Advanced Features ✓
- [ ] JavaScript injection works on external sites
- [ ] Developer tools accessible (F12)
- [ ] Error handling graceful (no crashes)
- [ ] Multiple page navigation works correctly

## Troubleshooting Common Issues

### WebKit Initialization Fails
```bash
# Check WebKit2GTK installation
pkg-config --exists webkit2gtk-4.0 && echo "WebKit2GTK found" || echo "WebKit2GTK missing"

# Check GTK installation
pkg-config --exists gtk+-3.0 && echo "GTK3 found" || echo "GTK3 missing"
```

### CGO Compilation Errors
```bash
# Verify CGO configuration
go env CGO_ENABLED CGO_CFLAGS CGO_LDFLAGS

# Test basic CGO compilation
echo 'package main; import "C"; func main() {}' | go run -x -
```

### Performance Regression
```bash
# Compare memory usage
# Wails version: ps aux | grep dumber-browser (baseline)  
# WebKit version: ps aux | grep dumber-browser (should be similar)

# Compare startup time  
time ./dumber-browser-wails &
time ./dumber-browser-webkit &
# WebKit should be equal or faster
```

### Keyboard Shortcuts Not Working
```bash
# Test GTK event handling in isolation
# Check that GTK application receives key events
# Verify WebKit UserContentManager script injection

# Test on different sites
# google.fr - should work
# Local file - should work  
# HTTPS site with CSP - may need handling
```

## Migration Strategy

### Phase 1: Parallel Implementation
- Keep existing Wails code intact
- Develop pkg/webkit alongside current implementation
- Use feature flags or separate binary for testing

### Phase 2: Feature Parity
- Achieve 100% functional equivalence with Wails version
- Pass all existing tests with WebKit implementation
- Validate performance meets constitutional requirements

### Phase 3: Full Migration  
- Switch default implementation to WebKit
- Remove Wails dependencies from go.mod
- Remove frontend TypeScript build system
- Update documentation to remove Wails references

### Phase 4: Cleanup
- Remove deprecated Wails code
- Optimize pkg/webkit for production use
- Prepare pkg/webkit for potential repository extraction

---

**Quickstart Status**: ✅ Ready - All validation steps defined, implementation can begin