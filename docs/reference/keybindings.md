# Keybindings

Dumber uses modal keybindings inspired by Zellij. Press a mode activation key, then use action keys within that mode.

## Mode Activation

| Mode | Key | Purpose |
|------|-----|---------|
| Pane Mode | `Ctrl+P` | Split, close, focus panes |
| Tab Mode | `Ctrl+T` | Create, close, switch tabs |
| Page Mode | `Ctrl+Y` | Scroll the active webpage with `h/j/k/l` and `Shift+J/K`; arrow keys stay native and other app shortcuts wait until exit |
| Resize Mode | `Ctrl+N` | Resize pane splits |
| Session Mode | `Ctrl+O` | Session management |

Press `Escape` or `Enter` to exit any mode.
Page Mode activation is passed through when the focused webpage element is already editable, so browser-page editors keep their own `Ctrl+Y` behavior.

Keybinding tables use uppercase letters as visual labels for unshifted letter keys. In config, use lowercase (for example, `["w"]` for Pane Mode eject). Shifted keys are shown with an explicit `Shift+` prefix.

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
| Eject to window | `W` |
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

## Page Mode (`Ctrl+Y`)

Page Mode is an explicit page-scrolling mode for the active pane only. It shows a local `PAGE` indicator on the owning pane, uses `workspace.styling.pane_mode_color` for the local accent, and exits automatically when focus moves into the omnibox, find bar, overlays, or an editable element inside the page. The default `timeout_ms` is `0`, so Page Mode does not auto-time out unless you configure one. Arrow keys continue to flow through the browser engine's native page-navigation path while Page Mode is active, while other app-level shortcuts stay suspended until you leave the mode.

Scroll execution depends on the active browser engine:
- **CEF**: Page Mode scroll commands (`h/j/k/l`, `Shift+J/K`) are translated to native Chromium key events (arrow keys, Page Up/Down) for reliable behaviour with SPA scroll containers.
- **WebKit**: Falls back to JavaScript-driven scroll delta injection handled by `BuildScrollByJS`, which resolves the nearest scrollable ancestor and coalesces held-key repeats via `requestAnimationFrame`.

| Action | Keys |
|--------|------|
| Scroll left | `H` |
| Scroll down | `J` |
| Scroll up | `K` |
| Scroll right | `L` |
| Scroll down fast | `Shift+J` |
| Scroll up fast | `Shift+K` |
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
| Toggle floating pane | `Alt+F` |
| Toggle History sidebar (native GTK sidebar panel only). Ctrl+H may conflict with the browser's default History shortcut; behavior can vary by browser. | `Ctrl+H` |
| Toggle Favorites system view in right split | unbound by default |
| Toggle Config system view in right split | unbound by default |
| Close pane (or release floating pane) | `Ctrl+W` |
| Next tab | `Ctrl+Tab` |
| Previous tab | `Ctrl+Shift+Tab` |
| Consume/expel left | `Alt+[` |
| Consume/expel right | `Alt+]` |
| Consume/expel up | `Alt+{` |
| Consume/expel down | `Alt+}` |

- `Alt+F` is the only floating-pane shortcut enabled by default.
- `Alt+F` toggles floating visibility and keeps floating pane state intact.
- `Ctrl+H` toggles the native GTK history sidebar. The sidebar shows browsing history grouped by day with search/filter, keyboard navigation (arrows, Home/End, Ctrl+arrows for day jumps), and activation modes (Enter to navigate while keeping the sidebar open, Ctrl+Enter to navigate while keeping the sidebar open, Shift+Enter to open in a new split). If the native sidebar is unavailable, the shortcut returns an error instead of falling back to `dumb://history`.
- `Ctrl+W` closes the active pane; when the floating pane is active, it fully releases that floating session.
- Any URL shortcut (for example `Alt+G`) must be defined explicitly in `workspace.floating_pane.profiles`.
- Floating profile shortcuts support modifier combos with `ctrl`, `shift`, and `alt` (for example `ctrl+shift+y` or `ctrl+alt+m`).

Warning: some `Alt+<key>` combinations may conflict with default WebKit shortcuts, website handlers, or your desktop environment.
If a shortcut does not trigger in Dumber, choose a different keybinding.

For details, see [Floating Pane](./floating-pane.md).

## Customization

All keybindings can be customized in `~/.config/dumber/config.toml`:

```toml
[workspace.pane_mode.actions]
split-right = ["arrowright", "r"]
close-pane = ["x", "q"]

[workspace.page_mode]
activation_shortcut = "ctrl+y"
timeout_ms = 0

[workspace.page_mode.actions.page-scroll-left]
keys = ["h"]

[workspace.page_mode.actions.page-scroll-down]
keys = ["j"]

[workspace.page_mode.actions.page-scroll-up]
keys = ["k"]

[workspace.page_mode.actions.page-scroll-right]
keys = ["l"]

[workspace.page_mode.actions.page-scroll-down-fast]
keys = ["shift+j"]

[workspace.page_mode.actions.page-scroll-up-fast]
keys = ["shift+k"]

[workspace.shortcuts.actions.close-pane]
keys = ["ctrl+w"]

[workspace.shortcuts.actions.toggle-floating-pane]
keys = ["alt+f"]

[workspace.shortcuts.actions.toggle-history-systemview]
keys = ["ctrl+h"]

[workspace.shortcuts.actions.toggle-favorites-systemview]
keys = []

[workspace.shortcuts.actions.toggle-config-systemview]
keys = []

[workspace.floating_pane]
width_pct = 0.82
height_pct = 0.72

[workspace.floating_pane.profiles.google]
keys = ["alt+g"]
url = "https://google.com"

[workspace.floating_pane.profiles.github]
keys = ["alt+h"]
url = "https://github.com"
```

See [Configuration](../config/index.md) for full details.
