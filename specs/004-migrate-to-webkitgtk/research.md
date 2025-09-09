# Research: WebKitGTK 6.0 Migration

## API Migration Research

### Decision: WebKitGTK 6.0 with GTK4
**Rationale**: 
- GTK4 provides native Vulkan renderer support
- WebKitGTK 6.0 is the only actively maintained version for GTK4
- Enables hardware-accelerated rendering via modern GPU APIs
- Better Wayland support and performance

**Alternatives considered**:
- Staying on WebKit2GTK 4.0: No Vulkan support, limited to OpenGL
- Using WebKit2GTK 4.1: Still GTK3, no Vulkan renderer
- Custom Vulkan implementation: Too complex, maintenance burden

### GTK3 to GTK4 Migration Patterns

**Key Changes**:
1. **Event handling**: GdkEvent parameter removed from signals
2. **Widget construction**: Direct use of g_object_new() instead of specific constructors
3. **Drawing**: No more GtkDrawingArea draw signal, use GtkSnapshot
4. **CSS**: GTK4 uses standard CSS, removed custom properties
5. **Layout**: New layout managers replace containers

**CGO Binding Updates**:
- pkg-config: `webkit2gtk-4.0` → `webkitgtk-6.0`
- pkg-config: `gtk+-3.0` → `gtk4`
- Header: `gtk/gtk.h` remains same
- Header: `webkit2/webkit2.h` → `webkit/webkit.h`

### Vulkan Renderer Configuration

**Decision: Hardware Acceleration Policy**
**Rationale**: Provides user control over GPU vs CPU rendering

**Configuration Options**:
1. `WEBKIT_HARDWARE_ACCELERATION_POLICY_ALWAYS`: Force GPU
2. `WEBKIT_HARDWARE_ACCELERATION_POLICY_NEVER`: Force CPU
3. `WEBKIT_HARDWARE_ACCELERATION_POLICY_ON_DEMAND`: Auto-detect (default)

**New Settings in WebKitGTK 6.0**:
- `webkit_settings_set_hardware_acceleration_policy()`
- `webkit_settings_set_enable_2d_canvas_acceleration()`
- `webkit_settings_set_enable_webgl()`
- `webkit_settings_set_draw_compositing_indicators()` (debugging)

### Deprecated APIs to Replace

| WebKit2GTK 4.0 API | WebKitGTK 6.0 Replacement |
|-------------------|---------------------------|
| `webkit_web_view_new_with_context()` | `g_object_new()` with properties |
| `webkit_web_context_set_sandbox_enabled()` | Always enabled, use `add_path_to_sandbox()` |
| `WebKitWebContext` | `WebKitNetworkSession` for networking |
| `webkit_web_view_run_javascript()` | Same, but async only |
| Process swap property | Always enabled |

### Build System Changes

**pkg-config flags**:
```bash
# Old (WebKit2GTK 4.0)
pkg-config --cflags --libs webkit2gtk-4.0 gtk+-3.0 javascriptcoregtk-4.0

# New (WebKitGTK 6.0)
pkg-config --cflags --libs webkitgtk-6.0 gtk4 javascriptcoregtk-6.0
```

### Performance Considerations

**GPU Rendering Benefits**:
- Vulkan backend via GTK4's unified GPU renderer
- Hardware-accelerated 2D canvas
- Improved WebGL performance
- Reduced CPU usage for compositing
- Better multi-threaded rendering

**Fallback Strategy**:
- Detect GPU capabilities at runtime
- Use `ON_DEMAND` policy for automatic selection
- Provide config flag for manual override
- Monitor performance metrics

### Global Rendering Configuration

**Decision: Add RenderingMode to config**
**Rationale**: User control over CPU/GPU rendering with smart defaults

**Implementation**:
```go
type RenderingMode string

const (
    RenderingModeAuto RenderingMode = "auto"  // Default, detect at runtime
    RenderingModeGPU  RenderingMode = "gpu"   // Force GPU acceleration
    RenderingModeCPU  RenderingMode = "cpu"   // Force software rendering
)
```

**Config Integration**:
- Add to `internal/config/config.go`
- Default to "auto" (GPU if available)
- CLI flag: `--rendering-mode=[auto|gpu|cpu]`
- Environment variable: `DUMBER_RENDERING_MODE`

## Summary

The migration to WebKitGTK 6.0 requires:
1. Complete GTK3 → GTK4 widget/event migration
2. Update all WebKit API calls to new versions
3. Implement hardware acceleration configuration
4. Add global rendering mode configuration
5. Test Vulkan renderer activation and fallback

All technical questions have been resolved through research.