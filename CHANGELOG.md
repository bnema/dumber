# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- **Omnibox initial history display**: Configurable behavior when opening empty omnibox via `omnibox.initial_behavior` ("recent", "most_visited", "none"). Shows initial history on open and when input is cleared. Default: recent visits.

### Fixed
- **Omnibox flash on load**: Load the injected GUI bundle at document-start and initialize the omnibox as soon as DOM is ready, ensuring it is ready without briefly flashing during page startup or new pane creation.

## [0.14.1] - 2025-11-18

### Added
- **Inline tab rename**: Press `Alt+T` then `r` to rename tabs directly in the tab bar. Enter saves, Escape cancels. Tab buttons temporarily allow focus during rename, then restore original focus behavior to prevent interfering with WebView keyboard input.
- **Cloudflare Turnstile CAPTCHA support**: Temporary CORS allowlist workaround for `challenges.cloudflare.com` to bypass WebKit's incomplete COEP credentialless implementation. Allows Turnstile-protected sites to function properly. Configurable via `enable_turnstile_workaround` (default: true).
- **Global content filtering toggle**: New `content_filtering.enabled` config option to completely disable ad blocking when needed. Whitelist system remains available for granular control. Defaults to enabled for backward compatibility.

### Fixed
- **Search shortcut placeholders**: Support both `{query}` and `%s` placeholder formats in search shortcuts, allowing users to use either style. Also fixed domain-only URLs (e.g., "github.com") being incorrectly treated as search queries.
- **OAuth popup handling**: Fixed OAuth popups not functioning in stacked panes and split operations by ensuring `setupPopupHandling()` is called in all pane creation code paths (finalizeStackCreation, CloseStackedPane, splitNode).
- **WebView ID message parsing**: Fixed JSON unmarshaling errors when frontend sends numeric webviewId values instead of strings. Message handler now gracefully normalizes both string and numeric webviewId formats with fallback parsing logic.
- **Video codec fallback**: Unblocked VP8 codec to allow fallback for ad segments, preventing playback interruptions when ads use different codecs than main content.
- **Video buffering**: Increased buffer sizes (64 MB, 20s) to handle bursty Twitch/AV1 segments and improve streaming reliability on variable bitrate platforms.

### Changed
- **Console output cleanup**: Disabled WebKit console.log() messages to stdout to prevent page script spam. DevTools console (F12) still shows all messages.
- **Content filtering configuration**: Refactored `content_filtering_whitelist` into structured `content_filtering` config with `enabled` boolean and `whitelist` array for better organization.

## [0.14.0] - 2025-11-18

### Added
- **Zellij-style tab system**: Each tab contains an independent workspace with panes, splits, and stacks. `Ctrl+T` enters tab mode with modal keyboard control (`n`=new, `x`=close, `l/h`=navigate). `Alt+1-9/0` for direct tab switching. `Ctrl+Tab`/`Ctrl+Shift+Tab` for next/previous tab. Tab bar auto-hides with single tab. Orange border indicator (distinct from blue pane mode).
- **International keyboard layout support**: Hardware keycode-based shortcuts for `Alt+number` keys work across all keyboard layouts (QWERTY, AZERTY, QWERTZ, etc.). Physical key position used instead of character mapping.
- **Click-to-activate stacked panes**: Click on any collapsed title bar to instantly activate that pane. Uses GTK4 gesture clicks for native interaction feel.
- **Title bar visual separators**: Subtle 1px borders between stacked title bars. Theme-aware colors automatically adjust for light/dark mode.
- **UI scaling for title bars**: New `workspace.styling.ui_scale` config option (default: 1.0). Scales font size, padding, height, and favicon size. Range: 0.5 to 3.0 (e.g., 1.2 = 120% size).
- **Favorites system**: Full favorites management with keyboard controls and persistent storage. Press `Tab` to switch between History and Favorites views. Press `Space` to add/remove items. Yellow border indicates favorited items in History view. View mode persists across sessions. Database-backed with RAM-first caching for performance.
- **Omnibox quick navigation**: Press `Ctrl+1` through `Ctrl+9` to instantly navigate to the first 9 results, or `Ctrl+0` for the 10th. Works in both History and Favorites views. Visual number hints appear on each item. Keyboard layout independent (works on QWERTY, AZERTY, QWERTZ, etc.).
- **Middle-click and Ctrl+click link handling**: Links can now be opened in new workspace panes using standard browser gestures (middle-click or Ctrl+left-click). New panes split in the configured direction (default: right) and respect `workspace.popups.placement` configuration. Implemented via WebKit's `decide-policy` signal to intercept navigation actions before they occur.
- **Configurable pane border styling**: Added `workspace.styling.inactive_border_width`, `workspace.styling.inactive_border_color`, and `workspace.styling.show_stacked_title_border` config options. Active and inactive borders default to 1px to prevent layout shift, with only color changing on focus. Borders use GTK theme variables by default and support smooth CSS transitions.
- **Configurable default search engine**: Added `default_search_engine` config option with explicit URL template format. DuckDuckGo is now the default (was Google). Users can set any search engine with `%s` placeholder (e.g., `default_search_engine = "https://duckduckgo.com/?q=%s"`).

