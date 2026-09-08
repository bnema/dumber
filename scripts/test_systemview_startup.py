#!/usr/bin/env python3
"""Systemview startup fixture (Plan 05 P4.2).

Static gates always run headless: the shell must fetch the WASM exactly
once (clone-based fallback), carry the readiness contract markers, and
the committed manifest must match disk artifacts.

The live launch smoke starts the candidate binary with an isolated
profile, watches for panics/fatal crashes, then terminates it gracefully.
Without a display the live part skips cleanly; in-page branches
(streaming fallback, HTTP/MIME/compile failures, mount/bind failure, Go
panic exit, disabled JS) follow the printed display protocol, whose
markers are asserted statically here.

Exit codes: 0 static gates and launch smoke green (in-page branches stay
MANUAL), 1 a gate failed, 2 static gates green with live parts skipped.
"""

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
SHELL_PATH = REPO_ROOT / "assets" / "systemviews" / "index.html"
MANIFEST_PATH = REPO_ROOT / "assets" / "systemviews" / "asset-manifest.json"
DOM_ADAPTER_PATH = REPO_ROOT / "internal" / "ui" / "systemviews" / "dom_js.go"
GENERATOR_PKG = "./cmd/systemviews-assets"

EXIT_OK = 0
EXIT_FAIL = 1
EXIT_SKIP_LIVE = 2


def fail(message):
    print(f"systemview-startup: FAIL: {message}", flush=True)
    return EXIT_FAIL


def check_shell_single_fetch():
    """P4.1: one fetch per branch, clone-based fallback, no refetch."""
    try:
        shell = SHELL_PATH.read_text(encoding="utf-8")
    except OSError as exc:
        return fail(f"cannot read shell: {exc}")
    problems = []
    if "response.clone()" not in shell:
        problems.append("missing response.clone() fallback")
    if "fallback.arrayBuffer()" not in shell:
        problems.append("fallback does not consume the retained clone")
    if re.search(r"return\s+instantiateFromBytes\(\)", shell):
        problems.append("zero-arg instantiateFromBytes() refetches (old double-fetch)")
    if shell.count("fetch(wasmURL") != 2:
        problems.append("expected exactly two fetch(wasmURL...) branch sites")
    if "response.ok" not in shell:
        problems.append("missing response.ok fast-fail")
    if problems:
        return fail("shell fetch structure: " + "; ".join(problems))
    print("systemview-startup: shell fetches WASM once per branch, fallback reuses the clone")
    return EXIT_OK


def check_shell_readiness_contract():
    """P4.2: aria-busy entry, data markers, terminal guard, single go.run."""
    try:
        shell = SHELL_PATH.read_text(encoding="utf-8")
    except OSError as exc:
        return fail(f"cannot read shell: {exc}")
    required = [
        'aria-busy="true"',
        "removeAttribute(",
        "aria-busy",
        'data-failed',
        "terminalErrorReported",
    ]
    missing = [marker for marker in required if marker not in shell]
    if missing:
        return fail("shell readiness contract missing: " + ", ".join(missing))
    if shell.count("go.run(") != 1:
        return fail("runtime must execute exactly once (single go.run call site)")
    for marker in ("go.exit =", "goExitCode", "exit status"):
        if marker not in shell:
            return fail(f"shell exit hook missing: {marker}")
    try:
        adapter = DOM_ADAPTER_PATH.read_text(encoding="utf-8")
    except OSError as exc:
        return fail(f"cannot read DOM adapter: {exc}")
    if '"data-ready"' not in adapter:
        return fail("DOM adapter never marks data-ready")
    print("systemview-startup: shell carries the readiness contract markers")
    return EXIT_OK


def check_manifest_on_disk():
    """P3.1: committed manifest parses and matches generator schema."""
    try:
        raw = MANIFEST_PATH.read_bytes()
    except OSError as exc:
        return fail(f"cannot read manifest (run make build-systemviews): {exc}")
    try:
        manifest = json.loads(raw)
    except json.JSONDecodeError as exc:
        return fail(f"manifest is not JSON: {exc}")
    if manifest.get("version") != 1:
        return fail("manifest version != 1")
    files = manifest.get("files", {})
    if sorted(files) != ["systemviews.css", "systemviews.wasm", "wasm_exec.js"]:
        return fail(f"manifest pins {sorted(files)}, want the three servables")
    for name, entry in files.items():
        digest = entry.get("sha256", "")
        if not re.fullmatch(r"[0-9a-f]{64}", digest):
            return fail(f"manifest pin for {name} is not a sha256 hex digest")
        if not isinstance(entry.get("size"), int) or entry["size"] < 0:
            return fail(f"manifest pin for {name} has a bad size")
    print(f"systemview-startup: manifest pins {len(files)} assets with valid digests")
    return EXIT_OK


def check_manifest_generator():
    """P3.1: generator --check against disk artifacts (needs go toolchain)."""
    if shutil.which("go") is None:
        print("systemview-startup: SKIP generator check (no go toolchain)")
        return EXIT_SKIP_LIVE
    proc = subprocess.run(
        ["go", "run", GENERATOR_PKG, "-check", "-dir", "assets/systemviews"],
        cwd=REPO_ROOT,
        capture_output=True,
        text=True,
        timeout=300,
    )
    if proc.returncode != 0:
        sys.stderr.write(proc.stderr)
        return fail("generator --check rejected the working tree")
    print(f"systemview-startup: {proc.stdout.strip()}")
    return EXIT_OK


CRASH_PATTERNS = ["panic:", "fatal error:", "SIGTRAP", "runtime error:", "Go panic"]


def display_available():
    return bool(os.environ.get("DISPLAY") or os.environ.get("WAYLAND_DISPLAY"))


