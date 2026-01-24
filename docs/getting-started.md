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
| Resize Mode | `Ctrl+N` | Resize pane splits |
| Session Mode | `Ctrl+O` | Session management |

Press `Escape` or `Enter` to exit any mode.

## Pane Mode Quick Reference

1. Press `Ctrl+P` to enter pane mode
2. Use arrow keys or `hjkl` to split in that direction
3. `Shift+arrows` to focus adjacent panes
4. `X` to close current pane

## Omnibox

Press `Ctrl+L` to open the omnibox for:
- URL navigation
- Search (uses default search engine)
- Bang shortcuts (`!g query` for Google, `!gh query` for GitHub)

## Configuration

Edit `~/.config/dumber/config.toml` or use:

```bash
dumber config edit
```

See [Configuration](./config/index.md) for all options.

## Launcher Integration

Use with rofi or fuzzel:

```bash
dumber dmenu | rofi -dmenu -show-icons | dumber dmenu --select
```

## Next Steps

- [Configuration Reference](./config/reference.md) - All settings
- [Keybindings](./reference/keybindings.md) - Full keyboard shortcuts
- [CLI Commands](./cli/index.md) - Command-line tools
