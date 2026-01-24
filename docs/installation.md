# Installation

## Quick Install (Linux x86_64)

```bash
curl -fsSL https://dumber.bnema.dev/install | bash
```

Installs to `~/.local/bin` if available, otherwise `/usr/local/bin` (requires sudo).

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
go build -o dumber ./cmd/dumber
```

### Build Dependencies

- Go 1.25+
- GTK4 development libraries
- WebKitGTK 2.42+ development libraries
- GStreamer development libraries

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
