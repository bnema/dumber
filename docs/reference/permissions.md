# Website Permissions

Dumber handles website permission requests for media access (microphone, camera, screen sharing) following W3C standards.

## Permission Types

### Auto-Allowed

These permissions are automatically granted without user interaction:

- **Display Capture** (screen sharing) - The XDG Desktop Portal handles the UI
- **Device Enumeration** (listing microphones/cameras) - Low security risk

### User-Confirmed

These permissions require explicit user approval:

- **Microphone** - Websites can request audio capture access
- **Camera** - Websites can request video capture access
- **Microphone + Camera** - Combined requests for both audio and video

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

Permissions are stored per-origin, not per-page. This means:

- `https://meet.google.com/room1` and `https://meet.google.com/room2` share the same permission
- `https://github.com` and `https://gist.github.com` share the same permission

### Permission States

- **Granted** - Permission was allowed (manually or via "Always Allow")
- **Denied** - Permission was blocked (manually or via "Always Deny")
- **Prompt** - No decision made yet, will show dialog on next request

### W3C Compliance

The permission system follows W3C specifications:

- Display capture permissions are **never persisted** (always prompt via portal)
- Mic/camera permissions **can be persisted** with user consent
- Permissions are scoped to origins (scheme + host)

## Managing Stored Permissions

Currently, stored permissions can only be cleared by:

1. **Site-specific reset**: The site can use `navigator.permissions.revoke()` (if supported)
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
