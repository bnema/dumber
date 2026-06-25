# Configuration Guide

**Location**: `~/.config/dumber/config.toml`
**Formats**: TOML (recommended), JSON, YAML

## Database

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `database.path` | string | `~/.local/share/dumber/dumber.db` | Database file path |

## History

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `history.max_entries` | int | `10000` | > 0 | Maximum number of history entries |
| `history.retention_period_days` | int | `365` | > 0 | Days to keep history (1 year) |
| `history.cleanup_interval_days` | int | `1` | > 0 | How often to run cleanup |

## Search Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `default_search_engine` | string | `"https://duckduckgo.com/?q=%s"` | Default search engine URL template (must contain `%s` placeholder) |
| `search_shortcuts` | map | See defaults | Map of shortcut aliases to URLs |

**Example:**
```toml
# Default search engine - used when no shortcut is specified
default_search_engine = "https://duckduckgo.com/?q=%s"

# Alternative search engines:
# default_search_engine = "https://www.google.com/search?q=%s"
# default_search_engine = "https://www.startpage.com/search?q=%s"
# default_search_engine = "https://search.brave.com/search?q=%s"
```

**Default shortcuts (usage: `!shortcut query`):**
```toml
# Examples:
#   !ddg golang      → DuckDuckGo search for "golang"
#   !g rust tutorial → Google search for "rust tutorial"
#   !gi cats         → Google Images search for "cats"
#   !gh opencode     → GitHub search for "opencode"
#   !yt music video  → YouTube search for "music video"

[search_shortcuts.ddg]
url = "https://duckduckgo.com/?q=%s"
description = "DuckDuckGo search"

[search_shortcuts.g]
url = "https://google.com/search?q=%s"
description = "Google Search"

[search_shortcuts.gi]
url = "https://google.com/search?tbm=isch&q=%s"
description = "Google Images"

[search_shortcuts.gh]
url = "https://github.com/search?q=%s"
description = "GitHub"

[search_shortcuts.yt]
url = "https://youtube.com/results?search_query=%s"
description = "YouTube"
```

## Dmenu / Launcher

These settings control the `dumber dmenu` CLI command for rofi/fuzzel integration.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `dmenu.max_history_days` | int | `30` | Number of days of history to show (0 = all) |
| `dmenu.show_visit_count` | bool | `true` | Show visit counts in output |
| `dmenu.show_last_visited` | bool | `true` | Show last visited dates in output |
| `dmenu.history_prefix` | string | `"🕒"` | Prefix for history items |
| `dmenu.shortcut_prefix` | string | `"🔍"` | Prefix for shortcuts |
| `dmenu.url_prefix` | string | `"🌐"` | Prefix for URLs |
| `dmenu.date_format` | string | `"2006-01-02 15:04"` | Go time format string |
| `dmenu.sort_by_visit_count` | bool | `true` | Sort by visit count instead of recency |

**CLI Usage:**
```bash
# Pipe mode (default) - outputs history for rofi/fuzzel with favicons
dumber dmenu | rofi -dmenu -show-icons -p "Browse: " | dumber dmenu --select
dumber dmenu | fuzzel --dmenu -p "Browse: " | dumber dmenu --select

# Interactive TUI mode
dumber dmenu --interactive

# Override history days via CLI flag (show last 7 days)
dumber dmenu --days 7

# Sort by most visited instead of recency
dumber dmenu --most-visited

# Combined: most visited from last 14 days
dumber dmenu --days 14 --most-visited
```

## Omnibox

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `omnibox.initial_behavior` | string | `"recent"` | `recent`, `most_visited`, `none` | Initial history display behavior |
| `omnibox.most_visited_days` | int | `30` | `>= 0` | Days of history to consider when `initial_behavior = "most_visited"` (`0` = all history) |
| `omnibox.auto_open_on_new_pane` | bool | `false` | - | Automatically open the omnibox after creating a new pane |

**Example:**
```toml
[omnibox]
initial_behavior = "recent"  # Show recent history when omnibox opens
most_visited_days = 30        # Days of history used for most_visited
auto_open_on_new_pane = false

# Alternative options:
# initial_behavior = "most_visited"  # Show most visited sites
# initial_behavior = "none"          # Show no initial suggestions
```

