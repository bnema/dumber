#!/usr/bin/env bash
# Collect exactly five isolated CEF/DMABUF cold starts. The application is
# deliberately terminated after the bounded observation window; a completed
# first-presentation summary, rather than process exit, is the success signal.
set -euo pipefail

readonly runs=5
readonly timeout_seconds="${DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS:-45}"
readonly binary="${DUMBER_FIRST_PRESENTATION_BIN:-$PWD/dist/dumber}"
readonly manifest="${DUMBER_BUILD_MANIFEST:-${binary}.manifest.json}"

fail_unsafe_output() {
  echo "first-presentation: unsafe output path: $1" >&2
  exit 2
}

# The measured runtime is always explicit. Never default to a hardcoded path
# or version: collection fails closed when the caller does not select one.
if [[ -z "${DUMBER_CEF_DIR:-}" ]]; then
  echo "first-presentation: DUMBER_CEF_DIR must be set to the selected CEF runtime directory" >&2
  exit 2
fi
readonly runtime="${DUMBER_CEF_DIR}"
# Never inherit a conflicting CEF_DIR override: the child environment below is
# built with env -i and receives exactly this selected directory.
readonly selected_cef_dir="${DUMBER_CEF_DIR}"

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
readonly script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly runtime_probe="$script_dir/cef_runtime_probe.py"

# Resolve provenance from the measured binary, not the collection checkout.
# Reads embedded build info via `go version -m` and requires a build manifest
# tied to the candidate SHA-256 when VCS data is absent (including
# buildvcs=false builds). Never queries the checkout module graph.
resolve_binary_provenance() {
  python3 - "$binary" "$manifest" "$upstream_module" <<'PY'
import hashlib, json, os, re, subprocess, sys
binary, manifest_path, module = sys.argv[1:]

def fail():
    # Do not expose cache locations or other machine-local values.
    raise SystemExit("first-presentation: immutable module provenance is unavailable")

with open(binary, "rb") as candidate:
    # Chunked streaming hash: hashlib.file_digest needs 3.11+, this stays
    # compatible with older 3.x interpreters.
    digest = hashlib.sha256()
    for chunk in iter(lambda: candidate.read(65536), b""):
        digest.update(chunk)
    binary_sha256 = digest.hexdigest()

try:
    buildinfo = subprocess.check_output(["go", "version", "-m", binary], text=True, stderr=subprocess.DEVNULL)
except (OSError, subprocess.CalledProcessError):
    raise SystemExit("first-presentation: immutable module provenance is unavailable")
if "=>" in buildinfo:
    raise SystemExit("first-presentation: immutable module provenance is unavailable")

mod_version = None
dep_version = None
vcs_revision = None
for line in buildinfo.splitlines():
    parts = line.split()
    if len(parts) < 2:
        continue
    if parts[0] == "mod" and len(parts) >= 3 and parts[1] == "github.com/bnema/dumber":
        mod_version = parts[2]
    if parts[0] == "dep" and len(parts) >= 3 and parts[1] == module:
        dep_version = parts[2]
    if parts[0] == "build" and len(parts) >= 2 and parts[1].startswith("vcs.revision="):
        vcs_revision = parts[1].split("=", 1)[1]
if dep_version is None:
    fail()
pseudo_match = re.fullmatch(r"(v\d+\.\d+\.\d+)-0\.\d{14}-([0-9a-f]{12})", dep_version)
tag_match = re.fullmatch(r"v\d+\.\d+\.\d+", dep_version)
if not pseudo_match and not tag_match:
    fail()
if vcs_revision is not None and not re.fullmatch(r"[0-9a-f]{40}", vcs_revision):
    fail()

try:
    with open(manifest_path, encoding="utf-8") as manifest_file:
        manifest = json.load(manifest_file)
except (OSError, ValueError):
    manifest = None

if manifest is None:
    # Without embedded VCS data the binary cannot be attributed; fail closed.
    if vcs_revision is None:
        raise SystemExit("first-presentation: build manifest is required for binaries without embedded VCS data")
    # Embedded VCS revision attributes the binary itself, but the full
    # upstream revision is not in build info; the manifest must still bind it.
    raise SystemExit("first-presentation: build manifest is required to bind dependency revisions")

if not isinstance(manifest, dict):
    raise SystemExit("first-presentation: build manifest is required for binaries without embedded VCS data")
if manifest.get("binary_sha256") != binary_sha256:
    raise SystemExit("first-presentation: build manifest does not match the measured binary")
source_revision = manifest.get("source_revision")
if not isinstance(source_revision, str) or not re.fullmatch(r"[0-9a-f]{40}", source_revision):
    raise SystemExit("first-presentation: build manifest does not match the measured binary")
if vcs_revision is not None and vcs_revision != source_revision:
    raise SystemExit("first-presentation: build manifest does not match the measured binary")
modules = manifest.get("modules")
if not isinstance(modules, dict) or module not in modules:
    raise SystemExit("first-presentation: build manifest does not match the measured binary")
entry = modules[module]
if not isinstance(entry, dict) or entry.get("version") != dep_version:
    raise SystemExit("first-presentation: build manifest does not match the measured binary")
revision = entry.get("revision")
if not isinstance(revision, str) or not re.fullmatch(r"[0-9a-f]{40}", revision):
    raise SystemExit("first-presentation: build manifest does not match the measured binary")
if pseudo_match:
    if not revision.startswith(pseudo_match.group(2)):
        raise SystemExit("first-presentation: build manifest does not match the measured binary")
    tag = pseudo_match.group(1)
else:
    tag = dep_version

print("\t".join((binary_sha256, source_revision, dep_version, tag, revision)))
PY
}

