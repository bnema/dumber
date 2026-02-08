# Session Exit Classification Runbook

This runbook classifies browser sessions using existing P0 marker files in the log directory:

- `session_<id>.startup.marker`
- `session_<id>.shutdown.marker`
- `session_<id>.abrupt.marker`

Marker directory (default):

```bash
LOG_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/dumber/logs"
```

Crash report directory (auto-generated on startup when abrupt exits are detected):

```bash
CRASH_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/dumber/logs/crashes"
```

## Classification Model

- `clean_exit` (`marker-confirmed`)
  - `shutdown.marker` is present.
- `main_process_crash_or_abrupt` (`marker-confirmed`)
  - `abrupt.marker` is present and no `shutdown.marker` exists.
- `external_kill_or_oom_inferred` (`best-effort`)
  - `startup.marker` exists, but `shutdown.marker` and `abrupt.marker` are both missing.
  - This is explicitly inferred, not confirmed.

Implementation path in code:

- `internal/bootstrap/session_exit_classification.go`
  - `ClassifySessionExitFromMarkers(lockDir, sessionID)`
  - `BuildSessionExitReport(lockDir)`

## Quick Marker Report (Shell)

```bash
LOG_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/dumber/logs"

for startup in "$LOG_DIR"/session_*.startup.marker; do
  [ -e "$startup" ] || continue
  base="$(basename "$startup")"
  sid="${base#session_}"
  sid="${sid%.startup.marker}"
  shutdown="$LOG_DIR/session_${sid}.shutdown.marker"
  abrupt="$LOG_DIR/session_${sid}.abrupt.marker"

  if [ -f "$shutdown" ]; then
    class="clean_exit"
    inf="marker-confirmed"
  elif [ -f "$abrupt" ]; then
    class="main_process_crash_or_abrupt"
    inf="marker-confirmed"
  else
    class="external_kill_or_oom_inferred"
    inf="best-effort"
  fi

  printf "%s\t%s\t%s\n" "$sid" "$class" "$inf"
done | sort
```

## Optional System-Log Correlation

Use this only when `external_kill_or_oom_inferred` needs more evidence.

1. Get the startup timestamp for a session:

```bash
SID="<session-id>"
LOG_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/dumber/logs"
cat "$LOG_DIR/session_${SID}.startup.marker"
```

2. Inspect user journal around that window:

```bash
journalctl --user --since "2026-02-07 10:00:00" --until "2026-02-07 10:10:00" | rg -i "dumber|killed|oom|sigkill|signal"
```

3. Inspect kernel/system OOM evidence:

```bash
journalctl -k --since "2026-02-07 10:00:00" --until "2026-02-07 10:10:00" | rg -i "oom|out of memory|killed process"
```

4. Optional coredump check:

```bash
coredumpctl list | rg -i "dumber"
```

5. Print GitHub-ready issue payload from generated report:

```bash
dumber crashes issue latest
```

Interpretation guidance:

- OOM/killed lines near the startup window strengthen `external_kill_or_oom_inferred`.
- No OOM evidence does not prove a clean exit; keep inference label explicit.
