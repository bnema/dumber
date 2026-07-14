# Crash stability self-review

- TreeRenderer owns each rendered SplitView and calls its idempotent cleanup before clearing/rebuilding mappings.
- Workspace rebuild releases old PaneViews; PaneView cleanup now clears mouse motion and loading-overlay ownership.
- Floating and CEF raw GTK tick callbacks retain their heap callback only while registered and clear their slots before explicit or automatic source removal.
- The one-shot CEF marker is root-cache scoped, regular-file-only on consume, preserves Vulkan render-stack variables, disables VAAPI, and drops unsafe Chromium overrides after a second GPU-process relaunch.
- Hover Detach owns and removes its GTK controller, disconnects all signal IDs, removes its GLib source, clears retained callbacks, and rejects stale queued work.
- Split, CEF BeginFrame, and floating-resize raw tick owners release canonical purego callback slots once after source/tick removal.
- CEF derives initial and transition visibility from GTK ancestor-aware IsVisible plus GetMapped; map/show/hide/unmap hooks deduplicate WasHidden while visible transitions continue viewport resize/invalidate.
- `f7f18b55` -> `25e7f910` and `2caaf66c` -> `5d4ae1d6` are signed RED-test-to-fix pairs for the remaining CEF and GTK blockers.
- GREEN: focused CEF test; GTK stress under `DISPLAY=:1 WAYLAND_DISPLAY=wayland-1 G_DEBUG=fatal-criticals PUREGO_CALLBACK_LEDGER=1`; targeted race barrier (`component`, `layout`, `ui`, `cef`); `go test -mod=vendor ./...`; `make lint`; `make test`; `make check`.
- The GTK stress is no longer opt-in: it executes 3,200 p3/p0 real controller enter/leave handler cycles with close/rebuild mutations on the default main context, asserts ownership before GTK mutation, and verifies actual raw TickCallback ledger releases and slot reuse. It skips only when `gtk.InitCheck` reports no native display.
- A broad `G_DEBUG=fatal-criticals go test -race ./...` also exposed pre-existing unrelated native test failures: ui app tests construct an application window before GApplication startup, and hover's synthetic source-ID test calls GLib SourceRemove on a non-existent mock ID. The targeted post-Favorites race barrier above is green.
