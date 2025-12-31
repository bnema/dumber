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
- **Persistent storage**: SQLite for history, favorites, per-domain zoom levels, and sessions.
- **Live configuration**: Single config file with hot reload when possible.
- **Session management**: Zellij-style session save/restore with automatic snapshots and resurrection.

## Status
Dumber is almost feature-complete. Next steps will focus on stability, performance, and bug fixing.

Core features work well for daily use but expect some rough edges. Some features are still alpha/experimental (notably consume-or-expel panes) and may change behavior between releases.

## Quick Start

```bash
# Download and install
wget https://github.com/bnema/dumber/releases/latest/download/dumber_linux_x86_64.tar.gz
tar -xzf dumber_linux_x86_64.tar.gz
mkdir -p ~/.local/bin && install -m 755 dumber_*/dumber ~/.local/bin/

# Run
dumber browse https://example.com
```

See [Install](#install) for AUR, Flatpak, building from source, and runtime dependencies.

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
| **m** (in pane mode) | Move Pane To Tab | Opens a tab picker modal |
| **M** (in pane mode) | Move Pane To Next Tab | Moves to the next tab, creates new tab if needed |
| **Enter** (in pane mode) | Confirm Action | Confirm pane operation |
| **Escape** (in pane mode) | Exit Pane Mode | Return to normal navigation |
| **Alt+Arrow Keys** | Navigate Panes | Move focus between panes |
| **Alt+Up/Down** | Navigate Stack | Navigate between stacked panes |
| **Alt+[** / **Alt+]** | Consume / Expel Pane | Experimental (alpha): merge into sibling stack, or expel out of stack |
| **Alt+Shift+[** / **Alt+Shift+]** | Consume / Expel Pane (Vertical) | Experimental (alpha): up/down variants |

#### Resize Mode
| Shortcut | Action | Notes |
|----------|--------|-------|
| **Ctrl+N** | Enter Resize Mode | Modal mode for pane resizing |
| **←/↓/↑/→** / **h/j/k/l** (in resize mode) | Move Divider | Resizes the nearest split by moving the divider |
| **H/J/K/L** (in resize mode) | Move Divider (Inverse) | Inverts the direction |
| **+ / -** (in resize mode) | Smart Resize | Grow/shrink the active pane (best-effort) |
| **Enter** (in resize mode) | Confirm | Exit resize mode |
| **Escape** (in resize mode) | Cancel | Exit resize mode |

#### Session Management
| Shortcut | Action | Notes |
|----------|--------|-------|
| **Ctrl+O** | Enter Session Mode | Modal mode for session operations |
| **Ctrl+Shift+S** | Open Session Manager | Direct access to session browser |
| **S** / **W** (in session mode) | Open Session Manager | Browse and restore sessions |
| **Escape** (in session mode) | Exit Session Mode | Return to normal navigation |

### Mouse Controls
| Action | Result | Notes |
|--------|---------|-------|
| **Drag pane divider** | Resize panes | Updates split ratio and persists in session snapshots |
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
| **Hold key 400ms** | Accent Picker | Shows accented variants in any text input (e.g., hold 'e' for è, é, ê, ë) |

All zoom changes are automatically persisted per-domain and restored on next visit.

## Install

### Pre-built Binaries (Recommended)

Pre-built binaries are available for Linux (x86_64/amd64) in the [releases page](https://github.com/bnema/dumber/releases). See [Quick Start](#quick-start) for installation instructions.

### Arch Linux (AUR)

Dumber is available on the [Arch User Repository (AUR)](https://aur.archlinux.org/packages?K=dumber-browser):

```bash
# Binary package (pre-built, recommended - fastest install)
yay -S dumber-browser-bin

# Or build from source (latest git - bleeding edge)
yay -S dumber-browser-git
```

Both packages install to `/usr/bin/dumber` and include desktop integration. The binary package downloads pre-built releases from GitHub, while the git package builds from the latest source code.

> **Note:** When installed via AUR, the self-updater is automatically disabled. Use `yay -Syu` or your preferred AUR helper to update.

### Flatpak

A Flatpak bundle is available in each release. This is the easiest way to install Dumber with all dependencies included:

```bash
# Download the Flatpak bundle from the latest release
wget https://github.com/bnema/dumber/releases/latest/download/dumber.flatpak

# Install it (user-level, no root required)
flatpak install --user dumber.flatpak

# Run it
flatpak run dev.bnema.Dumber browse
```

To uninstall:
```bash
flatpak uninstall --user dev.bnema.Dumber
```

### Dependencies

Required for both pre-built binaries and source builds:

**Arch Linux:**
```bash
# Core WebKitGTK 6.0 and GTK4
sudo pacman -S webkitgtk-6.0 gtk4

# GStreamer for media playback
sudo pacman -S gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugins-ugly gst-libav gst-plugin-pipewire pipewire pipewire-pulse

# Hardware video decoding (required for YouTube, Twitch, etc.)
sudo pacman -S gst-plugin-va

# VA-API drivers (choose based on your GPU)
# AMD: sudo pacman -S mesa
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
- Open a URL, local file, or search:
  - `dumber browse https://example.com`
  - `dumber browse example.com`         # scheme auto‑added
  - `dumber browse ./test.html`         # local file path (converted to file://)
  - `dumber browse dumb://home`         # built-in home page (stats, shortcuts, etc.)
  - `dumber browse "!g golang"`        # search via bang shortcut
- Show version information:
  - `dumber about`                      # version, commit, and build date
- Check for and install updates:
  - `dumber update`                     # check and install if available
  - `dumber update --force`             # force reinstall (skips version check)
- Launcher integration (dmenu‑style examples with favicon support):
  - rofi:   `dumber dmenu | rofi -dmenu -show-icons -p "🔍 " | dumber dmenu --select`
  - fuzzel: `dumber dmenu | fuzzel --dmenu -p "🔍 " | dumber dmenu --select`
  - `dumber dmenu --interactive`        # built-in TUI fuzzy finder
  - `dumber dmenu --days 7`             # show history from last 7 days (default: 30)
  - `dumber dmenu --most-visited`       # sort by visit count instead of recency
- Manage browsing history:
  - `dumber history`                    # interactive history browser (timeline tabs + fuzzy search)
  - `dumber history --json`             # output recent entries as JSON
  - `dumber history --json --max 50`    # limit JSON output to N entries
  - `dumber history stats`              # show history statistics
  - `dumber history clear`              # interactive time-range cleanup
- Clean up data:
  - `dumber purge`                      # interactive selection (TUI)
  - `dumber purge --force`              # remove everything (no prompts)
- Manage configuration:
  - `dumber config status`              # show config path and migration availability
  - `dumber config migrate`             # add missing default settings to config
  - `dumber config migrate --yes`       # skip confirmation prompt
- Manage sessions:
  - `dumber sessions`                   # interactive session browser (TUI)
  - `dumber sessions list`              # list saved sessions
  - `dumber sessions list --json`       # output sessions as JSON
  - `dumber sessions list --limit 50`   # limit number of sessions shown
  - `dumber sessions restore <id>`      # restore a session in a new window
  - `dumber sessions delete <id>`       # delete a saved session
- Manage logs:
  - `dumber logs`                       # list sessions with log files
  - `dumber logs <session>`             # show logs for a session (full ID or unique suffix)
  - `dumber logs -f <session>`          # follow logs in real time
  - `dumber logs -n 200 <session>`      # show last N lines
  - `dumber logs clear`                 # clean up old log files
  - `dumber logs clear --all`           # remove all log files
- Generate documentation:
  - `dumber gen-docs`                   # install man pages to ~/.local/share/man/man1/
  - `dumber gen-docs --format markdown` # generate markdown docs to ./docs/
  - `dumber gen-docs --output ./man`    # generate to custom directory

### Dmenu mode invocation
You can invoke dmenu mode in two ways:
- Piped launcher (generate options): `dumber dmenu`
- Receive selection from stdin: `dumber dmenu --select`

For a built-in fuzzy finder UI, use: `dumber dmenu --interactive`

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

## Session Management

Dumber automatically saves your browser session (tabs, panes, split ratios, URLs) and allows you to restore previous sessions.

### How it Works
1. **Automatic Snapshots**: Session state is saved automatically (debounced, every 5s by default) and on graceful shutdown
2. **Session Manager**: Press `Ctrl+O → s` or `Ctrl+Shift+S` to open the session manager modal
3. **Browse Sessions**: View active and exited sessions with tab/pane previews
4. **Restore**: Select an exited session to restore it in a new browser window
5. **CLI Access**: Use `dumber sessions` for an interactive TUI, or `dumber sessions list/restore/delete` for scripting



## Configuration

Configuration file: `~/.config/dumber/config.toml` (supports TOML, JSON, YAML)

A default config is created on first run. Changes are applied automatically (hot reload). Override any setting with `DUMB_BROWSER_*` or `DUMBER_*` environment variables.

**Full documentation**: [docs/CONFIG.md](docs/CONFIG.md) - Complete reference for all settings, workspace controls, shortcuts, styling, and more.


## Development

Dumber uses pure-Go bindings (no CGO) but requires GTK4/WebKitGTK runtime libraries for the GUI.

### Environment

Set `ENV=dev` to use a local `.dev/dumber/` directory for config, data, state, and cache instead of XDG paths. Useful for testing without affecting your real browser data.

### Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Build frontend + binary (recommended) |
| `make build-quick` | Build binary only (faster for backend work) |
| `make build-frontend` | Build webui pages (homepage, error, config) |
| `make dev` | Run with `go run` |
| `make test` | Run tests |
| `make test-race` | Run tests with race detection |
| `make test-cover` | Run tests with coverage report |
| `make lint` | Run golangci-lint |
| `make lint-fix` | Run golangci-lint with auto-fix |
| `make fmt` | Format Go code |
| `make generate` | Generate SQLC code |
| `make mocks` | Generate mock implementations |
| `make install-tools` | Install dev tools (sqlc, golangci-lint) |
| `make check` | Verify tools, build, and tests work |
| `make clean` | Remove build artifacts |

### Flatpak

| Target | Description |
|--------|-------------|
| `make flatpak-deps` | Install Flatpak build dependencies |
| `make flatpak-build` | Build Flatpak bundle |
| `make flatpak-install` | Install Flatpak locally for testing |
| `make flatpak-run` | Run the installed Flatpak |
| `make flatpak-clean` | Clean Flatpak build artifacts |

### Release

| Target | Description |
|--------|-------------|
| `make release-snapshot` | Build snapshot with goreleaser |
| `make release` | Create full release with goreleaser |

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
  - VA-API drivers: `mesa` (AMD), `libva-nvidia-driver` (NVIDIA), `libva-intel-driver intel-media-driver` (Intel)
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
