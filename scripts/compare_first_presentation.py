#!/usr/bin/env python3
"""Strictly compare paired five-run Wayland/Vulkan presentation evidence."""
import json
import math
import sys
from pathlib import Path

RUNS = 5
UPSTREAM_SHA = "2a5b796c8befa686b663ecfba4fb00dcd870d539"
ALLOWED_TOP_LEVEL = {"metadata.json", "baseline.json", *(f"run-{i:02d}.json" for i in range(1, RUNS + 1))}


def fail(message):
    raise SystemExit(f"wayland-vulkan comparison: {message}")


def load(path):
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as err:
        fail(f"invalid JSON {path.name}: {err}")
    if not isinstance(value, dict):
        fail(f"{path.name} must contain an object")
    return value


def totals(root):
    names = {entry.name for entry in root.iterdir()}
    if names != ALLOWED_TOP_LEVEL:
        fail(f"{root} contains non-allowlisted artifacts")
    metadata = load(root / "metadata.json")
    upstream = metadata.get("upstream")
    if not isinstance(upstream, dict) or upstream.get("revision") != UPSTREAM_SHA:
        fail(f"{root}: unpinned or missing purego-cef2gtk provenance")
    values = []
    for index in range(1, RUNS + 1):
        run = load(root / f"run-{index:02d}.json")
        summary = run.get("summary")
        if run.get("run") != f"run-{index:02d}" or run.get("valid") is not True or not isinstance(summary, dict):
            fail(f"{root}: invalid run-{index:02d}")
        total = summary.get("total_ms")
        if type(total) is not int or total < 0:
            fail(f"{root}: invalid run-{index:02d} total")
        values.append(total)
    aggregate = load(root / "baseline.json")
    reported = aggregate.get("total_ms")
    ordered = sorted(values)
    expected = {
        "min": ordered[0],
        "median": ordered[len(ordered) // 2],
        "max": ordered[-1],
        "p95": ordered[math.ceil(0.95 * RUNS) - 1],
    }
    if aggregate.get("runs") != RUNS or reported != expected:
        fail(f"{root}: aggregate is not five-run median/nearest-rank-p95 evidence")
    return metadata, expected


def main():
    if len(sys.argv) != 3:
        fail("usage: compare_first_presentation.py BASELINE_DIR CANDIDATE_DIR")
    baseline_root, candidate_root = (Path(arg).resolve() for arg in sys.argv[1:])
    if baseline_root == candidate_root:
        fail("baseline and candidate evidence must be distinct")
    baseline_metadata, baseline = totals(baseline_root)
    candidate_metadata, candidate = totals(candidate_root)
    for key in ("runtime", "comparison", "render_configuration"):
        if baseline_metadata.get(key) != candidate_metadata.get(key):
            fail(f"unpaired provenance: {key} differs")
    for metric in ("median", "p95"):
        limit = baseline[metric] * 1.10
        if candidate[metric] > limit:
            fail(f"candidate {metric} regression exceeds 10% ({candidate[metric]} > {limit:g})")
    print(json.dumps({
        "runs": RUNS,
        "status": "passed",
        "baseline": {"median_ms": baseline["median"], "p95_ms": baseline["p95"]},
        "candidate": {"median_ms": candidate["median"], "p95_ms": candidate["p95"]},
    }, sort_keys=True))


if __name__ == "__main__":
    main()
