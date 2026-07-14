#!/usr/bin/env bash
# Collect exactly five isolated CEF/DMABUF cold starts. The application is
# deliberately terminated after the bounded observation window; a completed
# first-presentation summary, rather than process exit, is the success signal.
set -euo pipefail

readonly runs=5
readonly timeout_seconds="${DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS:-45}"
readonly runtime="${DUMBER_CEF_DIR:-$HOME/.local/share/cef-147-runtime}"
readonly binary="${DUMBER_FIRST_PRESENTATION_BIN:-$PWD/dist/dumber}"
readonly output="${DUMBER_FIRST_PRESENTATION_OUTPUT:-$PWD/phase1/first-presentation}"

[[ -x "$binary" ]] || { echo "first-presentation: executable not found: $binary" >&2; exit 2; }
[[ -d "$runtime" ]] || { echo "first-presentation: CEF runtime not found: $runtime" >&2; exit 2; }
[[ -n "${WAYLAND_DISPLAY:-}${DISPLAY:-}" ]] || { echo "first-presentation: a current Wayland/X11 display is required" >&2; exit 2; }

rm -rf "$output"
mkdir -p "$output"
python3 - "$output/metadata.json" "$runtime" "$binary" "$timeout_seconds" <<'PY'
import json, os, subprocess, sys
path, runtime, binary, timeout = sys.argv[1:]
json.dump({
  "runs": 5,
  "runtime": runtime,
  "binary": binary,
  "timeout_seconds": int(timeout),
  "git_revision": subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip(),
  "environment": {key: os.environ.get(key, "") for key in ("WAYLAND_DISPLAY", "DISPLAY", "XDG_CURRENT_DESKTOP")},
  "fixed_environment": {"DUMBER_RENDER_STACK": "vulkan-dmabuf", "PUREGO_CEF2GTK_BACKEND": "gdk-dmabuf", "GSK_RENDERER": "vulkan"}
}, open(path, "w"), indent=2, sort_keys=True)
PY

for number in $(seq 1 "$runs"); do
  run="$(printf 'run-%02d' "$number")"
  root="$output/$run"
  mkdir -p "$root"/{config,data,state,cache}
  mkdir -p "$root/config/dumber"
  cat >"$root/config/dumber/config.toml" <<EOF
[logging]
level = "debug"
format = "json"
enable_file_log = false

[engine.cef]
cef_dir = "$runtime"
EOF
  # Every directory below is newly created for this one launch. Do not inherit
  # a profile, shader cache, CEF root cache, or mutable app configuration.
  set +e
  env -i \
    HOME="$HOME" PATH="$PATH" LANG="${LANG:-C.UTF-8}" \
    WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-}" DISPLAY="${DISPLAY:-}" XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-}" \
    XDG_CONFIG_HOME="$root/config" XDG_DATA_HOME="$root/data" XDG_STATE_HOME="$root/state" XDG_CACHE_HOME="$root/cache" \
    DUMBER_CEF_ROOT_CACHE_PATH="$root/cef-root-cache" DUMBER_RENDER_STACK="vulkan-dmabuf" \
    PUREGO_CEF2GTK_BACKEND="gdk-dmabuf" PUREGO_CEF2GTK_ANGLE_BACKEND="vulkan" GSK_RENDERER="vulkan" \
    timeout --signal=TERM --kill-after=5s "${timeout_seconds}s" "$binary" browse about:blank >"$root/process.log" 2>&1
  exit_code=$?
  set -e
  # timeout is expected for a GUI kept alive after the first presentation.
  if [[ "$exit_code" != 0 && "$exit_code" != 124 ]]; then
    echo "$run: launch exited unexpectedly ($exit_code); see $root/process.log" >&2
    exit 1
  fi
  python3 - "$root/process.log" "$output/$run.json" "$run" <<'PY'
import json, sys
log, destination, run = sys.argv[1:]
order = ["process_entry", "config_complete", "cef_library_load_begin", "cef_initialized", "browser_create_requested", "first_accelerated_paint_received", "first_dmabuf_texture_swap", "first_gtk_presentation"]
records, summary = [], None
for line in open(log, errors="replace"):
    try: event = json.loads(line)
    except json.JSONDecodeError: continue
    if event.get("message") == "startup_trace: milestone": records.append(event)
    if event.get("message") == "startup_trace: first presentation": summary = event
names = [event.get("milestone") for event in records]
times = [event.get("t_ms") for event in records]
valid = names == order and all(isinstance(t, int) for t in times) and times == sorted(times)
valid = valid and summary is not None and summary.get("backend") == "gdk-dmabuf" and not summary.get("incomplete_reason")
result = {"run": run, "valid": valid, "milestones": records, "summary": summary}
json.dump(result, open(destination, "w"), indent=2)
if not valid:
    raise SystemExit(f"{run}: invalid or incomplete non-DMABUF timeline")
PY
done

python3 - "$output" <<'PY'
import json, math, statistics, sys
root = sys.argv[1]
runs = [json.load(open(f"{root}/run-{n:02d}.json")) for n in range(1, 6)]
if len(runs) != 5 or not all(run["valid"] for run in runs): raise SystemExit("expected exactly five valid runs")
totals = sorted(run["summary"]["total_ms"] for run in runs)
p95 = totals[max(0, math.ceil(.95 * len(totals)) - 1)]
json.dump({"runs": 5, "total_ms": {"min": totals[0], "median": statistics.median(totals), "max": totals[-1], "p95": p95}}, open(f"{root}/baseline.json", "w"), indent=2)
PY

echo "first-presentation artifacts: $output"
