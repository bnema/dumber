# Configuration Reference

**Primary format**: TOML (recommended)
**Location**: `~/.config/dumber/config.toml`
**Also supports**: JSON (`.json`) and YAML (`.yaml`)
**JSON Schema**: `~/.config/dumber/config.schema.json` (auto-generated for JSON format)

## Why TOML?

TOML is the recommended format for dumber configuration because:
- **Clear structure**: Explicit sections make complex configs easy to read and modify
- **No ambiguity**: No whitespace sensitivity or type coercion issues
- **Comments supported**: Add documentation directly in your config
- **Industry standard**: Used by Cargo, Hugo, and many modern tools

You can still use JSON or YAML if you prefer - dumber automatically detects the format.

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
| `dmenu.max_history_items` | int | `20` | Max items shown in launcher |
| `dmenu.show_visit_count` | bool | `true` | Show visit counts in output |
| `dmenu.show_last_visited` | bool | `true` | Show last visited dates in output |
| `dmenu.history_prefix` | string | `"🕒"` | Prefix for history items |
| `dmenu.shortcut_prefix` | string | `"🔍"` | Prefix for shortcuts |
| `dmenu.url_prefix` | string | `"🌐"` | Prefix for URLs |
| `dmenu.date_format` | string | `"2006-01-02 15:04"` | Go time format string |
| `dmenu.sort_by_visit_count` | bool | `true` | Sort by popularity |

**CLI Usage:**
```bash
# Pipe mode (default) - outputs history for rofi/fuzzel with favicons
dumber dmenu | rofi -dmenu -show-icons -p "Browse: " | dumber dmenu --select
dumber dmenu | fuzzel --dmenu -p "Browse: " | dumber dmenu --select

# Interactive TUI mode
dumber dmenu --interactive

# Override max items via CLI flag
dumber dmenu --max 50
```

## Omnibox

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `omnibox.initial_behavior` | string | `"recent"` | `recent`, `most_visited`, `none` | Initial history display behavior |

**Example:**
```toml
[omnibox]
initial_behavior = "recent"  # Show recent history when omnibox opens

# Alternative options:
# initial_behavior = "most_visited"  # Show most visited sites
# initial_behavior = "none"          # Show no initial suggestions
```

## Logging

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `logging.level` | string | `"info"` | `debug`, `info`, `warn`, `error` | Log level |
| `logging.format` | string | `"text"` | `text`, `json` | Log format |
| `logging.max_age` | int | `7` | >= 0 | Days to keep logs |
| `logging.log_dir` | string | `~/.local/state/dumber/logs/` | any | Log directory |
| `logging.enable_file_log` | bool | `true` | - | Enable file logging |
| `logging.capture_console` | bool | `false` | - | Capture browser console to logs |

## Appearance

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `appearance.sans_font` | string | `"Fira Sans"` | Sans-serif font |
| `appearance.serif_font` | string | `"Fira Sans"` | Serif font |
| `appearance.monospace_font` | string | `"Fira Code"` | Monospace font |
| `appearance.default_font_size` | int | `16` | Font size in points |
| `appearance.color_scheme` | string | `"default"` | `prefer-dark`, `prefer-light`, `default` |

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

## Debug Options

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `debug.enable_devtools` | bool | `true` | Enable browser developer tools (F12, Inspect Element) |

