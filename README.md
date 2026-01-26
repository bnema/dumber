# Dumber

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT) [![Go Report Card](https://goreportcard.com/badge/github.com/bnema/dumber)](https://goreportcard.com/report/github.com/bnema/dumber)

A TTY-inspired browser multiplexer for tiling window managers.

- Website: [dumber.bnema.dev](https://dumber.bnema.dev)
- Documentation: [dumber.bnema.dev/docs](https://dumber.bnema.dev/docs)

---

## What is Dumber?

Dumber is a Wayland-native browser that works like your terminal multiplexer. Tabs hold workspaces. Workspaces hold panes. Panes split and stack like Zellij. Everything is keyboard-driven.

No bloat. No Chrome. No Electron.



https://github.com/user-attachments/assets/232822af-08e4-4a74-9416-87f79c96b118



**Features:**
- Zellij-style panes with modal keyboard navigation
- Wayland native (Sway, Hyprland, River, Niri)
- Session resurrection with auto-snapshots
- Built-in ad blocking (UBlock-based)
- GPU accelerated video (VA-API/VDPAU)
- Launcher integration (rofi/fuzzel/dmenu)
- Search bangs (!g, !gi, !ddg)
- Single config.toml with hot reload

## Documentation

Full documentation is available at **[dumber.bnema.dev](https://dumber.bnema.dev)**

- [Documentation](https://dumber.bnema.dev/docs) - Installation, configuration, CLI reference
- [Keybindings](https://dumber.bnema.dev/docs/reference/keybindings) - Keyboard shortcuts

## Quick Start

```bash
# Install
curl -fsSL https://dumber.bnema.dev/install | sh

# Launch
dumber browse
```

### Arch Linux (AUR)

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

### Dependencies

**Arch Linux:**

```bash
sudo pacman -S webkitgtk-6.0 gtk4 gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugin-va
```

**Debian/Ubuntu:**

```bash
sudo apt install libwebkitgtk-6.0-4 libgtk-4-1 gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad
```

> ⚠️ Ubuntu 24.04 ships GLib 2.80, but Dumber requires GLib 2.84+. Use Arch, Fedora 41+, or the Flatpak.

## Keyboard-Driven

Four modal modes. Enter a mode, take action, escape out. Vim and Zellij users will feel right at home.

| Mode | Key | Actions |
|------|-----|---------|
| Pane | `Ctrl+P` | Split · Stack · Close · Move |
| Tab | `Ctrl+T` | New · Close · Switch · Rename |
| Resize | `Ctrl+N` | Grow · Shrink · hjkl/arrows |
| Session | `Ctrl+O` | Save · Restore · Browse |

## Development

Dumber uses pure-Go bindings (no CGO) but requires GTK4/WebKitGTK runtime libraries for the GUI.

### Environment

Set `ENV=dev` to use `.dev/dumber/` for config/data instead of XDG paths.

### Build From Source

**Prerequisites:**
- Go 1.25+
- Node.js 20+ (for frontend assets)
- WebKitGTK 6.0 and GTK4 dev packages

```bash
git clone https://github.com/bnema/dumber
cd dumber
make build
./dist/dumber browse
```

### Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Build frontend + binary |
| `make build-quick` | Build binary only (skip frontend) |
| `make dev` | Run with `go run` |
| `make test` | Run tests |
| `make lint` | Run golangci-lint |
| `make flatpak-build` | Build Flatpak bundle |

## Contributing

We welcome bug fixes, performance/stability improvements, and WebUI/UX enhancements. We're not accepting new feature PRs.

**All PRs must target the `next` branch.** We test there first before releasing to `main`.

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## Community

- [Report bugs](https://github.com/bnema/dumber/issues)
- [Releases](https://github.com/bnema/dumber/releases)

## License

MIT - See `LICENSE` for details.
