# New webkit Package Design (gotk4-based)

## Package Structure

```
pkg/webkit/
├── webview.go          # WebView wrapper around gotk4-webkitgtk
├── window.go           # Window wrapper around gtk.Window
├── widgets.go          # Widget helpers (Box, Paned, Label, Image)
├── mainloop.go         # GTK main loop management
├── shortcuts.go        # Keyboard shortcut handling
├── config.go           # WebView configuration
├── memory.go           # Memory management (if needed)
├── types.go            # Common types (WindowType, WindowFeatures, etc.)
├── errors.go           # Error definitions

```

## Core Types

### WebView
```go
type WebView struct {
    view *webkit.WebView    // gotk4-webkitgtk WebView

    // Event handlers
    onScriptMessage func(string)
    onTitleChanged  func(string)
    onURIChanged    func(string)
    onPopup         func(string) *WebView
    onClose         func()

    // State
    id        uint64
    destroyed bool
    config    *Config
}
```

### Window
```go
type Window struct {
    win *gtk.Window

    // Shortcuts (if needed at window level)
    shortcuts map[string]func()
}
```

### Widget Helpers
Replace uintptr-based API with proper gotk4 types:
```go
// Instead of: func NewPaned(orientation Orientation) uintptr
func NewPaned(orientation gtk.Orientation) *gtk.Paned

// Instead of: func PanedSetStartChild(paned uintptr, child uintptr)
func (p *Paned) SetStartChild(child gtk.Widgetter)
```

## API Design Philosophy

### 1. Type-Safe Widgets
- No more `uintptr` for widgets
- Use gotk4's proper types (`gtk.Widgetter`, `gtk.Widget`, etc.)
- Compile-time type checking

### 2. Idiomatic Go
- Methods instead of functions where appropriate
- Use Go interfaces for polymorphism
- Standard error handling (no `ErrNotImplemented` stubs)

### 3. Clean Event Handling
- Use Go closures directly (no CGO export functions)
- Type-safe callbacks
- Clear handler registration

## Migration Path

### Phase 1: Core Types (This PR)
1. Remove old `pkg/webkit/`
2. Create new structure with gotk4
3. Implement WebView, Window basics
4. Implement Widget helpers

### Phase 2: Update Callers
1. Update `internal/app/browser/` to use new API
2. Convert uintptr-based code to gotk4 types
3. Update workspace manager

### Phase 3: Advanced Features
1. Shortcuts system
2. Content filtering integration
3. Memory management
4. Custom URI schemes

## Key API Changes

### Before (CGO + uintptr)
```go
// Creating WebView
view := webkit.NewWebView(cfg)
widget := view.Widget()  // returns uintptr

// Creating paned
paned := webkit.NewPaned(webkit.OrientationHorizontal)  // returns uintptr
webkit.PanedSetStartChild(paned, widget)

// Window child
window.SetChild(widget)  // takes uintptr
```

### After (gotk4)
```go
// Creating WebView
view := webkit.NewWebView(cfg)
widget := view.AsWidget()  // returns gtk.Widgetter

// Creating paned
paned := webkit.NewPaned(gtk.OrientationHorizontal)  // returns *Paned
paned.SetStartChild(widget)

// Window child
window.SetChild(widget)  // takes gtk.Widgetter
```

## Backwards Compatibility Bridge

For gradual migration, provide conversion helpers:
```go
// In internal/convert.go
func WidgetToPtr(w gtk.Widgetter) uintptr
func PtrToWidget(ptr uintptr) gtk.Widgetter
```

This allows mixing old and new code during migration.

## Benefits

### Code Reduction
- **Estimated**: ~5000 lines → ~1500 lines (70% reduction)
- No CGO bridge functions
- No export functions
- Simpler widget management

### Type Safety
- Compile-time checking
- No runtime `uintptr` casting
- Clear widget ownership

### Maintainability
- Auto-generated bindings (easy to update)
- Standard Go patterns
- Better IDE support
- Clear error messages

## Implementation Order

1. ✅ Design document
2. ⏭️ types.go + errors.go (simple types, no dependencies)
3. ⏭️ mainloop.go (GTK initialization and main loop)
4. ⏭️ window.go (basic window wrapper)
5. ⏭️ widgets.go (Paned, Box, Label, Image helpers)
6. ⏭️ webview.go (WebView wrapper with event handlers)
7. ⏭️ shortcuts.go (keyboard shortcut system)
8. ⏭️ config.go + memory.go (configuration and memory management)

## Success Criteria

- ✅ All webkit package tests pass
- ✅ Browser starts up successfully
- ✅ WebView loads pages correctly
- ✅ Multi-pane workspace works
- ✅ Keyboard shortcuts functional
- ✅ No memory leaks
- ✅ Startup time < 500ms maintained
