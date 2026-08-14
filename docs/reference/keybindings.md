# Keybindings

Dumber uses modal keybindings inspired by Zellij. Press a mode activation key, then use action keys within that mode.

## Mode Activation

| Mode | Key | Purpose |
|------|-----|---------|
| Pane Mode | `Ctrl+P` | Split, close, focus panes |
| Tab Mode | `Ctrl+T` | Create, close, switch tabs |
| Vim Mode | `Ctrl+Y` | Scroll, navigate headings, and focus page inputs with Vim-style commands; arrow keys stay native and page focus stays inside the page while other app shortcuts wait until exit |
| Resize Mode | `Ctrl+N` | Resize pane splits |
| Session Mode | `Ctrl+O` | Session management |

Press `Escape` or `Enter` to exit any mode.
`Ctrl+Y` can activate Vim Mode even when a page input is already focused, so you can use Vim commands without first moving focus away from the editor.

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

## Vim Mode (`Ctrl+Y`)

Vim Mode is an explicit Vim-style navigation mode for the active pane only. It applies a pane-local accent from `workspace.styling.pane_mode_color`, and when `workspace.styling.mode_indicator_toaster_enabled` is true, shows a persistent bottom-left `VIM MODE` toaster while the mode is active; the toaster hides on exit, when the mode is toggled off, or when the toaster is disabled in config. The mode exits automatically when focus moves into the omnibox, find bar, overlays, or an editable element inside the page. The default `timeout_ms` is `0`, so Vim Mode does not auto-time out unless you configure one. Arrow keys continue to flow through the browser engine's native page-navigation path while Vim Mode is active, while other app-level shortcuts stay suspended until you leave the mode. `Ctrl+Y` remains available when an editable page control is already focused.

CEF and WebKit execute Vim Mode scroll commands (`h/j/k/l`, `Shift+J/K`) with the shared `BuildPageScrollByJS` resolver. Each step starts under the viewport center, walks up through ancestors that can move in the requested direction, and hands scrolling to the document when a nested container reaches its boundary. Cross-origin frame contents remain best-effort.

`gi` focuses the next visible, editable page input. With no eligible input focused it starts at the first one; repeated `gi` commands cycle through the inputs. `]]` and `[[` move through visible `h1`–`h6` headings and outline the selected heading with the current Dumber theme accent. When a page input is focused, `Tab` and `Shift+Tab` keep focus traversal inside the page; traversal stops safely at the page boundaries instead of moving into the host window. These live input, heading, and page-focus motions currently use the CEF engine path; the WebKit fallback supports Vim scrolling but not these semantic motions yet.

| Action | Keys |
|--------|------|
| Scroll left | `H` |
| Scroll down | `J` |
| Scroll up | `K` |
| Scroll right | `L` |
| Scroll down fast | `Shift+J` |
| Scroll up fast | `Shift+K` |
| Focus next page input | `gi` |
| Next heading | `]]` |
| Previous heading | `[[` |
| Page focus traversal | `Tab`, `Shift+Tab` |
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
| Toggle Favorites sidebar (native GTK bookmarks panel) | `Ctrl+B` |
| Toggle current page favorite/bookmark | `Ctrl+D` |
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
- `Ctrl+B` toggles the native GTK Favorites sidebar. Search matches favorite titles, URLs, and tag names. `Tab`/`Shift+Tab` traverse search, every tag filter, and the favorites list; arrow keys navigate favorites once the list is focused. The `+` at the right end of the tag-filter row creates a named tag. With a favorite selected in the list, `+` opens its tag binding controls. `Enter` and `Ctrl+Enter` open the selected favorite in the current pane while keeping the sidebar open; `Shift+Enter` opens it in a new split. Inside the sidebar, `a` adds, `e` edits, `s` opens shortcut mode, `Delete` starts delete confirmation, `/` focuses search, `Esc` clears/cancels/closes, `r` reloads, and `c` clears search and filters.
- `Ctrl+D` toggles the active page as a favorite/bookmark. Favorite shortcut metadata can be assigned in the sidebar, but global `Alt+1..9` remains tab switching.
- `Ctrl+W` closes the active pane; when the floating pane is active, it fully releases that floating session.
- Any URL shortcut (for example `Alt+G`) must be defined explicitly in `workspace.floating_pane.profiles`.
- Floating profile shortcuts support modifier combos with `ctrl`, `shift`, and `alt` (for example `ctrl+shift+y` or `ctrl+alt+m`).

Warning: some `Alt+<key>` combinations may conflict with browser-engine shortcuts, website handlers, or your desktop environment.
If a shortcut does not trigger in Dumber, choose a different keybinding.

For details, see [Floating Pane](./floating-pane.md).

## Customization

All keybindings can be customized in `~/.config/dumber/config.toml`:

```toml
[workspace.pane_mode.actions]
split-right = ["arrowright", "r"]
close-pane = ["x", "q"]

[workspace.vim_mode]
activation_shortcut = "ctrl+y"
timeout_ms = 0

[workspace.vim_mode.actions.vim-scroll-left]
keys = ["h"]

[workspace.vim_mode.actions.vim-scroll-down]
keys = ["j"]

[workspace.vim_mode.actions.vim-scroll-up]
keys = ["k"]

[workspace.vim_mode.actions.vim-scroll-right]
keys = ["l"]

[workspace.vim_mode.actions.vim-scroll-down-fast]
keys = ["shift+j"]

[workspace.vim_mode.actions.vim-scroll-up-fast]
keys = ["shift+k"]

[workspace.vim_mode.actions.focus-input]
keys = ["gi"]

[workspace.vim_mode.actions.heading-next]
keys = ["]]"]

[workspace.vim_mode.actions.heading-prev]
keys = ["[["]

[workspace.shortcuts.actions.close-pane]
keys = ["ctrl+w"]

[workspace.shortcuts.actions.toggle-floating-pane]
keys = ["alt+f"]

[workspace.shortcuts.actions.toggle-history-systemview]
keys = ["ctrl+h"]

[workspace.shortcuts.actions.toggle-favorites-sidebar]
keys = ["ctrl+b"]

[workspace.shortcuts.actions.toggle-current-page-favorite]
keys = ["ctrl+d"]

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
