# Render lab

A development-only driver that starts one Dumber binary on a local scrolling
fixture page so rendering behaviour can be compared by hand or by counters. It
is not part of the application: no application code imports anything here, and
the launcher never installs, replaces, or restarts the user's Dumber.

The lab answers two questions:

- Does a candidate build show the same content as the immutable baseline?
- Where does an accelerated frame spend its time before it reaches the screen?

Only the second question has machine-checkable counters, and only for a build
that carries the `gdk_pipeline` instrumentation (see *Metrics*). Everything
else is human judgement.

**The candidate is a measurement build.** Unless its manifest says
`ownership-verified-candidate`, it contains no rendering, ownership or pacing
change: comparing it with the baseline compares two functionally identical
pipelines while collecting telemetry. A candidate built from the same source as
the baseline differs only in the bridge revision carrying instrumentation, which
is inert while profiling is off. Every run prints this explicitly under
`what changed:`.

Comparisons that do change behaviour:

- `--fps monitor` versus `--fps 60` (adaptive 120 Hz versus a fixed 60 Hz).
- `--render-path current` versus `--render-path dmabuf-copy`: the borrowed
  DMA-BUF handed to GSK, versus copied into an owned texture presented through
  GtkGLArea (ANGLE Vulkan with GDK DMA-BUF and GSK Vulkan, versus ANGLE GL/EGL
  with GSK OpenGL).
- `--external-begin-frame` on versus off.

## Building the two variants

Both variants must be built from their own checkout. The baseline is always a
pristine tree built with `GOWORK=off`; the candidate links a local bridge
worktree through a generated, ignored `go.work` unless a published bridge
revision is pinned.

```bash
python3 scripts/render_lab_build.py \
  --baseline-repo   "$DUMBER_BASE" \
  --candidate-repo  "$DUMBER_WORK" \
  --bridge-repo     "$BRIDGE_WORK"
```

Options:

- `--only baseline|candidate|both` — rebuild one variant for iteration.
- `--published-bridge-sha SHA` — final handoff mode: no workspace substitution,
  the candidate must resolve the pinned bridge module without a `replace`.
- `--candidate-status instrumentation-only|ownership-verified-candidate` —
  records what the candidate actually proves. `ownership-verified-candidate`
  requires an approved, evidence-backed ownership change; never set it because a
  build succeeded.

Artifacts land under `<candidate-repo>/dist/render-lab/`:

```
baseline/dumber        immutable baseline binary
baseline/manifest.json render-lab-v1 provenance record
candidate/dumber       candidate binary
candidate/manifest.json
*/build.log            captured build output (may contain local paths)
*/source-tracked-*.patch  uncommitted diff of each repo at build time
go.work                generated workspace (iteration mode only)
runs/                  one directory per launcher run
```

The manifest binds the binary SHA-256, both repository HEADs, every changed
source path with its content digest, the resolved bridge module identity, the
toolchain, and the fixture digest. It uses sanitized labels, not absolute
paths. A baseline binary is never overwritten by a different build; delete it
deliberately if the baseline must move.

## Running an instance

```bash
python3 scripts/render_lab.py --variant candidate --fps monitor
python3 scripts/render_lab.py --variant baseline  --fps monitor
python3 scripts/render_lab.py --variant candidate --fps 60
python3 scripts/render_lab.py --variant candidate --duration 30 --profile
python3 scripts/render_lab.py --variant candidate --dry-run
```

| Option | Values | Default |
| --- | --- | --- |
| `--variant` | `baseline`, `candidate` | `candidate` |
| `--fps` | `monitor`, `60`, `120`, `165` | `monitor` |
| `--render-path` | `current`, `dmabuf-copy` | `current` |
| `--stack` | `vulkan`, `egl` | derived from `--render-path` |
| `--external-begin-frame` | flag | off |
| `--profile` | flag | off |
| `--trace-geometry` | flag | off |
| `--duration SECONDS` | positive number | unbounded |
| `--dry-run` | flag | off |
| `--output-root PATH` | directory | `dist/render-lab/runs` |

There is no shell or CEF-flag passthrough in this version.

## Two DMA-BUF consumption paths

