# Configuration Reference

**Primary format**: JSON
**Location**: `~/.config/dumber/config.json`
**Also supports**: YAML (`.yaml`) and TOML (`.toml`)
**JSON Schema**: `~/.config/dumber/config.schema.json` (auto-generated)

## IDE Integration

For autocompletion and validation in VS Code or other editors, add this to the top of your `config.json`:

```json
{
  "$schema": "./config.schema.json",
  "default_zoom": 1.2,
  ...
}
```

The schema is automatically generated when you first create a config file.

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

## Search Shortcuts

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `search_shortcuts` | map | See defaults | Map of shortcut aliases to URLs |

**Default shortcuts:**
```json
{
  "search_shortcuts": {
    "g": {
      "url": "https://google.com/search?q=%s",
      "description": "Google Search"
    },
    "gh": {
      "url": "https://github.com/search?q=%s",
      "description": "GitHub"
    },
    "yt": {
      "url": "https://youtube.com/results?search_query=%s",
      "description": "YouTube"
    }
  }
}
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
```json
{
  "appearance": {
    "light_palette": {
      "background": "#f8f8f8",
      "surface": "#f2f2f2",
      "surface_variant": "#ececec",
      "text": "#1a1a1a",
      "muted": "#6e6e6e",
      "accent": "#404040",
      "border": "#d2d2d2"
    }
  }
}
```

**Dark palette:**
```json
{
  "appearance": {
    "dark_palette": {
      "background": "#0e0e0e",
      "surface": "#1a1a1a",
      "surface_variant": "#141414",
      "text": "#e4e4e4",
      "muted": "#848484",
      "accent": "#a8a8a8",
      "border": "#363636"
    }
  }
}
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
| `codec_preferences.block_vp8` | bool | `true` | - | Block VP8 codec |
| `codec_preferences.av1_hardware_only` | bool | `false` | - | Disable AV1 software fallback |
| `codec_preferences.disable_vp9_hardware` | bool | `false` | - | Disable VP9 hardware accel |
| `codec_preferences.video_buffer_size_mb` | int | `16` | > 0 | Video buffer size in MB |
| `codec_preferences.queue_buffer_time_sec` | int | `10` | > 0 | Queue buffer time in seconds |
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

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace.enable_zellij_controls` | bool | `true` | Enable Zellij-style keybindings |

### Pane Mode

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace.pane_mode.activation_shortcut` | string | `"ctrl+p"` | Pane mode activation key |
| `workspace.pane_mode.timeout_ms` | int | `3000` | Pane mode timeout (ms) |
| `workspace.pane_mode.action_bindings` | map | See below | Pane mode key bindings |

**Default pane mode bindings:**
```json
{
  "workspace": {
    "pane_mode": {
      "action_bindings": {
        "arrowright": "split-right",
        "arrowleft": "split-left",
        "arrowup": "split-up",
        "arrowdown": "split-down",
        "r": "split-right",
        "l": "split-left",
        "u": "split-up",
        "d": "split-down",
        "x": "close-pane",
        "enter": "confirm",
        "escape": "cancel"
      }
    }
  }
}
```

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
| `workspace.styling.border_width` | int | `2` | Border width in pixels |
| `workspace.styling.border_color` | string | `"@theme_selected_bg_color"` | Border color (CSS or theme var) |
| `workspace.styling.pane_mode_border_color` | string | `"#FFA500"` | Pane mode border color (orange) |
| `workspace.styling.transition_duration` | int | `120` | Transition duration (ms) |
| `workspace.styling.border_radius` | int | `0` | Border radius in pixels |

## Content Filtering

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `content_filtering_whitelist` | array | See below | Domains to skip ad blocking |

**Default whitelist:**
```json
{
  "content_filtering_whitelist": [
    "twitch.tv",
    "passport.twitch.tv",
    "gql.twitch.tv"
  ]
}
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
