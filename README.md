<h1 align="center">Dumber</h1>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License: MIT"></a>
  <a href="https://github.com/bnema/dumber/releases"><img src="https://img.shields.io/badge/platform-Linux%20Wayland%20only-blue?style=flat-square" alt="Platform: Linux Wayland"></a>
  <a href="https://github.com/bnema/dumber/commits/main"><img src="https://badgen.net/github/last-commit/bnema/dumber/main?icon=github" alt="Last commit"></a>
  <a href="https://github.com/bnema/dumber/stargazers"><img src="https://badgen.net/github/stars/bnema/dumber?icon=github" alt="GitHub stars"></a>
</p>

<p align="center">Minimal Keyboard-driven web browser for tiling WMs, inspired by Zellij, built in Go.</p>

<p align="center"><a href="https://bnema.dev/dumber">Website</a> · <a href="https://bnema.dev/dumber/docs">Documentation</a> · <a href="https://bnema.dev/dumber/docs/reference/keybindings">Keybindings</a></p>

https://github.com/user-attachments/assets/232822af-08e4-4a74-9416-87f79c96b118

## Overview

Dumber uses three layout levels:

- **Tabs** group separate browser contexts.
- **Workspaces** hold a layout of panes.
- **Panes** display web pages and can be split, stacked, moved, resized, or closed.

Most browser-management actions are exposed through modal keybindings. Enter a mode, run one or more commands, then leave the mode. The default modes cover pane management, tabs, Vim-style scrolling, resizing, and session commands.

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

- Pane, tab, Vim, resize, and session modes
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
curl -fsSL https://raw.githubusercontent.com/bnema/dumber/v0.32.0/install.sh | DUMBER_VERSION=v0.32.0 bash
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

For dependencies, distribution notes, and troubleshooting, see the [installation documentation](https://bnema.dev/dumber/docs).

## Keyboard modes

Dumber uses modal keybindings for browser management.

| Mode | Default key | Used for |
|------|-------------|----------|
| Pane | `Ctrl+P` | Split, stack, close, and move panes |
| Tab | `Ctrl+T` | Create, close, switch, and rename tabs |
| Vim Mode | `Ctrl+Y` | Navigate the active webpage with configurable Vim-style sequences; arrow keys stay native and other app shortcuts wait until exit |
| Resize | `Ctrl+N` | Resize panes with `hjkl` or arrow keys |
| Session | `Ctrl+O` | Snapshot, restore, and browse sessions |

## Floating pane

The floating pane is a temporary browser pane that can be toggled without changing the main workspace layout.

- `Alt+F` toggles the floating pane and preserves its state while hidden.
- `Ctrl+W` closes the active pane; if the floating pane is active, it is fully released for a fresh next open.
- Profile shortcuts such as `Alt+G` are optional and configured under `workspace.floating_pane.profiles`.
- Some `Alt+<key>` bindings may conflict with browser-engine shortcuts or desktop-level handlers.

See the [floating pane reference](https://bnema.dev/dumber/docs/reference/floating-pane) for setup and behavior details.

## Configuration

Dumber is configured with a single TOML file.

```toml
[engine]
type = "cef"

[workspace.floating_pane]
enabled = true
```

The config reloads while Dumber is running. See the [configuration documentation](https://bnema.dev/dumber/docs) for all options.

## Extensions and ad blocking

CEF does not expose the Extensions API, so browser extensions are not supported. While WebKitGTK can use
injected filter lists, this approach is not available with CEF.

For consistent ad and tracker blocking across applications and devices, use network-level DNS filtering such as
[AdGuard](https://adguard.com/) or [Pi-hole](https://pi-hole.net/). DNS filtering is more reliable than treating ad blocking as a browser extension, and it protects every device that uses your network.

## Browser engine

Dumber uses Chromium Embedded Framework by default. WebKitGTK is available as a second option but I don't use it daily so it is more prone to bugs.

On Arch Linux, install the CEF runtime with:

```bash
sudo pacman -S cef libwebp
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

### Rendering

CEF uses Dumber's GPU-first Wayland render stack by default: GDK DMABUF presentation with ANGLE/GSK Vulkan. For driver compatibility, switch to the EGL/OpenGL stack with `engine.cef.render_stack = "egl"`; the default is `"vulkan"`.

CEF OSR frame rate adapts to the active Wayland monitor refresh rate when both `engine.cef.adaptive_windowless_frame_rate = true` and `engine.cef.windowless_frame_rate = 0` are set, which are the defaults. Adaptive mode is capped by `engine.cef.windowless_frame_rate_max = 240`. Setting `engine.cef.windowless_frame_rate` to a positive value forces a fixed rate instead. This adaptive polling path is Wayland-specific; other platforms fall back to the configured fixed/default CEF behavior.

## Platform notes

### Dependencies

**Arch Linux:**

```bash
sudo pacman -S cef webkitgtk-6.0 gtk4 libwebp gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugin-va
```

**Debian/Ubuntu:**

```bash
sudo apt install libwebkitgtk-6.0-4 libgtk-4-1 gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad
```

> Ubuntu 24.04 ships GLib 2.80, but Dumber requires GLib 2.84+. Use Arch, Fedora 41+, or the Flatpak.

## Development

Dumber uses pure-Go bindings. The GUI uses GTK4, runs on CEF by default, and can use WebKitGTK as a fallback backend.

Set `ENV=dev` to isolate the development process under `.dev/dumber/`. Dumber sets `HOME` plus `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, and `XDG_CACHE_HOME` there before GTK or the browser engine starts, so development runs do not use production profiles, cookies, caches, or logs.

### Build from source

**Prerequisites:**

- Go 1.27+
- GTK4 development packages
- CEF runtime (default backend)
- WebKitGTK 6.0 development/runtime packages (fallback backend and runtime checks)
- Brotli for compressed systemviews assets
- libwebp runtime for WebP favicon decoding (for example `libwebp` on Arch)

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
| `make verify-generated` | Verify tracked generated systemviews artifacts are committed |
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