### Changed
- **Target="_blank" and gesture link behavior**: Links with `target="_blank"`, Ctrl+click, and middle-click now open in stacked panes mode by default instead of split-right. New config option `workspace.popups.blank_target_behavior` (values: "split", "stacked", "tabbed") controls this behavior separately from JavaScript popups. Prevents browser exit when closing parent pane by marking _blank panes as popup-type.
- **Config format**: TOML is now the default format (JSON/YAML still supported). Action bindings inverted to `action→keys` structure for maintainability with O(1) runtime lookup. Added comprehensive validation.
- **Code cleanup**: Removed dead code files and emojis from GUI modules
- **Bridge architecture**: Unified main-world to isolated-world bridge system with comprehensive documentation. Single dispatcher for all window function bridges (omnibox, favorites, toasts). Better separation between CustomEvents (shared between worlds) and window functions (require bridging).
- **Omnibox spacing**: Increased vertical spacing between suggestion items from 0.65rem to 0.85rem for better readability
- **Smart Escape key**: First press clears input text, second press closes omnibox. Empty input closes immediately.

### Fixed
- **Ctrl+Tab shortcuts**: Fixed focus cycling behavior, now properly switches between tabs
- **Active tab visibility**: Fixed identical styling - active tab now clearly visible with distinct gray shade
- **AZERTY keyboard support**: Alt+number shortcuts now work on all keyboard layouts using hardware keycodes
- **Keyboard layout detection**: Fixed always detecting "en" - now correctly detects fr/de/etc layouts
- **Zoom visual glitch**: Fixed zoom level for next page being applied before navigation completed, causing visible zoom change on current page. Zoom now applies on load-committed event instead of URI change.
- **Toast notifications**: Fixed toast notifications not appearing for copy URL (Ctrl+Shift+C) and clipboard operations. Event name was `dumber:showToast` but ToastContainer listens for `dumber:toast`.
- **Page reload shortcuts**: Restored `Ctrl+R`, `Ctrl+Shift+R`, `F5`, and `Ctrl+F5` shortcuts. They were registered but not actually wired to the window-level handler.
- **Bootstrap initialization**: Fixed omnibox component not mounting due to invalid toast function check in isolated world. Toast functions now bridge correctly via CustomEvents, allowing omnibox to initialize properly.
- **Empty state display**: History view now hides empty state when input is empty, showing it only when user has typed and no results are found. Favorites view always shows empty state with helpful hints.

## [0.13.0] - 2025-10-24

### Added
- **Color scheme config**: `appearance.color_scheme` setting to force dark/light theme (`"prefer-dark"`, `"prefer-light"`, `"default"`)
- **DNS prefetch**: Added capability to prefetch DNS for links to speed up future requests
- **Page cache (bfcache)**: Enabled WebKit back/forward cache for instant navigation
- **Smooth scrolling**: Enabled WebKit smooth scrolling for better UX
- **JSON Schema**: Auto-generated config schema for IDE autocompletion and validation
- **Parallel cache loading**: Zoom, certificate, and fuzzy caches load concurrently at startup (<100ms target)
- **Certificate validation cache**: In-memory cache for TLS certificate decisions with graceful shutdown flushing

### Changed
- **Database performance**: Enabled WAL mode with optimized PRAGMAs (64MB cache, memory-mapped I/O, 5s busy timeout) for 70k reads/sec, 3.6k writes/sec
- **Zoom performance**: In-memory cache eliminates database reads during page transitions
- **History writes**: Batched writes with background processor (flush every 5s or 50 items) to reduce I/O overhead
- **BREAKING: Keyboard shortcuts**: All shortcuts now use `ctrl+` instead of `cmdorctrl+`. Existing configs with `cmdorctrl` still work via backward compatibility. Delete `~/.config/dumber/config.json` to regenerate with new defaults.
- **Cache management**: Graceful cache flushing on shutdown prevents data loss; startup timing instrumentation with performance warnings

### Fixed
- **Graceful shutdown deadlock**: Resolved GTK main loop deadlock on Ctrl+Q by removing mutex locks and GTK calls after main loop exit
- **CSS provider error handling**: Now properly handles errors from GTK CSS provider with logging
- **Cache operations**: Added context timeouts to cache flush and history queue operations for graceful failure
- Command typos now show proper error instead of opening in browser
- History queue now uses buffered channel to prevent blocking on shutdown

## [0.12.1] - 2025-10-18

### Added
- **Favicon refactoring**: WebKitGTK FaviconDatabase as single source of truth, removing fragile file-based cache and ico/bmp conversion dependencies
  - Native favicon detection and automatic PNG export at 32x32 for CLI (dmenu)
  - SVG favicon rasterization support
  - Async texture loading with callbacks for non-blocking UI updates
  - FaviconService handles favicon collection across all panes

### Changed
- Removed old favicon cache system (go-ico, go-bmp dependencies)
- Simplified omnibox favicon handling: uses database favicon_url only

