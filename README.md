# Dumber, fully unfeatured unbloated browser for tiling WMs

<p align="center">
  <img src="assets/logo.svg" alt="Dumber Logo" width="256" height="256" />
</p>


https://github.com/user-attachments/assets/394f5f7e-a475-42da-b483-5a58b2ea2f51



A dumb browser that works like your favorite terminal multiplexer.

## Features
- **Wayland native**: No X11/XWayland dependency, works with Sway, Hyprland, River, Niri, etc.
- **Tabs and workspaces**: Each tab holds a workspace with split or stacked panes.
- **Keyboard-driven**: Keyboard workflow inspired by Zellij.
- **Smart popup handling**: OAuth flows, window.open(), and popup deduplication for tiling WMs.
- **GPU rendering**: Hardware video acceleration with automatic VA-API/VDPAU detection.
- **Built-in ad blocking**: UBlock-based network + cosmetic filtering.
- **Launcher integration**: dmenu-style with rofi/fuzzel support, shows favicons and history.
- **Search shortcuts**: Quick search via bangs (e.g., `!g golang` for Google, `!gi cats` for Google Images).
- **Customizable themes**: Light and dark palettes with semantic color tokens.
- **Persistent storage**: SQLite for history, zoom levels, and settings.
- **Live configuration**: Single config file with hot reload when possible.

## Status
Dumber recently completed a full rewrite in pure Go using the `puregotk` and `puregotk-webkit` libraries.

The project is now structured around clean architecture, which makes it much easier to iterate on features and improve testability.

Early development with regular releases. Core features work well for daily use but expect some rough edges.

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

#### Tab Management
| Shortcut | Action | Notes |
|----------|--------|-------|
| **Ctrl+T** | Enter Tab Mode | Modal mode for tab operations |
| **Ctrl+Tab** | Next Tab | Quick tab switching |
| **Ctrl+Shift+Tab** | Previous Tab | Quick tab switching |
| **Ctrl+W** | Close Tab | Close current tab |
| **N** / **C** (in tab mode) | New Tab | Create new tab |
| **X** (in tab mode) | Close Tab | Close current tab |
| **L** / **Tab** (in tab mode) | Next Tab | Navigate to next tab |
| **H** / **Shift+Tab** (in tab mode) | Previous Tab | Navigate to previous tab |
| **Escape** (in tab mode) | Exit Tab Mode | Return to normal navigation |

#### Pane Management
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
| **Escape** | Close/Clear | Clear input if text present, close if empty |
| **Enter** | Execute/Navigate | Navigate to URL or jump to match |
| **Shift+Enter** | Previous Match | Find mode: go to previous match |
| **Alt+Enter** | Center Match | Find mode: center on match, keep open |
| **Tab** | Toggle View | Omnibox mode: switch between history and favorites |
| **Space** | Toggle Favorite | Omnibox mode: favorite/unfavorite selected item |
| **Ctrl+1-9, 0** | Quick Navigate | Jump to result by number (1-10) |
| **↑/↓ Arrow** | Navigate Results | Browse suggestions/matches |

All zoom changes are automatically persisted per-domain and restored on next visit.

## Quick Start

**Option 1: Use Pre-built Binaries (Recommended)**

