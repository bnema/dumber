# Crash stability self-review

- TreeRenderer owns each rendered SplitView and calls its idempotent cleanup before clearing/rebuilding mappings.
- Workspace rebuild releases old PaneViews; PaneView cleanup now clears mouse motion and loading-overlay ownership.
- Floating and CEF raw GTK tick callbacks retain their heap callback only while registered and clear their slots before explicit or automatic source removal.
- The one-shot CEF marker is root-cache scoped, regular-file-only on consume, preserves Vulkan render-stack variables, disables VAAPI, and drops unsafe Chromium overrides after a second GPU-process relaunch.
- All focused GREEN tests, vendor suite, lint, make test, and make check passed. The only focused race failure is FavoritesSidebar, independently reproduced in detached origin/next 68e8ab6f.
- GTK pressure test was invoked with DISPLAY=:1, WAYLAND_DISPLAY=wayland-1, G_DEBUG=fatal-criticals, and callback ledger enabled; the repository test is a documented placeholder and does not drive GTK callbacks.