The accelerated frame CEF hands over is a borrowed DMA-BUF. There are two ways
the bridge consumes it today, and `--render-path` selects between them. This is
the comparison a human can judge by feel.

| Path | What it does | Stack it selects |
| --- | --- | --- |
| `current` (default) | Hands the borrowed DMA-BUF to GSK, which wraps it: ANGLE Vulkan, GDK DMA-BUF texture, GSK Vulkan. | `vulkan` |
| `dmabuf-copy` | Copies the DMA-BUF into a texture the bridge owns and presents it through GtkGLArea: ANGLE GL/EGL, GSK OpenGL. | `egl` |

```bash
python3 scripts/render_lab.py --render-path current     --fps monitor
python3 scripts/render_lab.py --render-path dmabuf-copy --fps monitor
```

`--stack vulkan|egl` still exists as the low-level selector. Passing both is
allowed only when they agree; a contradiction is refused instead of silently
picking one.

What the copy path is and is not:

- It never keeps the borrowed descriptor as the presented object, so the frame
  the presenter shows is one the bridge allocated.
- It does **not** wait for the producer to finish writing, because no completion
  signal is available through the CEF API. It is a different consumption
  strategy, not a verified ownership fix, and it is not certified safe.
- It copies once per frame on the GTK thread, so expect it to cost more CPU per
  frame, not less, and it exercises a different GSK renderer (OpenGL instead of
  Vulkan).

Keep the frame-rate mode identical between the two runs, and use the same pane
size. Profiling is orthogonal: add `--profile` to either side when you want the
counters.

`--trace-geometry` is a named diagnostic, not a passthrough: it makes the bridge
print the OSR geometry contract it computes, so a sizing or scale problem can be
read from the log instead of guessed at. It prints one line per observation like:

```
cef2gtk-osr-geometry callback=view-rect backend=glarea widget_logical=1022x1726 \
  osr_rect=1278x2158 device_scale=1.250 backing_scale_enabled=true \
  expected_device=1278x2158
```

`osr_rect` must equal `expected_device`; if it does not, the frame CEF renders is
not the size of the widget's framebuffer, and the presenter will leave part of
the widget empty. Both paths print the same numbers on a given output.

`--fps monitor` keeps CEF's OSR rate adapted to the active monitor refresh
rate, capped at 240, with 60 as the fallback when the refresh rate is unknown.
A numeric `--fps` disables adaptation and requests that rate. Both variants
receive the same generated configuration; only the frame-rate mode differs.

The fixture page has a sticky control panel: Start/Stop auto-scroll, Reset, and
a 600/1200 CSS px/s selector. Auto-scroll is driven by `requestAnimationFrame`
elapsed time, stops at the bottom or when the page is hidden, and stalls as
soon as the user scrolls, presses a key, or touches the page. It never starts
by itself when the desktop requests reduced motion. The panel's "page rAF"
number is a page-level interval, **not** a GTK presentation time.

## Isolation

Each launch creates `dist/render-lab/runs/dumber-render-lab-XXXX/` with mode
0700 and runs the browser with that directory as its working directory. Dumber's
development environment then resolves HOME and the XDG homes under
`<run>/.dev/dumber/`. The launcher also sets them explicitly, points
`DUMBER_CEF_ROOT_CACHE_PATH` inside the run root, and writes a fixed
`config.toml` before launch.

Preserved from the host: `XDG_RUNTIME_DIR`, `WAYLAND_DISPLAY`, the session bus,
and audio connectivity. Dumber derives its development IPC socket from the run
root, so two launches cannot share a launch socket and neither can reach an
installed session.

Sanitized before launch: every inherited `DUMBER_*` and `PUREGO_CEF2GTK_*`
variable, `GSK_RENDERER`, `GTK_DEBUG`, `GDK_DEBUG`, and generic proxy variables.
`CEF_DIR` is preserved so both variants load the same libcef, and only a
version string and a path hash are recorded. Environment values are never
printed.

Stopping is scoped: the browser runs in its own process group. `--duration`,
Ctrl-C, or closing the window terminates that group with `SIGTERM`, waits up to
10 s, and only then escalates to `SIGKILL`. The launcher never matches process
names and never signals a browser it did not start. Measurements from a forced
exit are labelled as such.

## Fixture server

