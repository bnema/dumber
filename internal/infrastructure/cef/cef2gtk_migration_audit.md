# purego-cef2gtk migration audit

Date: 2026-04-29
Branch: `feature/use-purego-cef2gtk`

This audit records the old Dumber CEF/GTK rendering/input stack references that must be replaced before the old files are deleted.

## Old-stack references

### `renderPipeline`

- Defined in `internal/infrastructure/cef/render_pipeline.go`.
- Owned by `WebView.pipeline` in `webview.go`.
- Created in `factory.go`.
- Used for view rect/screen info, popup paint, CPU `OnPaint`, diagnostics, cursor updates, begin-frame `GtkGLArea`, native widget, and resize callbacks.

Classification:

- Rendering/GL upload: replace with `purego-cef2gtk.View.RenderHandler` and accelerated DMABUF path.
- Widget/GLArea access: replace with the thin Dumber `Cef2gtkAdapter`.
- Size/resize: use `purego-cef2gtk` size observer/size APIs plus small Dumber browser lifecycle callbacks.
- Software paint/PBO tests: delete or move responsibility to `purego-cef2gtk` tests.

### `inputBridge`

- Defined in `internal/infrastructure/cef/input_bridge.go`.
- Created and attached in `factory.go`.
- Host updated/cleared in `webview_handlers.go`.
- Focus queried in loading/focus paths.
- Middle-click link behavior wired in `factory.go` via `OnLinkMiddleClick` and cached hover URI.

Classification:

- Mouse/key/scroll/IME/paste/focus forwarding: replace with `purego-cef2gtk.View.AttachInput`, `SetInputHost`, and `DetachInput`.
- Host attach/update/detach: call through Dumber GTK scheduling path.
- Focus state: use bridge focus helper on GTK thread.
- Middle-click link open: preserve through `purego-cef2gtk.InputOptions.OnMiddleClick`.
- Old input translation tests: delete from Dumber or replace with lifecycle/callback wiring tests.

### `glLoader`

- Defined in `internal/infrastructure/cef/gl_loader.go`.
- Owned by `Engine.gl` and created in `engine_init.go`.
- Passed to factory and consumed by `renderPipeline`.

Classification: delete from Dumber. GL/EGL loading belongs to `purego-cef2gtk` internals.

### `resizeReconciler`

- Defined in `internal/infrastructure/cef/resize_reconciler.go`.
- Owned by `WebView.resizeReconciler`.
- Started from resize callbacks and paint completion callbacks.

Classification:

- Paint-based retry/diagnostics: obsolete after deleting Dumber software paint pipeline.
- Browser resize notification: keep only a small Dumber lifecycle path that calls CEF host `WasResized`/`Invalidate` from bridge size events.

## Non-obvious callouts

- Load watchdog diagnostics in `webview.go` currently log `renderPipelineSnapshot`; replace with bridge diagnostics, accepting less Dumber-specific render detail unless a real product need appears.
- Cursor updates in `webview_handlers.go` currently call `wv.pipeline.glArea.SetCursorFromName`; replace with `Cef2gtkAdapter.SetCursorFromName` on GTK thread.
- Loading/focus checks currently use `wv.input.hasGTKFocus`; replace with bridge focus helper on GTK thread.
- Browser close currently clears old input host directly; replace with GTK-thread `SetInputHost(nil)` or `DetachInput`.
- Begin-frame loop currently uses `pipeline.glArea`; replace with adapter `GLArea` if external begin frame remains enabled.
- `NativeWidget` currently returns old GLArea pointer; replace with adapter native widget pointer.
- Popup/select handling needs runtime verification. `purego-cef2gtk` currently treats CPU `OnPaint` as diagnostic-only and popup callbacks as no-op.
- Text selection updates Dumber state and should be preserved through render-handler hook/composition while paint handling delegates to the bridge.

## Upstream bridge gaps addressed during Phase 2

`purego-cef2gtk` now exposes or implements:

- real size access and size observers;
- device scale factor access;
- `GetScreenInfo` based on bridge size/scale;
- text-selection render hook;
- middle-click input hook;
- focus and cursor helpers.

Remaining runtime risk: popup/select behavior should be smoke-tested after Dumber is fully migrated.
