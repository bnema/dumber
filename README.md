# Dumber — a tiny, dumb browser for Wayland

Dumber is a minimalist browser and launcher companion focused on speed and simplicity. It ships a built‑in WebKit2GTK window for navigation and a CLI with dmenu‑style launcher integration (rofi, fuzzel).

- Native GUI via WebKit2GTK (CGO) with an embedded homepage UI.
- CLI that parses URLs, supports search shortcuts (e.g., `g:query`), and integrates with rofi/dmenu.
- Persistent history and per‑domain zoom levels (SQLite, XDG locations).
- Configurable via `config.json` and environment variables, with live reload.

## Wayland‑First
- Dumb on purpose, designed for Wayland window managers (Sway, Hyprland, River, Niri, etc..) — a tiny companion that plays nicely with your launcher.
- Uses a native WebKit2GTK window; no external browser required and no X11/XWayland dependency.
- Optimized for dmenu‑style launchers (rofi, fuzzel, wofi) and keyboard‑driven workflows.

## Status
- Very early stage — expect sharp edges and breaking changes, but it works well enough to browse and play with the launcher flows.

## Features
- Built‑in browser window (no external browser needed)
- Custom `dumb://` scheme serving embedded frontend assets
- Keyboard shortcuts: DevTools (F12), Omnibox toggle (Ctrl/Cmd+L), Zoom In/Out/Reset
- Persistent history with search and stats
- Per‑domain zoom persistence
- dmenu‑style launcher integration (rofi, fuzzel) with history and shortcut suggestions

## Quick Start
Prerequisites:
- Go 1.25+
- Node.js 20+ (for building the embedded frontend)
- For GUI build: WebKit2GTK and GTK dev packages (examples)
  - Debian/Ubuntu: `libwebkit2gtk-4.1-dev libgtk-3-dev build-essential`
  - Arch: `webkit2gtk gtk3 base-devel`

Build options:
- GUI (recommended):
  - `make build-gui`      # builds with `-tags=webkit_cgo`
  - Run: `./dist/dumber`  # opens the embedded homepage
- CLI‑only (no GUI, CGO disabled):
  - `make build-static`
  - Run: `./dist/dumber-static version`
- Full build pipeline (frontend + Go):
  - `make build`

Development:
- `make dev` runs `go run .` (non‑CGO; GUI stubs log to console)
- `make test` runs tests; `make lint` runs golangci‑lint

## Install
- CLI‑only (no native window):
  - `go install github.com/bnema/dumber@latest`
  - Useful everywhere; runs the CLI and non‑CGO flows. In this mode the GUI code is stubbed (no native window shown).
- GUI build (Wayland + WebKit2GTK via CGO):
  - Ensure WebKit2GTK/GTK dev packages are installed (see prerequisites) and CGO is enabled in your environment.
  - `go install -tags webkit_cgo github.com/bnema/dumber@latest`
  - If pkg‑config can’t find headers, set `PKG_CONFIG_PATH` appropriately or prefer `make build-gui`.

## Usage
- Open a URL or search:
  - `dumber browse https://example.com`
  - `dumber browse example.com`        # scheme auto‑added
  - `dumber browse g:golang`           # Google search via shortcut
- Launch the GUI directly:
  - `dumber`                           # no args → opens GUI homepage
- Launcher integration (dmenu‑style examples):
  - rofi:   `dumber dmenu | rofi -dmenu -p "Browse: " | dumber dmenu --select`
  - fuzzel: `dumber dmenu | fuzzel --dmenu -p "Browse: " | dumber dmenu --select`

### Dmenu mode invocation
You can invoke dmenu mode in two ways:
- Subcommand (recommended): `dumber dmenu` … and `dumber dmenu --select`
- Root flag (generate options only): `dumber --dmenu`

Note: The root flag path only generates options; for processing a selection (`--select`), use the `dmenu` subcommand as the receiving command.

In GUI mode the app serves an embedded homepage via `dumb://homepage`, and frontend assets under `dumb://app/...`.

## Configuration
Dumber follows the XDG Base Directory spec:
- Config: `~/.config/dumber/config.json`
- Data:   `~/.local/share/dumber`
- State:  `~/.local/state/dumber` (default DB lives here)

A default `config.json` is created on first run. Key fields:
- `database.path`: SQLite database path (defaults to XDG state dir `history.db`)
- `search_shortcuts`: map of search shortcuts, e.g.
  ```json
  {
    "g":  { "url": "https://www.google.com/search?q=%s", "description": "Google" },
    "gh": { "url": "https://github.com/search?q=%s",    "description": "GitHub" }
  }
  ```
- `appearance`: default fonts and base font size for pages without specified fonts
- `dmenu`: options for launcher integration (history count, prefixes, etc.)

Environment variables use the `DUMB_BROWSER_` prefix (managed by Viper). Examples:
- `DUMB_BROWSER_DATABASE_PATH=...`
- `DUMB_BROWSER_DMENU_MAX_HISTORY_ITEMS=50`

Config changes are watched and applied at runtime when possible.

## Data Model
SQLite tables (created automatically):
- `history(id, url, title, visit_count, last_visited, created_at)`
- `shortcuts(shortcut, url_template, description, created_at)`
- `zoom_levels(domain PRIMARY KEY, zoom_factor, updated_at)`

## Building With WebKit2GTK
- The GUI requires building with `-tags=webkit_cgo` and CGO enabled.
- Ensure WebKit2GTK and GTK dev headers are installed (see prerequisites).
- Make targets handle both frontend and Go build steps:
  - `make build-gui` (CGO enabled)
  - `make build` (CGO enabled by default in the Makefile’s main build)

Without the native tag, a stub backend is used: you can still run CLI flows and see logs, but no native window is displayed.

## Project Structure
- `main.go`: CLI/GUI entrypoint, `dumb://` scheme, service wiring
- `internal/cli`: Cobra commands (`browse`, `dmenu`, `version`)
- `internal/config`: Viper‑backed config, XDG helpers, defaults
- `internal/db`: SQLite schema init and sqlc‑generated queries
- `services`: app services (parser/history/config/browser)
- `pkg/webkit`: WebKit2GTK bindings (CGO and stub implementations)
- `frontend`: TypeScript homepage UI bundled into the binary

## Security & Privacy
- History and zoom settings are stored locally in SQLite under XDG state.
- The `tmp/` directory is ignored by Git and kept out of history.

## Roadmap
- Full Vim-style motion and keyboard navigation across pages and UI.
- Performance work: faster startup, lower memory, snappier UI.
- Basic ad-block extension — because fuck ads.
- Other shiny things I’m not yet aware of.

## License
This project is licensed under the MIT License. See `LICENSE` for details.