## Logging

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `logging.level` | string | `"info"` | `trace`, `debug`, `info`, `warn`, `error`, `fatal` | Log level |
| `logging.format` | string | `"text"` | `text`, `json`, `console` | Log format |
| `logging.max_age` | int | `7` | `>= 0` | Days to keep logs |
| `logging.max_files` | int | `100` | `>= 0` | Session log files to keep (`0` disables count cleanup) |
| `logging.log_dir` | string | `~/.local/state/dumber/logs` | any path | Log directory |
| `logging.enable_file_log` | bool | `true` | - | Enable file logging |
| `logging.capture_console` | bool | `false` | - | Capture browser console to logs |
| `logging.capture_gtk_logs` | bool | `false` | - | Capture GTK and WebKit logs for debugging. |

## Appearance

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `appearance.sans_font` | string | `"Fira Sans"` | Sans-serif font |
| `appearance.serif_font` | string | `"Fira Sans"` | Serif font |
| `appearance.monospace_font` | string | `"Fira Code"` | Monospace font |
| `appearance.default_font_size` | int | `16` | Font size in points |
| `appearance.color_scheme` | string | `"default"` | `prefer-dark`, `prefer-light`, `default` |
| `appearance.external_theme.enabled` | bool | `false` | Enable an external palette source |
| `appearance.external_theme.provider` | string | `"noctalia"` | External provider. Only `noctalia` is supported |
| `appearance.external_theme.format` | string | `"colors-json"` | External file format: `colors-json` or `dumber-json` |
| `appearance.external_theme.path` | string | `$XDG_CONFIG_HOME/noctalia/colors.json` for `colors-json`; built-in dumber-json template path for `dumber-json` | Path to the external theme JSON file |

### Color Palettes

**Light palette:**
```toml
[appearance.light_palette]
background = "#f8f8f8"
surface = "#f2f2f2"
surface_variant = "#ececec"
text = "#1a1a1a"
muted = "#6e6e6e"
accent = "#404040"
border = "#d2d2d2"
```

**Dark palette:**
```toml
[appearance.dark_palette]
background = "#0e0e0e"
surface = "#1a1a1a"
surface_variant = "#141414"
text = "#e4e4e4"
muted = "#848484"
accent = "#a8a8a8"
border = "#363636"
```

### External Theme: Noctalia

Dumber can integrate with Noctalia by reading the native Noctalia palette file directly. No user template is required for the default integration.

```toml
[appearance.external_theme]
enabled = true
provider = "noctalia"
format = "colors-json" # default
# Optional. Defaults to $XDG_CONFIG_HOME/noctalia/colors.json.
path = "~/.config/noctalia/colors.json"
```

The external file overrides palette colors only. Fonts, UI scale, pane-mode colors, and other appearance settings still come from Dumber config.

#### `colors-json` contract

`colors-json` reads Noctalia's active `colors.json` palette. Example file: [`docs/examples/noctalia-colors.json`](../examples/noctalia-colors.json).

Dumber maps Noctalia roles like this:

| Noctalia key | Dumber palette field |
|--------------|----------------------|
| `mShadow` | `background` when present, otherwise `mSurface` |
| `mSurface` | `surface` |
| `mHover` | `surface_variant` when present, otherwise `mSurfaceVariant` |
| `mOnSurface` | `text` |
| `mOnSurfaceVariant` | `muted` |
| `mPrimary` | `accent` |
| `mOutline` | `border` |

Every mapped value must be a CSS-safe 6-digit hex color such as `#aabbcc`.

`mShadow` and `mHover` are optional nuance roles. When present, they are validated and used to preserve Dumber's background/surface/input contrast.

Because Noctalia's native file contains the active palette only, Dumber applies that mapped palette to both resolved light and dark palettes so the app follows the file immediately.

#### Advanced: `dumber-json` user template

`dumber-json` remains supported for an explicit Dumber-specific Noctalia user template:

```toml
[appearance.external_theme]
enabled = true
provider = "noctalia"
format = "dumber-json"
# Optional for dumber-json. Defaults to $XDG_CONFIG_HOME/dumber/noctalia-theme.json.
path = "~/.config/dumber/noctalia-theme.json"
```

The file must be JSON with root `light` and `dark` objects. Optional `source`, `name`, and `mode` metadata fields are accepted; `name` or `source` is used only as source metadata. Dumber chooses the active palette from `appearance.color_scheme` and the system color-scheme preference.

Supported palette fields are:

- `background`
- `surface`
- `surface_variant`
- `text`
- `muted`
- `accent`
- `border`

Each non-empty palette value must be a CSS-safe 6-digit hex color such as `#AABBCC`. Empty or omitted palette fields inherit from the configured Dumber palette for that mode.

