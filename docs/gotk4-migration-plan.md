# Migration Plan: Manual GTK4 CGO → gotk4 + Regeneration Strategy

## YES, You Can Regenerate gotk4-webkitgtk Bindings!

### Current State
- **gotk4-webkitgtk**: Last updated Jan 8, 2024 (~1 year old)
- **gotk4**: Actively maintained (last update Jul 3, 2025)
- **Status**: ✅ **WORKS WITH LATEST GOTK4** (confirmed by recent test)
- **WebKitGTK 6.0**: ✅ **SUPPORTED** (import path: `webkit/v6`)

### Good News from Research
The last comment on the compatibility issue (recent) confirms:
```go
import webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
// Works perfectly with gotk4 v0.3.2!
```

---

## Option 1: Use Existing gotk4-webkitgtk (RECOMMENDED)

### Why This Works
- Bindings were regenerated Jan 2024 with GTK4 WebKitGTK
- Compatible with latest gotk4 (v0.3.2+)
- Already supports webkitgtk-6.0
- **No need to regenerate unless you need bleeding-edge APIs**

### Migration Steps
```bash
# Add dependencies
go get github.com/diamondburned/gotk4/pkg@latest
go get github.com/diamondburned/gotk4-webkitgtk/pkg@latest

# Imports will be:
import (
    "github.com/diamondburned/gotk4/pkg/gtk/v4"
    "github.com/diamondburned/gotk4/pkg/gdk/v4"
    webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
)
```

---

## Option 2: Regenerate Bindings for Latest WebKitGTK

### When to Regenerate
- Need WebKitGTK APIs added after Jan 2024
- Want to contribute upstream improvements
- Customize bindings for your specific needs

### How to Regenerate (3 Methods)

#### Method A: Using Nix (Recommended by gotk4 maintainer)

```bash
# 1. Install Nix (if not already)
curl --proto '=https' --tlsv1.2 -sSf -L https://install.determinate.systems/nix | sh -s -- install

# 2. Clone gotk4-webkitgtk
git clone https://github.com/diamondburned/gotk4-webkitgtk.git
cd gotk4-webkitgtk

# 3. Update shell.nix to use webkitgtk_6_0 (already configured!)
# See: buildInputs includes webkitgtk_6_0

# 4. Enter Nix environment
nix-shell

# 5. Regenerate bindings
go generate

# Bindings will be regenerated with your system's WebKitGTK version!
```

#### Method B: Fork abergmeier's Updated Generator

A community member already fixed the generator:
```bash
# Clone the updated fork
git clone https://github.com/abergmeier/gotk4-webkitgtk.git -b webkitv6
cd gotk4-webkitgtk

# This fork uses gotk4-adwaita's newer genmain package
# Should work with latest WebKitGTK 6.0
```

#### Method C: Update Generator Yourself

Follow gotk4-adwaita's pattern:
```bash
# 1. Clone both repos
git clone https://github.com/diamondburned/gotk4-adwaita.git
git clone https://github.com/diamondburned/gotk4-webkitgtk.git

# 2. Compare generator.go files
# gotk4-adwaita uses newer genmain package
# gotk4-webkitgtk uses older girgen directly

# 3. Update gotk4-webkitgtk/generator.go to match adwaita pattern
# Key changes needed:
# - Import "github.com/diamondburned/gotk4/gir/cmd/genmain"
# - Use genmain.Main() instead of direct girgen calls
# - Add preprocessors for module name fixes (soup, javascriptcore)
# - Add TLS protocol version polyfill
```

---

## Migration Plan: From Manual CGO to gotk4

### Phase 1: Proof of Concept (1-2 days)

Test gotk4 compatibility with simple example:

```go
package main

import (
    "os"
    webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
    "github.com/diamondburned/gotk4/pkg/gio/v2"
    "github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func main() {
    app := gtk.NewApplication("com.dumber.test", gio.ApplicationDefaultFlags)

    app.ConnectActivate(func() {
        window := gtk.NewApplicationWindow(app)
        window.SetTitle("Dumb Browser Test")
        window.SetDefaultSize(1024, 768)

        webView := webkit.NewWebView()
        webView.LoadURI("https://example.com")

        // Type-safe, clean API!
        webView.ConnectLoadChanged(func(event webkit.LoadEvent) {
            uri := webView.URI()
            println("Loaded:", uri)
        })

        window.SetChild(webView)
        window.Present()
    })

    os.Exit(app.Run(os.Args))
}
```