## Rendering, UI Scale & Zoom

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `rendering.mode` | string | `"gpu"` | `auto`, `gpu`, `cpu` | WebKit hardware acceleration policy |
| `rendering.disable_dmabuf_renderer` | bool | `false` | - | Disable WebKit DMA-BUF renderer (may fix flicker on Wayland; slower) |
| `rendering.force_compositing_mode` | bool | `false` | - | Force WebKit compositing mode (`WEBKIT_FORCE_COMPOSITING_MODE`) |
| `rendering.disable_compositing_mode` | bool | `false` | - | Disable WebKit compositing mode (`WEBKIT_DISABLE_COMPOSITING_MODE`) |
| `rendering.gsk_renderer` | string | `"vulkan"` | `auto`, `opengl`, `vulkan`, `cairo` | GTK renderer selection (`GSK_RENDERER`) |
| `rendering.disable_mipmaps` | bool | `false` | - | Disable GTK mipmaps (`GSK_GPU_DISABLE=mipmap`) |
| `rendering.prefer_gl` | bool | `false` | - | Prefer OpenGL over GLES (`GDK_DEBUG=gl-prefer-gl`) |
| `rendering.draw_compositing_indicators` | bool | `false` | - | Draw WebKit compositing indicators (debug) |
| `rendering.show_fps` | bool | `false` | - | Show WebKit FPS counter (`WEBKIT_SHOW_FPS`) |
| `rendering.sample_memory` | bool | `false` | - | Enable WebKit memory sampling (`WEBKIT_SAMPLE_MEMORY`) |
| `rendering.debug_frames` | bool | `false` | - | Enable GTK frame timing debug (`GDK_DEBUG=frames`) |
| `default_ui_scale` | float | `1.0` | > 0 | GTK widget UI scale (1.0=100%, 2.0=200%) |
| `default_webpage_zoom` | float | `1.2` | > 0 | Default page zoom (1.0=100%, 1.2=120%) |

## Workspace Configuration

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

### Global Shortcuts

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace.shortcuts.close_pane` | string | `"ctrl+w"` | Close active pane (closes tab if last pane) |
| `workspace.shortcuts.next_tab` | string | `"ctrl+tab"` | Next tab shortcut |
| `workspace.shortcuts.previous_tab` | string | `"ctrl+shift+tab"` | Previous tab shortcut |

> **Note:** New tab creation uses modal tab mode (Ctrl+T then n/c). This follows the Zellij-style modal keyboard interface.

### Popup Behavior

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `workspace.popups.behavior` | string | `"split"` | `split`, `stacked`, `tabbed`, `windowed` | Popup opening mode |
| `workspace.popups.placement` | string | `"right"` | `right`, `left`, `top`, `bottom` | Split direction |
| `workspace.popups.open_in_new_pane` | bool | `true` | - | Open popups in workspace |
| `workspace.popups.follow_pane_context` | bool | `true` | - | Follow parent pane context |
| `workspace.popups.blank_target_behavior` | string | `"pane"` | `pane`, `tab` | `_blank` target handling |
| `workspace.popups.enable_smart_detection` | bool | `true` | - | Use WindowProperties for detection |
| `workspace.popups.oauth_auto_close` | bool | `true` | - | Auto-close OAuth popups |

### Workspace Styling

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace.styling.border_width` | int | `1` | Active pane border width (px) - overlay |
| `workspace.styling.border_color` | string | `"@theme_selected_bg_color"` | Active pane border color |
| `workspace.styling.pane_mode_border_width` | int | `4` | Pane mode border width (px) - Ctrl+P N overlay |
| `workspace.styling.pane_mode_border_color` | string | `"#4A90E2"` | Pane mode border color (blue) |
| `workspace.styling.tab_mode_border_width` | int | `4` | Tab mode border width (px) - Ctrl+P T overlay |
| `workspace.styling.tab_mode_border_color` | string | `"#FFA500"` | Tab mode border color (orange) |
| `workspace.styling.transition_duration` | int | `120` | Border transition duration (ms) |

## Media

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `media.hardware_decoding` | string | `"auto"` | `auto`, `force`, `disable` | Hardware video decoding mode |
| `media.prefer_av1` | bool | `true` | - | Prefer AV1 codec when available |
| `media.show_diagnostics` | bool | `true` | - | Show media diagnostics warnings at startup |
| `media.force_vsync` | bool | `false` | - | Force VSync for video playback (may help with tearing) |
| `media.gl_rendering_mode` | string | `"auto"` | `auto`, `gles2`, `gl3`, `none` | OpenGL API selection for video rendering |
| `media.gstreamer_debug_level` | int | `0` | `0-5` | GStreamer debug verbosity (0=off) |
| `media.video_buffer_size_mb` | int | `64` | > 0 | Video buffer size in MB for smoother streaming |
| `media.queue_buffer_time_sec` | int | `20` | > 0 | Queue prebuffer time in seconds |