Example rendered file: [`docs/examples/noctalia-dumber-theme.json`](../examples/noctalia-dumber-theme.json). Example template: [`docs/examples/noctalia-dumber-theme.template.json`](../examples/noctalia-dumber-theme.template.json).

When Noctalia rewrites the watched file, Dumber reapplies the same resolved theme path used by config reloads. A malformed or temporarily missing file keeps the last valid external palette when one exists; disabling `appearance.external_theme.enabled` clears that last-good external palette and returns to Dumber's configured palettes.

## Debug Options

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `debug.enable_devtools` | bool | `true` | Enable browser developer tools (F12, Inspect Element) |

## Rendering, UI Scale & Zoom

CEF is the default browser engine. WebKitGTK remains available as a fallback via `engine.type = "webkit"`; `engine.webkit.*` settings only affect that fallback engine.

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `engine.type` | string | `"cef"` | `cef`, `webkit` | Browser engine selection; WebKitGTK is the fallback option |
| `engine.cef.render_stack` | string | `"vulkan"` | `vulkan`, `egl` | CEF GPU render stack |
| `engine.cef.adaptive_windowless_frame_rate` | bool | `true` | - | Enable adaptive CEF OSR FPS polling when `windowless_frame_rate = 0` |
| `engine.cef.windowless_frame_rate` | int32 | `0` | `>= 0` | Explicit CEF OSR FPS cap; 0 uses adaptive mode if enabled |
| `engine.cef.windowless_frame_rate_max` | int32 | `240` | `>= 0` | Hard cap for adaptive CEF OSR FPS; 0 uses the built-in cap |
| `engine.cef.input.scroll_wheel_multiplier` | float | `1.0` | `> 0` | Mouse wheel scroll sensitivity multiplier |
| `engine.cef.input.scroll_precise_multiplier` | float | `2.5` | `> 0` | Precise/surface scroll sensitivity multiplier for touchpads and high-resolution wheels |
| `engine.cef.input.scroll_horizontal_multiplier` | float | `1.0` | `> 0` | Horizontal scroll sensitivity multiplier |
| `engine.cef.input.scroll_vertical_multiplier` | float | `1.0` | `> 0` | Vertical scroll sensitivity multiplier; combines with `scroll_precise_multiplier` for touchpads |
| `engine.cef.input.scroll_max_delta` | int32 | `0` | `>= 0` | Maximum absolute scroll delta after scaling; 0 disables clamping |
| `engine.cef.input.touchpad_navigation_enabled` | bool | `true` | - | Enable two-finger touchpad swipe back/forward navigation |
| `engine.cef.input.touchpad_navigation_min_delta` | float | `200.0` | `> 0` | Minimum accumulated horizontal swipe delta required for navigation |
| `engine.cef.input.touchpad_navigation_max_vertical_ratio` | float | `0.5` | `> 0` | Maximum vertical-to-horizontal delta ratio allowed for navigation swipes |
| `engine.webkit.gsk_renderer` | string | `"auto"` | `auto`, `opengl`, `vulkan`, `cairo` | WebKit fallback GTK renderer selection (`GSK_RENDERER`) |
| `engine.webkit.disable_dmabuf_renderer` | bool | `false` | - | Disable WebKit fallback DMA-BUF renderer |
| `engine.webkit.force_compositing_mode` | bool | `false` | - | Force WebKit fallback compositing mode (`WEBKIT_FORCE_COMPOSITING_MODE`) |
| `engine.webkit.disable_compositing_mode` | bool | `false` | - | Disable WebKit fallback compositing mode (`WEBKIT_DISABLE_COMPOSITING_MODE`) |
| `engine.webkit.disable_mipmaps` | bool | `false` | - | Disable GTK mipmaps for the WebKit fallback (`GSK_GPU_DISABLE=mipmap`) |
| `engine.webkit.prefer_gl` | bool | `false` | - | Prefer OpenGL over GLES for the WebKit fallback (`GDK_DEBUG=gl-prefer-gl`) |
| `engine.webkit.draw_compositing_indicators` | bool | `false` | - | Draw WebKit fallback compositing indicators (debug) |
| `engine.webkit.show_fps` | bool | `false` | - | Show WebKit fallback FPS counter (`WEBKIT_SHOW_FPS`) |
| `engine.webkit.sample_memory` | bool | `false` | - | Enable WebKit fallback memory sampling (`WEBKIT_SAMPLE_MEMORY`) |
| `engine.webkit.debug_frames` | bool | `false` | - | Enable GTK frame timing debug for the WebKit fallback (`GDK_DEBUG=frames`) |
| `engine.webkit.force_vsync` | bool | `false` | - | Force VSync for WebKit fallback video playback |
| `engine.webkit.gl_rendering_mode` | string | `"auto"` | `auto`, `gles2`, `gl3`, `none` | WebKit fallback GStreamer/OpenGL API selection |
| `engine.webkit.gstreamer_debug_level` | int | `0` | `0-5` | WebKit fallback GStreamer debug verbosity |
| `default_ui_scale` | float | `1.0` | `> 0` | GTK widget UI scale (1.0=100%, 2.0=200%) |
| `default_webpage_zoom` | float | `1.2` | `> 0` | Default page zoom (1.0=100%, 1.2=120%) |

