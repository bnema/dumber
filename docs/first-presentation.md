# First presentation startup trace

Dumber records one cold-start timeline for each GUI process. The trace begins
at `process_entry`, before CEF subprocess handling, and accepts these one-shot
milestones only in order:

1. `process_entry`
2. `config_complete`
3. `cef_library_load_begin` (immediately before `InitWithApp`)
4. `cef_initialized`
5. `browser_create_requested`
6. `first_accelerated_paint_received`
7. `first_dmabuf_texture_swap` (after `GtkPicture.SetPaintable` succeeds)
8. `first_gtk_presentation` (the subsequent GTK frame-clock after-paint)

CEF library-load completion is intentionally not recorded: `InitWithApp` is an
opaque operation. Duplicate, unknown, and out-of-order transitions are
rejected. At the last milestone Dumber emits exactly one normal-level JSON
summary (`startup_trace: first presentation`) with the selected `backend`, an
`incomplete_reason`, total milliseconds, and monotonic milestones.

## Reproducible collection

Build the candidate, then run:

```bash
DUMBER_FIRST_PRESENTATION_BIN="$PWD/dist/dumber" \
DUMBER_MACHINE_GPU_PROFILE=integrated-gpu \
  scripts/collect_first_presentation.sh
```

By default each collection is a fresh directory below
`$XDG_STATE_HOME/dumber/roadmap-evidence` (or
`$HOME/.local/state/dumber/roadmap-evidence` when `XDG_STATE_HOME` is unset),
not in the repository. `DUMBER_FIRST_PRESENTATION_OUTPUT` may override it only
with a new absolute directory below an existing non-symlink parent. The
collector never clears or reuses a caller-supplied path.

The script requires the current display and the CEF 147 runtime at
`~/.local/share/cef-147-runtime` (override with `DUMBER_CEF_DIR`). It performs
exactly five bounded launches with fresh XDG homes and CEF root cache, fixes the
DMABUF/Vulkan renderer, and writes exactly `metadata.json`, `run-01.json`
through `run-05.json`, and `baseline.json`. Metadata includes only coarse,
comparison-safe OS/architecture, display protocol, and machine/GPU profile
labels. Set `DUMBER_MACHINE_GPU_PROFILE` to one of `generic-gpu`,
`integrated-gpu`, `discrete-gpu`, `hybrid-gpu`, `virtual-gpu`, or `unknown-gpu`;
do not use a device model or other identifier. Publish only the seven reviewed
JSON files to an external Gist. Raw logs and temporary XDG homes are removed and
must not be published. A missing or incomplete timeline, non-DMABUF backend, or
invalid run fails collection.
