# CEF accelerated first-presentation startup trace

Dumber records this cold-start timeline **only for the accelerated CEF → DMABUF
→ GTK path**. It is not a backend-neutral startup metric: the WebKit backend
does not add milestones to this trace and never emits this CEF summary.

For a selected CEF GUI process, Dumber captures neutral `process_entry` and
`config_complete` timestamps, then activates and seeds this CEF-owned trace only
after configuration selects CEF. CEF helper processes, WebKit GUI startup, the
standalone omnibox, and CLI commands never initialize this trace, record its
milestones, or emit its summary. A selected CEF GUI trace accepts these one-shot
milestones only in order:

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

Build the candidate and its build manifest, then run:

```bash
DUMBER_CEF_DIR=/path/to/selected-cef-runtime \
DUMBER_FIRST_PRESENTATION_BIN="$PWD/dist/dumber" \
DUMBER_BUILD_MANIFEST="$PWD/dist/dumber.manifest.json" \
DUMBER_MACHINE_GPU_PROFILE=integrated-gpu \
  scripts/collect_first_presentation.sh
```

The manifest records the measured binary SHA-256, the 40-character source
revision, and the selected `github.com/bnema/purego-cef2gtk` version plus its
full 40-character revision. Generate it at build time; collection verifies the
manifest against `go version -m` output for the measured binary and fails
closed on missing or mismatched attribution.

The collector is for the accelerated CEF/DMABUF/GTK contract above. By default
each collection is a fresh directory below
`$XDG_STATE_HOME/dumber/roadmap-evidence` (or
`$HOME/.local/state/dumber/roadmap-evidence` when `XDG_STATE_HOME` is unset),
not in the repository. `DUMBER_FIRST_PRESENTATION_OUTPUT` may override it only
with a new absolute directory below an existing non-symlink parent. The
collector never clears or reuses a caller-supplied path.

The script requires the current display and an explicit `DUMBER_CEF_DIR`
selecting the measured runtime; there is no default path or version.
`scripts/cef_runtime_probe.py` runs in a separate bounded subprocess, opens
only the selected `libcef.so` with standard-library ctypes, calls the
header-verified `cef_version_info(int)` entries (0-7), and reports numeric
version fields plus the library hash, never its path. The candidate must still
pass its normal loader ABI/version validation; the probe never bypasses it.
Runtimes below Chrome major 150 fail collection. It performs
exactly five bounded launches with fresh XDG homes and CEF root cache, fixes the
DMABUF/Vulkan renderer, and writes exactly `metadata.json`, `run-01.json`
through `run-05.json`, and `baseline.json`. Metadata includes only coarse,
comparison-safe OS/architecture, display protocol, and machine/GPU profile
labels. Set `DUMBER_MACHINE_GPU_PROFILE` to one of `generic-gpu`,
`integrated-gpu`, `discrete-gpu`, `hybrid-gpu`, `virtual-gpu`, or `unknown-gpu`;
do not use a device model or other identifier. Publish only the seven reviewed
JSON files to an external Gist. Raw logs and temporary XDG homes are removed and
must not be published. `metadata.json` binds the measured binary SHA-256 to
its source revision via `go version -m` plus the build manifest, and records
the selected `github.com/bnema/purego-cef2gtk` version, tag, and full revision
from that same binary-bound evidence; it never derives provenance from the
collection checkout. The child processes receive exactly the selected runtime
via `CEF_DIR`, never a conflicting inherited override. Branch selectors,
replacement contamination, version/manifest mismatches, or a missing manifest
fail collection. A missing or incomplete timeline, non-DMABUF backend, or invalid
run also fails collection.
