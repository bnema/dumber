#!/usr/bin/env bash
# Strict, five-by-five Wayland/Vulkan evidence gate. A missing prerequisite,
# incomplete lifecycle, timeout, or CEF GPU error is a failure, never a skip.
set -euo pipefail

readonly runs=5
readonly timeout_seconds="${DUMBER_WAYLAND_VULKAN_TIMEOUT_SECONDS:-45}"
readonly runtime="${DUMBER_CEF_DIR:-$HOME/.local/share/cef-147-runtime}"
readonly baseline_bin="${DUMBER_WAYLAND_VULKAN_BASELINE_BIN:-}"
readonly candidate_bin="${DUMBER_WAYLAND_VULKAN_CANDIDATE_BIN:-}"
readonly baseline_source="${DUMBER_WAYLAND_VULKAN_BASELINE_SOURCE:-$PWD}"
readonly candidate_source="${DUMBER_WAYLAND_VULKAN_CANDIDATE_SOURCE:-$PWD}"
readonly upstream_module="github.com/bnema/purego-cef2gtk"
readonly upstream_version="v0.8.4-0.20260714143951-2a5b796c8bef"
readonly upstream_revision="2a5b796c8befa686b663ecfba4fb00dcd870d539"

fail() { echo "wayland-vulkan gate: $*" >&2; exit 2; }

repo_root="$(git -c safe.directory="$PWD" rev-parse --show-toplevel)" || fail "must run in a checkout"
repo_root="$(realpath -e -- "$repo_root")"
[[ -n "${WAYLAND_DISPLAY:-}" && -n "${XDG_RUNTIME_DIR:-}" ]] || fail "a live Wayland display is required"
[[ -d "$runtime" ]] || fail "CEF runtime not found: $runtime"
[[ -x "$baseline_bin" && -x "$candidate_bin" ]] || fail "both executable baseline and candidate binaries are required"
[[ "$(go list -mod=vendor -m -f '{{.Version}}' "$upstream_module")" == "$upstream_version" ]] || fail "purego-cef2gtk is not pinned to PR #28"

# Evidence must be freshly made outside the checkout. GitHub Actions uses its
# runner temp; local invocations use XDG state. The raw process logs stay under
# work_root and are removed before the script returns.
if [[ -v DUMBER_WAYLAND_VULKAN_OUTPUT ]]; then
  output="$DUMBER_WAYLAND_VULKAN_OUTPUT"
