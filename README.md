# Dumber, fully unfeatured unbloated browser for tiling WMs

<p align="center">
  <img src="assets/logo.svg" alt="Dumber Logo" width="256" height="256" />
</p>

Dumber is a minimalist browser and launcher companion focused on speed and simplicity. It ships a built‑in WebKitGTK (GTK4) window for navigation and a CLI with dmenu‑style launcher integration (rofi, fuzzel).

- Native GUI via WebKitGTK 6 (GTK4, CGO) with an embedded homepage UI.
- CLI that parses URLs, supports search shortcuts (e.g., `g:query`), and integrates with rofi/dmenu.
- Persistent history and per‑domain zoom levels (SQLite, XDG locations).
- Configurable via `config.json` and environment variables, with live reload.

## Wayland‑First
- Dumb on purpose, designed for Wayland window managers (Sway, Hyprland, River, Niri, etc..) it's a tiny companion that interact nicely with your launcher.
- Uses a native Wayland WebKitGTK-6.0 window; no external browser required and no X11/XWayland dependency.
- Optimized for dmenu‑style launchers (rofi, fuzzel, wofi) and keyboard‑driven workflows.

## Status
- Very early stage, expect sharp edges and breaking changes, but it works well enough to browse and play with the launcher flows and the pane management system.

## Features
- **Zellij-inspired pane management**: Split browser into multiple panes with keyboard shortcuts (Ctrl+P to enter pane mode)
- GPU Rendering and Hardware video acceleration
- Built-in ad blocker (UBlock-based, work in progress)
- dmenu‑style launcher integration (rofi, fuzzel) with history and shortcut suggestions
- Keyboard and mouse controls: comprehensive shortcuts and gestures
- Persistent history with search and stats
- Per‑domain zoom persistence
- One config file, fully customizable

## Controls & Shortcuts

### Keyboard Shortcuts

#### Browser Controls
| Shortcut | Action | Notes |
|----------|--------|-------|
| **F12** | Open Developer Tools | WebKit inspector |
| **Ctrl/Cmd+L** | Open Omnibox | URL/search input with history |
| **Ctrl/Cmd+F** | Find in Page | Search text within current page |
| **Ctrl/Cmd+=** | Zoom In | Firefox-compatible zoom levels |
| **Ctrl/Cmd++** | Zoom In | Alternative plus key |
| **Ctrl/Cmd+-** | Zoom Out | Works across keyboard layouts |
| **Ctrl/Cmd+0** | Reset Zoom | Return to 100% zoom |
| **Ctrl/Cmd+Shift+C** | Copy URL | Copy current URL to clipboard with toast |
| **Ctrl/Cmd+R** / **F5** | Reload Page | Refresh current page |
| **Ctrl/Cmd+Shift+R** | Hard Reload | Refresh ignoring cache |
| **Ctrl+←** / **Ctrl+→** | Navigate Back/Forward | Browser history navigation |

#### Zellij-Inspired Pane Management
| Shortcut | Action | Notes |
|----------|--------|-------|
| **Ctrl/Cmd+P** | Enter Pane Mode | Modal mode for pane operations |
| **→** / **R** (in pane mode) | Split Right | Create new pane to the right |
| **←** / **L** (in pane mode) | Split Left | Create new pane to the left |
| **↑** / **U** (in pane mode) | Split Up | Create new pane above |
| **↓** / **D** (in pane mode) | Split Down | Create new pane below |
| **X** (in pane mode) | Close Pane | Close current pane |
| **Enter** (in pane mode) | Confirm Action | Confirm pane operation |
| **Escape** (in pane mode) | Exit Pane Mode | Return to normal navigation |
| **Alt+Arrow Keys** | Navigate Panes | Move focus between panes |

### Mouse Controls
| Action | Result | Notes |
|--------|---------|-------|
| **Ctrl+Scroll Up** | Zoom In | Smooth zoom control |
| **Ctrl+Scroll Down** | Zoom Out | Smooth zoom control |
| **Mouse Button 8** | Navigate Back | Side button (back) |
| **Mouse Button 9** | Navigate Forward | Side button (forward) |
| **Two-finger Swipe** | Back/Forward | When supported by touchpad |

