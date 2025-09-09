# Quickstart: WebKitGTK 6.0 Migration

## Prerequisites

### System Requirements
```bash
# Check GTK4 installation
pkg-config --modversion gtk4  # Should be >= 4.0

# Check WebKitGTK 6.0
pkg-config --modversion webkitgtk-6.0  # Should be >= 2.40

# Check Vulkan support (optional but recommended)
vulkaninfo | grep "Vulkan Instance Version"
```

### Install Dependencies (Arch Linux)
```bash
sudo pacman -S webkit2gtk-6.0 gtk4 vulkan-tools
```

### Install Dependencies (Ubuntu/Debian)
```bash
sudo apt install libwebkitgtk-6.0-dev libgtk-4-dev vulkan-tools
```

## Build and Test

### 1. Switch to Feature Branch
```bash
git checkout 004-migrate-to-webkitgtk
```

### 2. Build the Browser
```bash
make clean
make build
```

### 3. Verify GPU Rendering

#### Test Auto-Detection (Default)
```bash
# Should auto-detect GPU availability
./dumber browse https://webglsamples.org/aquarium/aquarium.html
```

#### Force GPU Mode
```bash
# Force GPU acceleration
./dumber browse --rendering-mode=gpu https://webglsamples.org/aquarium/aquarium.html

# Or using environment variable
DUMBER_RENDERING_MODE=gpu ./dumber browse https://example.com
```

#### Force CPU Mode
```bash
# Force software rendering
./dumber browse --rendering-mode=cpu https://example.com

# Or shorthand
./dumber browse --disable-gpu https://example.com
```

#### Debug GPU Compositing
```bash
# Show GPU layer borders
./dumber browse --debug-gpu https://example.com
```

### 4. Check Rendering Status
```bash
# While browser is running, check status
curl http://localhost:8080/api/rendering/status
```

Expected response:
```json
{
  "mode": "auto",
  "gpu_active": true,
  "gpu_available": true,
  "vulkan_enabled": true,
  "errors": [],
  "performance": {
    "fps": 60,
    "cpu_usage_percent": 15.2,
    "gpu_usage_percent": 45.8,
    "memory_mb": 256
  }
}
```

## Performance Testing

### 1. Benchmark Page Load
```bash
# Measure with GPU
time ./dumber browse --rendering-mode=gpu https://browserbench.org/MotionMark/

# Measure with CPU
time ./dumber browse --rendering-mode=cpu https://browserbench.org/MotionMark/
```

### 2. Monitor Resource Usage
```bash
# In another terminal, monitor GPU
nvidia-smi -l 1  # For NVIDIA
radeontop        # For AMD

# Monitor CPU
htop
```

### 3. Test WebGL Performance
```bash
# High-demand WebGL test
./dumber browse https://webglsamples.org/dynamic-cubemap/dynamic-cubemap.html

# Check FPS counter in browser
```

## Troubleshooting

### GPU Not Detected
```bash
# Check Vulkan drivers
vulkaninfo

# Check OpenGL
glxinfo | grep "OpenGL renderer"

# Force CPU mode if GPU issues
./dumber browse --disable-gpu
```

### Performance Issues
```bash
# Enable debug indicators
./dumber browse --debug-gpu https://example.com

# Check for compositing issues (red borders = repaints)
```

### Build Errors
```bash
# Ensure correct pkg-config
pkg-config --libs webkitgtk-6.0 gtk4

# Clean and rebuild
make clean
make build
```

## Validation Checklist

- [ ] Browser builds successfully with WebKitGTK 6.0
- [ ] Browser starts in < 500ms
- [ ] GPU acceleration activates when available
- [ ] CPU fallback works when GPU unavailable
- [ ] WebGL content renders correctly
- [ ] No visual artifacts or rendering issues
- [ ] History and bookmarks preserved
- [ ] All existing features work
- [ ] Performance improved with GPU enabled
- [ ] Resource usage within acceptable limits

## Migration Verification

### Check API Usage
```bash
# Verify new GTK4/WebKit6 symbols
ldd ./dumber | grep -E "(gtk-4|webkit.*6)"

# Should see:
# libgtk-4.so.1
# libwebkitgtk-6.0.so.4
```

### Test Key Features
1. **Navigation**: URLs load correctly
2. **History**: Previous data accessible
3. **Zoom**: Ctrl+Plus/Minus works
4. **DevTools**: F12 opens inspector
5. **Dmenu**: Integration still works

## Report Issues

If you encounter issues:
1. Note the rendering mode used
2. Check `/tmp/dumber.log` for errors
3. Run with `--debug-gpu` for visual debugging
4. Report with output of `vulkaninfo` and `glxinfo`