# Keybindings

Dumber uses modal keybindings inspired by Zellij. Press a mode activation key, then use action keys within that mode.

## Mode Activation

| Mode | Key | Purpose |
|------|-----|---------|
| Pane Mode | `Ctrl+P` | Split, close, focus panes |
| Tab Mode | `Ctrl+T` | Create, close, switch tabs |
| Resize Mode | `Ctrl+N` | Resize pane splits |
| Session Mode | `Ctrl+O` | Session management |

Press `Escape` or `Enter` to exit any mode.

## Pane Mode (`Ctrl+P`)

| Action | Keys |
|--------|------|
| Split right | `→`, `R` |
| Split left | `←`, `L` |
| Split up | `↑`, `U` |
| Split down | `↓`, `D` |
| Stack pane | `S` |
| Close pane | `X` |
| Move to tab | `M` |
| Move to next tab | `Shift+M` |
| Focus right | `Shift+→`, `Shift+L` |
| Focus left | `Shift+←`, `Shift+H` |
| Focus up | `Shift+↑`, `Shift+K` |
| Focus down | `Shift+↓`, `Shift+J` |
| Consume/expel left | `[` |
| Consume/expel right | `]` |
| Consume/expel up | `{` |
| Consume/expel down | `}` |
| Confirm | `Enter` |
| Cancel | `Escape` |

## Tab Mode (`Ctrl+T`)

| Action | Keys |
|--------|------|
| New tab | `N`, `C` |
| Close tab | `X` |
| Next tab | `L`, `Tab` |
| Previous tab | `H`, `Shift+Tab` |
| Rename tab | `R` |
| Confirm | `Enter` |
| Cancel | `Escape` |

## Resize Mode (`Ctrl+N`)

| Action | Keys |
|--------|------|
| Increase left | `H`, `←` |
| Increase down | `J`, `↓` |
| Increase up | `K`, `↑` |
| Increase right | `L`, `→` |
| Decrease left | `Shift+H` |
| Decrease down | `Shift+J` |
| Decrease up | `Shift+K` |
| Decrease right | `Shift+L` |
| Increase (smart) | `+`, `=` |
| Decrease (smart) | `-` |
| Confirm | `Enter` |
| Cancel | `Escape` |

## Session Mode (`Ctrl+O`)

| Action | Keys |
|--------|------|
| Session manager | `S`, `W` |
| Confirm | `Enter` |
| Cancel | `Escape` |

## Global Shortcuts

These work outside modal modes:

| Action | Keys |
|--------|------|
| Close pane | `Ctrl+W` |
| Next tab | `Ctrl+Tab` |
| Previous tab | `Ctrl+Shift+Tab` |
| Consume/expel left | `Alt+[` |
| Consume/expel right | `Alt+]` |
| Consume/expel up | `Alt+{` |
| Consume/expel down | `Alt+}` |

## Customization

All keybindings can be customized in `~/.config/dumber/config.toml`:

```toml
[workspace.pane_mode.actions]
split-right = ["arrowright", "r"]
close-pane = ["x", "q"]

[workspace.shortcuts.actions.close_pane]
keys = ["ctrl+w"]
```

See [Configuration](../config/index.md) for full details.