### Fixed
- Mouse button navigation: Added support for hardware mouse buttons 8/9 (back/forward)
- Touchpad gestures: Two-finger horizontal swipe for back/forward navigation
- Web app shortcuts: Ctrl+C, Ctrl+V, Ctrl+A now work in Gmail, VSCode web, etc.
- Dark mode flash on startup: Set GTK dark theme preference and WebView background color before window/content creation
- Cmd+Q and window close button now trigger graceful shutdown instead of killing the browser process
- WebView cleanup: Properly destroy WebViews to prevent memory leaks and duplicate handler registrations when closing stacked panes
- CLI help: Running `dumber` without arguments now shows available commands (use `--gui` or `browse` to launch GUI)

## [0.12.0] - 2025-10-16

### Added
- **Native favicon support**: WebKit FaviconDatabase integration with automatic detection and caching for all panes
- ENV=dev support to isolate test builds from production config/data in .dev/dumber/ directory
- Content filtering whitelist config (Twitch enabled by default)
- `dumber config` command to open config file in $VISUAL/$EDITOR or print path with `--path`
- **Native WebKit popup lifecycle**: Implemented WebKit's create/ready-to-show/close signals for proper popup management, eliminating manual WebView creation that bypassed WebKit's internal architecture
- **Popup behavior configuration**: Added `popup_behavior` config with four modes: `split` (default), `stacked`, `tabbed`, and `windowed` for user control over popup placement
- **Backend-driven pane mode**: Complete rewrite of pane mode from JavaScript to Go backend with native keyboard handling. Ctrl+P enters mode, x/l/r/d/u/arrows for actions, Escape exits. Blocks all other keys during mode.
- **Pane mode visual border**: Zellij-style border around workspace root when pane mode is active, replacing old toast notification system. Uses GTK margins with configurable `pane_mode_border_color` (defaults to #FFA500 orange). Window background color shows through margins to create visible border effect

### Changed
- **Popup architecture refactoring**: Removed ~280 lines of JavaScript window.open interception in favor of WebKit's native popup signals. Popups now follow WebKitGTK's expected lifecycle instead of being intercepted and manually created
- **GUI bootstrap refactoring**: Extracted 400+ lines of common initialization logic into reusable bootstrap.ts module shared between injected pages and special schemes (homepage, etc.)
- **Workspace pane mode**: Moved ~450 lines of JavaScript state machine to Go backend for more reliable keyboard handling and eliminated race conditions between webviews

### Fixed
- **TLS certificate validation restored**: Re-implemented persistent certificate error handling (broken during gotk4 migration)
  - Interactive three-option dialog: "Go Back", "Proceed Once (Unsafe)", and "Always Accept This Site"
  - SQLite-based hostname decision storage (hostname-only matching - GIO certificate properties are unstable)
  - Smart expiration: "Proceed Once" temporary (not stored), "Always Accept" persists for 30 days
  - Automatic cleanup of expired validations on application startup
  - **Known issue**: Accepted certificates not pre-loaded into WebKit session on startup - stored decisions only apply when TLS error occurs again
- **Content blocking restored**: Re-implemented WebKit native ad/tracker blocking (broken during gotk4 migration) using UserContentFilterStore API
- **Script injection spam**: Fixed 100+ duplicate "Sending color palettes" messages by injecting scripts only in top frame instead of all iframes
- **Favicon race conditions**: Eliminated duplicate downloads and file handle errors via mutex protection and proper handler registration
- **Homepage theme switching**: Fixed color palette application for proper light/dark mode transitions
- **Workspace pane borders**: Hide active pane border when only one pane exists to match Zellij UX
- Enabled missing WebKitGTK6 features: WebRTC, MediaSource, LocalStorage, WebAudio, MediaStream, Clipboard
- Cosmetic filter duplicate injection in frames
- Config file duplicate keys (snake_case/camelCase) by using Viper's SafeWriteConfigAs
- Config not reading newly created default config file
- Config writing to disk on every load/reload
- **Popup SIGSEGV crashes**: Eliminated segmentation violations during popup lifecycle by respecting WebKit's signal-based popup management. Fixes OAuth popup crashes and GTK bloom filter corruption
- **GTK focus event duplicates**: Added timestamp-based deduplication to prevent multiple focus enter/leave events in the same millisecond from WebKitGTK nested widgets
- **Popup close behavior**: Fixed app exit when closing popups by excluding them from remaining pane count check
- **Pane mode border cleanup**: Fixed border persisting after pane splits by saving container reference when applying margins. Ensures proper cleanup even when workspace tree structure changes during split operations
- **Stacked pane active border**: Fixed active border incorrectly appearing on stacked pane containers instead of individual panes
- **Stacked pane creation**: Fixed premature Realize() call causing widget lifecycle issues during stack creation
- **Browser exit on last pane close**: Fixed browser not properly quitting when closing the last webview via Ctrl+P x by calling GTK main loop quit instead of window close
- **Zoom level persistence restored**: Fixed per-domain zoom settings not being saved to database (broken during gotk4 migration). Added missing `notify::zoom-level` signal connection in setupEventHandlers to properly persist zoom changes

## [0.11.0] - 2025-10-07

### Added
- **Auto-open omnibox on blank panes**: Omnibox automatically opens when a new pane is created with about:blank, providing immediate search/navigation access
- **Configurable default zoom**: Added `default_zoom` config (defaults to 1.2/120%). Per-domain zoom still overrides this
- **Pin/favorite sites**: Pin websites from history or quick access to prioritize them in "Jump back in" section. Uses localStorage persistence with star icons for pin/unpin actions
- **Zellij-style stacked panes**: Added Ctrl+P → 's' to stack panes instead of splitting, with Alt+Up/Down navigation between stacked panes. Features collapsed page title bars showing  for inactive panes and full WebView interaction for the active pane. Includes GTK4 CSS styling for proper visual feedback
- **Stacked pane favicons**: Title bars display site favicons with async caching. Click title bars to switch between stacked panes
- **Color palette configuration**: Light and dark color palettes now configurable in `appearance` config with semantic tokens (background, surface, text, muted, accent, border). Injected as CSS custom properties at document start
- **Search shortcut badges**: Visual badges in omnibox input showing active shortcut prefix (e.g., "gh:", "npm:") with description labels
- **Search shortcuts API**: Frontend bridge to fetch search shortcuts from backend `/api/config` endpoint with automatic normalization

### Changed
- **Terminal-style UI theme**: Monospace fonts (JetBrains Mono, Fira Code), sharp corners (no border-radius), dashed borders, adjusted shadows and spacing throughout omnibox components
- **History suggestions**: Use favicon from database when available, omit automatic search shortcuts from suggestions (rely on explicit prefix commands)
- **Search shortcuts defaults**: Updated defaults to include ddg, go, mdn, npm packages while reordering others
- **Chrome user agent**: Updated to version 141.0.0.0 for better site compatibility
- **CSS color system**: Removed hardcoded theme colors, now use dynamic CSS custom properties from Go-injected palette
- **History item layout**: Removed "• domain" from titles, now highlight domain within the full URL using same color as title text
- **Quick access logic**: Removed 20-item limit, now shows all sites with 2+ visits (pinned sites bypass visit requirement)
- **History pagination**: Show 15 items initially (up from 10) with "Show N more" button
- **Homepage layout**: Redesigned with improved visual hierarchy and dynamic color tokens
- **Favicon handlers**: All webviews now register favicon change handlers for consistent caching across panes

### Fixed
- **Mobile URL overflow**: Fixed long URLs breaking layout on narrow screens by adding proper width constraints and CSS fixes
- **GTK shortcut GLib-CRITICAL error**: Fixed NULL string assertion in shortcut callback system
- **SVG favicon handling**: Skip SVG favicons to prevent conversion errors (no raster conversion dependencies)
- **Focus management architecture**: Refactored workspace focus system from `active` field to `currentlyFocused` with dedicated FocusManager for improved reliability and consistency across pane operations
- **Stacked pane focus conflicts**: Fixed focus stealing issues in stacked panes by implementing focus-aware navigation that preserves active pane when spawning siblings
- **WebView title updates**: Enhanced webview title change handling to properly update stacked pane title bars in addition to database storage
- **GTK4 widget lifecycle**: Improved widget parenting/unparenting operations with proper GTK4 validation and automatic cleanup to prevent critical warnings
- **Stacked pane close behavior**: Fixed remaining pane becoming non-interactable (gray background) when closing stacked panes via Ctrl+W or Ctrl+P+X. Added proper widget visibility restoration and focus management during stack-to-regular conversion
- **Stacked pane title updates**: Fixed collapsed panes not showing current page titles when switching between stacked panes. Now follows Zellij-style layout where hidden panes display their up-to-date page titles in title bars with correct positioning above and below the active pane

## [0.10.0] - 2025-09-24

### Added
- **Print functionality**: Added Ctrl+Shift+P shortcut to open native print dialogs from any pane. Ctrl+P is blocked at WebKit level to prevent conflicts with pane mode shortcuts
- **WebKit native favicon detection**: Implemented WebKit's built-in favicon database for automatic favicon detection and URI storage. Enhanced favicon support for all formats (PNG, SVG, ICO) without external dependencies

### Changed
- **Homepage redesign**: Brutal design with component-scoped styling, dynamic height matching, inline domain-URL layout with fade effects, and tighter spacing

### Fixed
- **WebView teardown and memory management**: Implemented comprehensive cleanup system to prevent crashes during WebView destruction. Added JavaScript teardown functionality with cleanup handlers for all event listeners, intervals, and global state. Enhanced pane cleanup with proper controller detachment and GTK widget detachment sequencing. Added WebView validity checks before JavaScript evaluation and improved workspace manager pane closing logic with better popup handling
- **Ctrl+W pane closing**: Fixed Ctrl+W shortcut to properly close active panes including popups. Implemented active WebView detection and proper popup vs regular pane handling via OnWorkspaceMessage for consistent cleanup
- **WebView script injection segfault**: Fixed segmentation violation when injecting JavaScript into destroyed WebViews during message handling. Added IsDestroyed() safety check to prevent GTK widget corruption
- **Alt+Arrow workspace navigation**: Fixed Alt+Arrow keys not working for pane navigation after WebView focus changes. Implemented C-level to GTK4 shortcut bridge that ensures workspace navigation works consistently across all panes
- **Global shortcuts protection**: Fixed window shortcuts (Ctrl+L, Ctrl+F, F12) not working in new panes due to overly aggressive webpage protection. Window shortcuts now properly bubble up to GTK while pane shortcuts remain blocked from webpages. Resolves omnibox and developer tools not responding to keyboard shortcuts in newly created panes
- **Omnibox keyboard event isolation**: Fixed keyboard event leakage from omnibox to underlying webpages that caused unintended page actions (e.g., typing 's' in omnibox triggering GitHub search). Implemented WebKit-level main-world event blocking that activates when omnibox opens, preventing page JavaScript from receiving keyboard events while preserving native GTK shortcuts (Ctrl+L, Ctrl+F, etc.) and omnibox functionality
- **Active pane detection**: Replaced complex pane ID abstraction with persistent webview IDs, fixing omnibox unresponsiveness after navigation. Omnibox now works immediately after page navigation without requiring mouse movement
- **Workspace architecture**: Simplified focus tracking by removing URI change handlers and pane ID injection complexity, improving reliability and reducing codebase by 85 lines
- **WebView ID propagation**: Added webview ID request/response mechanism for stub builds to ensure JavaScript side knows its webview ID for proper focus validation
- **Workspace theme integration**: Fixed white borders on inactive panes by implementing dynamic border colors based on GTK theme preference (dark mode: #333333 borders, light mode: #dddddd borders)
- **GTK window background**: Added theme-aware window and pane background colors to prevent white bleeding through transparent elements
- **Print dialog window handling**: Fixed GTK window destroy errors during WebView cleanup by only destroying windows when CreateWindow was actually enabled
- **Modifier key logging noise**: Filtered out Ctrl, Shift, Alt, and Super key presses from accelerator miss logging to reduce console spam

## [0.9.0] - 2025-09-22

### Added
- **Zellij-style workspace management**: Complete pane splitting system with binary tree layout, focus tracking, and Zellij-inspired keybindings (Ctrl+P for pane mode, arrow keys for splits, 'x' to close)
- **Multi-pane WebView architecture**: Container-based widget management with proper reparenting, lifecycle handling, and per-pane controllers
- **Window-level global shortcuts**: Centralized shortcut handling to prevent conflicts between multiple WebView instances
- **Workspace configuration**: New config section with Zellij controls toggle, pane mode bindings, tab shortcuts, and popup behavior settings
- **Configurable workspace styling**: Pane border appearance (width, color, transition duration, radius) now customizable via config with proper viewport stability
- **Popup handling for tiling WM**: Complete popup window support where popups open as workspace panes instead of floating windows, respecting tiling window manager design principles
- **Universal popup auto-close**: Popup windows (OAuth, window.open(), etc.) automatically close when JavaScript calls window.close(), providing standard browser behavior adapted for workspace panes
- **Advanced popup deduplication system**: Intelligent duplicate popup prevention with SHA256 fingerprinting and 200ms debounce window to eliminate popup spam
- **OAuth auto-close integration**: RFC 6749 compliant OAuth callback detection with automatic popup closure for seamless authentication flows
- **Cross-WebView popup communication**: localStorage-based parent-popup bridge enabling proper `window.opener` functionality in tiling workspace environment
- **WebView identity tracking**: Unique WebView ID system for precise popup targeting and enhanced debugging capabilities

### Improved
- WebKit integration now forces GPU compositing by default while still honoring explicit config overrides.
- Smooth scrolling is enabled (when supported) so wheel and gesture animations feel closer to Chromium/Firefox.
- Title updates, zoom persistence, and domain zoom lookups are now done off the GTK main thread with results marshalled back safely, eliminating UI hitches during navigation.
- Added optional DOM-level zoom (enabled by default) to avoid native viewport rescaling; falls back to WebKit zoom if the CSS path fails.
- DOM zoom now seeds the saved level before the first paint and reuses it during document-start scripts, preventing pages from flashing back to 100%.
- Native accelerator bridge debounces duplicate GTK key events earlier, so Ctrl±/Ctrl+0 only fires once per physical key press.
- Omnibox-triggered navigations now reuse the navigation controller, so saved zoom levels are applied before first paint and redundant lookups are avoided.
- Configuration loading writes the missing `use_dom_zoom` flag into existing config files so DOM zoom defaults persist without manual edits.
- Omnibox UI now relies on shared Tailwind theme tokens and the injected color-scheme service for light/dark parity.
- Omnibox overlay now self-heals if hostile pages remove the injected shadow host, ensuring Ctrl/Cmd+L works everywhere.
- Navigation shortcuts changed from Alt+Arrow to Ctrl/Cmd+Arrow for better workspace compatibility.
- GUI components now support workspace-aware focus management with pane-specific event handling.
- **Global zoom shortcuts**: Unified zoom handling at window level (Ctrl+/=/0/-) ensuring zoom applies to currently active pane in multi-pane workspace
- **Workspace focus management**: Enhanced focus throttling (100ms) and active state tracking to prevent infinite loops and conflicts between panes
- **Popup management**: Intelligent popup vs tab detection based on window features, with improved auto-close logic for OAuth and login flows
- **Console logging**: WebView ID context injection for better debugging and log correlation across multiple panes

### Changed
- **Zoom shortcuts architecture**: Moved from per-WebView registration to centralized window-level handling for consistency with other global shortcuts

## [0.8.0] - 2025-09-17

### Added
- **GTK color scheme bridge**: Dedicated document-start module keeps WebKit theme in sync with GTK preferences and exposes unified runtime updater
- **Color scheme build pipeline**: Separate Vite entry produces reusable `color-scheme.js` asset shared between native and injected contexts
- **Complete Svelte 5 GUI migration**: Omnibox and find components migrated from raw JavaScript to modern Svelte 5 with runes
- **Tailwind CSS v4 integration**: Unified design system with PostCSS and JIT compilation
- **TypeScript keyboard service**: Centralized shortcut management with full type safety
- **Click-anywhere-to-close**: Omnibox closes when clicking outside for better UX
- **Homepage Svelte 5 component**: New standalone homepage with history display and keyboard shortcuts reference
- **Page generator system**: Vite plugin for generating dumb:// protocol pages (homepage, config, about) with extensible architecture
- **History deletion API**: New DELETE endpoint at `/history/delete?id=N` for removing individual history entries
- **Database migrations**: Embedded migrations now run automatically at startup to keep sqlite schema in sync
- **Inline CSS type declarations**: Ambient module definitions for raw CSS imports improve TypeScript ergonomics in injected bundles
- **Global Shadow DOM system**: Unified shadow host utility for component isolation with shared ShadowRoot and CSS reset management
- **Unified page-world bridge**: Integrated API bridge with event-driven communication between page-world and isolated-world contexts

### Improved
- **Asset MIME detection**: Expanded scheme handler lookup covers fonts, images, and JSON to prevent incorrect `text/plain` responses
- **Toast theming**: Runtime theme observer syncs toast styling with the browser's dark mode state for consistent visuals
- **Shadow host styling**: Injected GUI now adopts Tailwind design tokens inside the shared shadow root with automatic dark-mode mirroring
- **Omnibox/Find system**: Complete migration from 500-line raw JavaScript to modular Svelte 5 components
- **Toast notifications**: Migrated from raw JavaScript to Svelte 5 components with proper animations
- **GUI build pipeline**: Vite-based bundling with hot reload and TypeScript support
- **Keyboard shortcuts**: Fixed Ctrl+F not working by adding missing 'f' key to keyboard dispatcher
- **State management**: Proper cleanup between omnibox and find modes prevents state conflicts
- **Homepage UX**: Enhanced layout with 70/30 flex ratio, compact keyboard shortcuts table, and statistics display
- **Interactive delete**: History entries show trash icon on hover with smooth fade-out animation on deletion
- **Top 5 most visited**: New section displaying frequently accessed sites with visit counts
- **Performance**: Instant shortcut response with pre-mounted DOM elements

### Changed
- **Homepage icons**: Replaced Lucide dependency with inline SVG/Unicode fallbacks to shrink bundle size while debugging upstream build issues
- **Major architecture shift**: Removed HTTP API routes in favor of direct WebKit message bridge communication for better performance and reduced complexity
- **API token support**: Added optional API security configuration with token-based authentication system
- **Shadow DOM improvements**: Enhanced isolation with WeakSet tracking to prevent duplicate reset injection and global shadow host utility for component sharing
- **Message handler refactoring**: Unified keyboard shortcut forwarding between native handlers and GUI components with DOM event bridge
- **Omnibox suggestions**: Native Go-based suggestion computation with search shortcuts and history integration, eliminating HTTP fetch dependencies
- **Toast styling**: Migrated from Tailwind CSS classes to inline styles within component scope for better Shadow DOM compatibility and isolation
- **JS to Svelte migration**: Replaced legacy inline injected scripts (see `pkg/webkit/ucm_js_cgo.go.deprecated`) with structured Svelte components mounted via the shared shadow root.

### Fixed
- **Service worker residue**: Homepage clears stale caches and unregisters old service workers to avoid 404s for legacy `_app` assets
- **Keyboard debounce**: Native accelerator bridge filters duplicate shortcut events within 120ms for DOM and CGO handlers
- **Ctrl+F shortcut**: Added missing 'f' key case in keyboard_cgo.go dispatcher
- **Browser freeze**: Removed infinite loop in mode switching reactive effects
- **State persistence**: Proper cleanup when switching between Ctrl+L and Ctrl+F modes
- **Homepage protocol**: Fixed dumb://homepage not working after GUI migration with proper Svelte 5 component and API integration
- **Omnibox positioning on homepage**: Fixed omnibox stuck in top-left corner by isolating homepage CSS styles and strengthening omnibox positioning with explicit fixed positioning and CSS isolation
- **Find mode persistence**: Fixed find view persisting after pressing Esc when reopening with Ctrl+L by ensuring toggle() resets mode to 'omnibox'
- **Favicon display**: Fixed missing favicons in homepage history after Svelte 5 migration by adding FaviconURL field to API response and implementing proper fallback display logic
- **Homepage API communication**: Migrated from HTTP fetch to WebKit message bridge for all homepage API calls (history, stats, search) with proper callback handling and timeout management
- **Omnibox accessibility**: Added keyboard navigation support with Tab/Enter activation for suggestion items and proper ARIA attributes for screen readers
- **Omnibox suggestions race conditions**: Fixed timing issues where suggestions arrived before API was ready by implementing pending suggestions queue and proper event bridging between page-world and isolated-world contexts

## [0.7.0] - 2025-09-14

### Added
- **Content blocking system**: Native WebKit ad blocking with EasyList integration, cosmetic filtering, and filter cache management
- **Filter cache purging**: New `--filter-cache/-F` flag in purge command
- **WebKit debug system**: Comprehensive debugging support with configurable categories for troubleshooting WebKit internal errors
- **Enhanced error handling**: Robust content filter injection with retry mechanisms and graceful fallbacks
- **Parallel compilation**: Multi-core GCC compilation support for faster builds

### Improved
- **Content filter timing**: Optimized script injection timing to prevent interference with WebKit preconnect operations
- **Anti-breakage scriptlets**: Safer JavaScript modifications that avoid WebKit internal conflicts
- **Build system**: Added parallel compilation with automatic core detection and success indicators
- **Persistent TLS certificate validation system**:
  - **Three-option certificate dialog**: "Go Back", "Proceed Once (Unsafe)", and "Always Accept This Site"
  - **Database-backed certificate storage**: SQLite-based persistence for user certificate decisions with SHA256 certificate hashing
  - **Smart decision handling**: Automatic application of stored decisions without repeated prompts
  - **Temporary vs permanent choices**: "Proceed Once" expires after 24 hours, "Always Accept" persists indefinitely
  - **Seamless user experience**: No more repetitive certificate warnings for trusted sites
- **Performance-optimized WebKit memory configuration system**:
  - **New `WebkitMemoryConfig`** configuration section with performance-focused defaults for faster page loading
  - **Aggressive caching strategy**: `web_browser` cache model with page cache enabled for instant back/forward navigation
  - **Balanced memory pressure management**: Conservative (40%), strict (60%), and kill (80%) thresholds optimized for performance vs stability
  - **Smart garbage collection**: 2-minute JavaScript GC intervals with process recycling after 50 page loads
  - **Reduced monitoring overhead**: 45-second memory monitoring intervals to minimize performance impact
  - **Environment variable overrides**: All memory settings configurable via `DUMBER_*` environment variables (`DUMBER_CACHE_MODEL`, `DUMBER_MEMORY_CONSERVATIVE`, etc.)
- **Early crash handling initialization**: Moved crash handler setup to application entry point for better crash recovery

### Changed
- **Modernized GTK dialog system**: Migrated from deprecated GTK 4.10 dialog functions to modern `GtkAlertDialog` API
  - **Future-proof implementation**: Eliminated all deprecation warnings by replacing `gtk_message_dialog_*` and `gtk_dialog_*` functions
  - **Async-first architecture**: Native async dialog handling with proper event loop integration
  - **Improved accessibility**: Better screen reader support and GTK4 theme integration
- **Major main.go refactoring**: Massive code reduction from 1071 lines to 61 lines (~95% reduction)
  - **Extracted browser application logic** to dedicated `internal/app/browser` package for better separation of concerns
  - **Streamlined entry point**: Clean main function focusing only on CLI vs GUI mode detection and application bootstrapping
  - **Improved maintainability**: Modular architecture with clear responsibilities between CLI and browser components
- **Refactored memory configuration architecture**:
  - Integrated WebKit memory settings into unified config system with Viper environment variable bindings
  - Eliminated redundant environment variable parsing in favor of centralized configuration management
  - Simplified webview initialization with direct config-to-WebKit conversion
- **Improved CLI logs command description**: Enhanced help text for better user experience

## [0.6.0] - 2025-09-12

### Added
- **Smart AV1 codec negotiation system for YouTube high-resolution streaming**:
  - **Resolution-aware MediaCapabilities API reporting**: Intelligent codec capability reporting based on video resolution, enabling YouTube to select optimal codecs for different quality levels
  - **YouTube format manifest manipulation**: Real-time format list reordering to prioritize AV1 for ≤1080p content while allowing VP9 fallback for higher resolutions
  - **Graduated codec preference system**: Smart localStorage preference (`2048`) instead of forced AV1 (`8192`), giving YouTube flexibility for optimal codec selection
  - **4K HDR 60fps AV1 support**: Successfully enables YouTube delivery of premium quality 4K HDR content with AV1 codec, HDR10 (PQ) color space, and BT.2020 wide color gamut
  - **Configurable AV1 resolution limits**: New `av1_max_resolution` config option with environment variable support (`DUMBER_AV1_MAX_RES`)
  - **Automatic codec fallback logic**: Seamless VP9 fallback for resolutions where AV1 performance may be suboptimal
  - **High bitrate streaming support**: Enables premium quality streaming (650+ Mbps) with proper buffer management
  - Fixes previous limitation where forcing AV1 would cap YouTube playback at 1080p maximum
- Page refresh functionality with standard keyboard shortcuts:
  - **Ctrl+R** / **Cmd+R**: Reload current page
  - **Ctrl+Shift+R** / **Cmd+Shift+R**: Hard reload (bypass cache)
  - **F5**: Alternative reload key
  - WebKit integration with proper `webkit_web_view_reload()` and `webkit_web_view_reload_bypass_cache()` calls
- **Twitch theater mode and fullscreen stability fix**:
  - **Complete removal of codec interference on Twitch**: Eliminates aggressive codec control that caused theater mode and fullscreen freezing
  - **Native Twitch codec selection**: Allows Twitch to handle codec negotiation without browser intervention
  - Fixes theater mode freeze and fullscreen black screen issues on Twitch

### Fixed
- Zoom level toast no longer shows on every page navigation within the same domain:
  - Added smart detection to differentiate user-initiated vs programmatic zoom changes
  - Toast now only appears when manually adjusting zoom or entering a new domain
  - Prevents annoying notifications during regular browsing within the same site
- Links with `target="_blank"` now load in current window instead of being ignored:
  - Added WebKit `create` signal handler to intercept new window requests
  - Redirects popup/new tab links to current window for seamless single-window browsing
  - Maintains user experience consistency in tab-less browser architecture

## [0.5.0] - 2025-09-12

### Added
- Hardware video acceleration with automatic GPU detection:
  - Multi-vendor GPU detection (AMD, NVIDIA, Intel) using glxinfo, lspci, and sysfs
  - Automatic VA-API driver selection (radeonsi for AMD, vdpau for NVIDIA, iHD for Intel)
  - WebKitGTK integration with GStreamer hardware acceleration backends
  - Environment variable configuration support (LIBVA_DRIVER_NAME, GST_VAAPI_ALL_DRIVERS)
  - Legacy VA-API fallback mode for compatibility with older systems
  - Video decoding acceleration for H.264, HEVC, VP9, and AV1 codecs
  - Significantly reduced CPU usage during video streaming (Twitch, YouTube, etc.)
  - Smart conflict prevention between legacy and modern VA-API implementations
- TLS certificate error handling with user confirmation dialog:
  - Comprehensive certificate information display (subject, issuer, validity dates)
  - Interactive warning dialog for untrusted certificate authorities
  - Certificate exception management with per-host persistence
  - Manual load triggering after user accepts certificate exceptions
  - Proper integration with WebKitGTK 6.0 network session API
- Zoom level toast notifications:
  - Real-time zoom level display with format: "Zoom level: ±X% (Y%)"
  - Smart debouncing to prevent notification spam during rapid zoom changes
  - Automatic cleanup of stuck or colliding toast messages
  - 150ms debounce timer with 2-second display duration
  - Emergency cleanup function for orphaned toast elements
- Comprehensive XDG-compliant logging system:
  - Complete application output capture (Go logs, fmt prints, WebKit C logs)
  - XDG Base Directory specification compliance (`~/.local/state/dumber/logs/`)
  - Automatic log rotation with size/age-based cleanup and compression
  - WebKit CGO log capture with GLib log handler integration
  - CLI log management commands (`dumber logs list`, `logs tail`, `logs clean`)
  - File-only logging for cache operations to prevent dmenu interference
  - Multi-output support with timestamped, tagged log entries
  - Configurable log levels, formats (text/JSON), and retention policies

### Fixed
- WebView background set to black to improve user experience during page loads
- Multiple linting issues and code quality improvements (reduced from 211 to ~150 issues)
- Omnibox responsive scaling and layout issues:
  - Fixed over-aggressive viewport scaling that made omnibox disproportionately large at high zoom levels
  - Resolved input field overflow extending beyond container boundaries
  - Improved box-sizing model to prevent layout issues
  - Added conditional separator display (hidden when no search results)
  - Omnibox now scales proportionally with webpage content across all zoom levels
- Dmenu history favicon display:
  - Complete favicon caching system for fuzzel/dmenu integration with real website icons
  - ICO file support using go-ico library for perfect Windows icon conversion to PNG
  - Smart brightness detection and color inversion for dark theme visibility
  - All cached favicons normalized to consistent 32x32 PNG format for uniform display
  - Anti-aliasing preservation during color inversion to prevent edge noise and pixelation
  - Asynchronous favicon downloading and caching with 7-day freshness and 30-day cleanup
  - Fuzzel icon protocol integration (`\0icon\x1f<path>`) for native icon display
  - Favicon URLs extracted from webpage metadata and stored in database during navigation
  - Fallback system removed to ensure consistent sizing - shows real favicons or no icon
  - XDG Base Directory compliance for favicon cache storage

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
