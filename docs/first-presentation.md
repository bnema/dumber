# CEF accelerated first-presentation startup trace

Dumber records this cold-start timeline **only for the accelerated CEF → DMABUF
→ GTK path**. It is not a backend-neutral startup metric: the WebKit backend
does not add milestones to this trace and never emits this CEF summary.

For a CEF GUI process, the trace begins at `process_entry`, before CEF
subprocess handling, and accepts these one-shot milestones only in order:

1. `process_entry`
2. `config_complete`
3. `cef_library_load_begin` (immediately before `InitWithApp`)
4. `cef_initialized`
5. `browser_create_requested`
6. `first_accelerated_paint_received`
7. `first_dmabuf_texture_swap` (after `GtkPicture.SetPaintable` succeeds)
8. `first_gtk_presentation` (the subsequent GTK frame-clock after-paint)

The CEF-to-GTK bridge owns the native accelerated-paint, DMABUF texture-swap,
and GTK presentation boundaries. Dumber records the bridge's ordered callbacks;
it does not synthesize them for another engine or rendering path. CEF
library-load completion is intentionally not recorded: `InitWithApp` is an
opaque operation. Duplicate, unknown, and out-of-order transitions are
rejected. At the last milestone Dumber emits exactly one normal-level JSON
summary (`startup_trace: first presentation`) with the selected CEF render
backend, an `incomplete_reason`, total milliseconds, and monotonic milestones.

A non-DMABUF CEF backend or a missing accelerated callback does not produce a
complete summary and is not comparable with this measurement. In particular,
WebKit startup must be measured with a separately defined WebKit-specific
contract rather than this CEF trace.

## Reproducible collection

Build the candidate, then run:

```bash
DUMBER_FIRST_PRESENTATION_BIN="$PWD/dist/dumber" \
DUMBER_MACHINE_GPU_PROFILE=integrated-gpu \
  scripts/collect_first_presentation.sh
```

The collector is for the accelerated CEF/DMABUF/GTK contract above. By default
each collection is a fresh directory below
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
must not be published. `metadata.json` derives the selected
`github.com/bnema/purego-cef2gtk` pseudo-version and its full immutable Git
origin hash from Go module metadata; branch selectors or missing origin metadata
fail collection. A missing or incomplete timeline, non-DMABUF backend, or invalid
run also fails collection.
