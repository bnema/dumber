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
| `omnibox.auto_open_on_new_pane` | bool | `false` | |
| `logging.level` | string | `info` | `debug`, `info`, `warn`, `error` |
| `logging.format` | string | `text` | `text`, `json` |
| `logging.max_age` | int | `7` | >= 0 |
| `logging.log_dir` | string | `~/.local/state/dumber/logs/` | |
| `logging.enable_file_log` | bool | `true` | |
| `logging.capture_console` | bool | `false` | |
| `logging.capture_gtk_webkit_logs` | bool | `false` | |
| `appearance.sans_font` | string | `Fira Sans` | |
| `appearance.serif_font` | string | `Fira Sans` | |
| `appearance.monospace_font` | string | `Fira Code` | |
| `appearance.default_font_size` | int | `16` | |
| `appearance.color_scheme` | string | `default` | `prefer-dark`, `prefer-light`, `default` |
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
| `rendering.mode` | string | `gpu` | `auto`, `gpu`, `cpu` |
| `rendering.disable_dmabuf_renderer` | bool | `false` | |
| `rendering.force_compositing_mode` | bool | `false` | |
| `rendering.disable_compositing_mode` | bool | `false` | |
| `rendering.gsk_renderer` | string | `auto` | `auto`, `opengl`, `vulkan`, `cairo` |
| `rendering.disable_mipmaps` | bool | `false` | |
| `rendering.prefer_gl` | bool | `false` | |
| `rendering.draw_compositing_indicators` | bool | `false` | |
| `rendering.show_fps` | bool | `false` | |
| `rendering.sample_memory` | bool | `false` | |
| `rendering.debug_frames` | bool | `false` | |
| `default_ui_scale` | float | `1.0` | > 0 |
| `default_webpage_zoom` | float | `1.2` | > 0 |
| `workspace.new_pane_url` | string | `about:blank` | |
| `workspace.switch_to_tab_on_move` | bool | `true` | |
| `workspace.pane_mode.activation_shortcut` | string | `ctrl+p` | |
| `workspace.pane_mode.timeout_ms` | int | `3000` | |
| `workspace.tab_mode.activation_shortcut` | string | `ctrl+t` | |
| `workspace.tab_mode.timeout_ms` | int | `3000` | |
| `workspace.resize_mode.activation_shortcut` | string | `ctrl+n` | |
| `workspace.resize_mode.timeout_ms` | int | `3000` | |
| `workspace.resize_mode.step_percent` | float | `5.0` | |
| `workspace.resize_mode.min_pane_percent` | float | `10.0` | |
| `workspace.shortcuts.actions.toggle_floating_pane.keys` | []string | `alt+f` | key strings |
| `workspace.shortcuts.actions.toggle_floating_pane.desc` | string | `Toggle floating pane` | |
| `workspace.floating_pane.width_pct` | float | `0.82` | `(0,1]` |
| `workspace.floating_pane.height_pct` | float | `0.72` | `(0,1]` |
| `workspace.floating_pane.profiles.<name>.keys` | []string | | at least one key |
| `workspace.floating_pane.profiles.<name>.url` | string | | required URL |
| `workspace.floating_pane.profiles.<name>.desc` | string | | |
| `workspace.popups.behavior` | string | `split` | `split`, `stacked`, `tabbed`, `windowed` |
| `workspace.popups.placement` | string | `right` | `right`, `left`, `top`, `bottom` |
| `workspace.popups.open_in_new_pane` | bool | `true` | |
| `workspace.popups.follow_pane_context` | bool | `true` | |
| `workspace.popups.blank_target_behavior` | string | `pane` | `pane`, `tab` |
| `workspace.popups.enable_smart_detection` | bool | `true` | |
| `workspace.popups.oauth_auto_close` | bool | `true` | |
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
| `media.hardware_decoding` | string | `auto` | `auto`, `force`, `disable` |
| `media.prefer_av1` | bool | `false` | |
| `media.show_diagnostics` | bool | `false` | |
| `media.force_vsync` | bool | `false` | |
| `media.gl_rendering_mode` | string | `auto` | `auto`, `gles2`, `gl3`, `none` |
| `media.gstreamer_debug_level` | int | `0` | 0-5 |
| `runtime.prefix` | string | `` | |
| `clipboard.auto_copy_on_selection` | bool | `true` | |
| `content_filtering.enabled` | bool | `true` | |
| `content_filtering.auto_update` | bool | `true` | |
| `update.enable_on_startup` | bool | `true` | |
| `update.auto_download` | bool | `false` | |
| `update.notify_on_new_settings` | bool | `true` | |
| `performance.profile` | string | `default` | `default`, `lite`, `max`, `custom` |
| `performance.skia_cpu_painting_threads` | int | `0` | |
| `performance.skia_gpu_painting_threads` | int | `-1` | |
| `performance.skia_enable_cpu_rendering` | bool | `false` | |
| `performance.web_process_memory_limit_mb` | int | `0` | |
| `performance.web_process_memory_poll_interval_sec` | float | `0` | |
| `performance.web_process_memory_conservative_threshold` | float | `0` | |
| `performance.web_process_memory_strict_threshold` | float | `0` | |
| `performance.network_process_memory_limit_mb` | int | `0` | |
| `performance.network_process_memory_poll_interval_sec` | float | `0` | |
| `performance.network_process_memory_conservative_threshold` | float | `0` | |
| `performance.network_process_memory_strict_threshold` | float | `0` | |
| `performance.webview_pool_prewarm_count` | int | `4` | |
| `performance.zoom_cache_size` | int | `256` | |
| `downloads.path` | string | `` | |