else
  if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
    root="${RUNNER_TEMP:-}"
  else
    root="${XDG_STATE_HOME:-$HOME/.local/state}"
  fi
  [[ "$root" == /* ]] || fail "external XDG/Actions temporary root must be absolute"
  mkdir -p -- "$root" || fail "cannot create external evidence root"
  root="$(realpath -e -- "$root")"
  output="$root/dumber-wayland-vulkan-gate-$(date -u +%Y%m%dT%H%M%S)-$$"
fi
[[ "$output" == /* && "$output" != / && "$output" != "$repo_root" && "$output" != "$repo_root"/* ]] || fail "output must be external to the checkout"
[[ ! -e "$output" && ! -L "$output" ]] || fail "output must be a fresh directory"
parent="$(dirname -- "$output")"
[[ -d "$parent" && ! -L "$parent" && "$(realpath -e -- "$parent")" == "$parent" ]] || fail "output parent must be an existing canonical directory"
mkdir -- "$output" || fail "could not create output"
readonly output

work_root="$(mktemp -d "${TMPDIR:-/tmp}/dumber-wayland-vulkan.XXXXXX")"
cleanup() { [[ -d "${work_root:-}" && ! -L "$work_root" ]] && rm -rf -- "$work_root"; }
trap cleanup EXIT

write_metadata() {
  local destination="$1" source_dir="$2" binary="$3"
  python3 - "$destination" "$source_dir" "$binary" "$timeout_seconds" "$upstream_module" "$upstream_version" "$upstream_revision" <<'PY'
import hashlib, json, os, platform, subprocess, sys
path, source, binary, timeout, module, version, revision = sys.argv[1:]
profile = os.environ.get("DUMBER_MACHINE_GPU_PROFILE", "unknown-gpu")
if profile not in {"intel-integrated", "amd", "unknown-gpu"}:
    raise SystemExit("invalid non-identifying machine GPU profile")
architecture = {"x86_64":"amd64", "amd64":"amd64", "aarch64":"arm64", "arm64":"arm64"}.get(platform.machine().lower(), "other")
with open(binary, "rb") as f: digest = hashlib.file_digest(f, "sha256").hexdigest()
revision_source = subprocess.check_output(["git", "-c", f"safe.directory={source}", "-C", source, "rev-parse", "HEAD"], text=True).strip()
json.dump({
 "runs": 5,
 "runtime": {"label":"cef", "version":"147"},
 "binary": {"label":"dumber", "sha256":digest},
 "timeout_seconds": int(timeout),
 "measured_source_revision": revision_source,
 "upstream": {"module":module, "version":version, "revision":revision},
 "comparison": {"os":"linux", "architecture":architecture, "display_protocol":"wayland", "machine_gpu_profile":profile},
 "render_configuration": {"backend":"gdk-dmabuf", "buffer_sharing":"dmabuf", "renderer":"vulkan"}
}, open(path, "w"), sort_keys=True, indent=2)
PY
}

parse_run() {
  local log="$1" destination="$2" name="$3"
  python3 - "$log" "$destination" "$name" <<'PY'
import json, re, sys
log, destination, name = sys.argv[1:]
order = ["process_entry", "config_complete", "cef_library_load_begin", "cef_initialized", "browser_create_requested", "first_accelerated_paint_received", "first_dmabuf_texture_swap", "first_gtk_presentation"]
records, summary = [], None
flags = {"dmabuf_import":False, "vulkan":False, "resize":False, "before_close":False, "engine_close":False, "gpu_1002":False}
for raw in open(log, encoding="utf-8", errors="replace"):
    # GPU process error 1002 is an authoritative blocked outcome even if a
    # renderer subsequently emits presentation-looking output.
    if re.search(r"(?:gpu|GPU).{0,120}(?:error[^0-9]{0,40})?1002|1002.{0,120}(?:gpu|GPU)", raw): flags["gpu_1002"] = True
    try: event = json.loads(raw)
    except json.JSONDecodeError: continue
    message = event.get("message", "")
    if message == "startup_trace: milestone": records.append({k:event.get(k) for k in ("milestone", "t_ms", "delta_ms")})
    elif message == "startup_trace: first presentation": summary = {k:event.get(k) for k in ("backend", "incomplete_reason", "total_ms")}
    # These fields are emitted by the PR #28 diagnostic path; fields instead
    # of raw log copying keeps artifacts allowlisted and machine-safe.
    elif message == "wayland_vulkan_gate: dmabuf import": flags["dmabuf_import"] = event.get("backend") == "gdk-dmabuf"
    elif message == "wayland_vulkan_gate: vulkan renderer": flags["vulkan"] = event.get("renderer") == "vulkan"
    elif message == "wayland_vulkan_gate: resize": flags["resize"] = True
    elif message == "wayland_vulkan_gate: OnBeforeClose": flags["before_close"] = True
    elif message == "wayland_vulkan_gate: Engine.Close": flags["engine_close"] = True
names, times, deltas = [r["milestone"] for r in records], [r["t_ms"] for r in records], [r["delta_ms"] for r in records]
valid = names == order and all(type(v) is int and v >= 0 for v in times + deltas) and times == sorted(times)
valid = valid and all(delta == current - previous for previous, current, delta in zip([0] + times, times, deltas))
valid = valid and summary is not None and summary.get("backend") == "gdk-dmabuf" and not summary.get("incomplete_reason") and type(summary.get("total_ms")) is int and summary["total_ms"] == times[-1]
valid = valid and not flags["gpu_1002"] and all(value for key, value in flags.items() if key != "gpu_1002")
json.dump({"run":name, "valid":valid, "milestones":records, "summary":summary, "checks":flags}, open(destination, "w"), sort_keys=True, indent=2)
if flags["gpu_1002"]: raise SystemExit(f"{name}: CEF GPU error 1002 (blocked)")
if not valid: raise SystemExit(f"{name}: incomplete Wayland Vulkan lifecycle or presentation")
PY
}

collect() {
  local label="$1" binary="$2" source_dir="$3" evidence="$output/$1"
  mkdir -- "$evidence"
  write_metadata "$evidence/metadata.json" "$source_dir" "$binary"
  for number in $(seq 1 "$runs"); do
    local name root status
    name="$(printf 'run-%02d' "$number")"; root="$work_root/$label-$name"
    mkdir -p "$root"/{config,data,state,cache,cef-root-cache}
    set +e
    env -i HOME="$HOME" PATH="$PATH" LANG="${LANG:-C.UTF-8}" WAYLAND_DISPLAY="$WAYLAND_DISPLAY" XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR" \
      XDG_CONFIG_HOME="$root/config" XDG_DATA_HOME="$root/data" XDG_STATE_HOME="$root/state" XDG_CACHE_HOME="$root/cache" DUMBER_CEF_ROOT_CACHE_PATH="$root/cef-root-cache" \
      DUMBER_RENDER_STACK=vulkan-dmabuf PUREGO_CEF2GTK_BACKEND=gdk-dmabuf PUREGO_CEF2GTK_ANGLE_BACKEND=vulkan GSK_RENDERER=vulkan \
      timeout --signal=TERM --kill-after=5s "${timeout_seconds}s" "$binary" browse about:blank >"$root/process.log" 2>&1
    status=$?
    set -e
    if [[ $status -eq 124 || $status -eq 137 ]]; then fail "$label/$name: timed out"; fi
    parse_run "$root/process.log" "$evidence/$name.json" "$name"
  done
  python3 - "$evidence" <<'PY'
import json, math, statistics, sys
root=sys.argv[1]; totals=[]
for n in range(1,6):
 r=json.load(open(f"{root}/run-{n:02d}.json"));
 if r.get("valid") is not True: raise SystemExit("invalid run")
 totals.append(r["summary"]["total_ms"])
totals.sort(); json.dump({"runs":5,"total_ms":{"min":totals[0],"median":statistics.median(totals),"max":totals[-1],"p95":totals[math.ceil(.95*5)-1]}},open(f"{root}/baseline.json","w"),sort_keys=True,indent=2)
PY
}

collect baseline "$baseline_bin" "$baseline_source"
collect candidate "$candidate_bin" "$candidate_source"
python3 "$(dirname -- "$0")/compare_first_presentation.py" "$output/baseline" "$output/candidate" >"$output/result.json"
printf 'wayland-vulkan gate artifacts: %s\n' "$output"