### Omnibox/Find Mode (when active)
| Shortcut | Action | Notes |
|----------|--------|-------|
| **Escape** | Close overlay | Exit omnibox or find mode |
| **Enter** | Execute/Navigate | Navigate to URL or jump to match |
| **Shift+Enter** | Previous Match | Find mode: go to previous match |
| **Alt+Enter** | Center Match | Find mode: center on match, keep open |
| **↑/↓ Arrow** | Navigate Results | Browse suggestions/matches |

All zoom changes are automatically persisted per-domain and restored on next visit.

## Quick Start
Prerequisites:
- Go 1.25+
- Node.js 20+ and npm (for building the embedded TypeScript frontend)
- For GUI build: WebKitGTK 6 and GTK4 dev packages (examples)
  - Debian/Ubuntu: `libwebkitgtk-6.0-dev libgtk-4-dev build-essential`
  - Arch: `webkitgtk-6.0 gtk4 base-devel`

Build options:
- GUI (default, recommended):
  - `make build`          # builds frontend + Go with `-tags=webkit_cgo`
  - Run: `./dist/dumber`  # opens the embedded homepage
- CLI‑only (no GUI, CGO disabled):
  - `make build-no-gui`
  - Run: `./dist/dumber-no-gui version`

Development:
- `make dev` runs `go run .` (non‑CGO; GUI stubs log to console)
- `make test` runs tests; `make lint` runs golangci‑lint

## Install
**Note**: `go install` will not work for this project because the frontend assets must be built first and embedded into the binary. Use the build commands instead:

- GUI build (default, recommended):
  - Ensure WebKit2GTK/GTK dev packages are installed (see prerequisites)
  - Clone the repo and run `make build` (builds frontend first, then Go with GUI support)
  - The resulting `./dist/dumber` binary includes the native GUI window
- CLI‑only (no native window):
  - Clone the repo and run `make build-no-gui`
  - The resulting `./dist/dumber-no-gui` binary runs CLI flows with GUI code stubbed

## Usage
- Open a URL or search:
  - `dumber browse https://example.com`
  - `dumber browse example.com`        # scheme auto‑added
  - `dumber browse g:golang`           # Google search via shortcut
- Launch the GUI directly:
  - `dumber`                           # no args → opens GUI homepage
- Show version information:
  - `dumber version`                   # display version, commit, and build date
- Launcher integration (dmenu‑style examples):
  - rofi:   `dumber dmenu | rofi -dmenu -p "Browse: " | dumber dmenu --select`
  - fuzzel: `dumber dmenu | fuzzel --dmenu -p "Browse: " | dumber dmenu --select`
- Manage browsing history:
  - `dumber history`                   # list recent history (default: 20 entries)
  - `dumber history list -n 50`       # list 50 recent entries
  - `dumber history search golang`    # search history for "golang"
  - `dumber history stats`            # show history statistics
  - `dumber history clear`            # clear all history (with confirmation)
  - `dumber history clear --force`    # clear all history (no confirmation)
- Clean up data and cache:
  - `dumber purge`                        # purge all data (with confirmation)
  - `dumber purge --force`                # purge all data (no confirmation)
  - `dumber purge -d -H -c`               # purge database and both caches
  - `dumber purge --browser-data`         # purge WebKit data (cookies, etc.)
- Manage logs:
  - `dumber logs list`                    # list available log files
  - `dumber logs tail`                    # tail current log file
  - `dumber logs clean`                   # clean up old log files

### Dmenu mode invocation
You can invoke dmenu mode in two ways:
- Subcommand (recommended): `dumber dmenu` … and `dumber dmenu --select`
- Root flag (generate options only): `dumber --dmenu`

Note: The root flag path only generates options; for processing a selection (`--select`), use the `dmenu` subcommand as the receiving command.

In GUI mode the app serves an embedded homepage via `dumb://homepage`, and frontend assets under `dumb://app/...`.

## Pane Management (Zellij-Inspired)

Dumber features a Zellij-inspired pane management system that allows you to split your browser window into multiple panes, each running independent web sessions.

