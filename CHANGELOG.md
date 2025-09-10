# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- TLS certificate error handling with user confirmation dialog:
  - Comprehensive certificate information display (subject, issuer, validity dates)
  - Interactive warning dialog for untrusted certificate authorities
  - Certificate exception management with per-host persistence
  - Manual load triggering after user accepts certificate exceptions
  - Proper integration with WebKitGTK 6.0 network session API

### Fixed
- WebView background set to black to improve user experience during page loads

## [0.4.0] - 2025-09-10

### Added
- High-performance fuzzy cache system for dmenu history:
  - Binary serialization with memory-mapped files for zero-copy access
  - Trigram indexing (3-character sequences) for fast candidate filtering  
  - Prefix trie (compressed tree structure) for prefix matching
  - Pre-sorted entry indices for O(1) top entries retrieval
  - Smart cache invalidation with filesystem persistence and background refresh
  - Comprehensive fuzzy matching algorithms: Jaro-Winkler similarity, Levenshtein distance, tokenized matching
  - Weighted scoring system combining URL, title, recency, and visit count scores
  - XDG Base Directory Specification compliance (cache stored in state directory)
  - Atomic operations for concurrent access control
  - Dependency injection using interfaces and gomock for testing
- Purge command for cleaning up dumber data and cache files:
  - Selective purging with flags: `--database`, `--history-cache`, `--browser-cache`, `--browser-data`, `--state`, `--config`
  - Clear distinction between history cache (dmenu fuzzy search) and browser cache (WebKit)
  - Browser data includes cookies, localStorage, sessionStorage
  - Confirmation prompt with `--force` flag to skip
  - Targets all data locations: SQLite database, WebKit cache/data, dmenu fuzzy cache, configuration files
- Ctrl+Shift+C keyboard shortcut to copy current URL to clipboard:
  - Cross-platform clipboard support with wl-copy (Wayland) and xclip (X11) fallback
  - Reusable toast notification system for user feedback
  - Injectable JavaScript component for consistent styling across all websites
  - Native backend integration for reliable clipboard access

### Changed
- Dmenu performance dramatically improved: 200ms → 4ms (98% faster)
- GetTopEntries optimized from O(n log n) to O(1) using pre-sorted indices
- Cache build time reduced to ~300μs for typical history datasets

### Fixed
- Dark theme detection now properly reads GNOME desktop interface color-scheme setting (`org.gnome.desktop.interface color-scheme`) instead of only checking `gtk-application-prefer-dark-theme`
- Eliminated O(n log n) sorting bottleneck on every dmenu call
- Memory-efficient compact entry storage with minimal footprint
- Concurrent cache access with proper read/write locking
- Process no longer hangs when closing window with Cmd+Q or window close button - implemented proper GTK4 `close-request` signal handling and OS signal handling for graceful shutdown

## [0.3.0] - 2025-01-11

### Added
- WebKit memory controls (opt‑in):
  - Env vars to tune memory/perf trade‑offs without code changes:
    - `DUMBER_CACHE_MODEL`, `DUMBER_ENABLE_PAGE_CACHE`
    - `DUMBER_ENABLE_MEMORY_MONITORING`, `DUMBER_MEMORY_LIMIT_MB`, `DUMBER_MEMORY_POLL_INTERVAL`
    - `DUMBER_MEMORY_CONSERVATIVE`, `DUMBER_MEMORY_STRICT`, `DUMBER_MEMORY_KILL`
    - `DUMBER_GC_INTERVAL`, `DUMBER_RECYCLE_THRESHOLD`
  - Version‑guarded CGO wrappers for memory pressure and GC hooks; safe no‑ops on unsupported WebKitGTK builds.
 - Find in page (Ctrl/Cmd+F):
   - Omnibox refactored into a reusable component with dedicated "find" mode.
   - Highlighting: yellow for all matches; active match in orange for clarity.
   - Match list with context (right side stops at nearest punctuation: . , ; : -).
   - Navigation UX:
     - ArrowUp/Down move selection and scroll the overlay list (page scroll disabled while open).
     - Enter centers on the active match and closes; Shift+Enter goes to previous; Alt+Enter centers without closing.
     - Hover focuses input and selects list items; click outside closes the overlay.
     - Faded overlay with subtle blur when reviewing matches; opacity restores on input focus/typing.
   - Programmatic API: `OpenFind(initial)`, `FindQuery(q)`, `CloseFind()` on `WebView`.

### Changed
- Performance‑first defaults retained:
  - Cache model set to “WebBrowser”, page cache enabled, DevTools available.
  - No implicit memory pressure or GC enabled by default; all memory tuning is opt‑in.

### Removed
- Deprecated offline app‑cache calls eliminated (no‑ops) to avoid warnings and brittle builds.

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
 - WebKit CGO hardening and API compatibility:
   - Apply memory pressure settings to `WebKitNetworkSession` (correct signature) when enabled.
   - Guarded C wrappers remove deprecation warnings across header variants; builds succeed on WebKitGTK 6.

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
