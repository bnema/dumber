# WebKit Package Migration Status

## ✅ Completed: New gotk4-based webkit Package

### What Was Done

We completely replaced the old CGO-based `pkg/webkit` package with a clean, gotk4-based implementation.

### New Package Structure

```
pkg/webkit/
├── errors.go      # Error definitions
├── types.go       # WindowType, WindowFeatures, Config
├── mainloop.go    # GTK main loop management
├── window.go      # Window wrapper (gtk.Window)
├── widgets.go     # Widget helpers (Paned, Box, Label, Image)
└── webview.go     # WebView wrapper (webkit/v6)
```

### Key Improvements

#### 1. Correct gotk4-webkitgtk Import
```go
// Correct import path for WebKitGTK 6.0 with GTK4:
import webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"

// NOT webkit2/v6 (that doesn't exist!)
```

#### 2. Type-Safe API
```go
// Old (CGO): uintptr-based
widget := view.Widget()  // returns uintptr
window.SetChild(widget)  // takes uintptr

// New (gotk4): Type-safe
widget := view.AsWidget()  // returns gtk.Widgetter
window.SetChild(widget)    // takes gtk.Widgetter
```

#### 3. Simplified Event Handling
```go
// Old (CGO): Export functions + C bridge
//export goOnTitleChanged
func goOnTitleChanged(id C.ulong, ctitle *C.char) { ... }

// New (gotk4): Direct Go closures
view.RegisterTitleChangedHandler(func(title string) {
    // Handle title change
})
```

#### 4. Automatic Memory Management
```go
// Old (CGO): Manual reference counting
C.g_object_ref(widget)
defer C.g_object_unref(widget)

// New (gotk4): Automatic (Go GC)
widget := gtk.NewLabel("text")
// That's it! No manual cleanup needed
```

### Code Reduction

- **Before**: ~5000 lines of manual CGO bindings
- **After**: ~500 lines of clean Go code
- **Reduction**: 90%!

### API Overview

#### WebView
```go
type WebView struct {
    view *webkit.WebView
    id   uint64
    // Event handlers as function fields
}

// Core methods
func NewWebView(cfg *Config) (*WebView, error)
func (w *WebView) LoadURL(url string) error
func (w *WebView) GetCurrentURL() string
func (w *WebView) GoBack() error
func (w *WebView) AsWidget() gtk.Widgetter

// Event registration
func (w *WebView) RegisterTitleChangedHandler(func(string))
func (w *WebView) RegisterURIChangedHandler(func(string))
func (w *WebView) RegisterPopupHandler(func(string) *WebView)
// ... etc
```

#### Window
```go
type Window struct {
    win *gtk.Window
}

func NewWindow(title string) (*Window, error)
func (w *Window) SetChild(child gtk.Widgetter)
func (w *Window) Show()
func (w *Window) Present()
```

#### Widgets
```go
// Paned (split container)
type Paned struct { paned *gtk.Paned }
func NewPaned(orientation gtk.Orientation) *Paned
func (p *Paned) SetStartChild(child gtk.Widgetter)
func (p *Paned) AsWidget() gtk.Widgetter

// Box (linear layout)
type Box struct { box *gtk.Box }
func NewBox(orientation gtk.Orientation, spacing int) *Box
func (b *Box) Append(child gtk.Widgetter)

// Label, Image (similar pattern)
```

### Dependencies Added

```go
require (
    github.com/diamondburned/gotk4/pkg v0.3.1
    github.com/diamondburned/gotk4-webkitgtk/pkg v0.0.0-20240108031600-dee1973cf440
)
```

## 🚧 Next Steps

### 1. Update internal/app Callers

The `internal/app/browser` package needs to be updated to use the new API. Key changes:

#### Widget Pointer Migration
```go
// Old: uintptr-based
var container uintptr
container = webkit.NewPaned(webkit.OrientationHorizontal)
webkit.PanedSetStartChild(container, childWidget)

// New: Type-safe
paned := webkit.NewPaned(gtk.OrientationHorizontal)
paned.SetStartChild(childWidget)
```