`engine.cef.input.scroll_precise_multiplier` controls touchpad/high-resolution wheel scroll speed, and `engine.cef.input.scroll_vertical_multiplier` applies an additional vertical-only scale. `engine.cef.input.touchpad_navigation_max_vertical_ratio` only filters horizontal back/forward swipe recognition; it does not tune vertical scroll speed.

`engine.cef.input.touchpad_navigation_min_delta` uses raw GTK touchpad surface units for back/forward gestures. The default `200.0` matches WebKit-style commit distance to reduce accidental navigation; raise or lower it in `config.toml` to tune gesture sensitivity.

### Legacy key migration

Existing configs using older keys are migrated to the current engine shape. New configs should use the canonical keys above.

| Legacy key | Current key |
|------------|-------------|
| `privacy.cookie_policy` | `engine.cookie_policy` |
| `privacy.itp_enabled` | `engine.webkit.itp_enabled` |
| `rendering.disable_dmabuf_renderer` | `engine.webkit.disable_dmabuf_renderer` |
| `rendering.force_compositing_mode` | `engine.webkit.force_compositing_mode` |
| `rendering.disable_compositing_mode` | `engine.webkit.disable_compositing_mode` |
| `rendering.gsk_renderer` | `engine.webkit.gsk_renderer` |
| `rendering.disable_mipmaps` | `engine.webkit.disable_mipmaps` |
| `rendering.prefer_gl` | `engine.webkit.prefer_gl` |
| `rendering.draw_compositing_indicators` | `engine.webkit.draw_compositing_indicators` |
| `rendering.show_fps` | `engine.webkit.show_fps` |
| `rendering.sample_memory` | `engine.webkit.sample_memory` |
| `rendering.debug_frames` | `engine.webkit.debug_frames` |
| `media.force_vsync` | `engine.webkit.force_vsync` |
| `media.gl_rendering_mode` | `engine.webkit.gl_rendering_mode` |
| `media.gstreamer_debug_level` | `engine.webkit.gstreamer_debug_level` |
| `runtime.prefix` | `engine.webkit.prefix` |
| `performance.*` | `engine.profile`, `engine.pool_prewarm_count`, `engine.zoom_cache_size`, or matching `engine.webkit.*` custom tuning keys |
| `rendering.mode` | Dropped; no current setting |

## Workspace Configuration

### General

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace.new_pane_url` | string | `"about:blank"` | URL loaded for new panes/tabs (supports `http(s)://`, `dumb://`, `file://`, `about:`) |
| `workspace.switch_to_tab_on_move` | bool | `true` | When moving a pane to another tab, automatically switch to the destination tab |

**Example:**
```toml
[workspace]
new_pane_url = "dumb://history"
```

### Pane Mode

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace.pane_mode.activation_shortcut` | string | `"ctrl+p"` | Pane mode activation key |
| `workspace.pane_mode.timeout_ms` | int | `3000` | Pane mode timeout (ms) |
| `workspace.pane_mode.actions` | map | See below | Action→keys mappings |

**Default pane mode actions:**
```toml
[workspace.pane_mode.actions]
split-right = ["arrowright", "r"]
split-left = ["arrowleft", "l"]
split-up = ["arrowup", "u"]
split-down = ["arrowdown", "d"]
stack-pane = ["s"]
close-pane = ["x"]
move-pane-to-tab = ["m"]
move-pane-to-next-tab = ["M", "shift+m"]
eject-pane-to-window = ["w"]

# Consume-or-expel (niri-style) - very alpha
consume-or-expel-left = ["["]
consume-or-expel-right = ["]"]
consume-or-expel-up = ["{"]
consume-or-expel-down = ["}"]

