# Website Permissions

Dumber handles website permission requests for media access (microphone, camera, screen sharing) following W3C standards.

## Permission Types

### Auto-Allowed

These permissions are automatically granted without user interaction:

- **Display Capture** (screen sharing) - The XDG Desktop Portal handles the UI
  
  > ⚠️ **Known Issue**: Screen sharing does **not work** on Wayland with WebKitGTK 2.50.x due to a GStreamer caps negotiation failure ([WebKit bug #306440](https://bugs.webkit.org/show_bug.cgi?id=306440)). This was fixed on January 29, 2026 and backported to the 2.52 stable branch. **Workaround**: Upgrade to WebKitGTK 2.52+ when available from your distribution. Camera and microphone permissions work correctly.

- **Device Enumeration** (listing microphones/cameras) - Low security risk

### User-Confirmed

These permissions require explicit user approval:

- **Microphone** - Websites can request audio capture access
- **Camera** - Websites can request video capture access

When both microphone and camera are requested together, a combined dialog is shown.

## Permission Dialog

When a site requests mic/camera access, a dialog appears with four options:

| Button | Action | Persistence |
|--------|--------|-------------|
| **Allow** | Grant permission once | Not saved |
| **Always Allow** | Grant and remember | Saved per origin |
| **Deny** | Block permission once | Not saved |
| **Always Deny** | Block and remember | Saved per origin |

## How Permissions Work

### Origin-Based Storage

Permissions are stored per-origin (scheme + host + port), not per-page.
Default ports are normalized away (`:80` for `http`, `:443` for `https`). This means:

- `https://meet.google.com/room1` and `https://meet.google.com/room2` share the same permission (same origin)
- `https://meet.google.com:443/room1` and `https://meet.google.com/room1` share the same permission (default `https` port)
- `https://meet.google.com:8443/room1` and `https://meet.google.com/room1` have different permissions (different port)
- `https://github.com` and `https://gist.github.com` have different permissions (different hosts)

### Permission States

- **Granted** - Permission was allowed (manually or via "Always Allow")
- **Denied** - Permission was blocked (manually or via "Always Deny")
- **Prompt** - No decision made yet, will show dialog on next request

### W3C Compliance

The permission system follows W3C specifications:

- Display capture permissions are **never persisted** (always prompt via portal)
- Mic/camera permissions **can be persisted** with user consent
- Permissions are scoped to origins (scheme + host + port)

## Managing Stored Permissions

Currently, stored permissions can only be cleared by:

1. **Site-specific reset**: `navigator.permissions.revoke()` was removed from the Permissions API and is not supported in modern browsers; direct users to browser/site permission settings (or account-level controls when available) to reset access.
2. **Full reset**: Remove `~/.local/share/dumber/dumber.db` (deletes all data including permissions)

A permission management UI is planned for a future release.

## Privacy Notes

- Permissions are stored in your local SQLite database (`~/.local/share/dumber/dumber.db`)
- No permission data is sent to any server
- "Always Deny" is useful for blocking persistent permission requests from unwanted sites
- The default response is "Deny" (conservative security posture)

## Troubleshooting

### Permission dialog not appearing

1. Check if the site uses HTTPS (required for media permissions)
2. Verify the site isn't already in your stored permissions
3. Check browser logs: `dumber --log-level=debug`

### Permission keeps prompting

- Display capture always prompts (W3C requirement)
- Check if database is writable: `ls -la ~/.local/share/dumber/`

### Site says permission denied but I clicked Allow

- Check if you clicked "Deny" or "Always Deny" previously
- Clear permissions by removing the database (see above)
