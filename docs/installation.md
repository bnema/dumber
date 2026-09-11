# Installation

## Quick Install (Linux x86_64)

```bash
curl -fsSL https://raw.githubusercontent.com/bnema/dumber/main/install.sh | bash
```

Installs to `~/.local/bin` if available, otherwise `/usr/local/bin` (requires sudo).

### Advanced Installation Options

Install a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/bnema/dumber/main/install.sh | DUMBER_VERSION=v0.26.2 bash
```

Install the latest prerelease:

```bash
curl -fsSL https://raw.githubusercontent.com/bnema/dumber/main/install.sh | DUMBER_PRERELEASE=1 bash
```

Combine both to install a specific prerelease:

```bash
curl -fsSL https://raw.githubusercontent.com/bnema/dumber/main/install.sh | DUMBER_VERSION=v0.27.0-rc.1 DUMBER_PRERELEASE=1 bash
```

## Flatpak

```bash
# From Flathub (when available)
flatpak install flathub dev.bnema.Dumber

# Or from release
flatpak install dumber.flatpak
```

## From Source

```bash
git clone https://github.com/bnema/dumber
cd dumber
make build
./dist/dumber browse
```

### Build Dependencies

- Go 1.27+
- GTK4 development libraries
- CEF runtime for the default backend
- WebKitGTK 6.0 development/runtime libraries for the fallback backend and runtime checks
- GStreamer development libraries for WebKit media playback and diagnostics
- Brotli for compressed systemviews assets

Systemviews assets are generated with `go tool templ` and Go's `js/wasm` toolchain; no root Node toolchain is required.

## Post-Install

Run the setup wizard to configure desktop integration:

```bash
dumber setup
```

This creates:
- Desktop entry (`~/.local/share/applications/`)
- Default configuration (`~/.config/dumber/config.toml`)

## Verify Installation

```bash
dumber about
dumber doctor
```
