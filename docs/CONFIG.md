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
| `database.path` | string | `~/.local/state/dumber/dumber.sqlite` | Database file path |

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

**Default shortcuts:**
```toml
[search_shortcuts.ddg]
url = "https://duckduckgo.com/?q=%s"
description = "DuckDuckGo search"

[search_shortcuts.g]
url = "https://google.com/search?q=%s"
description = "Google Search"

[search_shortcuts.gh]
url = "https://github.com/search?q=%s"
description = "GitHub"

[search_shortcuts.yt]
url = "https://youtube.com/results?search_query=%s"
description = "YouTube"
```

## Dmenu / Launcher

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `dmenu.max_history_items` | int | `20` | Max items shown in launcher |
| `dmenu.show_visit_count` | bool | `true` | Show visit counts |
| `dmenu.show_last_visited` | bool | `true` | Show last visited dates |
| `dmenu.history_prefix` | string | `"🕒"` | Prefix for history items |
| `dmenu.shortcut_prefix` | string | `"🔍"` | Prefix for shortcuts |
| `dmenu.url_prefix` | string | `"🌐"` | Prefix for URLs |
| `dmenu.date_format` | string | `"2006-01-02 15:04"` | Go time format string |
| `dmenu.sort_by_visit_count` | bool | `true` | Sort by popularity |

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
| `logging.filename` | string | `""` | any | Empty = stdout |
| `logging.max_size` | int | `100` | > 0 | Max log size in MB |
| `logging.max_backups` | int | `3` | >= 0 | Number of log backups |
| `logging.max_age` | int | `7` | >= 0 | Days to keep logs |
| `logging.compress` | bool | `true` | - | Compress old logs |
| `logging.log_dir` | string | `~/.local/state/dumber/logs/` | any | Log directory |
| `logging.enable_file_log` | bool | `true` | - | Enable file logging |
| `logging.capture_stdout` | bool | `false` | - | Capture stdout to log |
| `logging.capture_stderr` | bool | `false` | - | Capture stderr to log |
| `logging.capture_c_output` | bool | `false` | - | Capture C/CGO output |
| `logging.capture_console` | bool | `false` | - | Capture browser console |
| `logging.debug_file` | string | `"debug.log"` | any | Debug log filename |
| `logging.verbose_webkit` | bool | `false` | - | Verbose WebKit logs |

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

## Video Acceleration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `video_acceleration.enable_vaapi` | bool | `true` | Enable VA-API hardware accel |
| `video_acceleration.auto_detect_gpu` | bool | `true` | Auto-detect GPU driver |
| `video_acceleration.vaapi_driver_name` | string | `""` | Driver name (auto-detected) |
| `video_acceleration.enable_all_drivers` | bool | `true` | Enable all VA-API drivers |
| `video_acceleration.legacy_vaapi` | bool | `false` | Legacy VA-API mode |

## Codec Preferences

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `codec_preferences.preferred_codecs` | string | `"av1,h264"` | Comma-separated | Codec priority order |
| `codec_preferences.force_av1` | bool | `false` | - | Force AV1 codec |
| `codec_preferences.block_vp9` | bool | `false` | - | Block VP9 codec |
| `codec_preferences.block_vp8` | bool | `false` | - | Block VP8 codec |
| `codec_preferences.av1_hardware_only` | bool | `false` | - | Disable AV1 software fallback |
| `codec_preferences.disable_vp9_hardware` | bool | `false` | - | Disable VP9 hardware accel |
| `codec_preferences.video_buffer_size_mb` | int | `64` | > 0 | Video buffer size in MB |
| `codec_preferences.queue_buffer_time_sec` | int | `20` | > 0 | Queue buffer time in seconds |
| `codec_preferences.custom_user_agent` | string | Chrome UA | any | Custom user agent string |
| `codec_preferences.av1_max_resolution` | string | `"1080p"` | `720p`, `1080p`, `1440p`, `4k`, `unlimited` | Max AV1 resolution |
| `codec_preferences.disable_twitch_codec_control` | bool | `true` | - | Disable codec control on Twitch |

