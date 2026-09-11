#!/usr/bin/env python3
"""Launch an isolated Dumber render-lab instance on a local scrolling fixture.

This is a development test driver, not an application component. It starts one
Dumber binary (baseline or candidate) with a private HOME/XDG/CEF-cache root,
serves one local fixture page over loopback, and records sanitized metadata for
the run. It never touches an installed Dumber profile or an existing browser
session, and it never kills processes it does not own.

    python3 scripts/render_lab.py --variant candidate --fps monitor
    python3 scripts/render_lab.py --variant baseline --duration 30 --profile
    python3 scripts/render_lab.py --variant candidate --dry-run

Exit codes: 0 success, 1 failure, 2 environment blocker (no display, missing
verified artifact).
"""

from __future__ import annotations

import argparse
import dataclasses
import hashlib
import http.server
import json
import os
import re
import signal
import subprocess
import sys
import tempfile
import threading
import time
import tomllib
import urllib.parse
from pathlib import Path

SCHEMA = "render-lab-v1"
RUN_SCHEMA = "render-lab-run-v1"
VARIANTS = ("baseline", "candidate")
FPS_CHOICES = ("monitor", "60", "120", "165")
STACKS = ("vulkan", "egl")
# Two ways to consume the accelerated frame the bridge receives. This is the
# comparison a human can judge; it selects the whole render stack, so it is not
# a pacing knob.
RENDER_PATHS = {
    "current": (
        "vulkan",
        "ANGLE Vulkan, GDK-wrapped DMA-BUF handed to GSK, GSK Vulkan",
    ),
    "dmabuf-copy": (
        "egl",
        "ANGLE GL/EGL, DMA-BUF copied into an owned texture, GtkGLArea, GSK OpenGL",
    ),
}
DEFAULT_RENDER_PATH = "current"
DEFAULT_IMPORT_PRIORITY = "default"
DEFAULT_RETIRED_TEXTURES = 2
RETIRED_TEXTURES_MAX = 16
FIXTURE_PATH = "scripts/render_lab/scroll.html"

DEFAULT_OUTPUT_ROOT = Path("dist/render-lab/runs")
MANIFEST_ROOT = Path("dist/render-lab")

# Inherited environment families that would otherwise change the measured
# environment between variants or leak the developer's configuration.
DROP_PREFIXES = ("DUMBER_", "PUREGO_CEF2GTK_")
DROP_KEYS = ("GSK_RENDERER", "GTK_DEBUG", "GDK_DEBUG", "GTK_RENDERER", "GSK_DEBUG")
PROXY_KEYS = (
    "http_proxy",
    "https_proxy",
    "all_proxy",
    "ftp_proxy",
    "HTTP_PROXY",
    "HTTPS_PROXY",
    "ALL_PROXY",
    "FTP_PROXY",
)
# Runtime selection for the CEF installation. Preserved across sanitization so
# the same libcef is used for every variant.
CEF_RUNTIME_KEYS = ("CEF_DIR",)

GRACEFUL_EXIT_SECONDS = 10.0
CONFIG_STATUS_TIMEOUT_SECONDS = 15.0

MONITOR_FALLBACK_FPS = 60
MONITOR_MAX_FPS = 240


class LabError(Exception):
    """Failure that maps to exit code 1."""


class EnvironmentBlocker(LabError):
    """Missing prerequisite that maps to exit code 2."""


