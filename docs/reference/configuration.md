# Configuration Reference

| Key | Type | Default | Valid Values |
|-----|------|---------|--------------|
| `database.path` | string | `~/.local/share/dumber/dumber.db` | |
| `history.max_entries` | int | `10000` | > 0 |
| `history.retention_period_days` | int | `365` | > 0 |
| `history.cleanup_interval_days` | int | `1` | > 0 |
| `default_search_engine` | string | `https://duckduckgo.com/?q=%s` | URL with `%s` |
| `search_shortcuts.<name>.url` | string | | URL with `%s` |
| `search_shortcuts.<name>.description` | string | | |
| `dmenu.max_history_days` | int | `30` | >= 0 |
| `dmenu.show_visit_count` | bool | `true` | |
| `dmenu.show_last_visited` | bool | `true` | |
| `dmenu.history_prefix` | string | `🕒` | |
| `dmenu.shortcut_prefix` | string | `🔍` | |
| `dmenu.url_prefix` | string | `🌐` | |
| `dmenu.date_format` | string | `2006-01-02 15:04` | Go time format |
| `dmenu.sort_by_visit_count` | bool | `true` | |
| `omnibox.initial_behavior` | string | `recent` | `recent`, `most_visited`, `none` |
| `omnibox.most_visited_days` | int | `30` | `>= 0` |
| `omnibox.auto_open_on_new_pane` | bool | `false` | |
| `logging.level` | string | `info` | `trace`, `debug`, `info`, `warn`, `error`, `fatal` |
| `logging.format` | string | `text` | `text`, `json`, `console` |
| `logging.max_age` | int | `7` | >= 0 |
| `logging.max_files` | int | `100` | >= 0 (0 disables count cleanup) |
| `logging.log_dir` | string | `~/.local/state/dumber/logs` | any path |
| `logging.enable_file_log` | bool | `true` | |
| `logging.capture_console` | bool | `false` | |
| `logging.capture_gtk_logs` | bool | `false` | |
| `appearance.sans_font` | string | `Fira Sans` | |
| `appearance.serif_font` | string | `Fira Sans` | |
| `appearance.monospace_font` | string | `Fira Code` | |
| `appearance.gtk_font` | string | `Adwaita Sans` | |
| `appearance.default_font_size` | int | `16` | |
| `appearance.color_scheme` | string | `default` | `prefer-dark`, `prefer-light`, `default` |
| `appearance.external_theme.enabled` | bool | `false` | |
| `appearance.external_theme.provider` | string | `noctalia` | `noctalia` |
| `appearance.external_theme.format` | string | `colors-json` | `colors-json`, `dumber-json` |
| `appearance.external_theme.path` | string | `$XDG_CONFIG_HOME/noctalia/colors.json` | path to Noctalia `colors.json` or a `dumber-json` theme file; when empty and `format = "dumber-json"`, dumber uses the built-in dumber-json template path |
| `appearance.light_palette.background` | string | `#f8f8f8` | |
| `appearance.light_palette.surface` | string | `#f2f2f2` | |
| `appearance.light_palette.surface_variant` | string | `#ececec` | |
| `appearance.light_palette.text` | string | `#1a1a1a` | |
| `appearance.light_palette.muted` | string | `#6e6e6e` | |
| `appearance.light_palette.accent` | string | `#404040` | |
| `appearance.light_palette.border` | string | `#d2d2d2` | |
| `appearance.dark_palette.background` | string | `#0e0e0e` | |
| `appearance.dark_palette.surface` | string | `#1a1a1a` | |
| `appearance.dark_palette.surface_variant` | string | `#141414` | |
| `appearance.dark_palette.text` | string | `#e4e4e4` | |
| `appearance.dark_palette.muted` | string | `#848484` | |
| `appearance.dark_palette.accent` | string | `#a8a8a8` | |
| `appearance.dark_palette.border` | string | `#363636` | |
| `debug.enable_devtools` | bool | `true` | |
| `engine.cef.log_file` | string | `` | CEF runtime log path |
| `engine.cef.log_severity` | int32 | `0` | `0`, `1`, `2`, `3`, `4`, `99` |
| `engine.cef.trace_handlers` | bool | `false` | |
| `engine.cef.enable_audio_handler` | bool | `true` | experimental |
| `engine.type` | string | `cef` | `cef`, `webkit` (CEF default; WebKitGTK fallback) |
| `engine.cookie_policy` | string | `always` | `always`, `no_third_party`, `never` |
| `engine.webkit.itp_enabled` | bool | `true` | WebKit fallback only |
| `engine.cef.render_stack` | string | `vulkan` | `vulkan`, `egl` |
| `engine.cef.adaptive_windowless_frame_rate` | bool | `true` | |
| `engine.cef.windowless_frame_rate` | int32 | `0` | >= 0 |
| `engine.cef.windowless_frame_rate_max` | int32 | `240` | >= 0 |
| `engine.cef.input.scroll_wheel_multiplier` | float | `1.0` | > 0 |
| `engine.cef.input.scroll_precise_multiplier` | float | `2.5` | > 0 |
| `engine.cef.input.scroll_horizontal_multiplier` | float | `1.0` | > 0 |
| `engine.cef.input.scroll_vertical_multiplier` | float | `1.0` | > 0 |
| `engine.cef.input.scroll_max_delta` | int32 | `0` | >= 0 |
| `engine.cef.input.touchpad_navigation_enabled` | bool | `true` | |
| `engine.cef.input.touchpad_navigation_min_delta` | float | `200.0` | > 0 |
| `engine.cef.input.touchpad_navigation_max_vertical_ratio` | float | `0.5` | > 0 |
| `engine.webkit.gsk_renderer` | string | `auto` | `auto`, `opengl`, `vulkan`, `cairo` (WebKit fallback only) |
| `engine.webkit.disable_dmabuf_renderer` | bool | `false` | WebKit fallback only |
| `engine.webkit.force_compositing_mode` | bool | `false` | WebKit fallback only |
| `engine.webkit.disable_compositing_mode` | bool | `false` | WebKit fallback only |
| `engine.webkit.disable_mipmaps` | bool | `false` | WebKit fallback only |
| `engine.webkit.prefer_gl` | bool | `false` | WebKit fallback only |
| `engine.webkit.draw_compositing_indicators` | bool | `false` | WebKit fallback only |
| `engine.webkit.show_fps` | bool | `false` | WebKit fallback only |
| `engine.webkit.sample_memory` | bool | `false` | WebKit fallback only |
| `engine.webkit.debug_frames` | bool | `false` | WebKit fallback only |
| `engine.webkit.force_vsync` | bool | `false` | WebKit fallback GStreamer only |
| `engine.webkit.gl_rendering_mode` | string | `auto` | `auto`, `gles2`, `gl3`, `none` (WebKit fallback only) |
| `engine.webkit.gstreamer_debug_level` | int | `0` | `0-5` (WebKit fallback only) |
| `default_ui_scale` | float | `1.0` | > 0 |
| `default_webpage_zoom` | float | `1.2` | > 0 |
| `sidebar_width` | int | `320` | `0` or `280-380` |
| `workspace.new_pane_url` | string | `about:blank` | |
| `workspace.switch_to_tab_on_move` | bool | `true` | |
| `workspace.tab_bar_position` | string | `bottom` | `top`, `bottom` |
| `workspace.hide_tab_bar_when_single_tab` | bool | `true` | |
| `workspace.pane_mode.activation_shortcut` | string | `ctrl+p` | |
| `workspace.pane_mode.timeout_ms` | int | `3000` | |
| `workspace.pane_mode.actions.<action>` | []string | see defaults | pane mode key mappings |
| `workspace.tab_mode.activation_shortcut` | string | `ctrl+t` | |
| `workspace.tab_mode.timeout_ms` | int | `3000` | |
| `workspace.tab_mode.actions.<action>` | []string | see defaults | tab mode key mappings |
| `workspace.resize_mode.activation_shortcut` | string | `ctrl+n` | |
| `workspace.resize_mode.timeout_ms` | int | `3000` | |
| `workspace.resize_mode.actions.<action>` | []string | see defaults | resize mode key mappings |
| `workspace.resize_mode.step_percent` | float | `5.0` | |
| `workspace.resize_mode.min_pane_percent` | float | `10.0` | |
| `workspace.shortcuts.actions` | map | see defaults | global shortcut action mappings |
| `workspace.shortcuts.actions.toggle_floating_pane.keys` | []string | `alt+f` | key strings |
| `workspace.shortcuts.actions.toggle_floating_pane.desc` | string | `Toggle floating pane` | |
| `workspace.floating_pane.width_pct` | float | `0.82` | `(0,1]` |
| `workspace.floating_pane.height_pct` | float | `0.72` | `(0,1]` |
| `workspace.floating_pane.profiles.<name>.keys` | []string | | at least one key |
| `workspace.floating_pane.profiles.<name>.url` | string | | required URL |
| `workspace.floating_pane.profiles.<name>.desc` | string | | |
| `workspace.browsing_contexts.behavior` | string | `split` | `split`, `stacked`, `tabbed`, `windowed` |
| `workspace.browsing_contexts.placement` | string | `right` | `right`, `left`, `top`, `bottom` |
| `workspace.browsing_contexts.open_in_new_pane` | bool | `true` | |
| `workspace.browsing_contexts.follow_pane_context` | bool | `true` | |
| `workspace.browsing_contexts.blank_target_behavior` | string | `stacked` | `split`, `stacked`, `tabbed` |
| `workspace.browsing_contexts.enable_smart_detection` | bool | `true` | |
| `workspace.browsing_contexts.oauth_auto_close` | bool | `true` | |
| `workspace.styling.border_width` | int | `1` | |
| `workspace.styling.border_color` | string | `@theme_selected_bg_color` | |
| `workspace.styling.mode_border_width` | int | `4` | |
| `workspace.styling.pane_mode_color` | string | `#4A90E2` | |
| `workspace.styling.tab_mode_color` | string | `#FFA500` | |
| `workspace.styling.session_mode_color` | string | `#9B59B6` | |
| `workspace.styling.resize_mode_color` | string | `#00D4AA` | |
| `workspace.styling.mode_indicator_toaster_enabled` | bool | `true` | |
| `workspace.styling.transition_duration` | int | `120` | |
| `session.auto_restore` | bool | `false` | |
| `session.snapshot_interval_ms` | int | `5000` | |
| `session.max_exited_sessions` | int | `50` | |
| `session.max_exited_session_age_days` | int | `7` | |
| `session.session_mode.activation_shortcut` | string | `ctrl+o` | |
| `session.session_mode.timeout_ms` | int | `3000` | |
| `session.session_mode.actions.<action>` | []string | see defaults | session mode key mappings |
| `media.hardware_decoding` | string | `auto` | `auto`, `force`, `disable` |
| `media.prefer_av1` | bool | `false` | |
| `media.show_diagnostics` | bool | `false` | |
| `engine.cef.cef_dir` | string | `` | CEF runtime directory |
| `engine.webkit.prefix` | string | `` | WebKitGTK fallback runtime prefix |
| `clipboard.auto_copy_on_selection` | bool | `true` | |
| `content_filtering.enabled` | bool | `true` | |
| `content_filtering.auto_update` | bool | `true` | |
| `update.enable_on_startup` | bool | `true` | |
| `update.auto_download` | bool | `false` | |
| `update.notify_on_new_settings` | bool | `true` | |
| `engine.profile` | string | `default` | `default`, `lite`, `balanced`, `max`, `custom` |
| `engine.webkit.skia_cpu_painting_threads` | int | `0` | >= 0 (WebKit fallback custom profile only) |
| `engine.webkit.skia_gpu_painting_threads` | int | `-1` | >= -1 (WebKit fallback custom profile only) |
| `engine.webkit.skia_enable_cpu_rendering` | bool | `false` | WebKit fallback custom profile only |
| `engine.webkit.web_process_memory_limit_mb` | int | `0` | >= 0 (WebKit fallback custom profile only) |
| `engine.webkit.web_process_memory_poll_interval_sec` | float | `0` | >= 0 (WebKit fallback custom profile only) |
| `engine.webkit.web_process_memory_conservative_threshold` | float | `0` | 0-1 (WebKit fallback custom profile only) |
| `engine.webkit.web_process_memory_strict_threshold` | float | `0` | 0-1 (WebKit fallback custom profile only) |
| `engine.webkit.network_process_memory_limit_mb` | int | `0` | >= 0 (WebKit fallback custom profile only) |
| `engine.webkit.network_process_memory_poll_interval_sec` | float | `0` | >= 0 (WebKit fallback custom profile only) |
| `engine.webkit.network_process_memory_conservative_threshold` | float | `0` | 0-1 (WebKit fallback custom profile only) |
| `engine.webkit.network_process_memory_strict_threshold` | float | `0` | 0-1 (WebKit fallback custom profile only) |
| `engine.pool_prewarm_count` | int | `4` | >= 0 |
| `engine.zoom_cache_size` | int | `256` | >= 0 |
| `downloads.path` | string | `` | |

Touchpad vertical scroll speed is controlled by `engine.cef.input.scroll_precise_multiplier` and the additional axis-specific `engine.cef.input.scroll_vertical_multiplier`. `engine.cef.input.touchpad_navigation_max_vertical_ratio` only filters horizontal back/forward swipe recognition; it does not tune vertical scroll speed.

`engine.cef.input.touchpad_navigation_min_delta` is measured in raw GTK touchpad surface units. The default `200.0` matches WebKit-style commit distance to reduce accidental back/forward navigation and can be overridden in `config.toml`.

CEF is the default engine. WebKitGTK is a fallback selected with `engine.type = "webkit"`; `engine.webkit.*` keys are WebKit-specific.

Legacy key migration:

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