def sanitized_launch_env(profile):
    """Build the child's environment: owned XDG/HOME/CEF dirs, no dev-mode.

    The fixture must not read the operator's configuration (XDG_CONFIG_HOME)
    nor follow ENV=dev, which would redirect all profile directories under
    the working directory instead of the isolated profile.
    """
    env = dict(os.environ)
    env.pop("ENV", None)
    env["HOME"] = str(profile / "home")
    env["XDG_DATA_HOME"] = str(profile / "data")
    env["XDG_STATE_HOME"] = str(profile / "state")
    env["XDG_CACHE_HOME"] = str(profile / "cache")
    env["XDG_CONFIG_HOME"] = str(profile / "config")
    return env


def run_launch_smoke(binary, cef_dir, timeout):
    """Start the candidate with an isolated profile, watch, terminate.

    This is a startup smoke test only: it proves the binary launches and
    survives the window without crashes. It never navigates to a
    systemview, so fetch counts, readiness inspection, streaming fallback
    and failure injection stay MANUAL via the printed display protocol.
    """
    if not display_available():
        print("systemview-startup: SKIP live launch (no DISPLAY or WAYLAND_DISPLAY)")
        return EXIT_SKIP_LIVE
    profile = Path(tempfile.mkdtemp(prefix="sv-startup-profile-"))
    env = sanitized_launch_env(profile)
    env["DUMBER_CEF_ROOT_CACHE_PATH"] = str(profile / "cef-cache")
    env["DUMBER_CEF_DIR"] = str(cef_dir)
    print(f"systemview-startup: launching {binary} with isolated profile {profile}")
    try:
        proc = subprocess.Popen(
            [str(binary)],
            env=env,
            cwd=profile,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
        )
    except OSError as exc:
        return fail(f"cannot execute binary: {exc}")
    try:
        try:
            output, _ = proc.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            output = None
        if output is not None:
            # Any early exit is unexpected for the GUI binary: a crash
            # without markers is still a failure, never a pass.
            print(f"systemview-startup: binary exited during smoke with code {proc.returncode}")
            print((output or "")[-4000:])
            for pattern in CRASH_PATTERNS:
                if pattern in (output or ""):
                    return fail(f"crash marker in startup output: {pattern}")
            return fail(f"binary exited during the smoke window (code {proc.returncode}); expected it to stay alive")
        # Still alive past the window: the startup path survived; terminate
        # gracefully, then force-kill only our own child on refusal.
        proc.terminate()
        try:
            output, _ = proc.communicate(timeout=15)
        except subprocess.TimeoutExpired:
            proc.kill()
            output, _ = proc.communicate(timeout=15)
            print("systemview-startup: WARN binary ignored SIGTERM and was killed")
        for pattern in CRASH_PATTERNS:
            if pattern in (output or ""):
                print((output or "")[-4000:])
                return fail(f"crash marker in startup output: {pattern}")
        print("systemview-startup: launch smoke survived the window with no crash markers")
        return EXIT_OK
    finally:
        shutil.rmtree(profile, ignore_errors=True)


DISPLAY_PROTOCOL = """\
systemview-startup display protocol -- MANUAL STEPS (not executed by this script):
  1. Open dumb://history -> #app gains data-ready="true", aria-busy gone.
     Shell HTML references .wasm/.js/.css with ?v=<64-hex> (view-source).
  2. DevTools: exactly one systemviews.wasm request per load; reload ->
     .wasm served 200 with Cache-Control immutable; shell answers no-store.
  3. Streaming fallback: serve .wasm as octet-stream via a local proxy ->
     console shows the streaming-fallback warning, page still mounts ready.
  4. HTTP failure: point the shell at a missing .wasm -> terminal
     "Failed to load systemviews WebAssembly: Error: HTTP 404" rendered
     once in .sv-error, data-failed="true", aria-busy gone.
  5. Compile failure: corrupt the .wasm bytes -> same terminal error path.
  6. Disabled JS: noscript message visible, no console activity.
  7. Go panic path: break bridge init -> nonzero process exit, console
     carries the error, error renders exactly once.
"""


def main():
    parser = argparse.ArgumentParser(description="systemview startup fixture")
    parser.add_argument("--binary", required=True, help="candidate dumber binary")
    parser.add_argument("--cef-dir", required=True, help="compatible CEF runtime dir")
    parser.add_argument("--launch-timeout", type=int, default=20, help="smoke window in seconds")
    args = parser.parse_args()

    binary = Path(args.binary)
    if not binary.is_file() or not os.access(binary, os.X_OK):
        return fail(f"binary is not executable: {binary}")
    cef_dir = Path(args.cef_dir)
    if not cef_dir.is_dir():
        return fail(f"CEF runtime dir missing: {cef_dir}")

    results = [
        check_shell_single_fetch(),
        check_shell_readiness_contract(),
        check_manifest_on_disk(),
        check_manifest_generator(),
    ]
    if any(result == EXIT_FAIL for result in results):
        return EXIT_FAIL
    live = run_launch_smoke(binary, cef_dir, args.launch_timeout)
    print(DISPLAY_PROTOCOL)
    if live == EXIT_FAIL:
        return EXIT_FAIL
    if live == EXIT_SKIP_LIVE or any(result == EXIT_SKIP_LIVE for result in results):
        print("systemview-startup: static gates green, live parts skipped")
        print("systemview-startup: in-page branches were NOT executed here; they are MANUAL via the protocol above")
        return EXIT_SKIP_LIVE
    print("systemview-startup: static gates and launch smoke green")
    print("systemview-startup: in-page branches were NOT executed here; they are MANUAL via the protocol above")
    return EXIT_OK


if __name__ == "__main__":
    sys.exit(main())
