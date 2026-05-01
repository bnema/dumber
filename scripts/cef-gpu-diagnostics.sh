#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/cef-gpu-diagnostics.sh [options]

Capture a short, repeatable Dumber/CEF GPU-video diagnostic bundle while a
YouTube video is playing.

Options:
  -p, --pid PID              Dumber browser/root PID. Defaults to oldest/root dumber process.
  -o, --out DIR              Output directory. Default: .dev/dumber/gpu-diagnostics/<timestamp>
  -n, --samples N            amdgpu_top JSON samples. Default: 10
  -i, --interval-ms MS       amdgpu_top sample interval. Default: 1000
  --session-log PATH         Copy a Dumber session log into the bundle.
  --codec-stats TEXT         Record manually copied YouTube "Stats for nerds" text.
  --codec-stats-file PATH    Record YouTube "Stats for nerds" text from a file.
  --devtools-port PORT       Save http://127.0.0.1:PORT/json/version and /json tabs if enabled.
  -h, --help                 Show this help.

Recommended run shape:
  1. Start Dumber with CEF/VAAPI/profiling enabled.
  2. Open YouTube Stats for nerds and copy the Codecs line (or full panel text).
  3. Run this script while playback is active.

The bundle includes process/thread CPU, process command lines, /proc DRM fdinfo,
amdgpu_top JSON capture, kernel DRM node metadata, optional DevTools tab metadata,
and optional YouTube codec stats text.
EOF
}

pid=""
out_dir=""
samples=10
interval_ms=1000
session_log=""
codec_stats=""
codec_stats_file=""
devtools_port=""

require_value() {
  local opt="$1"
  local value="${2:-}"
  if [[ -z "$value" || "$value" == -* ]]; then
    echo "$opt requires a value" >&2
    usage >&2
    exit 2
  fi
}

validate_devtools_port() {
  local value="$1"
  if [[ ! "$value" =~ ^[1-9][0-9]*$ ]] || (( value < 1 || value > 65535 )); then
    echo "--devtools-port must be an integer between 1 and 65535" >&2
    exit 2
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -p|--pid) require_value "$1" "${2:-}"; pid="$2"; shift 2 ;;
    -o|--out) require_value "$1" "${2:-}"; out_dir="$2"; shift 2 ;;
    -n|--samples) require_value "$1" "${2:-}"; samples="$2"; shift 2 ;;
    -i|--interval-ms) require_value "$1" "${2:-}"; interval_ms="$2"; shift 2 ;;
    --session-log) require_value "$1" "${2:-}"; session_log="$2"; shift 2 ;;
    --codec-stats) require_value "$1" "${2:-}"; codec_stats="$2"; shift 2 ;;
    --codec-stats-file) require_value "$1" "${2:-}"; codec_stats_file="$2"; shift 2 ;;
    --devtools-port) require_value "$1" "${2:-}"; validate_devtools_port "$2"; devtools_port="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "$pid" ]]; then
  pid="$(pgrep -o -x dumber || true)"
fi
if [[ -n "$pid" && ! "$pid" =~ ^[1-9][0-9]*$ ]]; then
  echo "--pid must be a positive integer" >&2
  exit 2
fi
if [[ -z "$pid" || ! -d "/proc/$pid" ]]; then
  echo "could not find a live dumber PID; pass --pid" >&2
  exit 1
fi
if ! [[ "$samples" =~ ^[1-9][0-9]*$ && "$interval_ms" =~ ^[1-9][0-9]*$ ]]; then
  echo "--samples and --interval-ms must be positive integers" >&2
  exit 2
fi

if [[ -z "$out_dir" ]]; then
  out_dir=".dev/dumber/gpu-diagnostics/$(date +%Y%m%d_%H%M%S)_pid${pid}"
fi
mkdir -p "$out_dir"

log() { printf '[cef-gpu-diagnostics] %s\n' "$*" >&2; }

collect_children() {
  local root="$1"
  local child
  printf '%s\n' "$root"
  for child in $(pgrep -P "$root" || true); do
    collect_children "$child"
  done
}

mapfile -t pids < <(collect_children "$pid" | awk '!seen[$0]++')
printf '%s\n' "${pids[@]}" > "$out_dir/pids.txt"

{
  echo "timestamp=$(date --iso-8601=seconds)"
  echo "root_pid=$pid"
  echo "samples=$samples"
  echo "interval_ms=$interval_ms"
  echo "uname=$(uname -a)"
  echo "wayland_display=${WAYLAND_DISPLAY:-}"
  echo "display=${DISPLAY:-}"
  command -v amdgpu_top >/dev/null && amdgpu_top --version 2>/dev/null || true
} > "$out_dir/metadata.txt"