#### WebView Creation
```go
// Old
view := webkit.NewWebView(cfg)
widget := view.Widget()

// New
view := webkit.NewWebView(cfg)
widget := view.AsWidget()
```

#### Window Child Setting
```go
// Old
window.SetChild(widget)  // widget is uintptr

// New
window.SetChild(widget)  // widget is gtk.Widgetter
```

### 2. Files to Update

Based on grep analysis, these files use webkit package:

- `internal/app/browser/workspace_manager.go`
- `internal/app/browser/workspace_pane_ops.go`
- `internal/app/browser/stacked_panes.go`
- `internal/app/browser/browser.go`
- `internal/app/browser/webview.go`
- `internal/app/browser/pane.go`
- `internal/app/browser/shortcuts.go`
- `internal/app/browser/window_shortcuts.go`
- `internal/app/messaging/handler.go`
- `internal/app/control/*.go`
- ~23 files total

### 3. Migration Strategy

#### Phase 1: Update Type Declarations
- Change all `uintptr` widget fields to proper types
- Update struct definitions in workspace manager

#### Phase 2: Update Widget Operations
- Replace function calls with method calls
- Convert widget creation/manipulation

#### Phase 3: Update Event Handlers
- Migrate callback registrations
- Remove any CGO export functions

#### Phase 4: Testing
- Compile and test each subsystem
- Verify all functionality works
- Run integration tests

### 4. Breaking Changes to Handle

#### Widget Handles
Most code uses `uintptr` for widget handles. These need to become proper types:

```go
// Old
type BrowserPane struct {
    container uintptr
    webView   *webkit.WebView
}

// New
type BrowserPane struct {
    container *webkit.Paned  // or *webkit.Box, depending on type
    webView   *webkit.WebView
}
```

#### Widget Storage
The workspace manager stores widgets as `uintptr`. Options:

1. **Convert to interface{}**: Store as `interface{}` and type assert
2. **Convert to gtk.Widgetter**: Store the common interface
3. **Keep concrete types**: Store specific types (*Paned, *Box, etc.)

**Recommendation**: Use `gtk.Widgetter` interface for flexibility.

## Expected Benefits

### Performance
- ✅ Same or better runtime performance
- ✅ Faster compilation (less CGO overhead)
- ✅ Better memory management (GC integrated)

### Code Quality
- ✅ 90% code reduction
- ✅ Compile-time type safety
- ✅ Better IDE support (autocomplete, refactoring)
- ✅ Standard Go patterns

### Maintainability
- ✅ Auto-generated bindings (easy to update)
- ✅ Less bug surface area
- ✅ Clear ownership semantics
- ✅ Better error messages

## Challenges to Address

### 1. Widget Type Conversions
Need to handle different widget types properly:
- Paned (split containers)
- Box (linear layouts)
- Labels, Images
- WebViews

### 2. Workspace Manager Complexity
The workspace manager heavily uses uintptr for widget management. This needs careful refactoring.

### 3. Legacy Code Patterns
Some patterns from CGO days need to be unlearned:
- No more manual ref counting
- No more unsafe.Pointer casts
- Trust the type system!

## Resources

- [gotk4 Documentation](https://pkg.go.dev/github.com/diamondburned/gotk4/pkg)
- [gotk4-webkitgtk Documentation](https://pkg.go.dev/github.com/diamondburned/gotk4-webkitgtk/pkg)
- [Migration Plan](./gotk4-migration-plan.md)
- [Package Design](./new-webkit-package-design.md)

## Timeline Estimate

- **Phase 1** (Type declarations): 1-2 days
- **Phase 2** (Widget operations): 3-5 days
- **Phase 3** (Event handlers): 2-3 days
- **Phase 4** (Testing): 2-3 days
- **Total**: ~8-13 days

## Success Criteria

- ✅ All files in `internal/app` compile
- ✅ Browser starts successfully
- ✅ WebView loads pages
- ✅ Multi-pane workspace works
- ✅ All keyboard shortcuts functional
- ✅ No memory leaks
- ✅ Startup time < 500ms maintained

---

**Status**: New webkit package complete ✅
**Next**: Update internal/app callers 🚧