# --------------------------------------------------------------------------
# Manifests and provenance
# --------------------------------------------------------------------------


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def sha256_text(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def load_manifest(repo_root: Path, variant: str) -> dict:
    manifest_path = repo_root / MANIFEST_ROOT / variant / "manifest.json"
    if not manifest_path.is_file():
        raise EnvironmentBlocker(
            f"missing {variant} manifest at {manifest_path}; "
            "run scripts/render_lab_build.py first"
        )
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as err:
        raise EnvironmentBlocker(f"unreadable {variant} manifest: {err}") from err
    if manifest.get("schema") != SCHEMA:
        raise EnvironmentBlocker(
            f"{variant} manifest schema {manifest.get('schema')!r} is not {SCHEMA!r}"
        )
    if manifest.get("variant") != variant:
        raise EnvironmentBlocker(
            f"{variant} manifest declares variant {manifest.get('variant')!r}"
        )
    manifest["_path"] = str(manifest_path)
    manifest["_sha256"] = sha256_file(manifest_path)
    return manifest


def resolve_binary(repo_root: Path, manifest: dict) -> Path:
    relpath = manifest.get("binary_relpath")
    if not relpath:
        raise EnvironmentBlocker("manifest has no binary_relpath")
    if Path(relpath).is_absolute() or ".." in Path(relpath).parts:
        raise EnvironmentBlocker(f"manifest binary_relpath is not repo-relative: {relpath!r}")
    binary = repo_root / relpath
    if not binary.is_file():
        raise EnvironmentBlocker(f"binary missing at {binary}")
    if not os.access(binary, os.X_OK):
        raise EnvironmentBlocker(f"binary is not executable: {binary}")
    expected = manifest.get("binary_sha256")
    actual = sha256_file(binary)
    if expected != actual:
        raise EnvironmentBlocker(
            f"{manifest['variant']} binary hash mismatch: manifest={expected} actual={actual}"
        )
    return binary


def cef_runtime_record(env: dict) -> dict:
    """Describe the inherited CEF runtime selection without publishing paths."""
    for key in CEF_RUNTIME_KEYS:
        value = env.get(key, "").strip()
        if not value:
            continue
        record = {"env_var": key, "path_sha256": sha256_text(value)[:16]}
        version_file = Path(value) / "version"
        try:
            record["version"] = version_file.read_text(encoding="utf-8").strip()[:64]
        except OSError:
            record["version"] = None
        return record
    return {"env_var": None, "path_sha256": None, "version": None}


# --------------------------------------------------------------------------
# Isolation, config, fixture
# --------------------------------------------------------------------------


def sanitized_environment(base_env: dict) -> dict:
    env = {}
    for key, value in base_env.items():
        if key.startswith(DROP_PREFIXES):
            continue
        if key in DROP_KEYS or key in PROXY_KEYS:
            continue
        env[key] = value
    # Loopback only; never proxy the local fixture connection.
    env["no_proxy"] = "127.0.0.1,localhost"
    env["NO_PROXY"] = "127.0.0.1,localhost"
    return env


def dev_root(run_root: Path) -> Path:
    return run_root / ".dev" / "dumber"


def dev_environment(run_root: Path, base_env: dict) -> dict:
    """Build the child environment for a private development-mode instance."""
    env = sanitized_environment(base_env)
    root = dev_root(run_root)
    env.update(
        {
            "ENV": "dev",
            "HOME": str(root / "home"),
            "XDG_CONFIG_HOME": str(root / "config"),
            "XDG_DATA_HOME": str(root / "data"),
            "XDG_STATE_HOME": str(root / "state"),
            "XDG_CACHE_HOME": str(root / "cache"),
            "DUMBER_CEF_ROOT_CACHE_PATH": str(run_root / "cef-root-cache"),
        }
    )
    return env


def resolve_stack(render_path: str, stack: str | None) -> str:
    """Resolve the render path and an explicit stack without contradicting them."""
    path_stack, _ = RENDER_PATHS[render_path]
    if stack is None:
        return path_stack
    if stack != path_stack:
        raise LabError(
            f"--stack {stack} contradicts --render-path {render_path}, which selects "
            f"{path_stack}; pass only one of them"
        )
    return stack


def generated_config_toml(fps_mode: str, stack: str) -> str:
    if fps_mode == "monitor":
        adaptive = "true"
        rate = MONITOR_FALLBACK_FPS
    else:
        adaptive = "false"
        rate = int(fps_mode)
    return (
        "# Generated by scripts/render_lab.py. The launcher never rewrites it after\n"
        "# launch; both variants get the same keys and only the frame-rate mode differs.\n"
        "[engine]\n"
        'type = "cef"\n'
        "\n"
        "[engine.cef]\n"
        f'render_stack = "{stack}"\n'
        f"adaptive_windowless_frame_rate = {adaptive}\n"
        f"windowless_frame_rate = {rate}\n"
        f"windowless_frame_rate_max = {MONITOR_MAX_FPS}\n"
        "\n"
        "[appearance]\n"
        'color_scheme = "prefer-dark"\n'
        "\n"
        "[appearance.external_theme]\n"
        "enabled = false\n"
    )


CONFIG_EXPECTED_KEYS = {
    ("engine", "type"): ("str", {"cef"}),
    ("engine.cef", "render_stack"): ("str", set(STACKS)),
    ("engine.cef", "adaptive_windowless_frame_rate"): ("bool", {True, False}),
    ("engine.cef", "windowless_frame_rate"): ("int", {0, 60, 120, 165}),
    ("engine.cef", "windowless_frame_rate_max"): ("int", {MONITOR_MAX_FPS}),
    ("appearance", "color_scheme"): ("str", {"prefer-dark"}),
    ("appearance.external_theme", "enabled"): ("bool", {False}),
}


def validate_config_toml(text: str) -> dict:
    """Structural validation of the generated profile against the current keys."""
    try:
        parsed = tomllib.loads(text)
    except tomllib.TOMLDecodeError as err:
        raise LabError(f"generated config is not valid TOML: {err}") from err
    seen = set()
    for section, values in parsed.items():
        if not isinstance(values, dict):
            raise LabError(f"generated config section {section!r} is not a table")
        for key, value in values.items():
            if isinstance(value, dict):
                for nested_key, nested_value in value.items():
                    seen.add((f"{section}.{key}", nested_key, nested_value))
                continue
            seen.add((section, key, value))
    for section, key, value in seen:
        expected = CONFIG_EXPECTED_KEYS.get((section, key))
        if expected is None:
            raise LabError(f"generated config contains unknown key {section}.{key}")
        kind, allowed = expected
        if kind == "bool" and not isinstance(value, bool):
            raise LabError(f"{section}.{key} must be a boolean")
        if kind == "int" and (isinstance(value, bool) or not isinstance(value, int)):
            raise LabError(f"{section}.{key} must be an integer")
        if kind == "str" and not isinstance(value, str):
            raise LabError(f"{section}.{key} must be a string")
        if value not in allowed:
            raise LabError(f"{section}.{key}={value!r} is not in {sorted(map(str, allowed))}")
    declared = {(section, key) for section, key, _ in seen}
    missing = set(CONFIG_EXPECTED_KEYS) - declared
    if missing:
        raise LabError(f"generated config is missing keys: {sorted(missing)}")
    return parsed


class FixtureHandler(http.server.BaseHTTPRequestHandler):
    """Serve exactly one fixture path; no directory listing, no passthrough."""

    server_version = "dumber-render-lab"
    fixture_bytes = b""
    fixture_sha256 = ""

    def log_message(self, *_args):  # noqa: D102 - silence default stderr logging
        return

    def _respond(self, include_body: bool) -> None:
        path = urllib.parse.urlsplit(self.path).path
        if path != "/scroll.html":
            self._send_not_found()
            return
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(self.fixture_bytes)))
        self.send_header("Cache-Control", "no-store")
        self.send_header("X-Fixture-SHA256", self.fixture_sha256[:16])
        self.end_headers()
        if include_body:
            self.wfile.write(self.fixture_bytes)

    def _send_not_found(self) -> None:
        body = b"not found\n"
        self.send_response(404)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:  # noqa: N802 - http.server API
        self._respond(True)

    def do_HEAD(self) -> None:  # noqa: N802 - http.server API
        self._respond(False)