focus-right = ["shift+arrowright", "shift+l"]
focus-left = ["shift+arrowleft", "shift+h"]
focus-up = ["shift+arrowup", "shift+k"]
focus-down = ["shift+arrowdown", "shift+j"]
confirm = ["enter"]
cancel = ["escape"]
```

> **Note:** Actions are inverted to key→action map in memory for O(1) lookup performance during navigation.

### Tab Mode

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace.tab_mode.activation_shortcut` | string | `"ctrl+t"` | Tab mode activation key |
| `workspace.tab_mode.timeout_ms` | int | `3000` | Tab mode timeout (ms) |
| `workspace.tab_mode.actions` | map | See below | Action→keys mappings |

**Default tab mode actions:**
```toml
[workspace.tab_mode.actions]
new-tab = ["n", "c"]
close-tab = ["x"]
next-tab = ["l", "tab"]
previous-tab = ["h", "shift+tab"]
rename-tab = ["r"]
confirm = ["enter"]
cancel = ["escape"]
```

> **Note:** Actions are inverted to key→action map in memory for O(1) lookup performance during navigation.

### Resize Mode

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace.resize_mode.activation_shortcut` | string | `"ctrl+n"` | Resize mode activation key |
| `workspace.resize_mode.timeout_ms` | int | `3000` | Resize mode timeout (ms) |
| `workspace.resize_mode.step_percent` | float | `5.0` | Split ratio step per keystroke (percent) |
| `workspace.resize_mode.min_pane_percent` | float | `10.0` | Minimum pane size (percent) |
| `workspace.resize_mode.actions` | map | See below | Action→keys mappings |

**Default resize mode actions:**
```toml
[workspace.resize_mode.actions]
resize-increase-left = ["h", "arrowleft"]
resize-increase-down = ["j", "arrowdown"]
resize-increase-up = ["k", "arrowup"]
resize-increase-right = ["l", "arrowright"]
resize-decrease-left = ["H"]
resize-decrease-down = ["J"]
resize-decrease-up = ["K"]
resize-decrease-right = ["L"]
resize-increase = ["+", "="]
resize-decrease = ["-"]
confirm = ["enter"]
cancel = ["escape"]
```

Notes:
- Directional actions (`resize-increase-*/resize-decrease-*`) move the split divider.
- Smart actions (`resize-increase` / `resize-decrease`) grow/shrink the active pane (best-effort) by picking a direction automatically.
- Timeout is refreshed on each resize keypress so you can keep adjusting without re-entering the mode.

### Global Shortcuts

Global shortcuts are configured under `workspace.shortcuts.actions` using the same `ActionBinding` structure as modal keybindings:

| Action | Default Keys | Description |
|--------|--------------|-------------|
| `toggle_floating_pane` | `alt+f` | Toggle the workspace floating pane (persistent session) |
| `close_pane` | `ctrl+w` | Close active pane (or release floating pane when floating is active) |
| `next_tab` | `ctrl+tab` | Switch to next tab |
| `previous_tab` | `ctrl+shift+tab` | Switch to previous tab |
| `consume_or_expel_left` | `alt+[` | Consume into left sibling stack, or expel left if stacked |
| `consume_or_expel_right` | `alt+]` | Consume into right sibling stack, or expel right if stacked |
| `consume_or_expel_up` | `alt+{` | Consume into upper sibling stack, or expel up if stacked |
| `consume_or_expel_down` | `alt+}` | Consume into lower sibling stack, or expel down if stacked |

**Example:**
```toml
[workspace.shortcuts.actions.close_pane]
  keys = ["ctrl+w"]
  desc = "Close active pane"

[workspace.shortcuts.actions.next_tab]
  keys = ["ctrl+tab", "alt+l"]
  desc = "Switch to next tab"

[workspace.shortcuts.actions.toggle_floating_pane]
  keys = ["alt+f"]
  desc = "Toggle floating pane"
```

> **Tip:** Use `dumb://config` → Keybindings tab to edit shortcuts visually.
>
> **Note:** New tab creation uses modal tab mode (Ctrl+T then n/c). This follows the Zellij-style modal keyboard interface.
>
> **Note:** Consume-or-expel is experimental and may change behavior between releases.
>
> **Warning:** Some `Alt+<key>` shortcuts may conflict with WebKit defaults, website handlers, or your desktop environment.
> If a binding does not fire, rebind it to a different key.

### Floating Pane

The floating workspace pane is a persistent overlay session: hiding it does not destroy its web content.

