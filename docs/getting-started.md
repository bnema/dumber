# Getting Started

## Launch the Browser

```bash
dumber browse
```

Or use the desktop entry after running `dumber setup`.

## Basic Navigation

Dumber uses modal keybindings inspired by Zellij:

| Mode | Activation | Purpose |
|------|------------|---------|
| Pane Mode | `Ctrl+P` | Split, close, focus panes |
| Tab Mode | `Ctrl+T` | Create, close, switch tabs |
| Vim Mode | `Ctrl+Y` | Scroll, navigate headings, and focus page inputs with Vim-style commands; arrow keys stay native and page focus stays inside the page while other app shortcuts wait until exit |
| Resize Mode | `Ctrl+N` | Resize pane splits |
| Session Mode | `Ctrl+O` | Session management |

Press `Escape` or `Enter` to exit any mode.
Vim Mode stays local to the active pane and automatically leaves the mode when focus moves into the omnibox, find bar, overlays, or a page editable.

## Pane Mode Quick Reference

1. Press `Ctrl+P` to enter pane mode
2. Use arrow keys or `hjkl` to split in that direction
3. `Shift+arrows` to focus adjacent panes
4. `X` to close current pane

## Vim Mode Quick Reference

1. Press `Ctrl+Y` to enter Vim Mode
2. Use `h`, `j`, `k`, `l` to scroll left, down, up, or right
3. Use `Shift+J` / `Shift+K` for faster vertical jumps
4. Arrow keys continue to use the browser engine's native page navigation while Vim Mode is active
5. Use `gi` to cycle through visible page inputs; use `]]` and `[[` to move through headings
6. With CEF, press `Enter` to open a link in the selected heading and leave Vim Mode; if the heading has no link, it only leaves the mode
7. `Tab` and `Shift+Tab` traverse focusable page controls while a page input is focused
8. Other app-level shortcuts stay suspended until you leave Vim Mode
9. Press `Escape` to leave the mode without activating the selected heading
10. `Ctrl+Y` can activate Vim Mode even when a page input or editor is already focused

> **Engine behavior**: CEF and WebKit execute Vim Mode scroll steps with the
> shared `BuildPageScrollByJS` resolver (viewport-center start, nested-scroller
> handoff, document fallback). The application repeater owns held-key cadence;
> each engine runs one immediate step per tick. Cross-origin frames are
> best-effort. CEF may use native precision-wheel input only before the browser
> frame is ready.

## Omnibox

Press `Ctrl+L` to open the omnibox for:
- URL navigation
- Search (uses default search engine)
- Bang shortcuts (`!g query` for Google, `!gh query` for GitHub)

## Floating Pane

- Press `Alt+F` to toggle the floating pane.
- Press `Ctrl+W` to close the active pane; if the floating pane is active, this fully releases it so the next open starts fresh.
- Some `Alt+<key>` bindings can conflict with browser-engine shortcuts or desktop-level handlers.

See [Floating Pane](./reference/floating-pane.md) for profile shortcuts and configuration.

## Configuration

Edit `~/.config/dumber/config.toml` or use:

```bash
dumber config open
```

See [Configuration](./config/index.md) for all options.

## Launcher Integration

Use with rofi or fuzzel:

```bash
dumber dmenu | rofi -dmenu -show-icons | dumber dmenu --select
```

## Website Permissions

Dumber includes a built-in permissions system for camera, microphone, and screen sharing:

- **Custom dialog** - Clean permission prompts replace native GTK dialogs
- **Persistent choices** - "Always Allow" and "Always Deny" are saved per-origin
- **Privacy-focused** - All permissions stored locally
- **Camera & Microphone** - Fully working on Wayland/PipeWire

> ⚠️ **WebKit fallback note**: Screen sharing does not currently work on Wayland with WebKitGTK 6.0. This is a known WebKitGTK limitation and does not describe the default CEF backend.

When a website requests camera or microphone access, you'll see a permission dialog with options to allow once, always allow, deny, or always deny.

See [Website Permissions](./reference/permissions.md) for details.

## Crash Reporting

If Dumber exits unexpectedly, crash reports are automatically generated:

```bash
# List all crash reports
dumber crashes

# View the latest crash report
dumber crashes show latest

# Generate GitHub issue payload
dumber crashes issue latest
```

See [Session Exit Classification](./reference/session-exit-classification-runbook.md) for troubleshooting.

## Next Steps

- [Configuration Reference](./reference/configuration.md) - All settings
- [Keybindings](./reference/keybindings.md) - Full keyboard shortcuts
- [CLI Commands](./cli/index.md) - Command-line tools
