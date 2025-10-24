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
- Early development stage with regular releases. The workspace management system is fully functional and the browser works well for daily use. Expect some rough edges but core features are stable.

## Features
- **Complete workspace management**: Zellij-inspired pane splitting with binary tree layout, stacked panes with title bars, focus tracking, and modal keyboard controls
- **Multi-pane WebView architecture**: Independent browsing sessions per pane with proper lifecycle management
- **Advanced popup handling**: Intelligent popup management for tiling WMs - OAuth flows, window.open(), and popup deduplication
- **Window-level global shortcuts**: Centralized shortcut handling to prevent conflicts between multiple WebView instances
- **GPU rendering and hardware video acceleration**: Automatic GPU detection with VA-API/VDPAU support
- **Built-in ad blocker**: UBlock-based content filtering (network blocking functional, cosmetic filtering in progress)
- **Configurable color palettes**: Customizable light and dark themes with semantic color tokens injected as CSS custom properties
- **Terminal-style UI design**: Monospace fonts, sharp corners, dashed borders with configurable appearance settings
- **dmenu‑style launcher integration**: rofi, fuzzel integration with favicon display, history and search shortcut suggestions
- **Comprehensive keyboard controls**: Complete keyboard-driven workflow with shortcuts and gestures
- **Persistent history and zoom**: SQLite-based history with configurable default zoom and per‑domain zoom persistence
- **Fully configurable**: Single config file with live reload and environment variable overrides

## Controls & Shortcuts

### Keyboard Shortcuts

#### Browser Controls
| Shortcut | Action | Notes |
|----------|--------|-------|
| **F12** | Open Developer Tools | WebKit inspector |
| **Ctrl+L** | Open Omnibox | URL/search input with history |
| **Ctrl+F** | Find in Page | Search text within current page |
| **Ctrl+=** | Zoom In | Firefox-compatible zoom levels |
| **Ctrl++** | Zoom In | Alternative plus key |
| **Ctrl+-** | Zoom Out | Works across keyboard layouts |
| **Ctrl+0** | Reset Zoom | Return to 100% zoom |
| **Ctrl+Shift+C** | Copy URL | Copy current URL to clipboard with toast |
| **Ctrl+Shift+P** | Print Page | Open native print dialog |
| **Ctrl+R** / **F5** | Reload Page | Refresh current page |
| **Ctrl+Shift+R** | Hard Reload | Refresh ignoring cache |
| **Ctrl+←** / **Ctrl+→** | Navigate Back/Forward | Browser history navigation |

#### Zellij-Inspired Pane Management
| Shortcut | Action | Notes |
|----------|--------|-------|
| **Ctrl+P** | Enter Pane Mode | Modal mode for pane operations |
| **→** / **R** (in pane mode) | Split Right | Create new pane to the right |
| **←** / **L** (in pane mode) | Split Left | Create new pane to the left |
| **↑** / **U** (in pane mode) | Split Up | Create new pane above |
| **↓** / **D** (in pane mode) | Split Down | Create new pane below |
| **S** (in pane mode) | Stack Pane | Create stacked pane (Zellij-style) |
| **X** (in pane mode) | Close Pane | Close current pane |
| **Enter** (in pane mode) | Confirm Action | Confirm pane operation |
| **Escape** (in pane mode) | Exit Pane Mode | Return to normal navigation |
| **Alt+Arrow Keys** | Navigate Panes | Move focus between panes |
| **Alt+Up/Down** | Navigate Stack | Navigate between stacked panes |

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
2. **Split or Stack Panes**: Use arrow keys or letter shortcuts:
   - `→` or `R` - Split right
   - `←` or `L` - Split left
   - `↑` or `U` - Split up
   - `↓` or `D` - Split down
   - `S` - Stack panes (Zellij-style, shows title bars for collapsed panes)
3. **Navigate**: Use `Alt+Arrow Keys` to move focus between split panes, or `Alt+Up/Down` to navigate within stacked panes
4. **Close**: Press `X` in pane mode to close the current pane

### Configuration
The pane system is fully configurable via `config.json`:
```json
"workspace": {
  "enable_zellij_controls": true,
  "pane_mode": {
    "activation_shortcut": "ctrl+p",
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

### How Configuration Works

- **Config file**: `~/.config/dumber/config.json` (supports `.yaml` and `.toml` too)
- **Format**: JSON/YAML/TOML - auto-detected
- **JSON Schema**: Auto-generated for IDE autocompletion and validation
- **Live reload**: Changes are automatically applied without restarting
- **Environment variables**: Override any setting with `DUMB_BROWSER_*` or `DUMBER_*` prefix
- **Default values**: Sensible defaults for all settings

### Quick Examples

```json
{
  "default_zoom": 1.2,
  "rendering_mode": "gpu",
  "search_shortcuts": {
    "g": { "url": "https://google.com/search?q=%s", "description": "Google" }
  },
  "workspace": {
    "enable_zellij_controls": true,
    "popups": { "behavior": "split" }
  }
}
```

Environment variable override:
```bash
DUMBER_RENDERING_MODE=cpu dumber
DUMB_BROWSER_LOGGING_LEVEL=debug dumber
```

**📖 Complete documentation**: See [docs/CONFIG.md](docs/CONFIG.md) for all available settings, defaults, and valid values.


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
  - Hardware accel: `gstreamer-vaapi mesa` (AMD), `libva-nvidia-driver` (NVIDIA), `libva-intel-driver intel-media-driver` (Intel)
- Debian/Ubuntu:
  - `sudo apt install gstreamer1.0-tools gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly gstreamer1.0-libav gstreamer1.0-pipewire`
  - Hardware accel: `gstreamer1.0-vaapi va-driver-all` (covers most GPUs)

## Contribute
I will gladly accept contributions, especially related to UI/UX enhancements. Feel free to open an issue and let's discuss it!

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
