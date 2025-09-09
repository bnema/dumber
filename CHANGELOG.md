# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Fixed
- Homepage Recent History layout:
  - Show single-line entries as “{Title} – {Domain} – {URL}”.
  - Truncate long URLs with a gradient fade; prevent horizontal overflow.
  - Adjusted styling so URL is darker for readability.
- Search shortcuts and config interop:
  - Backend SearchShortcut now serializes `url`/`description` with JSON tags.
  - Frontend normalizes field casing and extracts base URLs, handling `%s` and `{query}` templates.
  - Omnibox shortcut parsing now URL-encodes queries and supports `%s` placeholders.
- Input and navigation reliability (GTK4/WebKitGTK 6):
  - Capture-phase key controller to ensure accelerators fire consistently.
  - Do not consume normal left/right clicks; only mouse buttons 8/9 trigger back/forward.
  - Added Alt+ArrowLeft/Right navigation and enabled two-finger swipe back/forward when supported.
  - Added Ctrl+scroll zoom in/out; refined accelerator matching so Ctrl− works across layouts.
  - Removed layout-specific hacks; added diagnostic logs for raw minus key events.
- Zoom persistence:
  - Added WebView zoom-changed hook; save per-domain zoom to SQLite on every change.
  - Log on successful save: “saved X.XX for domain.tld” and on load: “loaded X.XX for domain.tld”.
  - Load and log initial zoom level on startup and on every navigation.
- Build/dev ergonomics:
  - Makefile loads `.env.local`; supports overriding `GOMODCACHE`, `GOCACHE`, `GOTMPDIR` for sandboxed builds.

## [0.2.0] - 2025-09-10

### Added
- WebKitGTK 6 (GTK4) migration for the built‑in browser window:
  - WebView constructed via `g_object_new(WEBKIT_TYPE_WEB_VIEW, …)` with a fresh `WebKitUserContentManager`.
  - `WebKitNetworkSession` with persistent cookies (SQLite) and cache directories.
- Native UCM messaging restored via `script-message-received::dumber` (JSCValue*), removing the JS fallback bridge.
- Rendering mode controls:
  - CLI flag `--rendering-mode=auto|gpu|cpu` and `DUMBER_RENDERING_MODE` env.
  - In‑app status endpoint `dumb://homepage/api/rendering/status`.
- Native zoom shortcuts at the WebKit layer (Ctrl/Cmd+=, +, −, 0) and GTK4 input controllers for keyboard/mouse.
- AZERTY keyboard fix: treat underscore as minus for zoom‑out.
- Runtime theme synchronization: watch GTK `gtk-application-prefer-dark-theme` and update page color‑scheme live.
- Makefile target `check-webkit` to print `pkg-config` versions of GTK4/WebKitGTK 6.0/JSC 6.0.
- Documentation updates:
  - README now targets WebKitGTK 6 (GTK4) and lists GStreamer package requirements for Arch and Debian/Ubuntu.
  - CLAUDE notes updated with migration status, GPU/Vulkan path, runtime theme sync, and persistence.

### Changed
- Build flags and includes now target `webkitgtk-6.0`, `gtk4`, and `javascriptcoregtk-6.0`.
- GTK4 APIs for windowing and event handling (GtkEventController, GtkGestureClick) replace GTK3/GdkEvent paths.

### Removed
- Legacy WebKit2GTK constructors and container APIs.
- JavaScript fallback for UCM message bridging.

### Fixed
- Header/enum compatibility for hardware acceleration policy and cookie policy across WebKit header variants.
- Linker error for `on_theme_changed` and gpointer usage in signal connections.
- Crashy paths mitigated by default “media safe” startup (can be disabled via `DUMBER_MEDIA_SAFE=0`).

### Notes
- GPU rendering path leverages GTK4’s renderer (Vulkan/OpenGL). Mode `auto` uses GPU when available and falls back gracefully to CPU.
- WebKitGTK relies on GStreamer; install recommended packages for audio/video (see README).

## [0.1.0] - 2025-09-09

### Added
- Initial project scaffolding, build system, and dependencies.
- Unified CLI + GUI architecture with search shortcuts and fuzzy parsing.
- Wails v3‑alpha browser integration (early phase).
- Global keyboard controls with script injection (zoom/devtools, etc.).
- Native WebKit2GTK backend migration:
  - Custom `dumb://` scheme and `/api` endpoints: `/config`, `/history/recent`, `/history/search`, `/history/stats`.
  - UserContentManager omnibox injection and theme initialization.
  - Persistent WebsiteDataManager with cookies.sqlite; per‑domain zoom and title persistence.
  - Keyboard/mouse accelerators (including mouse back/forward); debounced triggers.
  - Homepage switched to first‑party API fetches; removed @wailsio/runtime usage.
- Documentation and licensing: README enhancements and LICENSE added.

### Changed
- Replaced Wails runtime with native WebKit2GTK CGO bindings and homepage API.
- Improved asset resolution and logging for the `dumb://homepage` scheme.

### Fixed
- Eliminated duplicate accelerator triggers via debouncing.
- Corrected asset resolution and URL normalization in scheme handler.