class FixtureServer(http.server.ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = False

    def handle_error(self, request, client_address):  # noqa: D102 - quiet by design
        return


def stop_fixture_server(server: FixtureServer) -> None:
    """Bounded stop: shutdown() returns once serve_forever observes the request."""
    server.shutdown()
    server.server_close()


def start_fixture_server(fixture_path: Path) -> tuple[FixtureServer, str, str]:
    payload = fixture_path.read_bytes()
    digest = hashlib.sha256(payload).hexdigest()
    handler = type(
        "BoundFixtureHandler",
        (FixtureHandler,),
        {"fixture_bytes": payload, "fixture_sha256": digest},
    )
    server = FixtureServer(("127.0.0.1", 0), handler)
    # serve_forever must be running for shutdown() to return; a daemon thread
    # keeps the launcher single-threaded while still allowing bounded cleanup.
    threading.Thread(target=server.serve_forever, daemon=True).start()
    host, port = server.server_address[:2]
    url = f"http://{host}:{port}/scroll.html"
    return server, url, digest


# --------------------------------------------------------------------------
# Launch
# --------------------------------------------------------------------------


@dataclasses.dataclass
class LaunchPlan:
    variant: str
    status: str
    binary: Path
    binary_sha256: str
    manifest_sha256: str
    repo_head: str
    bridge_head: str
    run_root: Path
    output_root: Path
    run_env: dict
    config_path: Path
    config_sha256: str
    fixture_path: Path
    fixture_url: str
    fixture_sha256: str
    fps_mode: str
    render_path: str
    stack: str
    external_begin_frame: bool
    profile_enabled: bool
    trace_geometry: bool
    graphics_offload: bool
    import_priority: str
    retired_textures: int
    profile_output: Path | None
    cef_runtime: dict
    dry_run: bool

    def argv(self) -> list[str]:
        return [str(self.binary), "browse", self.fixture_url]

    def metadata(self) -> dict:
        return {
            "schema": RUN_SCHEMA,
            "variant": self.variant,
            "status": self.status,
            "fps": self.fps_mode,
            "render_path": self.render_path,
            "stack": self.stack,
            "external_begin_frame": self.external_begin_frame,
            "profile": self.profile_enabled,
            "trace_geometry": self.trace_geometry,
            "render_knobs": {
                "graphics_offload": self.graphics_offload,
                "import_priority": self.import_priority,
                "retired_textures": self.retired_textures,
            },
            "binary_sha256": self.binary_sha256,
            "manifest_sha256": self.manifest_sha256,
            "repo_head": self.repo_head,
            "bridge_head": self.bridge_head,
            "repo_root_label": self.binary.parent.name,
            "run_id": self.run_root.name,
            "run_root_sha256": sha256_text(str(self.run_root))[:16],
            "fixture_sha256": self.fixture_sha256,
            "config_sha256": self.config_sha256,
            "cef_runtime": self.cef_runtime,
            "dry_run": self.dry_run,
        }


def build_launch_plan(
    *,
    repo_root: Path,
    output_root: Path,
    variant: str,
    fps_mode: str,
    render_path: str,
    stack: str | None,
    external_begin_frame: bool,
    profile_enabled: bool,
    trace_geometry: bool,
    base_env: dict,
    dry_run: bool,
    graphics_offload: bool = True,
    import_priority: str = DEFAULT_IMPORT_PRIORITY,
    retired_textures: int = DEFAULT_RETIRED_TEXTURES,
) -> LaunchPlan:
    resolved_stack = resolve_stack(render_path, stack)
    manifest = load_manifest(repo_root, variant)
    binary = resolve_binary(repo_root, manifest)
    fixture_path = repo_root / FIXTURE_PATH
    if not fixture_path.is_file():
        raise EnvironmentBlocker(f"fixture missing at {fixture_path}")

    output_root = Path(output_root)
    if output_root.is_symlink():
        raise EnvironmentBlocker(
            f"output root {output_root} is a symlink; refusing to write run artifacts through it"
        )
    output_root.mkdir(parents=True, exist_ok=True)
    run_root = Path(tempfile.mkdtemp(prefix="dumber-render-lab-", dir=str(output_root)))
    os.chmod(run_root, 0o700)

    config_dir = dev_root(run_root) / "config"
    config_dir.mkdir(parents=True, exist_ok=True)
    config_text = generated_config_toml(fps_mode, resolved_stack)
    validate_config_toml(config_text)
    config_path = config_dir / "config.toml"
    config_path.write_text(config_text, encoding="utf-8")
    os.chmod(config_path, 0o600)

    run_env = dev_environment(run_root, base_env)
    profile_output = None
    if profile_enabled:
        profile_output = run_root / "cef2gtk_profile.jsonl"
        run_env["DUMBER_CEF2GTK_PROFILE"] = "1"
        run_env["DUMBER_CEF2GTK_PROFILE_INTERVAL"] = "1s"
        run_env["DUMBER_CEF2GTK_PROFILE_OUTPUT"] = str(profile_output)
    # Render-path experiment knobs are always set explicitly so a run does not
    # depend on the caller's environment, and the recorded values are the ones
    # the binary received.
    run_env["PUREGO_CEF2GTK_GDK_GRAPHICS_OFFLOAD"] = "1" if graphics_offload else "0"
    run_env["PUREGO_CEF2GTK_GDK_IMPORT_PRIORITY"] = import_priority
    run_env["PUREGO_CEF2GTK_GDK_RETIRED_TEXTURES"] = str(retired_textures)
    run_env["DUMBER_CEF_EXTERNAL_BEGIN_FRAME"] = "1" if external_begin_frame else "0"
    if trace_geometry:
        # Named diagnostic, not a passthrough: it prints the geometry contract
        # the bridge computes for this run so a sizing bug can be read instead of
        # guessed at.
        run_env["PUREGO_CEF2GTK_TRACE_OSR"] = "1"
        run_env["PUREGO_CEF2GTK_TRACE_SCALE"] = "1"

    return LaunchPlan(
        variant=variant,
        status=str(manifest.get("status", "unknown")),
        binary=binary,
        binary_sha256=manifest["binary_sha256"],
        manifest_sha256=manifest["_sha256"],
        repo_head=str(manifest.get("repo_head", "")),
        bridge_head=str(manifest.get("bridge_head", "")),
        run_root=run_root,
        output_root=output_root,
        run_env=run_env,
        config_path=config_path,
        config_sha256=sha256_text(config_text),
        fixture_path=fixture_path,
        fixture_url="",
        fixture_sha256=sha256_file(fixture_path),
        fps_mode=fps_mode,
        render_path=render_path,
        stack=resolved_stack,
        external_begin_frame=external_begin_frame,
        profile_enabled=profile_enabled,
        trace_geometry=trace_geometry,
        graphics_offload=graphics_offload,
        import_priority=import_priority,
        retired_textures=retired_textures,
        profile_output=profile_output,
        cef_runtime=cef_runtime_record(base_env),
        dry_run=dry_run,
    )


def _display_available(env: dict) -> bool:
    return bool(env.get("WAYLAND_DISPLAY")) or bool(env.get("DISPLAY"))


def run_config_status(plan: LaunchPlan) -> dict:
    """Best-effort config-load smoke using the same binary, recorded privately."""
    log_path = plan.run_root / "config_status.txt"
    try:
        completed = subprocess.run(
            [str(plan.binary), "config", "status"],
            cwd=str(plan.run_root),
            env=plan.run_env,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            timeout=CONFIG_STATUS_TIMEOUT_SECONDS,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired) as err:
        return {"ran": False, "detail": type(err).__name__}
    output = completed.stdout or b""
    log_path.write_bytes(output)
    os.chmod(log_path, 0o600)
    suspicious = re.search(rb"\berror\b", output, re.IGNORECASE) is not None
    return {"ran": True, "exit_code": completed.returncode, "suspicious_error": suspicious}


def _signal_owned_group(pid: int, pgid: int, sig: int) -> None:
    try:
        current = os.getpgid(pid)
    except ProcessLookupError:
        return
    if current == pgid:
        try:
            os.killpg(pgid, sig)
        except ProcessLookupError:
            return
        return
    # The child left our group; only signal the process we spawned ourselves.
    try:
        os.kill(pid, sig)
    except ProcessLookupError:
        return


def terminate_owned(process: subprocess.Popen, pgid: int) -> str:
    """Terminate only lab-owned processes; returns a termination label."""
    if process.poll() is not None:
        return "natural"
    _signal_owned_group(process.pid, pgid, signal.SIGTERM)
    deadline = time.monotonic() + GRACEFUL_EXIT_SECONDS
    while time.monotonic() < deadline:
        if process.poll() is not None:
            return "graceful"
        time.sleep(0.05)
    _signal_owned_group(process.pid, pgid, signal.SIGKILL)
    try:
        process.wait(timeout=GRACEFUL_EXIT_SECONDS)
    except subprocess.TimeoutExpired:
        return "forced-timeout"
    return "forced"


def execute_plan(plan: LaunchPlan, duration: float | None) -> int:
    server, url, fixture_digest = start_fixture_server(plan.fixture_path)
    if fixture_digest != plan.fixture_sha256:
        stop_fixture_server(server)
        raise LabError("fixture changed between planning and launch")
    plan.fixture_url = url
    log_path = plan.run_root / "dumber.log"
    metadata = plan.metadata()
    metadata["started_at"] = time.time()
    metadata["duration_seconds"] = duration
    config_status = {"ran": False, "detail": "dry-run"}
    termination = "dry-run"
    exit_code = None
    try:
        if not plan.dry_run:
            config_status = run_config_status(plan)
            metadata["config_status"] = config_status
            with log_path.open("wb") as log_handle:
                process = subprocess.Popen(
                    plan.argv(),
                    cwd=str(plan.run_root),
                    env=plan.run_env,
                    stdin=subprocess.DEVNULL,
                    stdout=log_handle,
                    stderr=subprocess.STDOUT,
                    start_new_session=True,
                )
                os.chmod(log_path, 0o600)
                pgid = os.getpgid(process.pid)
                metadata["child_pgid"] = pgid
                deadline = None if duration is None else time.monotonic() + duration
                try:
                    while True:
                        if process.poll() is not None:
                            break
                        if deadline is not None and time.monotonic() >= deadline:
                            termination = terminate_owned(process, pgid)
                            metadata["bounded_by_duration"] = True
                            break
                        time.sleep(0.1)
                except KeyboardInterrupt:
                    termination = terminate_owned(process, pgid)
                    metadata["interrupted"] = True
                if process.poll() is None:
                    termination = terminate_owned(process, pgid)
                elif termination == "dry-run":
                    termination = "natural"
                exit_code = process.returncode
    finally:
        stop_fixture_server(server)

    metadata["ended_at"] = time.time()
    metadata["termination"] = termination
    metadata["exit_code"] = exit_code
    metadata["profile_output"] = (
        plan.profile_output.name if plan.profile_output is not None else None
    )
    if metadata["ended_at"] is not None and metadata.get("started_at") is not None:
        metadata["wall_seconds"] = round(metadata["ended_at"] - metadata["started_at"], 3)

    summary = {"gdk_pipeline": None, "note": "profiling disabled"}
    if plan.profile_enabled and plan.profile_output is not None:
        summary = summarize_profile(plan.profile_output)
    metadata["profile_summary"] = summary

    write_run_artifacts(plan, metadata)
    print_run_report(plan, metadata, summary)

    if plan.dry_run:
        return 0
    if exit_code not in (0, None) and termination == "natural":
        return 1
    return 0


def write_run_artifacts(plan: LaunchPlan, metadata: dict) -> None:
    run_json = plan.run_root / "run.json"
    run_json.write_text(json.dumps(metadata, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    os.chmod(run_json, 0o600)
    summary_lines = [
        f"variant: {metadata['variant']} ({metadata['status']})",
        f"fps: {metadata['fps']}  stack: {metadata['stack']}",
        f"external_begin_frame: {metadata['external_begin_frame']}  profile: {metadata['profile']}",
        f"termination: {metadata['termination']}  exit_code: {metadata['exit_code']}",
        f"wall_seconds: {metadata.get('wall_seconds')}",
        f"binary_sha256: {metadata['binary_sha256']}",
        f"fixture_sha256: {metadata['fixture_sha256']}",
    ]
    (plan.run_root / "summary.txt").write_text("\n".join(summary_lines) + "\n", encoding="utf-8")
    os.chmod(plan.run_root / "summary.txt", 0o600)
    if plan.profile_output is not None and plan.profile_output.exists():
        os.chmod(plan.profile_output, 0o600)


def render_change_note(status: str) -> str:
    """State plainly whether this build carries a rendering change."""
    if status == "baseline":
        return "reference build: no rendering change, no instrumentation"
    if status == "ownership-verified-candidate":
        return "includes a verified ownership change"
    if status == "perf-experiment":
        return (
            "render-path and pacing experiment: offload, import priority, texture "
            "retention, external BeginFrame and frame-rate pinning are applied; "
            "unverified"
        )
    return (
        "measurement only: this binary carries no rendering, ownership or pacing "
        "change"
    )


def print_run_report(plan: LaunchPlan, metadata: dict, summary: dict) -> None:
    print(f"variant:        {plan.variant} ({plan.status})")
    print(f"what changed:   {render_change_note(plan.status)}")
    print(f"binary:         {plan.binary}")
    print(f"binary sha256:  {plan.binary_sha256}")
    print(f"run root:       {plan.run_root}")
    print(f"run id:         {plan.run_root.name}")
    print(f"fixture url:    {plan.fixture_url or '(dry-run: not started)'}")
    path_detail = RENDER_PATHS[plan.render_path][1]
    print(f"render path:    {plan.render_path} — {path_detail}")
    print(f"fps mode:       {plan.fps_mode}   stack: {plan.stack}")
    print(f"external BFr:   {plan.external_begin_frame}   profile: {plan.profile_enabled}")
    print(
        f"render knobs:   offload={plan.graphics_offload} "
        f"import_priority={plan.import_priority} "
        f"retired_textures={plan.retired_textures}"
    )
    if plan.trace_geometry:
        print("geometry trace: on (diagnostic; see the run log)")
    if plan.dry_run:
        print("dry-run:        launch validated, no browser started")
        return
    print(
        f"termination:    {metadata.get('termination', 'unknown')}   "
        f"exit code: {metadata.get('exit_code')}"
    )
    print(f"log:            {plan.run_root / 'dumber.log'}")
    if not plan.profile_enabled:
        print("metrics:        unavailable (profiling disabled)")
        return
    pipeline = summary.get("gdk_pipeline")
    if pipeline is None:
        print(f"metrics:        unavailable ({summary.get('note', 'no profile data')})")
        print_present_summary(summary)
        return
    print("gdk_pipeline (candidate-only instrumentation):")
    for name in DURATION_SERIES:
        series = pipeline.get(name) or {}
        if not series.get("samples_total"):
            print(f"  {name}: unavailable")
            continue
        print(
            f"  {name}: samples={series['samples_total']} "
            f"windows={series['windows_with_samples']} "
            f"p50={_format_ms(series.get('p50'))} "
            f"p95={_format_ms(series.get('p95'))} "
            f"p99={_format_ms(series.get('p99'))} "
            f"(sampled, {series.get('quantile_note')})"
        )
    counts = pipeline.get("counters") or {}
    if not counts:
        print("  counters: unavailable")
        return
    print(
        "  counters: received={frames_received} replaced={pending_replaced} "
        "imported={frames_imported} swapped={frames_swapped} "
        "paint_cycles={gtk_paint_cycles}".format(**counts)
    )
    print(
        "  feedback:  available={feedback_available} "
        "unavailable={feedback_unavailable}".format(**counts)
    )
    print("  overwritten_before_paint={swaps_overwritten_before_paint}".format(**counts))
    print_present_summary(summary)


def print_present_summary(summary: dict) -> None:
    """Print present-path and input counters in every build, not just the candidate."""
    present = summary.get("present") or {}
    input_counters = summary.get("input") or {}
    windows = summary.get("windows")
    if not present or not windows:
        return
    received = present.get("frames_received") or 0
    events = input_counters.get("scroll_events") or 0
    per_window_events = events / windows if windows else 0.0
    per_window_frames = received / windows if windows else 0.0
    print(
        "  present:  frames_received={frames_received} frames_queued={frames_queued} "
        "frames_rendered={frames_rendered} import_failures={import_failures}".format(**present)
    )
    print(
        "  input:    scroll_events={scroll_events} "
        "external_begin_frames={external_begin_frames_sent}".format(**input_counters)
    )
    print(
        f"  per window: {per_window_frames:.1f} frames, {per_window_events:.1f} GTK scroll "
        f"events, scroll_dy={summary.get('scroll_abs_dy_sum', 0.0):.0f} px"
    )
    print(
        f"  observer:  import_copy samples={summary.get('import_copy_samples')} "
        f"gtk_wait samples={summary.get('gtk_wait_samples')} "
        f"goroutines_max={summary.get('goroutines_max')}"
    )


def _format_ms(value: float | None) -> str:
    return "unavailable" if value is None else f"{value:.4f}ms"


# --------------------------------------------------------------------------
# Profile summary
# --------------------------------------------------------------------------

DURATION_SERIES = (
    "received_to_import_ms",
    "queue_wait_ms",
    "import_elapsed_ms",
    "received_to_swap_ms",
)
COUNTER_KEYS = (
    "frames_received",
    "pending_replaced",
    "frames_imported",
    "frames_swapped",
    "gtk_paint_cycles",
    "swaps_overwritten_before_paint",
    "feedback_available",
    "feedback_unavailable",
)
# Present-path counters every build reports, including unmodified baselines.
PRESENT_KEYS = (
    "frames_received",
    "frames_queued",
    "frames_rendered",
    "import_failures",
)
# Input counters that separate wheel input from library-driven motion.
INPUT_KEYS = (
    "scroll_events",
    "external_begin_frames_sent",
)


def _median(values: list[float]) -> float | None:
    if not values:
        return None
    ordered = sorted(values)
    middle = len(ordered) // 2
    if len(ordered) % 2 == 1:
        return ordered[middle]
    return (ordered[middle - 1] + ordered[middle]) / 2


def summarize_profile(path: Path) -> dict:
    """Summarize bounded per-window snapshots; never invent missing metrics."""
    if not path.is_file():
        return {"gdk_pipeline": None, "note": "profile output missing"}
    windows = 0
    counters = {key: 0 for key in COUNTER_KEYS}
    present = {key: 0 for key in PRESENT_KEYS}
    input_counters = {key: 0 for key in INPUT_KEYS}
    scroll_abs_dy = 0.0
    import_copy_samples = 0
    gtk_wait_samples = 0
    goroutines_max = 0
    saw_pipeline = False
    series_values: dict[str, dict[str, list[float]]] = {
        name: {"p50": [], "p95": [], "p99": []} for name in DURATION_SERIES
    }
    series_samples: dict[str, int] = {name: 0 for name in DURATION_SERIES}
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except OSError as err:
        return {"gdk_pipeline": None, "note": f"unreadable profile output: {err}"}
    for line in lines:
        line = line.strip()
        if not line:
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError:
            continue
        snapshot = record.get("snapshot") if isinstance(record, dict) else None
        if not isinstance(snapshot, dict):
            continue
        windows += 1
        for key in PRESENT_KEYS:
            value = snapshot.get(key)
            if isinstance(value, (int, float)) and not isinstance(value, bool):
                present[key] += int(value)
        for key in INPUT_KEYS:
            value = snapshot.get(key)
            if isinstance(value, (int, float)) and not isinstance(value, bool):
                input_counters[key] += int(value)
        scroll_abs_dy += float(snapshot.get("scroll_abs_dy_sum") or 0.0)
        for key, target in (("import_copy_cpu", "import_copy"), ("gtk_wait_cpu", "gtk_wait")):
            series = snapshot.get(key)
            if isinstance(series, dict) and isinstance(series.get("count"), int):
                if target == "import_copy":
                    import_copy_samples += series["count"]
                else:
                    gtk_wait_samples += series["count"]
        gc = snapshot.get("gc")
        if isinstance(gc, dict) and isinstance(gc.get("num_goroutine"), int):
            goroutines_max = max(goroutines_max, gc["num_goroutine"])
        pipeline = snapshot.get("gdk_pipeline")
        if not isinstance(pipeline, dict):
            continue
        saw_pipeline = True
        for key in COUNTER_KEYS:
            value = pipeline.get(key)
            if isinstance(value, (int, float)) and not isinstance(value, bool):
                counters[key] += int(value)
        for name in DURATION_SERIES:
            series = pipeline.get(name)
            if not isinstance(series, dict):
                continue
            samples = series.get("samples")
            if isinstance(samples, int):
                series_samples[name] += samples
            for quantile in ("p50", "p95", "p99"):
                value = series.get(quantile)
                if isinstance(value, (int, float)) and not isinstance(value, bool):
                    series_values[name][quantile].append(float(value))
    if windows == 0:
        return {
            "gdk_pipeline": None,
            "note": (
                "no profile window was written: the profiler emits a window only while "
                "accelerated frames arrive, so a hidden or idle page produces none"
            ),
            "windows": 0,
        }
    present_summary = {
        "present": present,
        "input": input_counters,
        "scroll_abs_dy_sum": scroll_abs_dy,
        "import_copy_samples": import_copy_samples,
        "gtk_wait_samples": gtk_wait_samples,
        "goroutines_max": goroutines_max,
        "windows": windows,
    }
    if not saw_pipeline:
        return {
            "gdk_pipeline": None,
            "note": "profile windows predate gdk_pipeline instrumentation",
            "windows": windows,
            **present_summary,
        }
    distributions = {}
    for name in DURATION_SERIES:
        distributions[name] = {
            "samples_total": series_samples[name],
            "windows_with_samples": len(series_values[name]["p50"]),
            "p50": _median(series_values[name]["p50"]),
            "p95": _median(series_values[name]["p95"]),
            "p99": _median(series_values[name]["p99"]),
            "quantile_note": "median of per-window sampled quantiles",
        }
    return {
        "gdk_pipeline": {
            "schema_version": 1,
            "counters": counters,
            **distributions,
        },
        "windows": windows,
        **present_summary,
    }


# --------------------------------------------------------------------------
# CLI
# --------------------------------------------------------------------------


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Launch an isolated Dumber render-lab instance on a local fixture page.",
    )
    parser.add_argument("--variant", choices=VARIANTS, default="candidate")
    parser.add_argument("--fps", choices=FPS_CHOICES, default="monitor")
    parser.add_argument(
        "--render-path",
        choices=tuple(RENDER_PATHS),
        default=DEFAULT_RENDER_PATH,
        help="how the accelerated frame is consumed (selects the render stack)",
    )
    parser.add_argument(
        "--stack",
        choices=STACKS,
        default=None,
        help="low-level render stack; must agree with --render-path if both are given",
    )
    parser.add_argument(
        "--external-begin-frame",
        action=argparse.BooleanOptionalAction,
        default=True,
        help="drive CEF BeginFrame from the GTK frame clock (A/B knob)",
    )
    parser.add_argument(
        "--no-graphics-offload",
        dest="graphics_offload",
        action="store_false",
        help="A/B knob: drop the GtkGraphicsOffload presenter wrapper",
    )
    parser.add_argument(
        "--idle-import-priority",
        dest="import_priority",
        action="store_const",
        const="idle",
        default=DEFAULT_IMPORT_PRIORITY,
        help="A/B knob: schedule the GTK-thread frame import at G_PRIORITY_DEFAULT_IDLE",
    )
    parser.add_argument(
        "--retired-textures",
        type=int,
        default=DEFAULT_RETIRED_TEXTURES,
        metavar="N",
        help="A/B knob: superseded textures kept referenced, 1..%d" % RETIRED_TEXTURES_MAX,
    )
    parser.add_argument("--profile", action="store_true")
    parser.add_argument(
        "--trace-geometry",
        action="store_true",
        help="diagnostic: print the OSR geometry contract the bridge computes",
    )
    parser.add_argument("--duration", type=float, default=None, metavar="SECONDS")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--output-root", type=Path, default=None)
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(sys.argv[1:] if argv is None else argv)
    if args.duration is not None and args.duration <= 0:
        print("render_lab: --duration must be positive", file=sys.stderr)
        return 1
    if not 1 <= args.retired_textures <= RETIRED_TEXTURES_MAX:
        print(
            f"render_lab: --retired-textures must be between 1 and {RETIRED_TEXTURES_MAX}",
            file=sys.stderr,
        )
        return 1
    os.umask(0o077)
    repo_root = Path(__file__).resolve().parent.parent
    output_root = args.output_root or (repo_root / DEFAULT_OUTPUT_ROOT)
    base_env = dict(os.environ)
    if not args.dry_run and not _display_available(base_env):
        print(
            "render_lab: no WAYLAND_DISPLAY or DISPLAY; refusing to launch a hidden browser",
            file=sys.stderr,
        )
        return 2
    try:
        plan = build_launch_plan(
            repo_root=repo_root,
            output_root=output_root,
            variant=args.variant,
            fps_mode=args.fps,
            render_path=args.render_path,
            stack=args.stack,
            external_begin_frame=args.external_begin_frame,
            profile_enabled=args.profile,
            trace_geometry=args.trace_geometry,
            graphics_offload=args.graphics_offload,
            import_priority=args.import_priority,
            retired_textures=args.retired_textures,
            base_env=base_env,
            dry_run=args.dry_run,
        )
        return execute_plan(plan, args.duration)
    except EnvironmentBlocker as err:
        print(f"render_lab: blocked: {err}", file=sys.stderr)
        return 2
    except LabError as err:
        print(f"render_lab: error: {err}", file=sys.stderr)
        return 1
    except FileNotFoundError as err:
        print(f"render_lab: missing runtime dependency: {err}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