log "writing process command lines and thread CPU"
: > "$out_dir/cmdlines.txt"
: > "$out_dir/thread_cpu.txt"
for p in "${pids[@]}"; do
  [[ -d "/proc/$p" ]] || continue
  {
    echo "--- pid=$p comm=$(cat "/proc/$p/comm" 2>/dev/null || true)"
    tr '\0' ' ' < "/proc/$p/cmdline" || true
    echo
  } >> "$out_dir/cmdlines.txt"
  ps -L -p "$p" -o pid,tid,ppid,pcpu,pmem,stat,comm,args --sort=-pcpu >> "$out_dir/thread_cpu.txt" 2>/dev/null || true
done

log "capturing DRM fdinfo"
mkdir -p "$out_dir/fdinfo"
for p in "${pids[@]}"; do
  [[ -d "/proc/$p/fdinfo" ]] || continue
  out="$out_dir/fdinfo/pid_${p}.txt"
  : > "$out"
  for fd in "/proc/${p}/fdinfo"/*; do
    [[ -r "$fd" ]] || continue
    if grep -Eq 'drm-|amdgpu|engine|vram|gtt|cycles' "$fd" 2>/dev/null; then
      echo "--- $fd" >> "$out"
      cat "$fd" >> "$out" 2>/dev/null || true
    fi
  done
  [[ -s "$out" ]] || rm -f "$out"
done

log "capturing DRM device metadata"
mkdir -p "$out_dir/dri"
for card in /sys/class/drm/card*; do
  [[ -e "$card" && ! "$card" =~ - ]] || continue
  name="$(basename "$card")"
  {
    echo "--- $card"
    readlink -f "$card/device" || true
    for f in vendor device revision subsystem_vendor subsystem_device; do
      [[ -r "$card/device/$f" ]] && printf '%s=' "$f" && cat "$card/device/$f"
    done
  } > "$out_dir/dri/${name}.txt"
done

if command -v amdgpu_top >/dev/null; then
  log "capturing amdgpu_top JSON (${samples} samples @ ${interval_ms}ms)"
  amdgpu_top -J -s "$interval_ms" -n "$samples" -p -gm > "$out_dir/amdgpu_top.json" 2> "$out_dir/amdgpu_top.stderr" || true
  if command -v jq >/dev/null; then
    jq '{period, devices: [.devices[]? | {name: .Info.DeviceName, render: .Info.DevicePath.render, activity: .gpu_activity, total_fdinfo: .["Total fdinfo"], processes: (.fdinfo // {})}]}' \
      "$out_dir/amdgpu_top.json" > "$out_dir/amdgpu_top.summary.json" 2>/dev/null || true
  fi
else
  log "amdgpu_top not found; skipping GPU samples"
fi

if [[ -n "$session_log" ]]; then
  if [[ ! -r "$session_log" ]]; then
    log "--session-log is not readable: $session_log"
    exit 2
  fi
  cp -- "$session_log" "$out_dir/$(basename -- "$session_log")"
fi
if [[ -n "$codec_stats_file" ]]; then
  if [[ ! -r "$codec_stats_file" ]]; then
    log "--codec-stats-file is not readable: $codec_stats_file"
    exit 2
  fi
  cp -- "$codec_stats_file" "$out_dir/youtube_stats_for_nerds.txt"
elif [[ -n "$codec_stats" ]]; then
  printf '%s\n' "$codec_stats" > "$out_dir/youtube_stats_for_nerds.txt"
else
  cat > "$out_dir/youtube_stats_for_nerds.README.txt" <<'EOF'
No YouTube codec stats were supplied. Re-run with --codec-stats or
--codec-stats-file after copying the YouTube "Stats for nerds" panel,
especially the Codecs line (e.g. vp09/av01/h264) and viewport/resolution.
EOF
fi

if [[ -n "$devtools_port" ]]; then
  validate_devtools_port "$devtools_port"
  log "capturing DevTools metadata from port $devtools_port"
  if ! curl --connect-timeout 1 --max-time 3 -fsS "http://127.0.0.1:${devtools_port}/json/version" > "$out_dir/devtools_version.json" 2> "$out_dir/devtools.stderr"; then
    log "failed to fetch DevTools version metadata from port $devtools_port"
    exit 1
  fi
  if ! curl --connect-timeout 1 --max-time 3 -fsS "http://127.0.0.1:${devtools_port}/json" > "$out_dir/devtools_tabs.json" 2>> "$out_dir/devtools.stderr"; then
    log "failed to fetch DevTools tab metadata from port $devtools_port"
    exit 1
  fi
fi

log "bundle complete: $out_dir"
printf '%s\n' "$out_dir"