- Default behavior includes exactly one floating shortcut: `Alt+F` (`toggle_floating_pane`).
- URL shortcuts (for example `Alt+G`) are user-defined via `workspace.floating_pane.profiles` and are empty by default.
- `Alt+F` toggles visibility only and preserves floating state.
- `Ctrl+W` on an active floating pane closes and releases that floating session for a fresh next open.
- Floating profile keybindings support modifier combos with `ctrl`, `shift`, and `alt` (for example `ctrl+shift+y` or `ctrl+alt+m`).

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace.floating_pane.width_pct` | float | `0.82` | Floating pane width as fraction of workspace width (`(0,1]`) |
| `workspace.floating_pane.height_pct` | float | `0.72` | Floating pane height as fraction of workspace height (`(0,1]`) |
| `workspace.floating_pane.profiles` | map | `{}` | Named URL profiles, each with `keys[]`, `url`, optional `desc` |

**Example:**

```toml
[workspace.shortcuts.actions.toggle_floating_pane]
keys = ["alt+f"]
desc = "Toggle floating pane"

[workspace.floating_pane]
width_pct = 0.82
height_pct = 0.72

[workspace.floating_pane.profiles]
# Empty by default. Add as many entries as needed.

[workspace.floating_pane.profiles.google]
keys = ["alt+g"]
url = "https://google.com"
desc = "Open floating pane on Google"

[workspace.floating_pane.profiles.github]
keys = ["alt+h"]
url = "https://github.com"
desc = "Open floating pane on GitHub"
```

### Browsing Context Behavior

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `workspace.browsing_contexts.behavior` | string | `"split"` | `split`, `stacked`, `tabbed`, `windowed` | Placement mode for script-opened browsing contexts |
| `workspace.browsing_contexts.placement` | string | `"right"` | `right`, `left`, `top`, `bottom` | Split direction when behavior is `split` |
| `workspace.browsing_contexts.open_in_new_pane` | bool | `true` | - | Allow new browsing contexts to open in the workspace |
| `workspace.browsing_contexts.follow_pane_context` | bool | `true` | - | Keep new browsing contexts aligned with the parent pane context |
| `workspace.browsing_contexts.blank_target_behavior` | string | `"stacked"` | `split`, `stacked`, `tabbed` | Placement mode for `_blank` / new-page link contexts |
| `workspace.browsing_contexts.enable_smart_detection` | bool | `true` | - | Use window properties to refine browsing-context classification |
| `workspace.browsing_contexts.oauth_auto_close` | bool | `true` | - | Auto-close OAuth browsing contexts after success |

### Workspace Styling

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace.styling.border_width` | int | `1` | Active pane border width (px) - overlay |
| `workspace.styling.border_color` | string | `"@theme_selected_bg_color"` | Active pane border color |
| `workspace.styling.mode_border_width` | int | `4` | Modal mode border width (px) - applies to all modes |
| `workspace.styling.pane_mode_color` | string | `"#4A90E2"` | Pane mode color (blue) - used for border and toaster |
| `workspace.styling.tab_mode_color` | string | `"#FFA500"` | Tab mode color (orange) - used for border and toaster |
| `workspace.styling.session_mode_color` | string | `"#9B59B6"` | Session mode color (purple) - used for border and toaster |
| `workspace.styling.resize_mode_color` | string | `"#00D4AA"` | Resize mode color (teal) - used for border and toaster |
| `workspace.styling.mode_indicator_toaster_enabled` | bool | `true` | Show toaster notification when modal modes are active |
| `workspace.styling.transition_duration` | int | `120` | Border transition duration (ms) |

**Example:**
```toml
[workspace.styling]
border_width = 1
border_color = "@theme_selected_bg_color"
mode_border_width = 4
pane_mode_color = "#4A90E2"      # Blue for pane mode
tab_mode_color = "#FFA500"       # Orange for tab mode
session_mode_color = "#9B59B6"   # Purple for session mode
resize_mode_color = "#00D4AA"    # Teal for resize mode
mode_indicator_toaster_enabled = true
transition_duration = 120
```

## Session

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `session.auto_restore` | bool | `false` | Automatically restore the last session on startup |
| `session.snapshot_interval_ms` | int | `5000` | Minimum interval between snapshots in milliseconds |
| `session.max_exited_sessions` | int | `50` | Maximum number of exited sessions to keep |
| `session.max_exited_session_age_days` | int | `7` | Maximum age in days for exited sessions (auto-deleted on startup) |

