# Dumber

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT) [![Go Report Card](https://goreportcard.com/badge/github.com/bnema/dumber)](https://goreportcard.com/report/github.com/bnema/dumber)

Dumber is a keyboard-driven web browser, built around panes, workspaces, and modal controls.

Tabs contain workspaces. Workspaces contain panes. Panes can be split, stacked, moved, resized, and closed from the keyboard.

The layout model is inspired by terminal multiplexers such as Zellij and tmux, but applied to web browsing this is particularly suited for Wayland compositors such as Niri or Hyprland.

[Website](https://dumber.bnema.dev) · [Documentation](https://dumber.bnema.dev/docs) · [Keybindings](https://dumber.bnema.dev/docs/reference/keybindings)

## Demo

https://github.com/user-attachments/assets/232822af-08e4-4a74-9416-87f79c96b118

The demo shows split panes, stacked panes, modal navigation, and workspace switching.

## Overview

Dumber uses three layout levels:

- **Tabs** group separate browser contexts.
- **Workspaces** hold a layout of panes.
- **Panes** display web pages and can be split, stacked, moved, resized, or closed.

Most browser-management actions are exposed through modal keybindings. Enter a mode, run one or more commands, then leave the mode. The default modes cover pane management, tabs, resizing, and session commands.

The browser chrome stays out of the way by default. Open the omnibox when you want to navigate; otherwise the pane is just the page or web app you are using. This makes Dumber work well as a side pane next to an editor, terminal, or another desktop application.

## Features

### Content-first browsing

- No permanent tab bar, bookmark bar, or toolbar around the page
- Omnibox and browser commands appear through keyboard shortcuts
- Web apps can feel closer to desktop applications because the browser frame is not always visible
- Works well as a narrow side pane next to an editor, terminal, or other application

### Pane-based browsing

- Split panes horizontally or vertically
- Stack panes in the same area
- Move, resize, and close panes with modal keybindings
- Keep related pages together in a workspace

### Keyboard workflow

- Pane, tab, resize, and session modes
- Vim/Zellij-style navigation patterns
- Search bangs such as `!g`, `!gi`, and `!ddg`
- Launcher integration for `rofi`, `fuzzel`, and `dmenu`

### Desktop integration

- Wayland-native on Sway, Hyprland, River, Niri, and similar compositors
- Floating pane with configurable profile shortcuts
- Single `config.toml` file with hot reload

### Browser features

- Chromium Embedded Framework backend by default
- WebKit backend available as a fallback
- Built-in ad blocking based on uBlock filter lists
- GPU-accelerated video through VA-API/VDPAU where supported
- Snapshot-based restoration of tabs, workspaces, and pane layout

## Installation

### Install script

```bash
curl -fsSL https://dumber.bnema.dev/install | sh
dumber browse
```

### Arch Linux

```bash
yay -S dumber-browser-bin   # Pre-built binary
yay -S dumber-browser-git   # Build from source
```

### Flatpak

```bash
wget https://github.com/bnema/dumber/releases/latest/download/dumber.flatpak
flatpak install --user dumber.flatpak
flatpak run dev.bnema.Dumber browse
```

For dependencies, distribution notes, and troubleshooting, see the [installation documentation](https://dumber.bnema.dev/docs).

## Keyboard modes

Dumber uses modal keybindings for browser management.

| Mode | Default key | Used for |
|------|-------------|----------|
| Pane | `Ctrl+P` | Split, stack, close, and move panes |
| Tab | `Ctrl+T` | Create, close, switch, and rename tabs |
| Resize | `Ctrl+N` | Resize panes with `hjkl` or arrow keys |
| Session | `Ctrl+O` | Snapshot, restore, and browse sessions |

## Floating pane

The floating pane is a temporary browser pane that can be toggled without changing the main workspace layout.

- `Alt+F` toggles the floating pane and preserves its state while hidden.
- `Ctrl+W` closes the active pane; if the floating pane is active, it is fully released for a fresh next open.
- Profile shortcuts such as `Alt+G` are optional and configured under `workspace.floating_pane.profiles`.
- Some `Alt+<key>` bindings may conflict with browser-engine shortcuts or desktop-level handlers.

See the [floating pane reference](https://dumber.bnema.dev/docs/reference/floating-pane) for setup and behavior details.

## Configuration

Dumber is configured with a single TOML file.

```toml
[engine]
type = "cef"

[workspace.floating_pane]
enabled = true
```

The config reloads while Dumber is running. See the [configuration documentation](https://dumber.bnema.dev/docs) for all options.

## Browser engine

Dumber uses Chromium Embedded Framework by default. WebKitGTK is available as a fallback backend.

On Arch Linux, install the CEF runtime with:

```bash
sudo pacman -S cef
```

For hardware video decoding, `cef-vaapi` is also available from the AUR as an optional CEF build.

CEF runtime lookup order:

1. `CEF_DIR`
2. `engine.cef.cef_dir`
3. `/usr/lib/cef`
4. `~/.local/share/cef`

Set `engine.cef.cef_dir` in config when `CEF_DIR` is unset:

```toml
[engine.cef]
cef_dir = "/custom/path"
```

Or set the higher-precedence `CEF_DIR` environment variable:

```bash
CEF_DIR=/custom/path dumber browse
```

WebKit can be selected explicitly:

```toml
[engine]
type = "webkit"
```

### Rendering notes

CEF uses Dumber's GPU-first Wayland render stack by default: GDK DMABUF presentation with ANGLE/GSK Vulkan. For driver compatibility, switch to the EGL/OpenGL stack with `engine.cef.render_stack = "egl"`; the default is `"vulkan"`.

CEF OSR frame rate adapts to the active Wayland monitor refresh rate when both `engine.cef.adaptive_windowless_frame_rate = true` and `engine.cef.windowless_frame_rate = 0` are set, which are the defaults. Adaptive mode is capped by `engine.cef.windowless_frame_rate_max = 240`. Setting `engine.cef.windowless_frame_rate` to a positive value forces a fixed rate instead. This adaptive polling path is Wayland-specific; other platforms fall back to the configured fixed/default CEF behavior.

## Platform notes

### Dependencies

**Arch Linux:**

```bash
sudo pacman -S cef webkitgtk-6.0 gtk4 gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugin-va
```

**Debian/Ubuntu:**

```bash
sudo apt install libwebkitgtk-6.0-4 libgtk-4-1 gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad
```

> Ubuntu 24.04 ships GLib 2.80, but Dumber requires GLib 2.84+. Use Arch, Fedora 41+, or the Flatpak.

## Status

Dumber is usable for regular web browsing on Wayland compositors, with the main focus on panes, keyboard control, floating workflows, and desktop integration.

It is not a drop-in replacement for every mainstream browser workflow. Extension compatibility, engine behavior, and media support depend on the selected backend and system libraries.

Bug reports and reproducible Wayland/backend issues are welcome.

## Development

Dumber uses pure-Go bindings. The GUI uses GTK4, runs on CEF by default, and can use WebKitGTK as a fallback backend.

Set `ENV=dev` to use `.dev/dumber/` for config and data instead of XDG paths.

### Build from source

**Prerequisites:**

- Go 1.26+
- GTK4 development packages
- CEF runtime (default backend)
- WebKitGTK 6.0 development/runtime packages (fallback backend and runtime checks)
- Brotli for compressed systemviews assets

Systemviews assets are generated with `go tool templ` and Go's `js/wasm` toolchain; no root Node toolchain is required.

```bash
git clone https://github.com/bnema/dumber
cd dumber
make build
./dist/dumber browse
```

### Make targets

| Target | Description |
|--------|-------------|
| `make build` | Build systemviews assets and binary |
| `make build-quick` | Build binary only, skipping systemviews assets |
| `make check` | Run local all-clear checks for tools, build, generated assets, tests, and constraints |
| `make dev` | Run with `go run` |
| `make test` | Run tests |
| `make lint` | Run the pinned golangci-lint version |
| `make staticcheck` | Run Staticcheck with the pinned tool version |
| `make verify-generated` | Verify generated systemviews artifacts are committed |
| `make flatpak-build` | Build Flatpak bundle |

Development tool versions are pinned in `Makefile` (`GOLANGCI_LINT_VERSION`, `STATICCHECK_VERSION`). Bump those values intentionally when refreshing lint/static analysis tooling.

## Contributing

Dumber is currently focused on stability, performance, browser-engine behavior, and core UI/UX polish.

Bug fixes, documentation improvements, packaging fixes, and targeted UX improvements are welcome. Larger feature ideas should start as issues before PRs.

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## Community

- [Report bugs](https://github.com/bnema/dumber/issues)
- [Releases](https://github.com/bnema/dumber/releases)

## License

MIT - See `LICENSE` for details.
