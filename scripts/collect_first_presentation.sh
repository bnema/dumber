#!/usr/bin/env bash
# Collect exactly five isolated CEF/DMABUF cold starts. The application is
# deliberately terminated after the bounded observation window; a completed
# first-presentation summary, rather than process exit, is the success signal.
set -euo pipefail

readonly runs=5
readonly timeout_seconds="${DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS:-45}"
readonly runtime="${DUMBER_CEF_DIR:-$HOME/.local/share/cef-147-runtime}"
readonly binary="${DUMBER_FIRST_PRESENTATION_BIN:-$PWD/dist/dumber}"

fail_unsafe_output() {
  echo "first-presentation: unsafe output path: $1" >&2
  exit 2
}

if [[ -v DUMBER_FIRST_PRESENTATION_OUTPUT ]]; then
  output="$DUMBER_FIRST_PRESENTATION_OUTPUT"
else
  # Evidence is state, not source: keep the default outside the checkout and
  # make every invocation a fresh child of the external evidence directory.
  readonly state_home="${XDG_STATE_HOME:-$HOME/.local/state}"
  [[ "$state_home" == /* ]] || fail_unsafe_output "XDG_STATE_HOME must be absolute"
  readonly evidence_root="$state_home/dumber/roadmap-evidence"
  mkdir -p -- "$evidence_root" || fail_unsafe_output "could not create XDG state directory"
  canonical_evidence_root="$(realpath -e -- "$evidence_root")" || \
    fail_unsafe_output "XDG state directory cannot be canonicalized"
  [[ "$canonical_evidence_root" == "$evidence_root" && ! -L "$evidence_root" ]] || \
    fail_unsafe_output "XDG state directory must not contain symbolic links"
  output="$evidence_root/first-presentation-$(date -u +%Y%m%dT%H%M%S)-$$"
fi
readonly output
readonly upstream_module="github.com/bnema/purego-cef2gtk"

# Resolve the version selected by this checkout, then obtain its immutable VCS
# origin from Go's cached module metadata. Do not infer a revision from a tag,
# a branch, or a truncated pseudo-version suffix.
resolve_upstream_provenance() {
  local selected_metadata selected_version downloaded_metadata

  selected_metadata="$(go list -m -json "$upstream_module" 2>/dev/null)" || {
    echo "first-presentation: immutable module provenance is unavailable" >&2
    exit 2
  }
  selected_version="$(python3 - "$upstream_module" "$selected_metadata" <<'PY'
import json, sys
try:
    metadata = json.loads(sys.argv[2])
    if metadata.get("Path") != sys.argv[1] or not isinstance(metadata.get("Version"), str):
        raise ValueError
    print(metadata["Version"])
except (AttributeError, TypeError, ValueError, json.JSONDecodeError):
    raise SystemExit("first-presentation: immutable module provenance is unavailable")
PY
)" || exit $?
  downloaded_metadata="$(go mod download -json "$upstream_module@$selected_version" 2>/dev/null)" || {
    echo "first-presentation: immutable module provenance is unavailable" >&2
    exit 2
  }

  python3 - "$upstream_module" "$selected_metadata" "$downloaded_metadata" <<'PY'
import json, re, sys

module, selected_raw, downloaded_raw = sys.argv[1:]

def fail():
    # Do not expose Go cache locations or other machine-local values.
    raise SystemExit("first-presentation: immutable module provenance is unavailable")

try:
    selected = json.loads(selected_raw)
    downloaded = json.loads(downloaded_raw)
    version = selected["Version"]
    if selected.get("Path") != module or downloaded.get("Path") != module:
        fail()
    if downloaded.get("Version") != version:
        fail()
    # A selected pseudo-version is an exact immutable selector, never a branch.
    match = re.fullmatch(r"(v\d+\.\d+\.\d+)-0\.\d{14}-([0-9a-f]{12})", version)
    if not match:
        fail()
    info_path = downloaded.get("Info")
    if not isinstance(info_path, str) or not info_path:
        fail()
    with open(info_path, encoding="utf-8") as info_file:
        info = json.load(info_file)
    origin = info.get("Origin") or downloaded.get("Origin")
    if info.get("Version") != version or not isinstance(origin, dict):
        fail()
    revision = origin.get("Hash")
    if origin.get("VCS") != "git" or origin.get("URL") != "https://github.com/bnema/purego-cef2gtk":
        fail()
    if not isinstance(revision, str) or not re.fullmatch(r"[0-9a-f]{40}", revision):
        fail()
    if not revision.startswith(match.group(2)):
        fail()
    # A named ref can move. Only the immutable hash itself is acceptable.
    if origin.get("Ref") not in (None, "", revision):
        fail()
except (AttributeError, KeyError, OSError, TypeError, ValueError, json.JSONDecodeError):
    fail()

print("\t".join((version, match.group(1), revision)))
PY
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
upstream_provenance="$(resolve_upstream_provenance)" || exit $?
IFS=$'\t' read -r upstream_version upstream_tag upstream_revision <<<"$upstream_provenance"
readonly upstream_version upstream_tag upstream_revision

# Raw logs and temporary XDG homes may contain machine-local paths. Keep them
# outside the committed artifact directory and always remove them.
work_root="$(mktemp -d "${TMPDIR:-/tmp}/dumber-first-presentation.XXXXXX")"
readonly work_root
cleanup_work_root() {
  [[ -n "${work_root:-}" && -d "$work_root" && ! -L "$work_root" ]] || return
  rm -rf -- "$work_root"
}
trap cleanup_work_root EXIT
python3 - "$output/metadata.json" "$binary" "$timeout_seconds" "$upstream_module" "$upstream_version" "$upstream_tag" "$upstream_revision" <<'PY'
import hashlib, json, os, platform, subprocess, sys
path, binary, timeout, upstream_module, upstream_version, upstream_tag, upstream_revision = sys.argv[1:]
with open(binary, "rb") as candidate:
  binary_sha256 = hashlib.file_digest(candidate, "sha256").hexdigest()

# These deliberately coarse labels support comparison without exposing a host,
# device name, driver version, path, or environment dump.
architecture = {"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine().lower(), "other")
os_label = {"linux": "linux", "darwin": "darwin", "windows": "windows"}.get(platform.system().lower(), "other")
display_protocol = "wayland" if os.environ.get("WAYLAND_DISPLAY") else "x11"
gpu_profile = os.environ.get("DUMBER_MACHINE_GPU_PROFILE", "generic-gpu")
allowed_gpu_profiles = {"generic-gpu", "integrated-gpu", "discrete-gpu", "hybrid-gpu", "virtual-gpu", "unknown-gpu"}
if gpu_profile not in allowed_gpu_profiles:
    raise SystemExit("DUMBER_MACHINE_GPU_PROFILE must be a non-identifying profile label")

json.dump({
  "runs": 5,
  "runtime": {"label": "cef", "version": "147"},
  "binary": {"label": "dumber", "sha256": binary_sha256},
  "timeout_seconds": int(timeout),
  "measured_source_revision": subprocess.check_output(["git", "-c", f"safe.directory={os.getcwd()}", "rev-parse", "HEAD"], text=True).strip(),
  "upstream": {"module": upstream_module, "version": upstream_version, "tag": upstream_tag, "revision": upstream_revision},
  "comparison": {"os": os_label, "architecture": architecture, "display_protocol": display_protocol, "machine_gpu_profile": gpu_profile},
  "render_configuration": {"backend": "gdk-dmabuf", "buffer_sharing": "dmabuf", "renderer": "vulkan"}
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