# Probe the explicitly selected runtime in a separate bounded subprocess.
# Reports numeric version fields plus library hash, never a path. The
# candidate must still pass its normal loader ABI/version validation.
probe_runtime() {
  timeout --signal=TERM --kill-after=5s 30s python3 "$runtime_probe" --cef-dir "$selected_cef_dir" 2>/dev/null || {
    echo "first-presentation: CEF runtime probe failed" >&2
    exit 2
  }
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

[[ -x "$binary" ]] || { echo "first-presentation: executable not found" >&2; exit 2; }
[[ -d "$runtime" ]] || { echo "first-presentation: CEF runtime not found" >&2; exit 2; }
[[ -n "${WAYLAND_DISPLAY:-}${DISPLAY:-}" ]] || { echo "first-presentation: a current Wayland/X11 display is required" >&2; exit 2; }
binary_provenance="$(resolve_binary_provenance)" || exit $?
IFS=$'\t' read -r binary_sha256 measured_source_revision upstream_version upstream_tag upstream_revision <<<"$binary_provenance"
readonly binary_sha256 measured_source_revision upstream_version upstream_tag upstream_revision
runtime_probe_json="$(probe_runtime)" || exit $?
readonly runtime_probe_json
# Enforce the loader minimum (CHROME_VERSION_MAJOR >= 150) at collection time.
# This records the observed runtime; it never bypasses the candidate ABI check.
python3 - "$runtime_probe_json" <<'PY'
import json, sys
probe = json.loads(sys.argv[1])
if not isinstance(probe.get("chrome_version_major"), int) or probe["chrome_version_major"] < 150:
    raise SystemExit("first-presentation: unsupported CEF runtime")
PY

# Raw logs and temporary XDG homes may contain machine-local paths. Keep them
# outside the committed artifact directory and always remove them.
work_root="$(mktemp -d "${TMPDIR:-/tmp}/dumber-first-presentation.XXXXXX")"
readonly work_root
cleanup_work_root() {
  [[ -n "${work_root:-}" && -d "$work_root" && ! -L "$work_root" ]] || return
  rm -rf -- "$work_root"
}
trap cleanup_work_root EXIT
python3 - "$output/metadata.json" "$binary_sha256" "$timeout_seconds" "$upstream_module" "$upstream_version" "$upstream_tag" "$upstream_revision" "$measured_source_revision" "$runtime_probe_json" <<'PY'
import json, os, platform, sys
path, binary_sha256, timeout, upstream_module, upstream_version, upstream_tag, upstream_revision, measured_source_revision, probe_raw = sys.argv[1:]
probe = json.loads(probe_raw)

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
  "runtime": {"label": "cef", "chrome_major": probe["chrome_version_major"], "cef_version_major": probe["cef_version_major"], "libcef_sha256": probe["libcef_sha256"]},
  "binary": {"label": "dumber", "sha256": binary_sha256},
  "timeout_seconds": int(timeout),
  "measured_source_revision": measured_source_revision,
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
cef_dir = "$selected_cef_dir"
EOF
  # Every directory below is newly created for this one launch. Do not inherit
  # a profile, shader cache, CEF root cache, or mutable app configuration.
  # CEF_DIR is set to exactly the selected runtime, never a conflicting
  # inherited override.
  set +e
  env -i \
    HOME="$HOME" PATH="$PATH" LANG="${LANG:-C.UTF-8}" \
    WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-}" DISPLAY="${DISPLAY:-}" XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-}" \
    DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-}" XAUTHORITY="${XAUTHORITY:-}" \
    XDG_CONFIG_HOME="$root/config" XDG_DATA_HOME="$root/data" XDG_STATE_HOME="$root/state" XDG_CACHE_HOME="$root/cache" \
    CEF_DIR="$selected_cef_dir" DUMBER_CEF_DIR="$selected_cef_dir" \
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