### Session Mode

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `session.session_mode.activation_shortcut` | string | `"ctrl+o"` | Session mode activation key |
| `session.session_mode.timeout_ms` | int | `3000` | Session mode timeout (ms) |
| `session.session_mode.actions` | map | See below | Action to keys mappings |

**Default session mode actions:**
```toml
[session.session_mode.actions]
session-manager = ["s", "w"]
confirm = ["enter"]
cancel = ["escape"]
```

**Example:**
```toml
[session]
auto_restore = false              # Don't auto-restore on startup
snapshot_interval_ms = 5000       # Save state every 5 seconds (debounced)
max_exited_sessions = 50          # Keep last 50 exited sessions
max_exited_session_age_days = 7   # Delete sessions older than 7 days on startup
```

## Media

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `media.hardware_decoding` | string | `"auto"` | `auto`, `force`, `disable` | Hardware video decoding mode |
| `media.prefer_av1` | bool | `false` | - | Prefer AV1 codec when available |
| `media.show_diagnostics` | bool | `false` | - | Show media diagnostics warnings at startup |

WebKit fallback GStreamer tuning is configured under `engine.webkit.force_vsync`, `engine.webkit.gl_rendering_mode`, and `engine.webkit.gstreamer_debug_level`.

**Hardware decoding modes:**
- `auto` (recommended): Hardware preferred with software fallback - fixes Twitch Error #4000
- `force`: Hardware only - fails if unavailable
- `disable`: Software only - higher CPU usage

**GPU auto-detection:**
Dumber automatically detects your GPU vendor (AMD/Intel/NVIDIA) and sets optimal VA-API driver settings:
- **AMD**: Uses `radeonsi` driver
- **Intel**: Uses `iHD` driver (modern, for Broadwell+)
- **NVIDIA**: Uses `nvidia` driver with EGL platform

**Example:**
```toml
[media]
hardware_decoding = "auto"    # HW preferred, SW fallback
prefer_av1 = false            # Let site choose codec
show_diagnostics = false      # Keep off for daily use
```

**WebKit fallback GStreamer diagnostics:**
```toml
[engine.webkit]
gstreamer_debug_level = 3
```

Set `media.show_diagnostics = false` and `engine.webkit.gstreamer_debug_level = 0` to return to normal mode.

**Diagnostics CLI:**
```bash
# Check GStreamer plugins and VA-API status
dumber doctor --media
```

## Runtime

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `engine.cef.cef_dir` | string | `""` | Optional CEF runtime directory containing `libcef.so` and resources |
| `engine.webkit.prefix` | string | `""` | Optional WebKitGTK fallback runtime prefix (e.g., `/opt/webkitgtk`) |

`engine.cef.cef_dir` overrides CEF runtime discovery. For the WebKitGTK fallback, `engine.webkit.prefix` prepends prefix-derived paths to:
- `PKG_CONFIG_PATH` (for version checks)
- `LD_LIBRARY_PATH` (for runtime library loading)
- `GI_TYPELIB_PATH` (GObject introspection)
- `XDG_DATA_DIRS` (schemas/resources)

**Example:**
```toml
[engine.cef]
cef_dir = "/opt/cef"

[engine.webkit]
prefix = "/opt/webkitgtk"
```

## Clipboard

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `clipboard.auto_copy_on_selection` | bool | `true` | Automatically copy selected text to clipboard (zellij/tmux-style) |

When enabled, selecting text in a web page immediately copies it to the clipboard with a brief toast notification. Does not apply to text selection in input fields or textareas.

**Example:**
```toml
[clipboard]
auto_copy_on_selection = true  # Enabled by default
```

## Content Filtering

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `content_filtering.enabled` | bool | `true` | Enable ad blocking |
| `content_filtering.auto_update` | bool | `true` | Automatically update filters |

Notes:
- Filter data is downloaded from `bnema/ublock-webkit-filters` GitHub releases.
- Domain whitelist is managed via the database (`content_whitelist` table), not the config file.

## Update

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `update.enable_on_startup` | bool | `true` | Check for updates when browser starts |
| `update.auto_download` | bool | `false` | Automatically download updates in background |
| `update.notify_on_new_settings` | bool | `true` | Show toast notification when new config settings are available |

**Example:**
```toml
[update]
enable_on_startup = true       # Check for updates on startup
auto_download = false          # Don't auto-download (prompt instead)
notify_on_new_settings = true  # Show toast when config migration available
```

**CLI Commands:**
```bash
# Check config status and available migrations
dumber config status

# Add missing settings with default values
dumber config migrate

# Skip confirmation prompt
dumber config migrate --yes
```

