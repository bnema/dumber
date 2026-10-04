# Keybindings

Dumber uses modal keybindings. Pick the activation style with `workspace.keymap`:

- `zellij` (default): each mode has its own activation key, listed below.
- `tmux`: one prefix key, then a single action key. See [Keymap presets](#keymap-presets).

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

Vim Mode is an explicit Vim-style navigation mode for the active pane only. Its frame uses `workspace.styling.vim_mode_color`, a bottom-left `VIM MODE` toast stays visible while the mode is active, and a shortcut legend appears after 500 ms. Set `workspace.styling.mode_legend` to `always` or `off` to change the legend timing. The mode exits automatically when focus moves into the omnibox, find bar, overlays, or an editable element inside the page. The default `timeout_ms` is `0`, so Vim Mode does not auto-time out unless you configure one. Arrow keys continue to flow through the browser engine's native page-navigation path while Vim Mode is active, while other app-level shortcuts stay suspended until you leave the mode. `Ctrl+Y` remains available when an editable page control is already focused.

CEF and WebKit execute Vim Mode scroll commands (`h/j/k/l`, `Shift+J/K`) with the shared `BuildPageScrollByJS` resolver. Each step starts under the viewport center, walks up through ancestors that can move in the requested direction, and hands scrolling to the document when a nested container reaches its boundary. Cross-origin frame contents remain best-effort.

`gi` focuses the next visible, editable page input. With no eligible input focused it starts at the first one; repeated `gi` commands cycle through the inputs. `]]` and `[[` move through visible `h1`–`h6` headings and outline the selected heading with the current Dumber theme accent. `Enter` activates the first link inside the selected heading, or a link wrapping that heading, then leaves Vim Mode; without an associated link, it only leaves the mode. `Escape` leaves without activation. When a page input is focused, `Tab` and `Shift+Tab` keep focus traversal inside the page; traversal stops safely at the page boundaries instead of moving into the host window. These live input, heading, activation, and page-focus actions currently use the CEF engine path; the WebKit fallback supports Vim scrolling but not these semantic actions yet.

Link hints label the visible targets in the viewport; type a label to pick one, `Backspace` to undo a letter, and `Escape` to close the hints and stay in Vim Mode. `f` follows a link or activates a control, `F` opens a link in a new pane, and `yf` copies a link URL.

`v` starts visual selection. A non-empty selection that is visible in the viewport is used as is. Otherwise text-anchor hints label the visible text blocks; type a label to put the caret at the start of that text, or press `Escape` to place it at the first visible text of at least 50 characters (any visible text if none is that long). The mode indicator follows the page state: `VIM MODE · HINTS`, `· CARET`, `· VISUAL`, `· VISUAL LINE`.

In caret mode a large blinking accent cursor marks the position and motions move it without selecting: `h/j/k/l`, `w/b/e`, `0`/`^`/`$`, `(`/`)` for sentences, `{`/`}` for paragraphs, `gg`/`G` for the document edges, and a count prefix such as `3w`. The view scrolls to keep the cursor visible. `v` starts a character selection from the caret and `V` selects whole lines; in both, the same motions extend the selection and `o` swaps its ends. `v` in a selection returns to the caret, `V` in a character selection switches to lines, and `v` in a line selection switches to characters. `y` or `Enter` copies the selection and returns to the caret at its end, so you can keep moving and copy again. `Escape` steps back from a selection to the caret, and `Escape` in caret mode ends visual selection and leaves plain Vim Mode.

Text-object yanks copy one block without selecting it: `yah` copies the current section (the heading nearest the top of the viewport through the next heading of the same or a higher level), and `yap`, `yac`, `yat`, and `yal` copy the first visible paragraph, code block, table, or list. The copied block flashes with the theme accent. Hints, visual selection, and yanks use the CEF engine path.

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
| Follow link or control (hints) | `f` |
| Open link in new pane (hints) | `F` |
| Yank link URL (hints) | `yf` |
| Visual selection (caret, character, line) | `v`, `V` in caret mode |
| Yank section / paragraph / code / table / list | `yah`, `yap`, `yac`, `yat`, `yal` |
| Activate selected heading link and exit | `Enter` |
| Exit without activation | `Escape` |

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

## Keymap Presets

```toml
[workspace]
keymap = "tmux"   # or "zellij" (default)
```

With `keymap = "tmux"`, the Pane, Tab, Resize, and Session activation keys are unbound. Press the prefix (`Ctrl+Space` by default), then one action key. Dumber runs the action and returns to normal mode; an unbound key or `Escape` cancels the prefix. Vim Mode keeps `Ctrl+Y` in both presets.

| After `Ctrl+Space` | Action |
|--------------------|--------|
| `%` | Split right |
| `"` | Split down |
| `x` | Close pane |
| `Shift+S` | Stack pane |
| `!` | Move pane to another tab |
| `←` `→` `↑` `↓` | Focus pane |
| `c` | New tab |
| `&` | Close tab |
| `n` / `p` | Next / previous tab |
| `,` | Rename tab |
| `s` | Session manager |
| `r` | Enter Resize Mode |
| `[` | Enter Vim Mode |

Change the prefix or its actions under `[workspace.prefix_mode]`. Symbols such as `%` and `"` match whichever Shift state your layout needs, so write them without `shift+`. `Ctrl+Space` is often used to switch input methods; pick another prefix (for example `alt+a`) if your desktop already uses it.

```toml
[workspace.prefix_mode]
activation_shortcut = "alt+a"
timeout_ms = 2000

[workspace.prefix_mode.actions.split-right]
keys = ["%", "v"]
```

## Global Shortcuts

These work outside modal modes in both keymaps. Every entry lives under `[workspace.shortcuts.actions]`, so you can rebind it or disable it with `keys = []`.

| Action | Keys |
|--------|------|
| Open omnibox | `Ctrl+L` |
| Find in page / next / previous | `Ctrl+F` / `F3`, `Ctrl+G` / `Shift+F3`, `Ctrl+Shift+G` |
| Reload / hard reload | `Ctrl+R`, `F5` / `Ctrl+Shift+R`, `Ctrl+F5` |
| Back / forward | `Ctrl+←` / `Ctrl+→` |
| Zoom in / out / reset | `Ctrl++`, `Ctrl+=` / `Ctrl+-` / `Ctrl+0` |
| Focus pane | `Alt+H/J/K/L`, `Alt+arrows` |
| Copy URL | `Ctrl+Shift+C` |
| Print | `Ctrl+Shift+P` |
| Session manager | `Ctrl+Shift+S` |
| Developer tools | `F12` |
| Fullscreen | `F11` |
| Quit | `Ctrl+Q` |
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

[workspace.vim_mode.actions.hint-follow]
keys = ["f"]

[workspace.vim_mode.actions.visual]
keys = ["v"]

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
