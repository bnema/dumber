# Data Model: WebKitGTK 6.0 Migration

## Configuration Model

### RenderingConfig
```go
type RenderingMode string

const (
    RenderingModeAuto RenderingMode = "auto"  // Detect GPU availability
    RenderingModeGPU  RenderingMode = "gpu"   // Force GPU acceleration
    RenderingModeCPU  RenderingMode = "cpu"   // Force software rendering
)

type RenderingConfig struct {
    Mode                    RenderingMode // Default: auto
    Enable2DCanvas          bool         // Default: true
    EnableWebGL             bool         // Default: true
    DrawCompositingIndicators bool       // Default: false (debug only)
}
```

### Updated WebKit Config
```go
type Config struct {
    // Existing fields
    InitialURL            string
    UserAgent            string
    EnableDeveloperExtras bool
    ZoomDefault          float64
    DataDir              string
    CacheDir             string
    DefaultSansFont      string
    DefaultSerifFont     string
    DefaultMonospaceFont string
    DefaultFontSize      int
    
    // New GPU rendering fields
    Rendering RenderingConfig
}
```

## API Mapping Model

### Core WebView APIs
| Component | WebKit2GTK 4.0 | WebKitGTK 6.0 | Status |
|-----------|---------------|---------------|---------|
| WebView Creation | `webkit_web_view_new_with_context()` | `g_object_new(WEBKIT_TYPE_WEB_VIEW, ...)` | Breaking |
| Context | `WebKitWebContext` | `WebKitNetworkSession` | Breaking |
| Settings | `WebKitSettings` | `WebKitSettings` (expanded) | Compatible |
| UserContentManager | Same | Same | Compatible |
| JavaScript | `webkit_web_view_run_javascript()` | Same (async only) | Compatible |

### GTK Widget APIs
| Component | GTK3 | GTK4 | Status |
|-----------|------|------|---------|
| Window | `gtk_window_new(GTK_WINDOW_TOPLEVEL)` | `gtk_window_new()` | Breaking |
| Container | `gtk_container_add()` | `gtk_window_set_child()` | Breaking |
| Events | `GdkEventKey*` | `GtkEventController` | Breaking |
| Show/Hide | `gtk_widget_show()` | Same | Compatible |
| Destroy | `gtk_widget_destroy()` | `gtk_window_destroy()` | Breaking |

### Hardware Acceleration APIs
```go
// New acceleration policy enum
type HardwareAccelerationPolicy int

const (
    PolicyOnDemand HardwareAccelerationPolicy = iota  // Auto-detect
    PolicyAlways                                      // Force GPU
    PolicyNever                                       // Force CPU
)

// Mapping to WebKit constants
var policyMap = map[RenderingMode]HardwareAccelerationPolicy{
    RenderingModeAuto: PolicyOnDemand,
    RenderingModeGPU:  PolicyAlways,
    RenderingModeCPU:  PolicyNever,
}
```

## Migration States

### File Migration Status
```go
type MigrationStatus string

const (
    StatusPending   MigrationStatus = "pending"
    StatusInProgress MigrationStatus = "in_progress"
    StatusComplete  MigrationStatus = "complete"
    StatusTested    MigrationStatus = "tested"
)

type FileMigration struct {
    Path     string
    Status   MigrationStatus
    Changes  []string  // List of API changes made
    Tests    []string  // Associated test files
}
```

## Error Handling Model

### GPU Fallback Errors
```go
type GPUError struct {
    Type    string  // "initialization", "driver", "memory"
    Message string
    Fallback bool   // Whether CPU fallback was successful
}

type RenderingStatus struct {
    Mode      RenderingMode
    GPUActive bool
    Errors    []GPUError
    Performance struct {
        FPS        float64
        CPUUsage   float64
        GPUUsage   float64
        MemoryMB   int
    }
}
```

## Build Configuration

### Package Config Changes
```makefile
# Old build flags
WEBKIT_PKG = webkit2gtk-4.0
GTK_PKG = gtk+-3.0
JS_PKG = javascriptcoregtk-4.0

# New build flags
WEBKIT_PKG = webkitgtk-6.0
GTK_PKG = gtk4
JS_PKG = javascriptcoregtk-6.0
```

## Validation Rules

### Rendering Mode Validation
- If GPU forced but unavailable → Error with suggestion to use auto
- If CPU forced → Warn about performance impact
- If auto → Log detected mode at startup

### Performance Thresholds
- Startup time must remain < 500ms
- GPU memory usage < 500MB
- CPU usage reduction > 20% when GPU active
- FPS improvement > 30% for animations

## State Transitions

### Rendering Mode State Machine
```
┌─────┐ Check GPU ┌──────┐ Success ┌─────┐
│Auto │────────────→│Detect│────────→│GPU  │
└─────┘            └──────┘         └─────┘
                       │ Fail           │ Error
                       ↓                ↓
                   ┌─────┐         ┌─────┐
                   │CPU  │←─────────│Retry│
                   └─────┘ Fallback └─────┘
```

## Data Persistence

No changes to existing SQLite schema required. Rendering mode stored in config file only.