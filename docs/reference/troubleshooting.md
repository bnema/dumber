# Troubleshooting

## Diagnostics

Run the doctor command to check your system:

```bash
dumber doctor           # Full check
dumber doctor --runtime # GTK4/WebKitGTK only
dumber doctor --media   # GStreamer/VA-API only
```

## Common Issues

### Browser won't start

**Symptoms:** Error about missing libraries or GTK/WebKitGTK version

**Solution:**
1. Run `dumber doctor --runtime`
2. Install missing dependencies:
   - Arch: `pacman -S webkit2gtk-4.1 gtk4`
   - Fedora: `dnf install webkit2gtk4.1 gtk4`
   - Ubuntu: `apt install libwebkit2gtk-4.1-0 libgtk-4-1`

### Video playback issues

**Symptoms:** Videos don't play, high CPU usage, or Error #4000 on Twitch

**Solution:**
1. Run `dumber doctor --media`
2. Check hardware decoding:
   ```toml
   # ~/.config/dumber/config.toml
   [media]
   hardware_decoding = "auto"  # Try "disable" if issues persist
   ```
3. Install VA-API drivers:
   - AMD: `libva-mesa-driver`
   - Intel: `intel-media-driver`
   - NVIDIA: `libva-nvidia-driver`

### Screen flickering on Wayland

**Symptoms:** Flicker when scrolling or rendering

**Solution:**
```toml
[rendering]
disable_dmabuf_renderer = true
```

### Fonts look wrong

**Symptoms:** Missing characters, wrong font rendering

**Solution:**
```toml
[appearance]
sans_font = "Your Preferred Font"
monospace_font = "Your Mono Font"
default_font_size = 16
```

### High memory usage

**Symptoms:** Browser uses too much RAM

**Solution:**
```toml
[performance]
profile = "lite"  # Reduces resource usage
```

### Session not restoring

**Symptoms:** Previous tabs/panes don't restore on startup

**Solution:**
1. Check session auto-restore:
   ```toml
   [session]
   auto_restore = true
   ```
2. List sessions: `dumber sessions list`
3. Manually restore: `dumber sessions restore <id>`

### Launcher integration not working

**Symptoms:** Rofi/fuzzel doesn't show history or icons

**Solution:**
1. Ensure PNG favicons are cached:
   ```bash
   dumber dmenu | head -5  # Should show entries with icon paths
   ```
2. Check rofi/fuzzel supports icons:
   ```bash
   dumber dmenu | rofi -dmenu -show-icons
   ```

## Logs

View application logs for debugging:

```bash
dumber logs              # List sessions
dumber logs <session-id> # View specific session
dumber logs -f <id>      # Follow in real-time
```

## Reset Configuration

If config is corrupted:

```bash
mv ~/.config/dumber/config.toml ~/.config/dumber/config.toml.bak
dumber browse  # Creates new default config
```

## Complete Reset

Remove all data and start fresh:

```bash
dumber purge
```

## Getting Help

- Check logs: `dumber logs`
- GitHub Issues: https://github.com/bnema/dumber/issues