## Debug Options

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `debug.enable_webkit_debug` | bool | `false` | Enable WebKit debug logs |
| `debug.webkit_debug_categories` | string | `"Network:preconnectTo,ContentFilters"` | Debug categories |
| `debug.enable_filtering_debug` | bool | `false` | Content filter debug |
| `debug.enable_webview_debug` | bool | `false` | WebView state debug |
| `debug.log_webkit_crashes` | bool | `true` | Log WebKit crashes |
| `debug.enable_script_debug` | bool | `false` | Script injection debug |
| `debug.enable_general_debug` | bool | `false` | General debug mode |
| `debug.enable_workspace_debug` | bool | `false` | Workspace navigation debug |
| `debug.enable_focus_debug` | bool | `false` | Focus state debug |
| `debug.enable_css_debug` | bool | `false` | CSS reconciler debug |
| `debug.enable_focus_metrics` | bool | `false` | Focus metrics tracking |
| `debug.enable_pane_close_debug` | bool | `false` | Pane close debug |

## Rendering & Zoom

| Key | Type | Default | Valid Values | Description |
|-----|------|---------|--------------|-------------|
| `rendering_mode` | string | `"gpu"` | `auto`, `gpu`, `cpu` | Rendering mode |
| `use_dom_zoom` | bool | `false` | - | Use DOM-based zoom |
| `default_zoom` | float | `1.2` | > 0 | Default zoom level (120%) |

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
close-pane = ["x"]
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

### Tab Shortcuts

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace.tabs.new_tab` | string | `"ctrl+t"` | New tab shortcut |
| `workspace.tabs.close_tab` | string | `"ctrl+w"` | Close tab shortcut |
| `workspace.tabs.next_tab` | string | `"ctrl+tab"` | Next tab shortcut |
| `workspace.tabs.previous_tab` | string | `"ctrl+shift+tab"` | Previous tab shortcut |

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
| `workspace.styling.ui_scale` | float | `1.0` | UI scale multiplier (1.0 = 100%, 1.2 = 120%) |

## Content Filtering

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `content_filtering.enabled` | bool | `true` | Enable ad blocking |
| `content_filtering.whitelist` | array | See below | Domains to skip ad blocking |
| `content_filtering.filter_lists` | array | See below | Filter list URLs to use |

**Default whitelist:**
```toml
[content_filtering]
enabled = true
whitelist = [
  "twitch.tv",
  "passport.twitch.tv",
  "gql.twitch.tv"
]
```

**Default filter lists:**
```toml
[content_filtering]
filter_lists = [
  # Core blocking
  "https://easylist.to/easylist/easylist.txt",           # Ads
  "https://easylist.to/easylist/easyprivacy.txt",        # Tracking
  # uBlock extras
  "https://raw.githubusercontent.com/uBlockOrigin/uAssets/master/filters/filters.txt",    # uBO optimizations
  "https://raw.githubusercontent.com/uBlockOrigin/uAssets/master/filters/annoyances.txt", # Cookie banners, popups
  "https://raw.githubusercontent.com/uBlockOrigin/uAssets/master/filters/quick-fixes.txt" # Site fixes
]
```

**Custom filter lists example:**
```toml
[content_filtering]
filter_lists = [
  # Minimal setup
  "https://easylist.to/easylist/easylist.txt",
  "https://easylist.to/easylist/easyprivacy.txt",
  # Regional list
  "https://easylist-downloads.adblockplus.org/liste_fr.txt"
]
```

## Environment Variables

All config values can be overridden via environment variables with the prefix `DUMB_BROWSER_`:

```bash
# Database
DUMB_BROWSER_DATABASE_PATH=/custom/path/db.sqlite

# Rendering
DUMBER_RENDERING_MODE=cpu
DUMBER_DEFAULT_ZOOM=1.5

# Video/Codec
DUMBER_VIDEO_ACCELERATION_ENABLE=true
LIBVA_DRIVER_NAME=iHD
DUMBER_PREFERRED_CODECS=av1,vp9,h264
```

**Note:** Some video/codec env vars use `DUMBER_*` prefix and match GStreamer/VA-API conventions.