The launcher binds `127.0.0.1` on an ephemeral port and serves exactly one path,
`/scroll.html`, from `scripts/render_lab/scroll.html`. Other paths get 404 with
no directory listing and no path echo. The page is self-contained: inline CSS
and JavaScript, a system font, 400 generated cards, and no external requests.
Both variants load the same bytes, and the fixture SHA-256 is recorded in the
run metadata.

## Metrics

`--profile` enables the bridge profiler and writes
`<run>/cef2gtk_profile.jsonl` (one JSON object per webview per profiling
window; the bridge writes at most one window per second). The launcher prints a
bounded summary and stores it in `run.json`.

A window is emitted while accelerated frames are arriving: the writer is driven
by frame activity, so a page that is hidden, occluded, or completely static
produces no window (or none after the initial load). Scroll the fixture, or use
its auto-scroll, when collecting numbers. The launcher reports this case
explicitly instead of showing zeroes.

The summary reports `gdk_pipeline` counters — received, replaced, imported,
swapped, paint cycles, swaps overwritten before a paint cycle, and
feedback-available/unavailable counts — plus four duration series:
`received_to_import_ms`, `queue_wait_ms`, `import_elapsed_ms`, and
`received_to_swap_ms`.

The summary also prints the present-path and input counters that every build
reports, including an unmodified baseline: frames received/queued/rendered,
import failures, GTK scroll events, external BeginFrames, the accumulated scroll
distance, and the samples the importer and GTK wait recorded. Divide those by
the window count to get frames and input events per second, which is what tells
wheel input apart from library-driven motion and from page raster work.

What these numbers are and are not:

- They are wall-clock elapsed durations measured on one Go monotonic clock, not
  CPU execution times. Older `*_cpu` names in the same snapshot are elapsed
  measurements too.
- Per-window quantiles are `sampled`: nearest-rank p50/p95/p99 over a bounded
  512-sample ring, reported with the retained and total sample counts and an
  `available` flag that is false when a window retained nothing. The launcher's
  cross-window summary reports the median of per-window quantiles; it does not
  pretend to pool samples it no longer has.
- Only frames that complete a picture swap contribute a duration sample. A frame
  superseded while still pending is counted in `pending_replaced` and samples
  nothing; a failed import is counted by the existing `import_failures` counter
  and samples nothing.
- `queue_wait_ms` belongs to the surviving frame after a replacement, not to the
  age of the scheduler source that may predate it.
- `frames_rendered` remains the GLArea backend's counter and is not GDK
  presentation FPS.
- A paint cycle is a GTK frame-clock association, not proof that the window was
  scanned out; `swaps_overwritten_before_paint` counts opportunities, not
  measured dropped frames.
- A build without the instrumentation reports `unavailable`. Missing feedback is
  never reported as zero.

Profiling stays genuinely off unless requested: with `--profile` absent, no
rings, callbacks, or file writes are created, and the launcher prints
`unavailable`.

## Reading a run

```
<run>/run.json               sanitized metadata and the metric summary
<run>/summary.txt            short human-readable recap
<run>/dumber.log             raw browser output (may contain paths and URLs)
<run>/cef2gtk_profile.jsonl  raw profile records, when profiling is enabled
<run>/config_status.txt      output of a pre-launch config read
```

Exit codes: `0` for a completed run, `1` for a failure worth reading about,
`2` for a missing prerequisite such as a verified binary or a display.

## Human comparison procedure

Same output, same pane size, same fixture, one variable at a time:

1. `baseline --fps monitor`, then `candidate --fps monitor`.
2. `baseline --fps 60`, then `candidate --fps 60`.
3. Optionally `candidate --fps 120 --external-begin-frame` and
   `--stack egl` as whole-stack comparisons.

Exercise the physical wheel, the touchpad, keyboard scrolling and fixture
auto-scroll separately; auto-scroll does not measure input feel. Wait ~10 s
after the page loads before judging a 30 s interval, and repeat each condition.
Use profile-off runs for feel and profile-on runs for diagnosis. Watch for the
first frame, resize, moving between outputs with different scale factors,
hide/show, repeated close, popup controls and focus/cursor behaviour.

Metrics from a build that lacks `gdk_pipeline` are `unavailable`, not zero. A
human visual verdict is not a release approval.