**Test this builds and runs on your system!**

### Phase 2: Core Migration (2-3 weeks)

#### File-by-file replacement strategy:

1. **Window wrapper** (`pkg/webkit/window.go`)
   ```go
   // Before
   type Window struct {
       win *C.GtkWidget
   }

   // After
   type Window struct {
       win *gtk.ApplicationWindow
   }
   ```

2. **Widget helpers** (workspace_widgets_cgo.go)
   - ~1350 lines → ~300 lines
   - All `C.gtk_*` → `gtk.*` methods

3. **Event controllers** (keyboard_cgo.go)
   - No more `//export` functions!
   - Direct Go closures

4. **WebView core** (webview_cgo.go)
   - Simplified from 2800 lines
   - Type-safe signal connections

### Phase 3: Advanced Features (1-2 weeks)

- UserContentManager integration
- Custom URI schemes
- Content filtering
- Shortcuts system

---

## Key Differences: Manual CGO vs gotk4

### Memory Management
```go
// Before: Manual reference counting
C.g_object_ref(widget)
defer C.g_object_unref(widget)

// After: Automatic (Go GC integrated)
widget := gtk.NewLabel("text")
// That's it! GC handles cleanup
```

### Type Safety
```go
// Before: Runtime panics
widget := (*C.GtkWidget)(unsafe.Pointer(ptr))

// After: Compile-time safety
widget := obj.Cast().(gtk.Widgetter)
```

### Signal Connections
```go
// Before: C bridge + export
//export goOnTitleChanged
func goOnTitleChanged(id C.ulong, ctitle *C.char) {
    title := C.GoString(ctitle)
    // handle...
}

// After: Direct Go closure
webView.ConnectTitleChanged(func(title string) {
    // handle...
})
```

---

## Recommended Approach

### For Your Project (Dumb Browser):

1. **Start with existing gotk4-webkitgtk** (Jan 2024 version)
   - It's recent enough for WebKitGTK 6.0
   - Already compatible with latest gotk4
   - Proven to work (see GitHub issue)

2. **Only regenerate if** you need specific APIs added after Jan 2024
   - Use Nix method (most reliable)
   - Or use abergmeier's fork (already updated)

3. **Parallel implementation**
   - Keep `webkit_cgo` build tag for current code
   - Add `gotk4` build tag for new implementation
   - Switch when stable

### Expected Timeline

| Phase | Original Estimate | With Existing Bindings |
|-------|------------------|------------------------|
| Setup | 1-2 days | ✅ 1 day (no regen needed) |
| Core Migration | 10-14 days | 10-14 days |
| Advanced Features | 7-10 days | 7-10 days |
| Testing | 3-5 days | 3-5 days |
| **Total** | **21-31 days** | **21-30 days** |

---

## Gotcha: Known Issues (From Research)

### Fixed Issues ✅
- ❌ "Not usable with gotk4" - **FIXED** (works with v0.3.2+)
- ❌ Module name bugs (soup, javascriptcore) - **FIXED** in community forks
- ❌ Missing TLSProtocolVersion - **WORKAROUND** available