## Performance Profiles

Performance profiles tune engine resource behavior. CEF is the default engine; the `engine.webkit.*` custom fields below apply only when using the WebKitGTK fallback.

> **Note:** Performance settings are applied at browser startup. Changes require a restart to take effect.

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `engine.profile` | string | `"default"` | `default`, `lite`, `balanced`, `max`, `custom` | Performance profile selection |
| `engine.pool_prewarm_count` | int | `4` | `>= 0` | WebViews to pre-create at startup |
| `engine.zoom_cache_size` | int | `256` | `>= 0` | Domain zoom levels to cache |

### Profiles

| Profile | Description | Use Case |
|---------|-------------|----------|
| `default` | No tuning, uses engine defaults | Compatibility baseline, troubleshooting |
| `lite` | Reduced resource usage | Low-RAM systems (< 4GB), battery saving |
| `balanced` | Moderate tuning | General use |
| `max` | Maximum responsiveness | Heavy pages (GitHub PRs, complex SPAs), high-end systems |
| `custom` | Manual control over all settings | Advanced users who want fine-grained control |

**Example:**
```toml
[engine]
profile = "default"
pool_prewarm_count = 4
zoom_cache_size = 256
```

### WebKit fallback custom profile settings

When `engine.profile = "custom"` and `engine.type = "webkit"`, you can configure individual WebKit tuning options:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `engine.webkit.skia_cpu_painting_threads` | int | `0` | Skia CPU rendering threads (0=unset) |
| `engine.webkit.skia_gpu_painting_threads` | int | `-1` | Skia GPU rendering threads (-1=unset, 0=disable) |
| `engine.webkit.skia_enable_cpu_rendering` | bool | `false` | Force CPU rendering |
| `engine.webkit.web_process_memory_limit_mb` | int | `0` | Web process memory limit in MB (0=unset) |
| `engine.webkit.web_process_memory_poll_interval_sec` | float | `0` | Memory check interval (0=WebKit default: 30s) |
| `engine.webkit.web_process_memory_conservative_threshold` | float | `0` | Conservative cleanup threshold (0=unset) |
| `engine.webkit.web_process_memory_strict_threshold` | float | `0` | Strict cleanup threshold (0=unset) |
| `engine.webkit.network_process_memory_limit_mb` | int | `0` | Network process memory limit in MB |
| `engine.webkit.network_process_memory_poll_interval_sec` | float | `0` | Network memory check interval |
| `engine.webkit.network_process_memory_conservative_threshold` | float | `0` | Network conservative threshold |
| `engine.webkit.network_process_memory_strict_threshold` | float | `0` | Network strict threshold |

**Custom WebKit fallback example:**
```toml
[engine]
type = "webkit"
profile = "custom"
pool_prewarm_count = 6
zoom_cache_size = 256

[engine.webkit]
skia_cpu_painting_threads = 4
skia_gpu_painting_threads = 2
web_process_memory_limit_mb = 1024
web_process_memory_conservative_threshold = 0.4
web_process_memory_strict_threshold = 0.6
```

> **Important:** Individual `engine.webkit.*` tuning fields are ignored unless `engine.profile = "custom"`.

## Downloads

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `downloads.path` | string | `""` | Download directory path (empty = `$XDG_DOWNLOAD_DIR` or `~/Downloads`) |

Downloads are saved to the configured directory with toast notifications for download started, completed, and failed events.

**Example:**
```toml
[downloads]
path = ""  # Use system default ($XDG_DOWNLOAD_DIR or ~/Downloads)

# Or specify a custom directory:
# path = "/home/user/my-downloads"
```

## Environment Variables

All config values can be overridden via environment variables with the prefix `DUMBER_`:

```bash
# Database
DUMBER_DATABASE_PATH=/custom/path/db.sqlite

# Engine/rendering (examples)
DUMBER_ENGINE_TYPE=cef
# CEF defaults to the GPU-first Vulkan DMABUF stack.
# Use EGL/OpenGL for driver compatibility:
# DUMBER_ENGINE_CEF_RENDER_STACK=egl
# WebKit fallback rendering overrides:
# DUMBER_ENGINE_WEBKIT_DISABLE_DMABUF_RENDERER=true
# DUMBER_ENGINE_WEBKIT_GSK_RENDERER=opengl
DUMBER_DEFAULT_WEBPAGE_ZOOM=1.5

# Logging
DUMBER_LOG_LEVEL=debug
DUMBER_LOG_FORMAT=json
```
