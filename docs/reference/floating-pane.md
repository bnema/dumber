# Floating Pane

The floating pane is an overlay workspace pane for quick-access pages like search, chat, email, or docs.

## Core Behavior

| Action | Default Key | Result |
|--------|-------------|--------|
| Toggle floating pane | `Alt+F` | Show or hide the floating pane while keeping its current page and state |
| Open profile in floating pane | User-defined (for example `Alt+G`) | Open or focus a named floating profile URL |
| Close active pane | `Ctrl+W` | Close active pane; if floating is active, fully release floating session/WebView |

### Toggle vs Close

- `Alt+F` is a visibility toggle. It hides or shows the floating pane without resetting content.
- `Ctrl+W` on an active floating pane is a full close. It releases that floating session and its WebView.
- After a floating pane is closed with `Ctrl+W`, opening it again starts from a fresh state.

## Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `workspace.floating_pane.width_pct` | float | `0.82` | Floating pane width as a fraction of workspace width (`(0,1]`) |
| `workspace.floating_pane.height_pct` | float | `0.72` | Floating pane height as a fraction of workspace height (`(0,1]`) |
| `workspace.floating_pane.profiles.<name>.keys` | `[]string` | - | Shortcut keys for a profile |
| `workspace.floating_pane.profiles.<name>.url` | string | - | URL to load for the profile |
| `workspace.floating_pane.profiles.<name>.desc` | string | - | Optional description |

## Example

```toml
[workspace.shortcuts.actions.toggle_floating_pane]
keys = ["alt+f"]
desc = "Toggle floating pane"

[workspace.floating_pane]
width_pct = 0.82
height_pct = 0.72

[workspace.floating_pane.profiles.gmail]
keys = ["alt+m"]
url = "https://mail.google.com"
desc = "Open Gmail in floating pane"

[workspace.floating_pane.profiles.google]
keys = ["alt+g"]
url = "https://google.com"
desc = "Open Google in floating pane"
```

## Practical Notes

- The default install only binds `Alt+F` for floating pane toggle.
- Profile shortcuts are opt-in and empty by default.
- Profile shortcuts support multi-modifier combos using `ctrl`, `shift`, and `alt` (for example `ctrl+shift+y` or `ctrl+alt+m`).
- Some `Alt+<key>` shortcuts can conflict with default WebKit shortcuts, website handlers, or your desktop environment.
- If a shortcut does not fire in Dumber, bind a different key.