### How it Works
1. **Enter Pane Mode**: Press `Ctrl+P` to enter pane mode (modal interface with timeout)
2. **Split Panes**: Use arrow keys or letter shortcuts to split:
   - `→` or `R` - Split right
   - `←` or `L` - Split left
   - `↑` or `U` - Split up
   - `↓` or `D` - Split down
3. **Navigate**: Use `Alt+Arrow Keys` to move focus between panes
4. **Close**: Press `X` in pane mode to close the current pane

### Features
- **Binary tree layout**: Panes are organized in a binary tree structure for optimal space usage
- **Focus management**: Visual borders and hover-to-focus for intuitive navigation
- **Independent sessions**: Each pane maintains its own browsing session, history, and zoom level
- **Popup handling**: Configurable popup placement (new pane, same pane, etc.)
- **Keyboard-driven**: All operations accessible via keyboard shortcuts

### Configuration
The pane system is fully configurable via `config.json`:
```json
"workspace": {
  "enable_zellij_controls": true,
  "pane_mode": {
    "activation_shortcut": "cmdorctrl+p",
    "timeout_ms": 3000,
    "action_bindings": {
      "arrowright": "split-right",
      "r": "split-right",
      "x": "close-pane",
      // ... full key mappings
    }
  },
  "popups": {
    "placement": "right",
    "open_in_new_pane": true
  },
  "styling": {
    "border_width": 2,
    "border_color": "@theme_selected_bg_color",
    "transition_duration": 120
  }
}
```

## Configuration

Dumber follows the XDG Base Directory spec:
- Config: `~/.config/dumber/config.json`
- Data:   `~/.local/share/dumber`
- State:  `~/.local/state/dumber` (default DB lives here)

A default `config.json` is created on first run. Config changes are watched and applied at runtime when possible.

### Complete Configuration Reference

#### Database Configuration
```json
"database": {
  "path": "~/.local/state/dumber/history.db",
  "max_connections": 1,
  "max_idle_time": "5m0s",
  "query_timeout": "30s"
}
```

#### History Management
```json
"history": {
  "max_entries": 10000,
  "retention_period_days": 365,
  "cleanup_interval_days": 1
}
```

#### Search Shortcuts
```json
"search_shortcuts": {
  "g":   { "url": "https://www.google.com/search?q=%s", "description": "Google search" },
  "gh":  { "url": "https://github.com/search?q=%s", "description": "GitHub search" },
  "yt":  { "url": "https://www.youtube.com/results?search_query=%s", "description": "YouTube search" },
  "w":   { "url": "https://en.wikipedia.org/wiki/%s", "description": "Wikipedia search" },
  "ddg": { "url": "https://duckduckgo.com/?q=%s", "description": "DuckDuckGo search" },
  "so":  { "url": "https://stackoverflow.com/search?q=%s", "description": "Stack Overflow search" },
  "r":   { "url": "https://www.reddit.com/search?q=%s", "description": "Reddit search" },
  "npm": { "url": "https://www.npmjs.com/search?q=%s", "description": "npm package search" },
  "go":  { "url": "https://pkg.go.dev/search?q=%s", "description": "Go package search" },
  "mdn": { "url": "https://developer.mozilla.org/en-US/search?q=%s", "description": "MDN Web Docs search" }
}
```

#### Dmenu Integration
```json
"dmenu": {
  "max_history_items": 20,
  "show_visit_count": true,
  "show_last_visited": true,
  "history_prefix": "🕒",
  "shortcut_prefix": "🔍",
  "url_prefix": "🌐",
  "date_format": "2006-01-02 15:04",
  "sort_by_visit_count": true
}
```

#### Logging Configuration
```json
"logging": {
  "level": "info",
  "format": "text",
  "filename": "",
  "max_size": 100,
  "max_backups": 3,
  "max_age": 7,
  "compress": true
}
```

#### Appearance Settings
```json
"appearance": {
  "sans_font": "Fira Sans",
  "serif_font": "Fira Sans", 
  "monospace_font": "Fira Code",
  "default_font_size": 16
}
```

#### Rendering Mode (GPU/CPU)
```json
"rendering_mode": "auto"
```
- `auto`: Detect GPU availability and use GPU when available (default)
- `gpu`: Force GPU acceleration (Vulkan/OpenGL via GTK4)
- `cpu`: Force software rendering

