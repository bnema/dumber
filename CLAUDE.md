# dumb-browser Development Guidelines

Auto-generated from all feature plans. Last updated: 2025-01-21

## Active Technologies  
- Go 1.25.1 + WebKitGTK 6.0 (GTK4), Cobra, Viper, SQLite3 (004-migrate-to-webkitgtk)
- CGO bindings for WebKitGTK 6.0 C API integration
- GPU rendering via Vulkan/OpenGL with configurable modes

## Project Structure
```
pkg/
└── webkit/       # WebKit2GTK Go bindings (extractable)

src/
├── models/       # Data models and entities
├── services/     # Business logic services  
├── cli/         # Command line interface
└── lib/         # Shared libraries

tests/
├── contract/    # Contract tests for WebKit bindings
├── integration/ # Integration tests with WebKit2GTK
└── unit/        # Unit tests
```

## Commands
```bash
# Testing and linting
make test
make lint

# Build application
make build

# Development
make dev

# Generate code (SQLC)
make generate

# Database operations
sqlite3 ~/.config/dumber/browser.db
```

## Code Style
Go 1.25.1: Follow standard Go conventions with gofmt
- Use structured logging (slog)
- Proper error handling (no panics)
- SQLC for type-safe database queries  
- CGO memory management with Go finalizers
- Constitutional simplicity principles

## Recent Changes
- 004-migrate-to-webkitgtk: Migrating to WebKitGTK 6.0 (GTK4) for Vulkan GPU rendering support (UCM restored, GTK4 input, persistent cookies/cache)
- 003-webkit2gtk-migration: Migrated from Wails to native WebKit2GTK with custom CGO bindings
- 002-browser-controls-ui: Added keyboard/mouse controls, zoom persistence, clipboard integration

## WebKitGTK 6.0 Integration
- pkg/webkit: Reusable WebKitGTK 6.0 Go bindings (GTK4)
- Hardware acceleration via Vulkan/OpenGL with configurable rendering modes
- Direct GTK4 event handling with GtkEventController
- WebKitNetworkSession replaces WebKitWebContext
- Rendering modes: auto (default), gpu, cpu via --rendering-mode flag

<!-- MANUAL ADDITIONS START -->
### Migration Status (004-migrate-to-webkitgtk)
- WebView construction via `g_object_new(WEBKIT_TYPE_WEB_VIEW, …)` (GTK4).
- Native UCM messaging (script-message-received::dumber) replaces fallback bridge.
- Persistent storage: cookies (SQLite) and cache via `WebKitNetworkSession`.
- Runtime theme sync: listens to GTK `gtk-application-prefer-dark-theme` and updates page color-scheme live.
- Native zoom shortcuts + GTK4 key/mouse controllers (including AZERTY fix for zoom-out).
- Media: GStreamer required; default “safe” startup demotes VAAPI and uses a safe sink to avoid crashes (can be disabled).

### GPU Rendering
- GTK4 renderer can use Vulkan; WebKit settings map `auto|gpu|cpu` to the appropriate hardware acceleration policy where available.
- Status endpoint: `dumb://homepage/api/rendering/status` reports current configured mode.

### Next
- Expose cookie policy/media mode toggles in config/CLI.
- “Clear browsing data” surfaced via CookieManager/WebsiteDataManager.
<!-- MANUAL ADDITIONS END -->
