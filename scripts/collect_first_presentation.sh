#!/usr/bin/env bash
# Collect exactly five isolated CEF/DMABUF cold starts. The application is
# deliberately terminated after the bounded observation window; a completed
# first-presentation summary, rather than process exit, is the success signal.
set -euo pipefail

readonly runs=5
readonly timeout_seconds="${DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS:-45}"
readonly runtime="${DUMBER_CEF_DIR:-$HOME/.local/share/cef-147-runtime}"
readonly binary="${DUMBER_FIRST_PRESENTATION_BIN:-$PWD/dist/dumber}"
if [[ -v DUMBER_FIRST_PRESENTATION_OUTPUT ]]; then
  output="$DUMBER_FIRST_PRESENTATION_OUTPUT"
else
  output="$PWD/phase1/first-presentation"
fi
readonly output
readonly upstream_module="github.com/bnema/purego-cef2gtk"
readonly upstream_tag="v0.8.4"
readonly upstream_revision="f217ece342dea3ef2a3f98671fcd16a39ad0037d"

fail_unsafe_output() {
  echo "first-presentation: unsafe output path: $1" >&2
  exit 2
}

# The artifact destination is caller-controlled. Never empty or recursively
# clear it: collection only writes to a newly-created directory whose parent is
# already canonical and contains no symbolic-link hop.
prepare_output() {
  local parent name canonical_parent canonical_output

  [[ -n "$output" ]] || fail_unsafe_output "path must not be empty"
  [[ "$output" == /* ]] || fail_unsafe_output "path must be absolute"
  [[ "$output" != "/" ]] || fail_unsafe_output "path must not be /"
  [[ "${output%/}" != "${HOME%/}" ]] || fail_unsafe_output "path must not be HOME"
  [[ "$output" != .. && "$output" != ../* && "$output" != */.. && "$output" != */../* ]] || \
    fail_unsafe_output "path must not contain parent traversal"
  [[ ! -e "$output" && ! -L "$output" ]] || fail_unsafe_output "path must be a fresh directory"

  parent="$(dirname -- "$output")"
  name="$(basename -- "$output")"
  [[ -n "$name" && "$name" != "." && "$name" != ".." ]] || fail_unsafe_output "invalid directory name"
  [[ -d "$parent" && ! -L "$parent" ]] || fail_unsafe_output "parent must be an existing directory"
  canonical_parent="$(realpath -e -- "$parent")" || fail_unsafe_output "parent cannot be canonicalized"
  [[ "$parent" == "$canonical_parent" ]] || fail_unsafe_output "parent must not contain symbolic links"

  mkdir -- "$output" || fail_unsafe_output "could not create fresh directory"
  canonical_output="$(realpath -e -- "$output")" || fail_unsafe_output "created directory cannot be canonicalized"
  [[ -d "$output" && ! -L "$output" && "$canonical_output" == "$canonical_parent/$name" ]] || \
    fail_unsafe_output "created directory changed unexpectedly"
}

prepare_output

[[ -x "$binary" ]] || { echo "first-presentation: executable not found: $binary" >&2; exit 2; }
[[ -d "$runtime" ]] || { echo "first-presentation: CEF runtime not found: $runtime" >&2; exit 2; }
[[ -n "${WAYLAND_DISPLAY:-}${DISPLAY:-}" ]] || { echo "first-presentation: a current Wayland/X11 display is required" >&2; exit 2; }
[[ "$(go list -m -f '{{.Version}}' "$upstream_module")" == "$upstream_tag" ]] || {
  echo "first-presentation: expected $upstream_module@$upstream_tag" >&2
  exit 2
}

# Raw logs and temporary XDG homes may contain machine-local paths. Keep them
# outside the committed artifact directory and always remove them.
readonly work_root="$(mktemp -d "${TMPDIR:-/tmp}/dumber-first-presentation.XXXXXX")"
cleanup_work_root() {
  [[ -n "${work_root:-}" && -d "$work_root" && ! -L "$work_root" ]] || return
  rm -rf -- "$work_root"
}
trap cleanup_work_root EXIT
python3 - "$output/metadata.json" "$binary" "$timeout_seconds" "$upstream_module" "$upstream_tag" "$upstream_revision" <<'PY'
import hashlib, json, os, subprocess, sys
path, binary, timeout, upstream_module, upstream_tag, upstream_revision = sys.argv[1:]
with open(binary, "rb") as candidate:
  binary_sha256 = hashlib.file_digest(candidate, "sha256").hexdigest()
json.dump({
  "runs": 5,
  "runtime": {"label": "cef", "version": os.environ.get("DUMBER_CEF_RUNTIME_VERSION", "147")},
  "binary": {"label": "dumber", "sha256": binary_sha256},
  "timeout_seconds": int(timeout),
  "measured_source_revision": subprocess.check_output(["git", "-c", f"safe.directory={os.getcwd()}", "rev-parse", "HEAD"], text=True).strip(),
  "upstream": {"module": upstream_module, "tag": upstream_tag, "revision": upstream_revision},
  "fixed_environment": {"DUMBER_RENDER_STACK": "vulkan-dmabuf", "PUREGO_CEF2GTK_BACKEND": "gdk-dmabuf", "GSK_RENDERER": "vulkan"}
}, open(path, "w"), indent=2, sort_keys=True)
PY

for number in $(seq 1 "$runs"); do
  run="$(printf 'run-%02d' "$number")"
  root="$work_root/$run"
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
    DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-}" XAUTHORITY="${XAUTHORITY:-}" \
    XDG_CONFIG_HOME="$root/config" XDG_DATA_HOME="$root/data" XDG_STATE_HOME="$root/state" XDG_CACHE_HOME="$root/cache" \
    DUMBER_CEF_ROOT_CACHE_PATH="$root/cef-root-cache" DUMBER_RENDER_STACK="vulkan-dmabuf" \
    PUREGO_CEF2GTK_BACKEND="gdk-dmabuf" PUREGO_CEF2GTK_ANGLE_BACKEND="vulkan" GSK_RENDERER="vulkan" \
    timeout --signal=TERM --kill-after=5s "${timeout_seconds}s" "$binary" browse about:blank >"$root/process.log" 2>&1
  set -e
  # A completed, valid first-presentation summary is the observation success
  # signal. The GUI may later be stopped by timeout or terminate independently.
  python3 - "$root/process.log" "$output/$run.json" "$run" <<'PY'
import json, sys
log, destination, run = sys.argv[1:]
order = ["process_entry", "config_complete", "cef_library_load_begin", "cef_initialized", "browser_create_requested", "first_accelerated_paint_received", "first_dmabuf_texture_swap", "first_gtk_presentation"]
records, summary = [], None
for line in open(log, errors="replace"):
    try: event = json.loads(line)
    except json.JSONDecodeError: continue
    if event.get("message") == "startup_trace: milestone":
        records.append({key: event.get(key) for key in ("milestone", "t_ms", "delta_ms")})
    if event.get("message") == "startup_trace: first presentation":
        summary = {key: event.get(key) for key in ("backend", "incomplete_reason", "total_ms")}
names = [event.get("milestone") for event in records]
times = [event.get("t_ms") for event in records]
deltas = [event.get("delta_ms") for event in records]
valid = names == order and all(type(t) is int for t in times) and times == sorted(times)
valid = valid and all(type(delta) is int for delta in deltas)
valid = valid and all(delta == current - previous for previous, current, delta in zip([0] + times, times, deltas))
valid = valid and summary is not None and summary.get("backend") == "gdk-dmabuf" and not summary.get("incomplete_reason")
valid = valid and type(summary.get("total_ms")) is int and summary["total_ms"] == times[-1]
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