### Environment Variables
All config values can be overridden with environment variables using the `DUMB_BROWSER_` prefix:

```bash
# Database
DUMB_BROWSER_DATABASE_PATH="./browser.db"
DUMB_BROWSER_DATABASE_MAX_CONNECTIONS=5

# Rendering mode (special variable name)
DUMBER_RENDERING_MODE="gpu"

# Dmenu settings
DUMB_BROWSER_DMENU_MAX_HISTORY_ITEMS=50
DUMB_BROWSER_DMENU_SHOW_VISIT_COUNT=false

# Logging
DUMB_BROWSER_LOGGING_LEVEL="debug"
DUMB_BROWSER_LOGGING_FORMAT="json"
```

### CLI Flags
The browse command supports additional runtime flags:
```bash
dumber browse --rendering-mode=gpu https://example.com
```


## Building With WebKitGTK 6 (GTK4)
- The GUI requires building with `-tags=webkit_cgo` and CGO enabled.
- Ensure WebKitGTK 6 and GTK4 dev headers are installed (see prerequisites).
- Make targets handle both frontend and Go build steps:
  - `make build-gui` (CGO enabled)
  - `make build` (CGO enabled by default in the Makefile’s main build)

Without the native tag, a stub backend is used: you can still run CLI flows and see logs, but no native window is displayed.

## Media (GStreamer) & Hardware Acceleration

WebKitGTK uses GStreamer for media playback. Dumber includes automatic hardware video acceleration support that detects your GPU and configures the appropriate drivers.

**Hardware Acceleration Features:**
- Automatic GPU detection (AMD, NVIDIA, Intel)
- VA-API and VDPAU driver configuration
- Support for H.264, HEVC, VP9, and AV1 codecs
- Significantly reduced CPU usage during video streaming

**Required packages:**
- Arch Linux:
  - `sudo pacman -S gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugins-ugly gst-libav gst-plugin-pipewire pipewire pipewire-pulse`
  - Hardware accel: `gstreamer-vaapi libva-mesa-driver` (AMD), `libva-vdpau-driver` (NVIDIA), `intel-media-driver` (Intel)
- Debian/Ubuntu:
  - `sudo apt install gstreamer1.0-tools gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly gstreamer1.0-libav gstreamer1.0-pipewire`
  - Hardware accel: `gstreamer1.0-vaapi va-driver-all` (covers most GPUs)

## Theme and Runtime Updates
- The homepage and pages respect your system’s light/dark preference.
- The app updates color‑scheme live when your GTK theme changes (no relaunch needed).

## Project Structure
- `main.go`: Entry point with CLI/GUI routing
- `internal/app/cli`: CLI application logic and Cobra commands
- `internal/app/browser`: Browser application logic and WebKit integration
- `internal/cli`: Cobra commands (`browse`, `dmenu`, `version`)
- `internal/config`: Viper‑backed config, XDG helpers, defaults
- `internal/db`: SQLite schema init and sqlc‑generated queries
- `internal/logging`: Structured logging system with file rotation
- `services`: Application services (parser/history/config/browser)
- `pkg/webkit`: WebKit2GTK bindings (CGO and stub implementations)
- `pkg/clipboard`: Cross-platform clipboard operations
- `frontend`: TypeScript homepage UI bundled into the binary

## Security & Privacy
- History and zoom settings are stored locally in SQLite under XDG state.
- The `tmp/` directory is ignored by Git and kept out of history.

## Roadmap
- ✅ WebKitGTK 6 (GTK4) migration (GPU/Vulkan path complete)
- ✅ GPU rendering (Vulkan via GTK4 renderer) with graceful CPU fallback
- ✅ Zellij-inspired pane management with binary tree layout and keyboard-driven workflow
- 🚧 UBlock-based content filtering (early stage - network blocking works, cosmetic filtering needs work)
- Full Vim-style motion and keyboard navigation across pages and UI
- Tab management system (browser tabs in addition to panes)
- Performance work: faster startup, lower memory, snappier UI
- Other shiny things I'm not yet aware of

## License
This project is licensed under the MIT License. See `LICENSE` for details.
