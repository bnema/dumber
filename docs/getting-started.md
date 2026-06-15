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
| Page Mode | `Ctrl+Y` | Scroll the active webpage with `h/j/k/l` and `Shift+J/K`; arrow keys still work natively |
| Resize Mode | `Ctrl+N` | Resize pane splits |
| Session Mode | `Ctrl+O` | Session management |

Press `Escape` or `Enter` to exit any mode.
Page Mode stays local to the active pane and automatically leaves the mode when focus moves into the omnibox, find bar, overlays, or a page editable.

## Pane Mode Quick Reference

1. Press `Ctrl+P` to enter pane mode
2. Use arrow keys or `hjkl` to split in that direction
3. `Shift+arrows` to focus adjacent panes
4. `X` to close current pane

## Page Mode Quick Reference

1. Press `Ctrl+Y` to enter Page Mode
2. Use `h`, `j`, `k`, `l` to scroll left, down, up, or right
3. Use `Shift+J` / `Shift+K` for faster vertical jumps
4. Arrow keys continue to use the browser engine's native page navigation while Page Mode is active
5. Press `Escape` or `Enter` to leave the mode
6. If a page input or editor is already focused, `Ctrl+Y` is passed through instead of stealing the keypress

## Omnibox

Press `Ctrl+L` to open the omnibox for:
- URL navigation
- Search (uses default search engine)
- Bang shortcuts (`!g query` for Google, `!gh query` for GitHub)

## Floating Pane

- Press `Alt+F` to toggle the floating pane.
- Press `Ctrl+W` to close the active pane; if the floating pane is active, this fully releases it so the next open starts fresh.
- Some `Alt+<key>` bindings can conflict with default WebKit shortcuts or desktop-level handlers.

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

> ⚠️ **Note**: Screen sharing does not currently work on Wayland with WebKitGTK 6.0. This is a known WebKitGTK limitation.

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