### Potential Issues
- If regenerating: Generator is outdated (use adwaita's genmain pattern)
- Nix learning curve (but Docker alternative exists)

---

## Decision Matrix

| Scenario | Recommendation |
|----------|---------------|
| **Just want to use gotk4** | Use existing gotk4-webkitgtk pkg |
| **Need latest WebKitGTK APIs** | Regenerate with Nix |
| **Want to contribute upstream** | Update generator + regenerate |
| **Avoid Nix** | Use abergmeier's fork |

---

## Final Recommendation

✅ **USE EXISTING gotk4-webkitgtk** - It works great!

Only regenerate if you:
- Have Nix experience
- Need bleeding-edge WebKitGTK APIs
- Want to contribute improvements

The Jan 2024 bindings support WebKitGTK 6.0 and work with latest gotk4. **This is sufficient for your migration.**

---

## Migration Architecture

### Current CGO Architecture
```
pkg/webkit/
├── window_cgo.go          (~60 lines - Window wrapper)
├── workspace_widgets_cgo.go (~1350 lines - Widget helpers)
├── keyboard_cgo.go        (~814 lines - Event controllers)
├── shortcuts_cgo.go       (~566 lines - Shortcut system)
├── webview_cgo.go         (~2800 lines - WebView core)
└── [15+ other _cgo.go files]

Total: ~5000+ lines of manual CGO bindings
```

### Target gotk4 Architecture
```
pkg/webkit/
├── window_gotk4.go        (~40 lines - Window wrapper)
├── widgets_gotk4.go       (~300 lines - Widget helpers)
├── events_gotk4.go        (~200 lines - Event controllers)
├── shortcuts_gotk4.go     (~150 lines - Shortcut system)
├── webview_gotk4.go       (~800 lines - WebView core)
└── [Simplified files]

Total: ~1500-2000 lines of idiomatic Go
Reduction: 60-70%
```

---

## Code Size Comparison

### Before: Manual CGO
- **Lines of Code**: ~5000+
- **C Bridge Functions**: 50+
- **Export Functions**: 15+
- **Unsafe Pointers**: Everywhere
- **Manual Memory Management**: Yes
- **Type Safety**: Runtime only

### After: gotk4
- **Lines of Code**: ~1500-2000
- **C Bridge Functions**: 0 (handled by gotk4)
- **Export Functions**: 0
- **Unsafe Pointers**: Minimal (internal to gotk4)
- **Manual Memory Management**: No (automatic)
- **Type Safety**: Compile-time

---

## Expected Benefits

### Code Quality
- ✅ 60-70% code reduction
- ✅ Compile-time type safety
- ✅ No manual memory management
- ✅ Idiomatic Go patterns
- ✅ Better IDE support

### Maintainability
- ✅ Auto-generated bindings (easy to update)
- ✅ Less bug surface area
- ✅ Standard Go error handling
- ✅ No C header dependencies in Go code

### Performance
- ✅ Same or better runtime performance
- ✅ Faster compilation (less CGO)
- ✅ Better GC integration

---

## Success Criteria

- ✅ All 23 files in `internal/app/browser/` compile with gotk4
- ✅ Startup time remains <500ms
- ✅ No memory leaks in 24h stress test
- ✅ All shortcuts work correctly
- ✅ Multi-pane workspace functions properly
- ✅ WebView lifecycle (create/destroy/reparent) stable
- ✅ Code is cleaner and more maintainable

---

## Resources

### Documentation
- [gotk4 Documentation](https://pkg.go.dev/github.com/diamondburned/gotk4/pkg)
- [gotk4-webkitgtk Documentation](https://pkg.go.dev/github.com/diamondburned/gotk4-webkitgtk/pkg)
- [gotk4 Examples](https://github.com/diamondburned/gotk4-examples)
- [GTK4 Documentation](https://docs.gtk.org/gtk4/)
- [WebKitGTK Documentation](https://webkitgtk.org/reference/webkit2gtk/stable/)

### Repositories
- [gotk4 Main](https://github.com/diamondburned/gotk4)
- [gotk4-webkitgtk Official](https://github.com/diamondburned/gotk4-webkitgtk)
- [gotk4-webkitgtk Fork (abergmeier)](https://github.com/abergmeier/gotk4-webkitgtk/tree/webkitv6)
- [gotk4-adwaita (Generator Reference)](https://github.com/diamondburned/gotk4-adwaita)

### Community
- [gotk4 Matrix Room](https://matrix.to/#/#gotk4:matrix.org)
- [GitHub Issues](https://github.com/diamondburned/gotk4/issues)

---

## Next Steps

1. ✅ **[DONE]** Research gotk4-webkitgtk compatibility
2. ✅ **[DONE]** Document migration plan
3. ⏭️ **[NEXT]** Test proof-of-concept example
4. ⏭️ Add gotk4 dependencies to project
5. ⏭️ Begin Phase 1: Core Infrastructure migration

**Ready to start Phase 1!**