**Hardware decoding modes:**
- `auto` (recommended): Hardware preferred with software fallback - fixes Twitch Error #4000
- `force`: Hardware only - fails if unavailable
- `disable`: Software only - higher CPU usage

**GL rendering modes:**
- `auto` (default): Let GStreamer choose the best OpenGL API
- `gles2`: Force GLES2 (better for some mobile/embedded GPUs)
- `gl3`: Force OpenGL 3.x desktop
- `none`: Disable GL-based rendering

**GPU auto-detection:**
Dumber automatically detects your GPU vendor (AMD/Intel/NVIDIA) and sets optimal VA-API driver settings:
- **AMD**: Uses `radeonsi` driver
- **Intel**: Uses `iHD` driver (modern, for Broadwell+)
- **NVIDIA**: Uses `nvidia` driver with EGL platform

**Buffering settings:**
- `video_buffer_size_mb`: Controls GStreamer buffer size. Larger buffers reduce rebuffering on bursty streams (Twitch, YouTube). Uses more memory.
- `queue_buffer_time_sec`: Controls prebuffer duration. Higher values allow more data to be buffered ahead of playback.

**Example:**
```toml
[media]
hardware_decoding = "auto"    # HW preferred, SW fallback
prefer_av1 = true             # AV1 is most efficient codec
show_diagnostics = true       # Log warnings if HW accel unavailable
force_vsync = false           # Let compositor handle VSync
gl_rendering_mode = "auto"    # GStreamer picks best GL API
gstreamer_debug_level = 0     # Increase to 3-5 for debugging
video_buffer_size_mb = 64     # 64 MB buffer for smooth streaming
queue_buffer_time_sec = 20    # 20 seconds prebuffer
```

**Diagnostics CLI:**
```bash
# Check GStreamer plugins and VA-API status
dumber doctor --media
```

## Runtime

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `runtime.prefix` | string | `""` | Optional runtime prefix (e.g., `/opt/webkitgtk`) used to locate newer GTK/WebKitGTK installs |

If set, dumber prepends prefix-derived paths to:
- `PKG_CONFIG_PATH` (for version checks)
- `LD_LIBRARY_PATH` (for runtime library loading)
- `GI_TYPELIB_PATH` (GObject introspection)
- `XDG_DATA_DIRS` (schemas/resources)

**Example:**
```toml
[runtime]
prefix = "/opt/webkitgtk"
```

## Content Filtering

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `content_filtering.enabled` | bool | `true` | Enable ad blocking |
| `content_filtering.auto_update` | bool | `true` | Automatically update filters |

Notes:
- Filter data is downloaded from `bnema/ublock-webkit-filters` GitHub releases.
- Domain whitelist is managed via the database (`content_whitelist` table), not the config file.

## Environment Variables

All config values can be overridden via environment variables with the prefix `DUMBER_`:

```bash
# Database
DUMBER_DATABASE_PATH=/custom/path/db.sqlite

# Rendering (examples)
# Defaults: rendering.disable_dmabuf_renderer=false, rendering.gsk_renderer="vulkan"
DUMBER_RENDERING_MODE=cpu
# DUMBER_RENDERING_DISABLE_DMABUF_RENDERER=true
# DUMBER_RENDERING_GSK_RENDERER=opengl
DUMBER_DEFAULT_WEBPAGE_ZOOM=1.5

# Logging
DUMBER_LOG_LEVEL=debug
DUMBER_LOG_FORMAT=json
```