Download the latest release from the [releases page](https://github.com/bnema/dumber/releases):
```bash
wget https://github.com/bnema/dumber/releases/latest/download/dumber_<version>_linux_x86_64.tar.gz
tar -xzf dumber_<version>_linux_x86_64.tar.gz
sudo install -m 755 dumber /usr/local/bin/
```

Then install runtime dependencies (see [Install](#install) section for details).

**Option 2: Build From Source**

Prerequisites:
- Go 1.25+
- Node.js 20+ and npm (for building the embedded TypeScript frontend)
- For GUI build: WebKitGTK 6 and GTK4 dev packages (examples)
  - Minimum runtime versions: WebKitGTK 6.0 >= `2.50`, GTK4 >= `4.20`, GLib >= `2.84`
  - Debian/Ubuntu: `libwebkitgtk-6.0-dev libgtk-4-dev build-essential`
  - Arch: `webkitgtk-6.0 gtk4 base-devel`
  - Verify your system with: `dumber doctor` (or configure `runtime.prefix` for `/opt` installs)

Build options:
- Default (recommended):
  - `make build`          # builds frontend + Go (pure Go; CGO off)
  - Run: `./dist/dumber`  # run the app
- Quick (no frontend rebuild):
  - `make build-quick`
  - Run: `./dist/dumber`

Development:
- `make dev` runs `go run ./cmd/dumber`
- `make test` runs tests; `make lint` runs golangci‑lint

## Install

### Pre-built Binaries (Recommended)

Pre-built binaries are available for Linux (x86_64/amd64) in the [releases page](https://github.com/bnema/dumber/releases). These include all frontend assets and are ready to use:

```bash
# Download and install latest release
wget https://github.com/bnema/dumber/releases/latest/download/dumber_<version>_linux_x86_64.tar.gz
tar -xzf dumber_<version>_linux_x86_64.tar.gz
sudo install -m 755 dumber /usr/local/bin/
```

Replace `<version>` with the desired version (e.g., `0.14.1`), or use the direct link from the releases page.

**Dependencies** (required for both pre-built binaries and source builds):

**Arch Linux:**
```bash
# Core WebKitGTK 6.0 and GTK4
sudo pacman -S webkitgtk-6.0 gtk4

# GStreamer for media playback
sudo pacman -S gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugins-ugly gst-libav gst-plugin-pipewire pipewire pipewire-pulse

# Hardware video decoding (required for YouTube, Twitch, etc.)
sudo pacman -S gst-plugin-va

# VA-API drivers (choose based on your GPU)
# AMD: sudo pacman -S libva-mesa-driver mesa
# NVIDIA: sudo pacman -S libva-nvidia-driver
# Intel: sudo pacman -S libva-intel-driver intel-media-driver
```

**Debian/Ubuntu:**

> ⚠️ **Note:** Ubuntu 24.04 LTS ships with GLib 2.80, but this project requires GLib 2.84+ due to newer GTK/WebKitGTK runtime requirements. Use Arch Linux, Fedora 41+, or another distribution with recent GLib packages.

```bash
# Core WebKitGTK and GTK4
sudo apt install libwebkitgtk-6.0-4 libgtk-4-1

# GStreamer for media playback
sudo apt install gstreamer1.0-tools gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly gstreamer1.0-libav gstreamer1.0-pipewire

# Hardware video decoding (gst-plugins-bad includes VA stateless decoders)
# VA-API drivers
sudo apt install va-driver-all
```

### Build From Source

**Note**: `go install` will not work for this project because the frontend assets must be built first and embedded into the binary.

**Prerequisites:**
- Go 1.25+
- Node.js 20+ and npm (for building the embedded TypeScript frontend)
- WebKitGTK 6 and GTK4 dev packages:
  - Minimum runtime versions: WebKitGTK 6.0 >= `2.50`, GTK4 >= `4.20`, GLib >= `2.84`
  - Debian/Ubuntu: `libwebkitgtk-6.0-dev libgtk-4-dev build-essential`
  - Arch: `webkitgtk-6.0 gtk4 base-devel`
  - Verify your system with: `dumber doctor` (or configure `runtime.prefix` for `/opt` installs)

**Build options:**
- GUI build (default, recommended):
  - Clone the repo and run `make build` (builds frontend first, then Go with GUI support)
  - The resulting `./dist/dumber` binary includes the native GUI window
- CLI‑only (no native window):
  - Clone the repo and run `make build-no-gui`
  - The resulting `./dist/dumber-no-gui` binary runs CLI flows with GUI code stubbed

## Usage
- Open a URL or search:
  - `dumber browse https://example.com`
  - `dumber browse example.com`        # scheme auto‑added
  - `dumber browse dumb://home`        # built-in home page (stats, shortcuts, etc.)
  - `dumber browse "!g golang"`         # Google search via bang shortcut
- Show version information:
  - `dumber about`                     # display version, commit, and build date
- Launcher integration (dmenu‑style examples with favicon support):
  - rofi:   `dumber dmenu | rofi -dmenu -show-icons -p "🔍 " | dumber dmenu --select`
  - fuzzel: `dumber dmenu | fuzzel --dmenu -p "🔍 " | dumber dmenu --select`
- Manage browsing history:
  - `dumber history`                  # list recent history (default: 20 entries)
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

In GUI mode the app serves an embedded home page via `dumb://home`, and frontend assets under `dumb://app/...`.

There is also a WIP config UI at `dumb://config` for viewing/applying settings with live reload.

## Tab Management

Each tab holds its own workspace. Each workspace can contain multiple panes (split or stacked). Switch between tabs to switch between independent workspaces.

### How it Works
1. **Enter Tab Mode**: Press `Ctrl+T` to enter tab mode (modal interface with timeout)
2. **Create/Close Tabs**: Use keyboard shortcuts:
   - `N` or `C` - New tab
   - `X` - Close current tab
3. **Navigate**: Use `Ctrl+Tab` / `Ctrl+Shift+Tab` for quick tab switching, or `L`/`H` in tab mode
4. **Exit**: Press `Escape` to exit tab mode or wait for timeout

Tab mode provides visual feedback with an orange border indicator.

## Pane Management

Split your browser window into multiple panes, each running independent web sessions.

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

## Configuration

Configuration file: `~/.config/dumber/config.toml` (supports TOML, JSON, YAML)

A default config is created on first run. Changes are applied automatically (hot reload). Override any setting with `DUMB_BROWSER_*` or `DUMBER_*` environment variables.

**Full documentation**: [docs/CONFIG.md](docs/CONFIG.md) - Complete reference for all settings, workspace controls, shortcuts, styling, and more.


## Building With WebKitGTK 6 (GTK4)
- Dumber uses pure-Go bindings (no CGO).
- You still need the system GTK4/WebKitGTK runtime libraries installed to run the GUI.
- Build targets:
  - `make build` (builds frontend + binary)
  - `make build-quick` (binary only; skips frontend build)

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
  - Hardware decoding: `gst-plugin-va` (stateless VA decoders - required for WebKitGTK)
  - VA-API drivers: `libva-mesa-driver mesa` (AMD), `libva-nvidia-driver` (NVIDIA), `libva-intel-driver intel-media-driver` (Intel)
- Debian/Ubuntu:
  - `sudo apt install gstreamer1.0-tools gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly gstreamer1.0-libav gstreamer1.0-pipewire`
  - Hardware decoding: `gstreamer1.0-plugins-bad` includes VA stateless decoders
  - VA-API drivers: `va-driver-all` (covers most GPUs)

## References
- puregotk: https://github.com/jwijenbergh/puregotk
- puregotk-webkit: https://github.com/bnema/puregotk-webkit
- ublock-webkit-filters (filter list releases): https://github.com/bnema/ublock-webkit-filters

## Contribute
I will gladly accept contributions, especially related to UI/UX enhancements. Feel free to open an issue and let's discuss it!

## License
This project is licensed under the MIT License. See `LICENSE` for details.
